package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
	sqlitedriver "modernc.org/sqlite"
)

func riskFailure(err error) error {
	if err == nil {
		return nil
	}
	var existing *repositories.RiskFailure
	if errors.As(err, &existing) {
		return err
	}
	for _, kind := range []error{repositories.ErrRiskUnavailable, domain.ErrForbidden, domain.ErrNotFound, domain.ErrUnauthorized, domain.ErrValidation, domain.ErrConflict, risk.ErrIntegrity, risk.ErrLimit, risk.ErrIneligible} {
		if errors.Is(err, kind) {
			return &repositories.RiskFailure{Kind: kind, Cause: err}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &repositories.RiskFailure{Kind: domain.ErrTimeout, Cause: err}
	}
	var engine *sqlitedriver.Error
	if errors.As(err, &engine) && (engine.Code()&255 == 5 || engine.Code()&255 == 6) {
		return &repositories.RiskFailure{Kind: domain.ErrConflict, Cause: err}
	}
	return &repositories.RiskFailure{Kind: repositories.ErrRiskUnavailable, Cause: err}
}
func riskRollbackFinished(err error) bool {
	var engine *sqlitedriver.Error
	return errors.As(err, &engine) && engine.Code() == 1 && engine.Error() == "SQL logic error: cannot rollback - no transaction is active (1)"
}
func riskDiscard(conn *sql.Conn) error {
	err := conn.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	return err
}
func riskBackendRollback(ctx context.Context, conn *sql.Conn) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, err := conn.ExecContext(cleanup, "ROLLBACK")
	if err == nil || riskRollbackFinished(err) {
		return nil
	}
	return err
}
func riskConnection(ctx context.Context, conn *sql.Conn) error {
	for _, check := range []struct {
		query string
		want  int
	}{
		{"PRAGMA foreign_keys", 1}, {"PRAGMA busy_timeout", 5000}, {"PRAGMA ignore_check_constraints", 0}, {"PRAGMA writable_schema", 0}, {"PRAGMA read_uncommitted", 0}, {"PRAGMA trusted_schema", 1},
	} {
		var actual int
		if err := conn.QueryRowContext(ctx, check.query).Scan(&actual); err != nil {
			return err
		}
		if actual != check.want {
			return repositories.ErrRiskUnavailable
		}
	}
	var journal, abi string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		return err
	}
	if journal != "wal" {
		return repositories.ErrRiskUnavailable
	}
	if err := conn.QueryRowContext(ctx, `SELECT aegis_risk_utc_nano_v1('2026-10-01T15:30:00.000000001+03:30')`).Scan(&abi); err != nil {
		return err
	}
	if abi != "2026-10-01T12:00:00.000000001Z" {
		return repositories.ErrRiskUnavailable
	}
	return nil
}

// No correctness mutex: SQLite WAL snapshot/unique/predecessor constraints
// serialize independent Stores. Retain the physical connection through cleanup.
func (s *Store) riskTransaction(ctx context.Context, operation func(*sql.Tx) error) (resultErr error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return riskFailure(err)
	}
	defer conn.Close()
	if err = riskConnection(ctx, conn); err != nil {
		return riskFailure(errors.Join(repositories.ErrRiskUnavailable, err, riskDiscard(conn)))
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return riskFailure(err)
	}
	finished := false
	defer func() {
		if !finished {
			_ = tx.Rollback()
			if err := riskBackendRollback(ctx, conn); err != nil {
				_ = riskDiscard(conn)
			}
		}
	}()
	if err = operation(tx); err != nil {
		txErr := tx.Rollback()
		backendErr := riskBackendRollback(ctx, conn)
		finished = true
		if backendErr != nil {
			return riskFailure(errors.Join(repositories.ErrRiskUnavailable, err, txErr, fmt.Errorf("risk backend rollback: %w", backendErr), riskDiscard(conn)))
		}
		if txErr != nil && !errors.Is(txErr, sql.ErrTxDone) {
			return riskFailure(errors.Join(repositories.ErrRiskUnavailable, err, fmt.Errorf("risk transaction rollback: %w", txErr)))
		}
		return riskFailure(err)
	}
	if err = tx.Commit(); err != nil {
		cleanupErr := riskBackendRollback(ctx, conn)
		finished = true
		if cleanupErr != nil {
			return riskFailure(errors.Join(repositories.ErrRiskUnavailable, err, fmt.Errorf("risk commit rollback: %w", cleanupErr), riskDiscard(conn)))
		}
		// A commit error is never converted into an idempotent success, even if
		// the driver outcome was uncertain. A later request must replay afresh.
		return riskFailure(err)
	}
	finished = true
	return nil
}
