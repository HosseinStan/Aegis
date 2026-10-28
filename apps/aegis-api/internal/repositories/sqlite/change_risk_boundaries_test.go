package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/risk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRiskRepositoryAdmissionBudgets(t *testing.T) {
	t.Run("full item count before source read", func(t *testing.T) {
		f := authorizationStore(t)
		e := riskRepoEvent(t, f, 0)
		_, err := f.s.db.Exec(`WITH RECURSIVE n(x) AS(SELECT 0 UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO change_event_items(id,change_event_id,ordinal,operation,context_path_json,command,created_at) SELECT printf('10000000-0000-4000-8000-%012d',x),?,x,'add','[]','description bounded','2026-10-04T13:00:00Z' FROM n`, e.ID)
		require.NoError(t, err)
		_, err = f.s.db.Exec(`UPDATE change_events SET item_count=10001 WHERE id=?`, e.ID)
		require.NoError(t, err)
		_, err = f.s.RecordRisk(context.Background(), riskRepoScope(f, e), nil, riskRepoAt(f))
		require.ErrorIs(t, err, risk.ErrLimit)
		require.Zero(t, riskRepoCount(t, f.s, "audit_logs"))
		require.Zero(t, riskRepoCount(t, f.s, "change_risk_assessments"))
	})
	t.Run("combined canonical budget before persistence", func(t *testing.T) {
		f := authorizationStore(t)
		e := riskRepoEvent(t, f, 7000)
		// Prove the fixture exceeds the actual byte budget, rather than
		// assuming a row count implies a particular encoded size.
		err := f.s.riskTransaction(context.Background(), func(tx *sql.Tx) error {
			m, err := riskMaterialize(context.Background(), tx, riskRepoScope(f, e), riskRepoAt(f).Format(risk.UTC9))
			require.ErrorIs(t, err, risk.ErrLimit)
			bytes := len(m.evidenceJSON) + len(m.bindingsJSON) + len(m.resultJSON)
			require.Greater(t, bytes, domain.RiskMaxCanonicalBytes)
			t.Logf("combined canonical UTF-8 bytes=%d limit=%d; source items=7000 below item limit", bytes, domain.RiskMaxCanonicalBytes)
			return err
		})
		require.ErrorIs(t, err, risk.ErrLimit)
		_, err = f.s.RecordRisk(context.Background(), riskRepoScope(f, e), nil, riskRepoAt(f))
		require.ErrorIs(t, err, risk.ErrLimit)
		for _, table := range []string{"change_risk_assessments", "change_risk_findings", "change_risk_definitions", "audit_logs"} {
			require.Zero(t, riskRepoCount(t, f.s, table))
		}
	})
}

func TestRiskRepositoryCASMatrix(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	ctx := context.Background()
	at := riskRepoAt(f)
	missing := uuid.NewString()
	_, err := f.s.RecordRisk(ctx, scope, &missing, at)
	require.ErrorIs(t, err, domain.ErrConflict)
	require.Zero(t, riskRepoCount(t, f.s, "audit_logs"))
	r1, err := f.s.RecordRisk(ctx, scope, nil, at)
	require.NoError(t, err)
	_, err = f.s.RecordRisk(ctx, scope, &missing, at)
	require.ErrorIs(t, err, domain.ErrConflict)
	retry, err := f.s.RecordRisk(ctx, scope, &r1.Assessment.ID, at.Add(time.Second))
	require.NoError(t, err)
	require.False(t, retry.Created)
	a := f.approve(t, f.request(t))
	link := f.link(a, e)
	link.LinkedAt = at.Add(time.Minute)
	require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
	_, err = f.s.RecordRisk(ctx, scope, &missing, at.Add(2*time.Minute))
	require.ErrorIs(t, err, domain.ErrConflict)
	_, err = f.s.RecordRisk(ctx, scope, nil, at.Add(2*time.Minute))
	require.ErrorIs(t, err, domain.ErrConflict)
	r2, err := f.s.RecordRisk(ctx, scope, &r1.Assessment.ID, at.Add(2*time.Minute))
	require.NoError(t, err)
	retry, err = f.s.RecordRisk(ctx, scope, &r2.Assessment.ID, at.Add(3*time.Minute))
	require.NoError(t, err)
	require.Equal(t, r2.Assessment, retry.Assessment)
	_, err = f.s.RecordRisk(ctx, scope, nil, at.Add(3*time.Minute))
	require.ErrorIs(t, err, domain.ErrConflict)
	require.Equal(t, 2, riskRepoCount(t, f.s, "change_risk_assessments"))
	require.Equal(t, 4, riskRepoCount(t, f.s, "audit_logs"))
}

