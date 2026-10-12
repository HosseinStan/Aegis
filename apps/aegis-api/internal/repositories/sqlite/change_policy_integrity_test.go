package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/policy"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sqlitedriver "modernc.org/sqlite"
)

func TestPolicyActivationRejectsCorruptDefinitionAndLiveRevocation(t *testing.T) {
	for _, scenario := range []string{"invalid-actor", "disabled", "revoked", "hash", "version", "unsorted-rules"} {
		t.Run(scenario, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			p := f.policy(t, completePolicyRules)
			actor := f.approver
			want := domain.ErrForbidden
			switch scenario {
			case "invalid-actor":
				actor = uuid.NewString()
			case "disabled":
				_, err := f.s.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, actor)
				require.NoError(t, err)
			case "revoked":
				_, err := f.s.db.Exec(`DELETE FROM user_roles WHERE user_id=?`, actor)
				require.NoError(t, err)
			default:
				want = domain.ErrValidation
				_, err := f.s.db.Exec(`DROP TRIGGER change_policy_definition_immutable`)
				require.NoError(t, err)
				query := `UPDATE change_policy_sets SET definition_hash=? WHERE id=?`
				value := any(strings.Repeat("0", 64))
				if scenario == "version" {
					_, err := f.s.db.Exec(`PRAGMA ignore_check_constraints=ON`)
					require.NoError(t, err)
					query = `UPDATE change_policy_sets SET engine_version=? WHERE id=?`
					value = 2
				}
				if scenario == "unsorted-rules" {
					query = `UPDATE change_policy_sets SET rules_json=? WHERE id=?`
					value = `[{"category":"interface","operation":"remove","decision":"allow"},{"category":"aaa","operation":"add","decision":"allow"}]`
				}
				_, err = f.s.db.Exec(query, value, p.ID)
				require.NoError(t, err)
			}
			_, err := f.s.ActivatePolicySet(ctx, f.device, p.ID, actor, f.now.Add(time.Hour))
			require.ErrorIs(t, err, want)
			var status string
			require.NoError(t, f.s.db.QueryRow(`SELECT status FROM change_policy_sets WHERE id=?`, p.ID).Scan(&status))
			require.Equal(t, "draft", status)
		})
	}
}

