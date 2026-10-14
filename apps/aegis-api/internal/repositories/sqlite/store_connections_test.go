package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sqlitedriver "modernc.org/sqlite"
)

func storeConnectionSettings(t *testing.T, conn *sql.Conn, journal string) any {
	t.Helper()
	ctx := context.Background()
	var foreignKeys, busy int
	var mode string
	require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys))
	require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy))
	require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode))
	require.Equal(t, 1, foreignKeys)
	require.Equal(t, 5000, busy)
	require.Equal(t, journal, mode)
	var raw any
	require.NoError(t, conn.Raw(func(value any) error { raw = value; return nil }))
	t.Logf("physical=%p foreign_keys=%d busy_timeout=%d journal_mode=%s", raw, foreignKeys, busy, mode)
	return raw
}

func TestStoreDeferredCommitFailureAfterReplacement(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(time.Hour))
	event := authorizationEvent(t, f.s, f.device)
	before, err := f.s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	conn, err := f.s.db.Conn(ctx)
	require.NoError(t, err)
	storeDiscardConnection(t, conn)
	conn, err = f.s.db.Conn(ctx)
	require.NoError(t, err)
	physical := storeConnectionSettings(t, conn, "wal")
	require.NoError(t, conn.Close())
	_, err = f.s.db.Exec("CREATE TABLE connection_commit_fault(id TEXT REFERENCES users(id) DEFERRABLE INITIALLY DEFERRED); CREATE TRIGGER connection_commit_fault_insert AFTER INSERT ON change_policy_evaluations BEGIN INSERT INTO connection_commit_fault VALUES('missing-user'); END;")
	require.NoError(t, err)
	_, err = f.s.PersistPolicyEvaluation(ctx, f.device, event.ID, f.approver, f.now.Add(4*time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	require.Zero(t, f.s.db.Stats().InUse)
	conn, err = f.s.db.Conn(ctx)
	require.NoError(t, err)
	require.Same(t, physical, storeConnectionSettings(t, conn, "wal"), "confirmed cleanup may safely reuse the connection")
	require.NoError(t, conn.Close())
	for _, table := range []string{"connection_commit_fault", "change_policy_evaluations", "change_policy_evaluation_items"} {
		var count int
		require.NoError(t, f.s.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		require.Zero(t, count)
	}
	after, err := f.s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = f.s.db.Exec("DROP TRIGGER connection_commit_fault_insert")
	require.NoError(t, err)
	result, err := f.s.PersistPolicyEvaluation(ctx, f.device, event.ID, f.approver, f.now.Add(4*time.Hour))
	require.NoError(t, err)
	require.Equal(t, "pass", result.Result)
	t.Log("real deferred COMMIT failure after replacement: propagated, checked clean reuse, zero partial evidence, subsequent PASS")
}

// The existing fault decoration is reused, but this connector deliberately
// executes NO PRAGMAs. Only the production-normalized DSN initializes its driver.
type storeFaultConnector struct {
	underlying driver.Driver
	dsn        string
	faults     *policyCleanupFaults
}

func (c *storeFaultConnector) Driver() driver.Driver { return c.underlying }
func (c *storeFaultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := c.underlying.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	c.faults.opens.Add(1)
	return &policyCleanupConn{Conn: conn, faults: c.faults}, nil
}

func TestStoreUncertainCleanupReplacement(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(time.Hour))
	event := authorizationEvent(t, f.s, f.device)
	before, err := f.s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	_, err = f.s.db.Exec("CREATE TABLE cleanup_parent(id INTEGER PRIMARY KEY); CREATE TABLE cleanup_child(id INTEGER REFERENCES cleanup_parent(id));")
	require.NoError(t, err)
	var path string
	require.NoError(t, f.s.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path))
	dsn, _, err := normalizeSQLiteDSN(path)
	require.NoError(t, err)
	underlying := f.s.db.Driver()
	require.NoError(t, f.s.db.Close())
	faults := &policyCleanupFaults{}
	faults.failCommit.Store(true)
	faults.failCleanup.Store(true)
	f.s.db = sql.OpenDB(&storeFaultConnector{underlying: underlying, dsn: dsn, faults: faults})
	f.s.db.SetMaxOpenConns(1)
	_, err = f.s.PersistPolicyEvaluation(ctx, f.device, event.ID, f.approver, f.now.Add(4*time.Hour))
	require.ErrorIs(t, err, errPolicyInjectedCommit)
	require.ErrorIs(t, err, errPolicyInjectedCleanup, "cleanup failure must not be hidden")
	require.EqualValues(t, 1, faults.cleanupAttempts.Load())
	require.EqualValues(t, 1, faults.closes.Load())
	require.Zero(t, f.s.db.Stats().OpenConnections, "uncertain physical connection must not be pooled")
	conn, err := f.s.db.Conn(ctx)
	require.NoError(t, err)
	storeConnectionSettings(t, conn, "wal")
	require.EqualValues(t, 2, faults.opens.Load())
	_, err = conn.ExecContext(ctx, "INSERT INTO cleanup_child VALUES(9999)")
	storeForeignKeyError(t, err)
	require.NoError(t, conn.Close())
	after, err := f.s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	for _, table := range []string{"change_policy_evaluations", "change_policy_evaluation_items"} {
		var count int
		require.NoError(t, f.s.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		require.Zero(t, count)
	}
	faults.failCommit.Store(false)
	faults.failCleanup.Store(false)
	result, err := f.s.PersistPolicyEvaluation(ctx, f.device, event.ID, f.approver, f.now.Add(4*time.Hour))
	require.NoError(t, err)
	require.Equal(t, "pass", result.Result)
	t.Log("uncertain cleanup: original+cleanup errors preserved; discarded; replacement FK/busy/WAL safe without connector repair")
}

func TestStoreGovernanceAfterReplacement(t *testing.T) {
	for _, outcome := range []string{"pass", "violation"} {
		t.Run(outcome, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			event := authorizationEvent(t, f.s, f.device)
			conn, err := f.s.db.Conn(ctx)
			require.NoError(t, err)
			storeDiscardConnection(t, conn)
			conn, err = f.s.db.Conn(ctx)
			require.NoError(t, err)
			storeConnectionSettings(t, conn, "wal")
			require.NoError(t, conn.Close())
			ticket := f.request(t)
			_, err = f.s.TransitionChangeAuthorization(ctx, f.device, ticket.ID, f.requester, "approved", f.now.Add(time.Hour))
			require.ErrorIs(t, err, domain.ErrForbidden)
			ticket = f.approve(t, ticket)
			require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, f.link(ticket, event)))
			history, _, err := f.s.ListChangeAuthorizationHistory(ctx, f.device, ticket.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			require.Len(t, history, 3)
			_, err = f.s.TransitionChangeAuthorization(ctx, f.device, ticket.ID, f.approver, "revoked", f.now.Add(5*time.Hour))
			require.NoError(t, err)
			link, err := f.s.GetChangeEventAuthorization(ctx, f.device, event.ID)
			require.NoError(t, err)
			require.Equal(t, ticket.ID, link.AuthorizationID)
			history, _, err = f.s.ListChangeAuthorizationHistory(ctx, f.device, ticket.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			require.Len(t, history, 4)
			require.Equal(t, "revoked", history[3].Action)
			// Probe actual Task #4 user/ticket/device FKs, avoiding other constraints.
			_, err = f.s.db.Exec("INSERT INTO change_authorization_history(id,authorization_id,event_id,action,actor_user_id,occurred_at) VALUES(?,?,NULL,'rejected',?,?)", uuid.NewString(), ticket.ID, uuid.NewString(), authorizationTimestamp(f.now))
			storeForeignKeyError(t, err)
			_, err = f.s.db.Exec("INSERT INTO change_authorization_history(id,authorization_id,event_id,action,actor_user_id,occurred_at) VALUES(?,?,NULL,'requested',?,?)", uuid.NewString(), uuid.NewString(), f.approver, authorizationTimestamp(f.now))
			storeForeignKeyError(t, err)
			_, err = f.s.db.Exec("INSERT INTO change_authorizations("+authorizationColumns+") SELECT ?,?,?,config_type,allowed_categories_json,window_start,window_end,status,requested_by,requested_at,approved_by,approved_at,rejected_by,rejected_at,revoked_by,revoked_at,created_at,updated_at FROM change_authorizations WHERE id=?", uuid.NewString(), "CHG-"+strings.ToUpper(uuid.NewString()), uuid.NewString(), ticket.ID)
			storeForeignKeyError(t, err)
			rules := []domain.PolicyRule{{Category: "aaa", Operation: "add", Decision: "allow"}, {Category: "interface", Operation: "remove", Decision: "allow"}}
			if outcome == "violation" {
				rules[0].Decision = "deny"
			}
			policySet := f.activatePolicy(t, f.policy(t, rules), f.now.Add(time.Hour))
			result, err := f.s.PersistPolicyEvaluation(ctx, f.device, event.ID, f.approver, f.now.Add(6*time.Hour))
			require.NoError(t, err)
			require.Equal(t, outcome, result.Result)
			loaded, err := f.s.GetPolicyEvaluation(ctx, f.device, event.ID)
			require.NoError(t, err)
			require.Equal(t, result, loaded)
			itemEvidence, total, err := f.s.ListPolicyEvaluationItems(ctx, f.device, event.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			require.Equal(t, 2, total)
			require.Len(t, itemEvidence, 2)
			source, err := f.s.GetChangeEvent(ctx, event.ID)
			require.NoError(t, err)
			require.Equal(t, "authorized", source.AuthorizationStatus)
			require.Equal(t, outcome, source.PolicyStatus)
			_, err = f.s.db.Exec("UPDATE change_event_items SET command='description tampered' WHERE change_event_id=?", event.ID)
			require.ErrorContains(t, err, "immutable finalized policy source item")
			_, err = f.s.db.Exec("UPDATE change_events SET item_count=item_count+1 WHERE id=?", event.ID)
			require.ErrorContains(t, err, "immutable finalized policy source event")
			_, err = f.s.db.Exec("DELETE FROM change_event_items WHERE change_event_id=?", event.ID)
			require.Error(t, err)
			_, err = f.s.RetirePolicySet(ctx, f.device, policySet.ID, f.approver, f.now.Add(7*time.Hour))
			require.NoError(t, err)
			again, err := f.s.GetPolicyEvaluation(ctx, f.device, event.ID)
			require.NoError(t, err)
			require.Equal(t, loaded, again)
			t.Logf("after physical replacement: Task #4 SoD/link/revocation/history/FKs and Task #5 %s/historical/source guards PASS", outcome)
		})
	}
}

func storeForeignKeyError(t *testing.T, err error) {
	t.Helper()
	var engineError *sqlitedriver.Error
	require.ErrorAs(t, err, &engineError)
	require.Equal(t, 787, engineError.Code(), "must be a real FK failure, not another constraint")
}

func storeDiscardConnection(t *testing.T, conn *sql.Conn) {
	t.Helper()
	require.ErrorIs(t, conn.Raw(func(any) error { return driver.ErrBadConn }), driver.ErrBadConn)
	require.ErrorIs(t, conn.Close(), sql.ErrConnDone)
}

// This MUST use production Open, without a connector that repairs PRAGMAs.
func TestStorePhysicalConnections(t *testing.T) {
	for _, synchronous := range []string{"default", "FULL", "EXTRA"} {
		t.Run(synchronous, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "physical.db")
			dsn := path
			if synchronous != "default" {
				for _, p := range []string{
					"ignore_check_constraints('OFF')", "main.writable_schema([NO])",
					"read_uncommitted(false)", "defer_foreign_keys(0)",
					"recursive_triggers(ON)", "synchronous(" + synchronous + ")",
					"cache_size(-2048)", "application_id(123)", "encoding('UTF-8')",
				} {
					if dsn == path {
						dsn += "?"
					} else {
						dsn += "&"
					}
					dsn += "_pragma=" + url.QueryEscape(p)
				}
			}
			s, err := Open(ctx, dsn)
			require.NoError(t, err)
			defer func() { require.NoError(t, s.Close()) }()
			var physicals []any
			for generation := 0; generation < 4; generation++ {
				conn, err := s.db.Conn(ctx)
				require.NoError(t, err)
				raw := storeConnectionSettings(t, conn, "wal")
				for _, old := range physicals {
					require.NotSame(t, old, raw, "discarded physical connection must not be reused")
				}
				physicals = append(physicals, raw) // Retain old objects; pointer reuse cannot hide replacement.
				var actual string
				require.NoError(t, conn.QueryRowContext(ctx, "SELECT file FROM pragma_database_list WHERE name='main'").Scan(&actual))
				intendedInfo, err := os.Stat(path)
				require.NoError(t, err)
				actualInfo, err := os.Stat(actual)
				require.NoError(t, err)
				require.True(t, os.SameFile(intendedInfo, actualInfo))
				for _, p := range []string{"writable_schema", "read_uncommitted", "defer_foreign_keys"} {
					var setting int
					require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA "+p).Scan(&setting))
					require.Zero(t, setting)
				}
				var sync int
				require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&sync))
				if synchronous == "EXTRA" {
					require.Equal(t, 3, sync)
				} else {
					require.Equal(t, 2, sync)
				}
				if synchronous != "default" {
					for p, expected := range map[string]int{"cache_size": -2048, "application_id": 123, "recursive_triggers": 1} {
						var setting int
						require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA "+p).Scan(&setting))
						require.Equal(t, expected, setting)
					}
					var encoding string
					require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA encoding").Scan(&encoding))
					require.Equal(t, "UTF-8", encoding)
				}
				if generation == 0 {
					_, err = conn.ExecContext(ctx, "CREATE TABLE connection_parent(id INTEGER PRIMARY KEY); CREATE TABLE connection_child(id INTEGER PRIMARY KEY,parent_id INTEGER NOT NULL REFERENCES connection_parent(id)); CREATE TABLE connection_check(id INTEGER PRIMARY KEY, value INTEGER NOT NULL CHECK(value > 0));")
					require.NoError(t, err)
				}
				var ignored int
				require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA ignore_check_constraints").Scan(&ignored))
				require.Zero(t, ignored)
				_, err = conn.ExecContext(ctx, "INSERT INTO connection_check VALUES(?,-1)", generation)
				var engineError *sqlitedriver.Error
				require.ErrorAs(t, err, &engineError)
				require.Equal(t, 275, engineError.Code(), "must be an actual SQLite CHECK constraint failure")
				var checked int
				require.NoError(t, conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM connection_check").Scan(&checked))
				require.Equal(t, generation, checked, "invalid CHECK row must not persist")
				_, err = conn.ExecContext(ctx, "INSERT INTO connection_check VALUES(?,1)", generation)
				require.NoError(t, err)
				_, err = conn.ExecContext(ctx, "INSERT INTO connection_child VALUES(?,9999)", generation)
				storeForeignKeyError(t, err)
				var count int
				require.NoError(t, conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM connection_child").Scan(&count))
				require.Equal(t, generation, count, "orphan must not commit")
				_, err = conn.ExecContext(ctx, "INSERT INTO connection_parent VALUES(?)", generation)
				require.NoError(t, err)
				_, err = conn.ExecContext(ctx, "INSERT INTO connection_child VALUES(?,?)", generation, generation)
				require.NoError(t, err)
				t.Logf("generation=%d CHECK rejected (275), orphan rejected (787); legitimate rows persisted", generation)
				if generation < 3 {
					storeDiscardConnection(t, conn)
					require.Zero(t, s.db.Stats().OpenConnections)
				} else {
					require.NoError(t, conn.Close())
				}
			}
		})
	}
}