func TestRiskRepositoryMissingArchivedDefinition(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	original, err := f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.NoError(t, err)
	// Dedicated corrupt DB only. Restore FK before the trusted reader runs.
	_, err = f.s.db.Exec(`PRAGMA foreign_keys=OFF`)
	require.NoError(t, err)
	riskRepoCorrupt(t, f.s, "change_risk_definitions", `DELETE FROM change_risk_definitions`)
	_, err = f.s.db.Exec(`PRAGMA foreign_keys=ON`)
	require.NoError(t, err)
	_, err = f.s.GetRisk(context.Background(), scope, original.Assessment.ID)
	require.ErrorIs(t, err, risk.ErrIntegrity)
	_, err = f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.ErrorIs(t, err, risk.ErrIntegrity)
	require.Equal(t, 1, riskRepoCount(t, f.s, "audit_logs"))
}

func TestRiskRepositoryIneligibleAnalysisAndModel(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			f := authorizationStore(t)
			e := riskRepoEvent(t, f, 0)
			var err error
			if version == 0 {
				_, err = f.s.db.Exec(`UPDATE change_events SET analysis_version=0,analyzed_at=NULL WHERE id=?`, e.ID)
			} else {
				_, err = f.s.db.Exec(`UPDATE change_events SET analysis_version=2 WHERE id=?`, e.ID)
			}
			require.NoError(t, err)
			preview, err := f.s.PreviewRisk(context.Background(), riskRepoScope(f, e), riskRepoAt(f))
			require.NoError(t, err)
			require.False(t, preview.Eligibility.Persistable)
			require.Equal(t, "unknown", preview.Result.TechnicalImpact)
			_, err = f.s.RecordRisk(context.Background(), riskRepoScope(f, e), nil, riskRepoAt(f))
			require.ErrorIs(t, err, risk.ErrIneligible)
		})
	}
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	original, err := f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.NoError(t, err)
	_, err = f.s.db.Exec(`PRAGMA foreign_keys=OFF`)
	require.NoError(t, err)
	riskRepoCorrupt(t, f.s, "change_risk_assessments", `UPDATE change_risk_assessments SET model_version=2`)
	_, err = f.s.db.Exec(`PRAGMA foreign_keys=ON`)
	require.NoError(t, err)
	_, err = f.s.GetRisk(context.Background(), scope, original.Assessment.ID)
	require.ErrorIs(t, err, risk.ErrIntegrity)
}

func TestRiskRepositoryRejectsChangedPhysicalSafety(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	ctx := context.Background()
	conn, err := f.s.db.Conn(ctx)
	require.NoError(t, err)
	original := storeConnectionSettings(t, conn, "wal")
	_, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, err = f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
	require.Error(t, err)
	require.Zero(t, riskRepoCount(t, f.s, "change_risk_assessments"))
	conn, err = f.s.db.Conn(ctx)
	require.NoError(t, err)
	require.NotSame(t, original, storeConnectionSettings(t, conn, "wal"))
	require.NoError(t, conn.Close())
	_, err = f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
	require.NoError(t, err)
}

func TestRiskRepositoryDetachedRollback(t *testing.T) {
	f := authorizationStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	conn, err := f.s.db.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, `BEGIN`)
	require.NoError(t, err)
	cancel()
	require.NoError(t, riskBackendRollback(ctx, conn), "cleanup must not inherit cancelled caller context")
	_, err = conn.ExecContext(context.Background(), `BEGIN; ROLLBACK`)
	require.NoError(t, err)
}

func TestRiskRepositoryHistoricalBindingRejectsNullCollections(t *testing.T) {
	f := authorizationStore(t)
	e := riskRepoEvent(t, f, 0)
	scope := riskRepoScope(f, e)
	original, err := f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.NoError(t, err)
	riskRepoCorrupt(t, f.s, "change_risk_assessments", `UPDATE change_risk_assessments SET source_bindings_json=json_set(source_bindings_json,'$.governance.history',NULL)`)
	_, err = f.s.GetRisk(context.Background(), scope, original.Assessment.ID)
	require.ErrorIs(t, err, risk.ErrIntegrity)
}
