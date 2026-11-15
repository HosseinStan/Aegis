package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sqlitedriver "modernc.org/sqlite"
)

func riskRepoFaults(t *testing.T, s *Store, faults *policyCleanupFaults) {
	t.Helper()
	var path string
	require.NoError(t, s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
	dsn, _, err := normalizeSQLiteDSN(path)
	require.NoError(t, err)
	underlying := s.db.Driver()
	require.NoError(t, s.db.Close())
	s.db = sql.OpenDB(&storeFaultConnector{underlying: underlying, dsn: dsn, faults: faults})
	s.db.SetMaxOpenConns(1)
}

func TestRiskRepositoryFailedCommitCleanup(t *testing.T) {
	for _, mode := range []string{"rollback succeeds", "driver rolled back", "cleanup uncertain"} {
		t.Run(mode, func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			faults := &policyCleanupFaults{rollbackBeforeError: mode == "driver rolled back"}
			faults.failCommit.Store(true)
			faults.failCleanup.Store(mode == "cleanup uncertain")
			riskRepoFaults(t, f.s, faults)
			response, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.ErrorIs(t, err, errPolicyInjectedCommit)
			require.Zero(t, response)
			require.EqualValues(t, 1, faults.cleanupAttempts.Load())
			if mode == "cleanup uncertain" {
				require.ErrorIs(t, err, errPolicyInjectedCleanup)
				require.EqualValues(t, 1, faults.closes.Load())
				require.Zero(t, f.s.db.Stats().OpenConnections)
			} else {
				require.Zero(t, faults.closes.Load())
			}
			require.Zero(t, riskRepoCount(t, f.s, "change_risk_assessments"))
			require.Zero(t, riskRepoCount(t, f.s, "change_risk_findings"))
			require.Zero(t, riskRepoCount(t, f.s, "change_risk_definitions"))
			require.Zero(t, riskRepoCount(t, f.s, "audit_logs"))
			if mode == "cleanup uncertain" {
				require.EqualValues(t, 2, faults.opens.Load())
			} else {
				require.EqualValues(t, 1, faults.opens.Load())
			}
			conn, err := f.s.db.Conn(ctx)
			require.NoError(t, err)
			storeConnectionSettings(t, conn, "wal")
			_, err = conn.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES(?,?)`, uuid.NewString(), uuid.NewString())
			var engine *sqlitedriver.Error
			require.ErrorAs(t, err, &engine)
			require.Equal(t, 787, engine.Code())
			require.NoError(t, conn.Close())
			faults.failCommit.Store(false)
			faults.failCleanup.Store(false)
			response, err = f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.NoError(t, err)
			require.True(t, response.Created)
			require.Zero(t, f.s.db.Stats().InUse)
			t.Logf("mode=%s cleanup=%d opens=%d closes=%d; no partial rows, replacement/reused connection FK=ON busy=5000 WAL, FK violation code=787", mode, faults.cleanupAttempts.Load(), faults.opens.Load(), faults.closes.Load())
		})
	}
}

type riskTestConnector struct {
	underlying driver.Driver
	dsn        string
	decorate   func(driver.Conn) driver.Conn
}

func (c *riskTestConnector) Driver() driver.Driver { return c.underlying }
func (c *riskTestConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := c.underlying.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return c.decorate(conn), nil
}
func riskRepoDecorate(t *testing.T, s *Store, decorate func(driver.Conn) driver.Conn) {
	t.Helper()
	var path string
	require.NoError(t, s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
	dsn, _, err := normalizeSQLiteDSN(path)
	require.NoError(t, err)
	base := s.db.Driver()
	require.NoError(t, s.db.Close())
	s.db = sql.OpenDB(&riskTestConnector{underlying: base, dsn: dsn, decorate: decorate})
	s.db.SetMaxOpenConns(1)
}

type riskRollbackConn struct {
	*policyCleanupConn
	failRollback *atomic.Bool
	lostCommit   *atomic.Bool
}

func (c *riskRollbackConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.policyCleanupConn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &riskRollbackTx{Tx: tx, failRollback: c.failRollback, lostCommit: c.lostCommit}, nil
}

type riskRollbackTx struct {
	driver.Tx
	failRollback *atomic.Bool
	lostCommit   *atomic.Bool
}

var errRiskOperationRollback = errors.New("injected operation rollback failure")

func (tx *riskRollbackTx) Rollback() error {
	if tx.failRollback != nil && tx.failRollback.Load() {
		return errRiskOperationRollback
	}
	return tx.Tx.Rollback()
}
func (tx *riskRollbackTx) Commit() error {
	if tx.lostCommit != nil && tx.lostCommit.Load() {
		if err := tx.Tx.Commit(); err != nil {
			return err
		}
		return errPolicyInjectedCommit
	}
	return tx.Tx.Commit()
}

func TestRiskRepositoryOperationRollbackChecked(t *testing.T) {
	for _, cleanupFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "backend resolves failed operation rollback", true: "both cleanup paths fail"}[cleanupFails], func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			faults := &policyCleanupFaults{}
			faults.failCleanup.Store(cleanupFails)
			fail := &atomic.Bool{}
			fail.Store(true)
			riskRepoDecorate(t, f.s, func(conn driver.Conn) driver.Conn {
				faults.opens.Add(1)
				return &riskRollbackConn{policyCleanupConn: &policyCleanupConn{Conn: conn, faults: faults}, failRollback: fail}
			})
			original := errors.New("original operation failure")
			err := f.s.riskTransaction(context.Background(), func(tx *sql.Tx) error {
				_, err := tx.Exec(`INSERT INTO audit_logs(id,request_id,action,succeeded,duration_ms,created_at) VALUES('uncommitted','test','test',1,0,'2026-10-04T17:00:00Z')`)
				if err != nil {
					return err
				}
				return original
			})
			require.ErrorIs(t, err, original)
			require.ErrorIs(t, err, errRiskOperationRollback)
			require.EqualValues(t, 1, faults.cleanupAttempts.Load())
			if cleanupFails {
				require.ErrorIs(t, err, errPolicyInjectedCleanup)
				require.EqualValues(t, 1, faults.closes.Load())
				require.Zero(t, f.s.db.Stats().OpenConnections)
			}
			fail.Store(false)
			faults.failCleanup.Store(false)
			require.Zero(t, riskRepoCount(t, f.s, "audit_logs"))
			result, err := f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
			require.NoError(t, err)
			require.True(t, result.Created)
		})
	}
}

func TestRiskRepositoryLostCommitResponse(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	faults := &policyCleanupFaults{}
	lost := &atomic.Bool{}
	lost.Store(true)
	riskRepoDecorate(t, f.s, func(conn driver.Conn) driver.Conn {
		return &riskRollbackConn{policyCleanupConn: &policyCleanupConn{Conn: conn, faults: faults}, lostCommit: lost}
	})
	response, err := f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.ErrorIs(t, err, errPolicyInjectedCommit)
	require.Zero(t, response)
	require.Equal(t, 1, riskRepoCount(t, f.s, "change_risk_assessments"))
	require.Equal(t, 1, riskRepoCount(t, f.s, "audit_logs"))
	lost.Store(false)
	response, err = f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f).Add(time.Second))
	require.NoError(t, err)
	require.False(t, response.Created)
	require.Equal(t, 2, riskRepoCount(t, f.s, "audit_logs"))
	require.Equal(t, 1, riskRepoCount(t, f.s, "change_risk_assessments"))
}

func TestRiskRepositoryExactRollbackOutcome(t *testing.T) {
	require.False(t, riskRollbackFinished(nil))
	require.False(t, riskRollbackFinished(sql.ErrTxDone))
	require.False(t, riskRollbackFinished(errors.New("SQL logic error: cannot rollback - no transaction is active (1)")))
	f := authorizationStore(t)
	conn, err := f.s.db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(context.Background(), "ROLLBACK")
	require.True(t, riskRollbackFinished(err))
	require.False(t, riskRollbackFinished(errors.Join(errors.New("not SQLite"), errors.New(err.Error()))))
}

type riskBarrierConn struct {
	*policyCleanupConn
	ready   chan<- struct{}
	release <-chan struct{}
	used    atomic.Bool
}

func (c *riskBarrierConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.policyCleanupConn.QueryContext(ctx, query, args)
	if err == nil && strings.HasPrefix(query, "SELECT id FROM change_risk_assessments WHERE event_id=") && c.used.CompareAndSwap(false, true) {
		select {
		case c.ready <- struct{}{}:
		case <-ctx.Done():
			rows.Close()
			return nil, ctx.Err()
		}
		select {
		case <-c.release:
		case <-ctx.Done():
			rows.Close()
			return nil, ctx.Err()
		}
	}
	return rows, err
}
func riskRepoBarrier(t *testing.T, s *Store, ready chan<- struct{}, release <-chan struct{}) {
	t.Helper()
	riskRepoDecorate(t, s, func(conn driver.Conn) driver.Conn {
		return &riskBarrierConn{policyCleanupConn: &policyCleanupConn{Conn: conn, faults: &policyCleanupFaults{}}, ready: ready, release: release}
	})
}

func TestRiskRepositoryCompetingPredecessors(t *testing.T) {
	f := authorizationStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	at := riskRepoAt(f)
	a := f.approve(t, f.request(t))
	root, err := f.s.RecordRisk(ctx, scope, nil, at)
	require.NoError(t, err)
	link := f.link(a, e)
	link.LinkedAt = at.Add(time.Minute)
	require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
	var path string
	require.NoError(t, f.s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
	other, err := Open(ctx, path)
	require.NoError(t, err)
	defer other.Close()
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	riskRepoBarrier(t, f.s, ready, release)
	riskRepoBarrier(t, other, ready, release)
	results := make(chan error, 2)
	for _, s := range []*Store{f.s, other} {
		go func(s *Store) {
			_, err := s.RecordRisk(ctx, scope, &root.Assessment.ID, at.Add(2*time.Minute))
			results <- err
		}(s)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, domain.ErrConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	require.Equal(t, 2, riskRepoCount(t, f.s, "change_risk_assessments"))
	require.Equal(t, 2, riskRepoCount(t, f.s, "audit_logs"))
	latest, err := f.s.GetRisk(ctx, scope, "")
	require.NoError(t, err)
	require.Equal(t, 2, latest.Revision)
	require.Equal(t, &root.Assessment.ID, latest.PreviousAssessmentID)
}

func TestRiskRepositoryConcurrentGovernanceAndGrantChanges(t *testing.T) {
	for _, change := range []string{"policy", "authorization", "revocation", "grant", "group", "disabled"} {
		t.Run(change, func(t *testing.T) {
			f := authorizationStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			e := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			at := riskRepoAt(f)
			a := f.approve(t, f.request(t))
			f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(time.Hour))
			link := f.link(a, e)
			if change == "revocation" {
				require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
			}
			if change == "group" {
				_, err := f.s.db.Exec(`DELETE FROM user_roles WHERE user_id=?`, f.approver)
				require.NoError(t, err)
				require.NoError(t, f.s.AssignRole(ctx, f.approver, "network_administrator"))
				_, err = f.s.db.Exec(`UPDATE devices SET device_group='campus' WHERE id=?`, f.device)
				require.NoError(t, err)
				_, err = f.s.db.Exec(`INSERT INTO user_device_groups(user_id,device_group) VALUES(?,'campus')`, f.approver)
				require.NoError(t, err)
			}
			var path string
			require.NoError(t, f.s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
			other, err := Open(ctx, path)
			require.NoError(t, err)
			defer other.Close()
			ready := make(chan struct{}, 1)
			release := make(chan struct{})
			riskRepoBarrier(t, f.s, ready, release)
			result := make(chan error, 1)
			go func() { _, err := f.s.RecordRisk(ctx, scope, nil, at); result <- err }()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			switch change {
			case "policy":
				_, err = other.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, at.Add(-time.Minute))
			case "authorization":
				link.LinkedAt = at.Add(-time.Minute)
				err = other.LinkChangeEventAuthorization(ctx, f.device, link)
			case "revocation":
				_, err = other.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "revoked", at.Add(-time.Minute))
			case "grant":
				_, err = other.db.Exec(`DELETE FROM role_permissions WHERE permission_id=(SELECT id FROM permissions WHERE name='changes.risk.evaluate')`)
			case "group":
				_, err = other.db.Exec(`UPDATE user_device_groups SET device_group='elsewhere' WHERE user_id=?`, f.approver)
			case "disabled":
				_, err = other.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, f.approver)
			}
			require.NoError(t, err)
			close(release)
			require.ErrorIs(t, <-result, domain.ErrConflict, "stale WAL snapshot must not seal or audit")
			require.Zero(t, riskRepoCount(t, f.s, "change_risk_assessments"))
			require.Zero(t, riskRepoCount(t, f.s, "audit_logs"))
			response, err := f.s.RecordRisk(ctx, scope, nil, at)
			if change == "grant" || change == "group" || change == "disabled" {
				require.ErrorIs(t, err, domain.ErrForbidden)
			} else {
				require.NoError(t, err)
				require.True(t, response.Created)
			}
		})
	}
}