func TestStoreRejectsUnsafePragmas(t *testing.T) {
	unsafe := map[string][]string{
		"ignore_check_constraints": {"ON", "1", "true", "yes", "+1", "01", "0x1", "1.0"},
		"writable_schema":          {"ON", "1", "true", "yes", "RESET"},
		"read_uncommitted":         {"ON", "1", "true", "yes"},
		"defer_foreign_keys":       {"ON", "1", "true", "yes"},
		"synchronous":              {"OFF", "0", "NORMAL", "1", "-1", "unknown"},
		"recursive_triggers":       {"OFF", "0", "false", "no"},
		"schema_version":           {"1"},
		"unreviewed_pragma":        {"1"},
	}
	for name, arguments := range unsafe {
		for _, argument := range arguments {
			for _, p := range []string{
				name + "(" + argument + ")",
				strings.ToUpper(name) + "=" + strings.ToLower(argument),
				`"` + name + `"('` + argument + `')`,
				"[" + name + "]=[" + argument + "]",
				`main."` + strings.ToUpper(name) + `"="` + argument + `"`,
				"temp.[" + name + "]('" + argument + "')",
				`"main".'` + name + `' = '` + argument + `'`,
			} {
				t.Run(p, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "must-not-create", "private-dsn.db")
					s, err := Open(context.Background(), path+"?%5Fpragma="+url.QueryEscape(p))
					require.Error(t, err)
					require.Nil(t, s)
					require.NotContains(t, err.Error(), path)
					require.NotContains(t, err.Error(), p)
					_, err = os.Stat(filepath.Dir(path))
					require.ErrorIs(t, err, os.ErrNotExist, "reject before creating/opening the database")
				})
			}
		}
	}
	for name, argument := range map[string]string{
		"foreign_keys": "ON", "busy_timeout": "5000", "journal_mode": "WAL",
		"ignore_check_constraints": "OFF", "writable_schema": "OFF",
		"read_uncommitted": "OFF", "defer_foreign_keys": "OFF",
		"synchronous": "FULL", "recursive_triggers": "ON",
	} {
		for _, second := range []string{argument, "unsafe"} {
			t.Run("duplicate-"+name+"-"+second, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "must-not-create", "database.db")
				dsn := path + "?_pragma=" + url.QueryEscape(name+"("+argument+")") +
					"&%5Fpragma=" + url.QueryEscape(`main."`+strings.ToUpper(name)+`"('`+second+`')`)
				s, err := Open(context.Background(), dsn)
				require.ErrorContains(t, err, "duplicate security PRAGMA")
				require.Nil(t, s)
				_, err = os.Stat(filepath.Dir(path))
				require.ErrorIs(t, err, os.ErrNotExist)
			})
		}
	}
}

