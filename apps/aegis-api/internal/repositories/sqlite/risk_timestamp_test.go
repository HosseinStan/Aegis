package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"testing"

	"company.local/aegis/apps/aegis-api/internal/risk"
	"github.com/stretchr/testify/require"
	sqlitedriver "modernc.org/sqlite"
)

func TestRiskTimestampProductionRegistration(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "timestamp.db"))
	require.NoError(t, err)
	defer s.Close()
	var old []any
	for generation := 0; generation < 4; generation++ {
		conn, err := s.db.Conn(ctx)
		require.NoError(t, err)
		physical := storeConnectionSettings(t, conn, "wal")
		for _, p := range old {
			require.NotSame(t, p, physical)
		}
		old = append(old, physical)
		var canonical string
		require.NoError(t, conn.QueryRowContext(ctx, "SELECT aegis_risk_utc_nano_v1(?)", "2026-10-01T15:30:00.000000001+03:30").Scan(&canonical))
		require.Equal(t, "2026-10-01T12:00:00.000000001Z", canonical)
		var flags, trusted int
		require.NoError(t, conn.QueryRowContext(ctx, "SELECT flags FROM pragma_function_list WHERE name=? AND builtin=0", riskTimestampFunctionV1).Scan(&flags))
		require.NotZero(t, flags&2048, "function must be registered deterministic")
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA trusted_schema").Scan(&trusted))
		require.Equal(t, 1, trusted, "pinned modernc default must support this reviewed trigger contract")
		t.Logf("generation=%d SQL ABI available; deterministic flags=%d trusted_schema=%d", generation, flags, trusted)
		for _, value := range []any{1, 1.5, []byte("2026-10-01T12:00:00Z"), "bad-secret-canary", "2026-10-01T12:00:00.0000000001Z", "2026-10-01T12:00:60Z"} {
			err := conn.QueryRowContext(ctx, "SELECT aegis_risk_utc_nano_v1(?)", value).Scan(&canonical)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "bad-secret-canary")
		}
		var optional any
		require.NoError(t, conn.QueryRowContext(ctx, "SELECT aegis_risk_utc_nano_v1(NULL)").Scan(&optional))
		require.Nil(t, optional)
		if generation < 3 {
			storeDiscardConnection(t, conn)
		} else {
			require.NoError(t, conn.Close())
		}
	}
	// Collision reports an error and does not replace the existing registration.
	require.Error(t, sqlitedriver.RegisterDeterministicScalarFunction(riskTimestampFunctionV1, 1, riskTimestampScalarV1))
}

func TestRiskTimestampSQLBridgeAndTrust(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "bridge.db"))
	require.NoError(t, err)
	defer s.Close()
	_, err = s.db.Exec("CREATE TABLE abi_bridge(raw TEXT NOT NULL,canonical TEXT NOT NULL CHECK(aegis_risk_utc_nano_v1(raw) IS canonical));")
	require.NoError(t, err)
	_, err = s.db.Exec("INSERT INTO abi_bridge VALUES(?,?)", "2026-10-01T15:30:00+03:30", "2026-10-01T12:00:00.000000000Z")
	require.NoError(t, err)
	_, err = s.db.Exec("INSERT INTO abi_bridge VALUES(?,?)", "2026-10-01T15:30:00+03:30", "2026-10-01T12:00:00.000000001Z")
	var engineError *sqlitedriver.Error
	require.ErrorAs(t, err, &engineError)
	require.Equal(t, 275, engineError.Code())
	_, err = s.db.Exec("CREATE TABLE abi_trigger(raw TEXT,canonical TEXT); CREATE TRIGGER abi_guard BEFORE INSERT ON abi_trigger WHEN aegis_risk_utc_nano_v1(NEW.raw) IS NOT NEW.canonical BEGIN SELECT RAISE(ABORT,'timestamp bridge mismatch'); END;")
	require.NoError(t, err)
	_, err = s.db.Exec("INSERT INTO abi_trigger VALUES(?,?)", "2026-10-01T12:00:00Z", "2026-10-01T12:00:00.000000001Z")
	require.ErrorContains(t, err, "timestamp bridge mismatch")
	_, err = s.db.Exec("PRAGMA trusted_schema=OFF")
	require.NoError(t, err)
	_, err = s.db.Exec("INSERT INTO abi_trigger VALUES(?,?)", "2026-10-01T12:00:00Z", "2026-10-01T12:00:00.000000000Z")
	require.Error(t, err, "v1.54 does not mark user functions innocuous: unsafe schema execution must fail closed")
	_, err = s.db.Exec("PRAGMA trusted_schema=ON")
	require.NoError(t, err)
	_, err = s.db.Exec("INSERT INTO abi_trigger VALUES(?,?)", "2026-10-01T12:00:00Z", "2026-10-01T12:00:00.000000000Z")
	require.NoError(t, err)
	var count int
	require.NoError(t, s.db.QueryRow("SELECT COUNT(*) FROM abi_trigger").Scan(&count))
	require.Equal(t, 1, count)
}

type riskNoABIConnector struct{ dsn string }

func (c *riskNoABIConnector) Driver() driver.Driver { return &sqlitedriver.Driver{} }
func (c *riskNoABIConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Isolated driver object deliberately has no package-global UDF registry.
	return (&sqlitedriver.Driver{}).Open(c.dsn)
}
func TestRiskTimestampMissingFunctionFailsClosed(t *testing.T) {
	db := sql.OpenDB(&riskNoABIConnector{dsn: filepath.Join(t.TempDir(), "missing.db")})
	defer db.Close()
	_, err := db.Exec("SELECT aegis_risk_utc_nano_v1('2026-10-01T12:00:00Z')")
	require.Error(t, err)
	for _, value := range []driver.Value{int64(1), float64(1), []byte("date"), "invalid"} {
		_, err := riskTimestampScalarV1(nil, []driver.Value{value})
		require.ErrorIs(t, err, risk.ErrIntegrity)
	}
}