func TestPolicyFinalizedSourceIntegrity(t *testing.T) {
	for _, outcome := range []string{domain.PolicyStatusPass, domain.PolicyStatusViolation} {
		t.Run(outcome, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			rules := []domain.PolicyRule{{Category: "aaa", Operation: "add", Decision: "allow"}, {Category: "interface", Operation: "remove", Decision: "allow"}}
			if outcome == domain.PolicyStatusViolation {
				rules[0].Decision = "deny"
			}
			f.activatePolicy(t, f.policy(t, rules), f.now.Add(time.Hour))
			e := authorizationEvent(t, f.s, f.device)
			other := authorizationEvent(t, f.s, f.otherDevice)
			alternate := eventBackup(f.device, "running", f.now.Add(-time.Hour))
			require.NoError(t, f.s.CreateBackup(ctx, alternate))
			original, _, err := f.s.ListChangeEventItems(ctx, e.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			before, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			evidence, err := f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(3*time.Hour))
			require.NoError(t, err, "header/items must not block the original atomic transition")
			require.Equal(t, outcome, evidence.Result)
			itemEvidence, _, err := f.s.ListPolicyEvaluationItems(ctx, f.device, e.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			finalEvent, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			before.PolicyStatus, before.UpdatedAt = outcome, evidence.EvaluatedAt
			require.Equal(t, before, finalEvent)

			for _, mutation := range []struct {
				column string
				value  any
			}{
				{"id", uuid.NewString()}, {"change_event_id", other.ID}, {"ordinal", 100}, {"operation", "add"},
				{"command", "description altered"}, {"context_path_json", `["interface Gi9"]`}, {"sensitive", 1},
				{"classification_version", 2}, {"category", "system"}, {"subcategory", "altered"},
				{"classification_rule", "altered.rule"}, {"classified_at", nil}, {"created_at", timestamp(f.now)},
			} {
				t.Run("item/"+mutation.column, func(t *testing.T) {
					_, err := f.s.db.Exec(`UPDATE change_event_items SET `+mutation.column+`=? WHERE id=?`, mutation.value, original[0].ID)
					require.ErrorContains(t, err, "immutable finalized policy source item")
				})
			}
			for _, mutation := range []struct {
				column string
				value  any
			}{
				{"id", uuid.NewString()}, {"device_id", f.otherDevice}, {"config_type", "startup"},
				{"from_backup_id", alternate.ID}, {"to_backup_id", alternate.ID}, {"detected_at", timestamp(e.DetectedAt.Add(time.Nanosecond))},
				{"added_lines", e.AddedLines + 1}, {"removed_lines", e.RemovedLines + 1}, {"analysis_version", 2},
				{"analyzed_at", nil}, {"item_count", e.ItemCount + 1}, {"created_at", timestamp(f.now)},
			} {
				t.Run("event/"+mutation.column, func(t *testing.T) {
					_, err := f.s.db.Exec(`UPDATE change_events SET `+mutation.column+`=? WHERE id=?`, mutation.value, e.ID)
					require.ErrorContains(t, err, "immutable finalized policy source event")
				})
			}
			_, err = f.s.db.Exec(`INSERT INTO change_event_items (`+changeEventItemColumns+`) SELECT ?, change_event_id, 2, operation,
 context_path_json, command, sensitive, created_at, classification_version, category, subcategory, classification_rule, classified_at
 FROM change_event_items WHERE id=?`, uuid.NewString(), original[0].ID)
			require.ErrorContains(t, err, "immutable finalized policy source item")
			_, err = f.s.db.Exec(`UPDATE change_event_items SET change_event_id=?, ordinal=2 WHERE change_event_id=? AND ordinal=0`, e.ID, other.ID)
			require.ErrorContains(t, err, "immutable finalized policy source item", "cannot move unfinalized evidence into a finalized event")
			_, err = f.s.db.Exec(`DELETE FROM change_event_items WHERE id=?`, original[0].ID)
			require.ErrorContains(t, err, "immutable finalized policy source item")
			for _, status := range []string{"not_evaluated", "pass", "violation"} {
				_, err = f.s.db.Exec(`UPDATE change_events SET policy_status=? WHERE id=?`, status, e.ID)
				require.ErrorContains(t, err, "immutable final policy status")
			}
			afterItems, _, err := f.s.ListChangeEventItems(ctx, e.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			require.Equal(t, original, afterItems)
			afterEvent, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, finalEvent, afterEvent)
			afterEvidence, err := f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
			require.NoError(t, err)
			require.Equal(t, evidence, afterEvidence)
			afterItemEvidence, _, err := f.s.ListPolicyEvaluationItems(ctx, f.device, e.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			require.Equal(t, itemEvidence, afterItemEvidence)
			var definition domain.PolicyDefinition
			require.NoError(t, json.Unmarshal([]byte(evidence.PolicyDefinitionSnapshot), &definition))
			reproduced, err := policy.Evaluate(definition, afterItems)
			require.NoError(t, err)
			require.Equal(t, outcome, reproduced.Outcome)
			require.Equal(t, evidence.TotalItems, reproduced.TotalItems)
			require.Equal(t, evidence.AllowedCount, reproduced.AllowedCount)
			require.Equal(t, evidence.DeniedCount, reproduced.DeniedCount)
			require.Equal(t, evidence.UncoveredCount, reproduced.UncoveredCount)
			_, err = f.s.RetirePolicySet(ctx, f.device, evidence.PolicySetID, f.approver, f.now.Add(4*time.Hour))
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE change_event_items SET command='description after-retirement' WHERE id=?`, original[0].ID)
			require.ErrorContains(t, err, "immutable finalized policy source item")
		})
	}
}

func TestPolicyNonFinalizedSourceRetainsExistingBehavior(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	e := authorizationEvent(t, f.s, f.device)
	items, _, err := f.s.ListChangeEventItems(ctx, e.ID, repositories.Page{Limit: 200})
	require.NoError(t, err)
	_, err = f.s.db.Exec(`UPDATE change_event_items SET command='description before-finalization' WHERE id=?`, items[0].ID)
	require.NoError(t, err)
	_, err = f.s.db.Exec(`INSERT INTO change_event_items (`+changeEventItemColumns+`) SELECT ?, change_event_id, 2, operation,
 context_path_json, command, sensitive, created_at, classification_version, category, subcategory, classification_rule, classified_at
 FROM change_event_items WHERE id=?`, uuid.NewString(), items[0].ID)
	require.NoError(t, err)
	_, err = f.s.db.Exec(`DELETE FROM change_event_items WHERE change_event_id=? AND ordinal=2`, e.ID)
	require.NoError(t, err)
	_, err = f.s.db.Exec(`UPDATE change_events SET analyzed_at=NULL, item_count=0 WHERE id=?`, e.ID)
	require.NoError(t, err)
}

func TestPolicyFinalizationAndAuthorizationBothOrders(t *testing.T) {
	for _, outcome := range []string{domain.PolicyStatusPass, domain.PolicyStatusViolation} {
		for _, first := range []string{"policy", "authorization"} {
			t.Run(outcome+"/"+first, func(t *testing.T) {
				f := authorizationStore(t)
				ctx := context.Background()
				rules := []domain.PolicyRule{{Category: "aaa", Operation: "add", Decision: "allow"}, {Category: "interface", Operation: "remove", Decision: "allow"}}
				if outcome == domain.PolicyStatusViolation {
					rules[0].Decision = "deny"
				}
				f.activatePolicy(t, f.policy(t, rules), f.now.Add(time.Hour))
				e := authorizationEvent(t, f.s, f.device)
				a := f.approve(t, f.request(t))
				link := f.link(a, e)
				link.LinkedAt = f.now.Add(210 * time.Minute)
				var evidence domain.ChangePolicyEvaluation
				var historyBefore []domain.ChangeAuthorizationHistory
				var err error
				if first == "policy" {
					evidence, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(3*time.Hour))
					require.NoError(t, err)
					require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
				} else {
					require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
					historyBefore, _, err = f.s.ListChangeAuthorizationHistory(ctx, f.device, a.ID, repositories.Page{Limit: 200})
					require.NoError(t, err)
					evidence, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
					require.NoError(t, err)
				}
				got, err := f.s.GetChangeEvent(ctx, e.ID)
				require.NoError(t, err)
				require.Equal(t, "authorized", got.AuthorizationStatus)
				require.Equal(t, outcome, got.PolicyStatus)
				if first == "policy" {
					require.Equal(t, link.LinkedAt, got.UpdatedAt)
				} else {
					require.Equal(t, evidence.EvaluatedAt, got.UpdatedAt)
				}
				gotLink, err := f.s.GetChangeEventAuthorization(ctx, f.device, e.ID)
				require.NoError(t, err)
				require.Equal(t, link, gotLink)
				history, _, err := f.s.ListChangeAuthorizationHistory(ctx, f.device, a.ID, repositories.Page{Limit: 200})
				require.NoError(t, err)
				require.Len(t, history, 3)
				if first == "authorization" {
					require.Equal(t, historyBefore, history)
				}
				loaded, err := f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
				require.NoError(t, err)
				require.Equal(t, evidence, loaded)
				// Independent revocation/history changes must remain operational too.
				_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "revoked", f.now.Add(5*time.Hour))
				require.NoError(t, err)
				loaded, err = f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
				require.NoError(t, err)
				require.Equal(t, evidence, loaded)
			})
		}
	}
}

// Test-only driver decoration injects errors at the real database/sql driver
// boundary. All statements, transactions and close/rollback effects still use
// SQLite; production needs no new abstraction or global connection changes.
type policyCleanupFaults struct {
	failCommit          atomic.Bool
	failCleanup         atomic.Bool
	rollbackBeforeError bool
	opens               atomic.Int64
	closes              atomic.Int64
	cleanupAttempts     atomic.Int64
}

type policyCleanupConnector struct {
	driver driver.Driver
	dsn    string
	faults *policyCleanupFaults
}

func (c *policyCleanupConnector) Driver() driver.Driver { return c.driver }
func (c *policyCleanupConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	if _, err := conn.(driver.ExecerContext).ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;", nil); err != nil {
		conn.Close()
		return nil, err
	}
	c.faults.opens.Add(1)
	return &policyCleanupConn{Conn: conn, faults: c.faults}, nil
}

type policyCleanupConn struct {
	driver.Conn
	faults *policyCleanupFaults
}

func (c *policyCleanupConn) Close() error  { c.faults.closes.Add(1); return c.Conn.Close() }
func (c *policyCleanupConn) IsValid() bool { return c.Conn.(driver.Validator).IsValid() }
func (c *policyCleanupConn) ResetSession(ctx context.Context) error {
	return c.Conn.(driver.SessionResetter).ResetSession(ctx)
}
func (c *policyCleanupConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c *policyCleanupConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == "ROLLBACK" {
		c.faults.cleanupAttempts.Add(1)
		if c.faults.failCleanup.Load() {
			return nil, errPolicyInjectedCleanup
		}
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}
func (c *policyCleanupConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &policyCleanupTx{Tx: tx, faults: c.faults}, nil
}

type policyCleanupTx struct {
	driver.Tx
	faults *policyCleanupFaults
}

var errPolicyInjectedCommit = errors.New("injected commit failure")
var errPolicyInjectedCleanup = errors.New("injected rollback cleanup failure")

func (tx *policyCleanupTx) Commit() error {
	if tx.faults.failCommit.Load() {
		if tx.faults.rollbackBeforeError {
			if err := tx.Tx.Rollback(); err != nil {
				return errors.Join(errPolicyInjectedCommit, err)
			}
		}
		return errPolicyInjectedCommit
	}
	return tx.Tx.Commit()
}

func TestPolicyFailedCommitConnectionCleanup(t *testing.T) {
	for _, mode := range []string{"rollback-success", "driver-already-rolled-back", "rollback-failure"} {
		t.Run(mode, func(t *testing.T) {
			f := authorizationStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(time.Hour))
			e := authorizationEvent(t, f.s, f.device)
			before, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			var path string
			require.NoError(t, f.s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
			require.NoError(t, f.s.db.Close())
			faults := &policyCleanupFaults{rollbackBeforeError: mode == "driver-already-rolled-back"}
			faults.failCommit.Store(true)
			faults.failCleanup.Store(mode == "rollback-failure")
			f.s.db = sql.OpenDB(&policyCleanupConnector{driver: &sqlitedriver.Driver{}, dsn: path, faults: faults})
			f.s.db.SetMaxOpenConns(1)
			_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
			require.ErrorIs(t, err, errPolicyInjectedCommit)
			require.EqualValues(t, 1, faults.cleanupAttempts.Load())
			if mode == "rollback-failure" {
				require.ErrorIs(t, err, errPolicyInjectedCleanup, "unexpected cleanup error must not be swallowed")
				require.EqualValues(t, 1, faults.closes.Load(), "uncertain physical connection must be discarded")
				require.Zero(t, f.s.db.Stats().OpenConnections)
			} else {
				require.Zero(t, faults.closes.Load(), "confirmed cleanup can safely reuse the connection")
			}
			after, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			for _, table := range []string{"change_policy_evaluations", "change_policy_evaluation_items"} {
				var count int
				require.NoError(t, f.s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count))
				require.Zero(t, count)
			}
			if mode == "rollback-failure" {
				require.EqualValues(t, 2, faults.opens.Load())
			} else {
				require.EqualValues(t, 1, faults.opens.Load())
			}
			faults.failCommit.Store(false)
			faults.failCleanup.Store(false)
			// A full new transaction, not just a SELECT, must succeed and persist
			// on the reused/new connection without inheriting stale transaction state.
			_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
			require.NoError(t, err)
			loaded, err := f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
			require.NoError(t, err)
			require.Equal(t, "pass", loaded.Result)
			require.Zero(t, f.s.db.Stats().InUse)
		})
	}
}

func TestPolicyRollbackFinishedRequiresSpecificSQLiteError(t *testing.T) {
	f := authorizationStore(t)
	_, err := f.s.db.Exec("ROLLBACK")
	require.Error(t, err)
	require.True(t, policyRollbackFinished(err))
	for _, err := range []error{sql.ErrTxDone, sql.ErrConnDone, context.DeadlineExceeded, errPolicyInjectedCleanup,
		errors.New("cannot rollback - no transaction is active")} {
		require.False(t, policyRollbackFinished(err))
	}
}

func TestPolicyDeniedAndUncoveredEvidenceAndDefensiveConstraints(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	p := f.activatePolicy(t, f.policy(t, []domain.PolicyRule{{Category: "aaa", Operation: "add", Decision: "deny"}}), f.now.Add(time.Hour))
	e := authorizationEvent(t, f.s, f.device)
	_, err := f.s.db.Exec(`UPDATE change_event_items SET ordinal=0 WHERE change_event_id=? AND ordinal=1`, e.ID)
	require.Error(t, err)
	evidence, err := f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
	require.NoError(t, err)
	require.Equal(t, "violation", evidence.Result)
	require.Equal(t, 1, evidence.DeniedCount)
	require.Equal(t, 1, evidence.UncoveredCount)
	require.Zero(t, evidence.AllowedCount)
	items, total, err := f.s.ListPolicyEvaluationItems(ctx, f.device, e.ID, repositories.Page{Limit: 1, Offset: 100})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, 2, total)
	_, _, err = f.s.ListPolicyEvaluationItems(ctx, f.device, e.ID, repositories.Page{Limit: 1, Offset: -1})
	require.ErrorIs(t, err, domain.ErrValidation)
	for _, query := range []string{`INSERT INTO change_policy_evaluations (` + evaluationColumns + `) SELECT 'other',event_id,policy_set_id,engine_version,policy_definition_hash,policy_definition_snapshot,result,total_items,allowed_count,denied_count,uncovered_count,evaluated_by,evaluated_at FROM change_policy_evaluations`,
		`INSERT INTO change_policy_evaluation_items SELECT evaluation_id,event_id,change_event_item_id,ordinal,category,operation,decision,matched_rule_key FROM change_policy_evaluation_items`,
		`INSERT INTO change_policy_sets (` + policyColumns + `) SELECT 'other',lower(reference),device_id,config_type,status,engine_version,rules_json,definition_hash,created_by,created_at,activated_by,activated_at,retired_by,retired_at,updated_at FROM change_policy_sets`} {
		_, err = f.s.db.Exec(query)
		require.Error(t, err)
	}
	// A second policy can never replace even a violation's existing final evidence.
	_, err = f.s.RetirePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(5*time.Hour))
	require.NoError(t, err)
	f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(5*time.Hour))
	_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(6*time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	got, err := f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
	require.NoError(t, err)
	require.Equal(t, evidence, got)
}