func TestStoreConnectionExpiry(t *testing.T) {
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "expiry.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, s.Close()) }()
	conn, err := s.db.Conn(context.Background())
	require.NoError(t, err)
	old := storeConnectionSettings(t, conn, "wal")
	s.db.SetConnMaxLifetime(time.Nanosecond)
	require.NoError(t, conn.Close())
	s.db.SetConnMaxLifetime(0)
	require.Positive(t, s.db.Stats().MaxLifetimeClosed)
	conn, err = s.db.Conn(context.Background())
	require.NoError(t, err)
	next := storeConnectionSettings(t, conn, "wal")
	require.NotSame(t, old, next)
	require.NoError(t, conn.Close())
	s.db.SetMaxIdleConns(0)
	conn, err = s.db.Conn(context.Background())
	require.NoError(t, err)
	raw := storeConnectionSettings(t, conn, "wal")
	require.NoError(t, conn.Close())
	conn, err = s.db.Conn(context.Background())
	require.NoError(t, err)
	require.NotSame(t, raw, storeConnectionSettings(t, conn, "wal"))
	require.NoError(t, conn.Close())
}

func TestNormalizeSQLiteDSN(t *testing.T) {
	pragma := func(text string) string { return "_pragma=" + url.QueryEscape(text) }
	cases := []string{
		"aegis.db", "./data/aegis.db", `C:\Network Files\Aegis\aegis.db`,
		"data/شبکه.db", "literal%20 + #.db",
		"file:aegis.db", "file:///C:/Network%20Files/aegis.db", "file://localhost/tmp/aegis.db",
		"file:%D8%B4%D8%A8%DA%A9%D9%87.db?mode=rwc&cache=private&_txlock=immediate",
		"file:aegis%23%25+name.db?tag=a+b&encoded=%2B%20%2f&empty=&" + pragma("cache_size(-2000)"),
		"aegis.db?" + pragma("foreign_keys(1)") + "&" + pragma("busy_timeout(5000)"),
		"aegis.db?" + pragma(`main."FOREIGN_KEYS" = 'ON'`) + "&" + pragma("temp.[busy_timeout]('5000')"),
		"aegis.db?" + pragma("foreign_keys([YES])") + "&" + pragma("journal_mode(WAL)"),
		"aegis.db?" + pragma("foreign_keys(true)") + "&" + pragma("encoding('UTF-8')"),
		"aegis.db?" + pragma(`application_id("123")`),
		":memory:", "file::memory:?cache=shared", "file:named-memory?mode=memory&cache=shared",
		":memory:?" + pragma("journal_mode(MEMORY)"),
		"file:memory?mode=memory&" + pragma("journal_mode(MEMORY)"),
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			normalized, filename, err := normalizeSQLiteDSN(dsn)
			require.NoError(t, err)
			base, query, _ := strings.Cut(dsn, "?")
			newBase, newQuery, _ := strings.Cut(normalized, "?")
			require.Equal(t, base, newBase)
			if strings.HasPrefix(base, "file:") || base == ":memory:" {
				require.Empty(t, filename)
			} else {
				require.Equal(t, base, filename)
			}
			values, err := url.ParseQuery(newQuery)
			require.NoError(t, err)
			counts := map[string]int{}
			for _, p := range values["_pragma"] {
				name, arg, ok := sqliteDSNPragma(p)
				require.True(t, ok)
				counts[name]++
				if name == "foreign_keys" {
					require.Equal(t, "1", arg)
				}
				if name == "busy_timeout" {
					require.Equal(t, "5000", arg)
				}
			}
			require.Equal(t, 1, counts["foreign_keys"])
			require.Equal(t, 1, counts["busy_timeout"])
			// Unrelated options retain their original bytes, including '+' versus '%20'.
			if query != "" {
				for _, field := range strings.Split(query, "&") {
					key, value, _ := strings.Cut(field, "=")
					decodedKey, _ := url.QueryUnescape(key)
					decodedValue, _ := url.QueryUnescape(value)
					name, _, _ := sqliteDSNPragma(decodedValue)
					if decodedKey != "_pragma" || (name != "foreign_keys" && name != "busy_timeout") {
						require.Contains(t, newQuery, field)
					}
				}
			}
			again, againFile, err := normalizeSQLiteDSN(normalized)
			require.NoError(t, err)
			require.Equal(t, normalized, again)
			require.Equal(t, filename, againFile)
		})
	}
}

