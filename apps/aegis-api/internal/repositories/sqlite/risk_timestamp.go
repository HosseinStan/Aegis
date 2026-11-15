package sqlite

import (
	"database/sql/driver"

	"company.local/aegis/apps/aegis-api/internal/risk"
	sqlitedriver "modernc.org/sqlite"
)

const riskTimestampFunctionV1 = "aegis_risk_utc_nano_v1"

// Go initializes this package before any caller can use Store.Open, including
// migrations. modernc v1.54 copies this registry onto EVERY new connection.
// A duplicate/failed registration is fatal; do not silently use another ABI.
func init() {
	if err := sqlitedriver.RegisterDeterministicScalarFunction(riskTimestampFunctionV1, 1, riskTimestampScalarV1); err != nil {
		panic("risk timestamp ABI v1 registration failed")
	}
}
func riskTimestampScalarV1(_ *sqlitedriver.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, risk.ErrIntegrity
	}
	if args[0] == nil {
		return nil, nil
	} // Field-specific nullability belongs to the SQL seal.
	raw, ok := args[0].(string)
	if !ok {
		return nil, risk.ErrIntegrity
	}
	canonical, err := risk.NormalizeTimestampV1(raw)
	if err != nil {
		return nil, risk.ErrIntegrity
	}
	return canonical, nil
}