func TestNormalizeSQLiteDSNRejectsConflicts(t *testing.T) {
	pragma := func(text string) string { return "_pragma=" + url.QueryEscape(text) }
	cases := []string{"", " ", "?mode=memory", "http://host/aegis.db", "FILE:aegis.db",
		"file:", "file://host/aegis.db", "file://user@localhost/aegis.db", "file:aegis.db#fragment",
		"file:%zz.db", "file:aegis%00.db", "file:aegis%ff.db", "aegis.db?mode=%xx",
		"aegis.db?=empty", "aegis.db?flag", "aegis.db?mode=rw&&cache=private", "aegis.db?mode=rw&",
		"aegis.db?mode=rw;cache=private", "aegis.db?mode=rw&mode=ro", "aegis.db?mode=%00",
		"aegis.db?\x00=value", "aegis\x00.db", "aegis.db?" + pragma("foreign_keys"),
		"aegis.db?" + pragma("foreign_keys(OFF)"), "aegis.db?" + pragma("foreign_keys=false"),
		"aegis.db?" + pragma(`main."foreign_keys"('no')`), "aegis.db?" + pragma("busy_timeout(0)"),
		"aegis.db?" + pragma("busy_timeout(4999)"), "aegis.db?" + pragma("busy_timeout(5001)"),
		"aegis.db?" + pragma("foreign_keys(1)") + "&" + pragma("foreign_keys(1)"),
		"aegis.db?" + pragma("busy_timeout(5000)") + "&" + pragma("main.busy_timeout(0)"),
		"aegis.db?" + pragma("foreign_keys(1)") + "&" + pragma("[foreign_keys](OFF)"),
		"aegis.db?" + pragma("journal_mode(DELETE)"),
		"aegis.db?mode=memory&" + pragma("journal_mode(MEMORY)"),
		"aegis.db?" + pragma("cache_size(100); PRAGMA foreign_keys=OFF"),
		"aegis.db?" + pragma("cache_size=100-- comment"), "aegis.db?" + pragma("foreign_keys/*comment*/(0)"),
		"aegis.db?" + pragma("foreign_keys(1)\n"), "aegis.db?" + pragma("foreign_keys('on''')"),
		"aegis.db?%5Fpragma=" + url.QueryEscape("foreign_keys(OFF)"),
		"aegis.db?" + pragma("foreign_keys(1 OR 0)"), "aegis.db?" + pragma("busy_timeout(5000+1)"),
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			normalized, filename, err := normalizeSQLiteDSN(dsn)
			require.Error(t, err)
			require.Empty(t, normalized)
			require.Empty(t, filename)
		})
	}
	path := filepath.Join(t.TempDir(), "must-not-create", "database.db")
	s, err := Open(context.Background(), path+"?"+pragma("foreign_keys(OFF)"))
	require.Error(t, err)
	require.Nil(t, s)
	_, err = os.Stat(filepath.Dir(path))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func storeFileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func TestStoreDSNFileIdentity(t *testing.T) {
	root := t.TempDir()
	// No parallel tests: a relative filename is resolved exactly as before.
	t.Chdir(root)
	cases := []struct {
		name, filename string
		uri            bool
		options        string
	}{
		{"relative", "relative.db", false, ""}, {"absolute", "absolute.db", false, ""},
		{"spaces", "Network Files with spaces.db", false, ""}, {"unicode", "شبکه-日本語.db", false, ""},
		{"literal-percent", "literal%20 + #.db", false, ""},
		{"file-uri", "uri.db", true, ""}, {"uri-options", "options.db", true, "mode=rwc&cache=private&_txlock=immediate"},
		{"encoded-uri", "encoded space+شبکه#%.db", true, ""},
		{"security-options", "security.db", true, "_pragma=foreign_keys%3DON&_pragma=busy_timeout%285000%29"},
		{"unrelated-pragma", "unrelated.db", true, "_pragma=cache_size%28-2048%29"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(root, c.filename)
			dsn := path
			if c.name == "relative" {
				dsn = c.filename
			}
			if c.uri {
				dsn = storeFileURI(path)
			}
			if c.options != "" {
				dsn += "?" + c.options
			}
			s, err := Open(context.Background(), dsn)
			require.NoError(t, err)
			defer func() { require.NoError(t, s.Close()) }()
			_, err = s.db.Exec("CREATE TABLE identity_marker(id INTEGER PRIMARY KEY); INSERT INTO identity_marker VALUES(41)")
			require.NoError(t, err)
			var old any
			for generation := 0; generation < 2; generation++ {
				conn, err := s.db.Conn(context.Background())
				require.NoError(t, err)
				physical := storeConnectionSettings(t, conn, "wal")
				if old != nil {
					require.NotSame(t, old, physical)
				}
				old = physical
				var actual string
				var marker int
				require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT file FROM pragma_database_list WHERE name='main'").Scan(&actual))
				intendedInfo, err := os.Stat(path)
				require.NoError(t, err)
				actualInfo, err := os.Stat(actual)
				require.NoError(t, err)
				require.True(t, os.SameFile(intendedInfo, actualInfo), "normalization must not change database identity")
				require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT id FROM identity_marker").Scan(&marker))
				require.Equal(t, 41, marker)
				if c.name == "unrelated-pragma" {
					var cache int
					require.NoError(t, conn.QueryRowContext(context.Background(), "PRAGMA cache_size").Scan(&cache))
					require.Equal(t, -2048, cache)
				}
				if generation == 0 {
					storeDiscardConnection(t, conn)
				} else {
					require.NoError(t, conn.Close())
				}
			}
		})
	}
}

func TestStoreReadOnlyURI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	s, err := Open(context.Background(), path)
	require.NoError(t, err)
	_, err = s.db.Exec("CREATE TABLE readonly_marker(id INTEGER); INSERT INTO readonly_marker VALUES(42)")
	require.NoError(t, err)
	require.NoError(t, s.Close())
	ro, err := Open(context.Background(), storeFileURI(path)+"?mode=ro")
	require.NoError(t, err)
	defer func() { require.NoError(t, ro.Close()) }()
	for generation := 0; generation < 2; generation++ {
		conn, err := ro.db.Conn(context.Background())
		require.NoError(t, err)
		storeConnectionSettings(t, conn, "wal")
		var marker int
		require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT id FROM readonly_marker").Scan(&marker))
		require.Equal(t, 42, marker)
		_, err = conn.ExecContext(context.Background(), "INSERT INTO readonly_marker VALUES(43)")
		var engineError *sqlitedriver.Error
		require.ErrorAs(t, err, &engineError)
		require.Equal(t, 8, engineError.Code()&0xff)
		if generation == 0 {
			storeDiscardConnection(t, conn)
		} else {
			require.NoError(t, conn.Close())
		}
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	absent, err := Open(context.Background(), storeFileURI(missing)+"?mode=ro")
	require.Error(t, err)
	require.Nil(t, absent)
	_, err = os.Stat(missing)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestStoreMemoryCompatibility(t *testing.T) {
	for _, dsn := range []string{":memory:", "file::memory:", "file:memory-" + uuid.NewString() + "?mode=memory",
		":memory:?_pragma=journal_mode%28MEMORY%29", "file:memory-" + uuid.NewString() + "?mode=memory&_pragma=journal_mode%28MEMORY%29"} {
		t.Run(dsn, func(t *testing.T) {
			s, err := Open(context.Background(), dsn)
			require.NoError(t, err)
			defer func() { require.NoError(t, s.Close()) }()
			conn, err := s.db.Conn(context.Background())
			require.NoError(t, err)
			old := storeConnectionSettings(t, conn, "memory")
			var count int
			require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM schema_migrations WHERE version='007_change_policy_evaluations.sql'").Scan(&count))
			require.Equal(t, 1, count, "required policy migration must be applied; later migrations are allowed")
			_, err = conn.ExecContext(context.Background(), "CREATE TABLE memory_marker(id INTEGER)")
			require.NoError(t, err)
			storeDiscardConnection(t, conn)
			conn, err = s.db.Conn(context.Background())
			require.NoError(t, err)
			require.NotSame(t, old, storeConnectionSettings(t, conn, "memory"))
			require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM sqlite_master WHERE name='memory_marker'").Scan(&count))
			require.Zero(t, count)
			require.NoError(t, conn.Close())
			t.Log("sole physical memory connection destruction loses data/schema; replacement settings remain safe")
		})
	}
	for _, dsn := range []string{"file::memory:?cache=shared", "file:shared-" + uuid.NewString() + "?mode=memory&cache=shared"} {
		t.Run(dsn, func(t *testing.T) {
			anchor, err := Open(context.Background(), dsn)
			require.NoError(t, err)
			defer func() { require.NoError(t, anchor.Close()) }()
			_, err = anchor.db.Exec("CREATE TABLE shared_marker(id INTEGER); INSERT INTO shared_marker VALUES(42)")
			require.NoError(t, err)
			s, err := Open(context.Background(), dsn)
			require.NoError(t, err)
			defer func() { require.NoError(t, s.Close()) }()
			for generation := 0; generation < 2; generation++ {
				conn, err := s.db.Conn(context.Background())
				require.NoError(t, err)
				storeConnectionSettings(t, conn, "memory")
				var marker int
				require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT id FROM shared_marker").Scan(&marker))
				require.Equal(t, 42, marker)
				if generation == 0 {
					storeDiscardConnection(t, conn)
				} else {
					require.NoError(t, conn.Close())
				}
			}
		})
	}
}
