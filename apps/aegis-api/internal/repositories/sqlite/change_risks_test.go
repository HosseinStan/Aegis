package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func riskRepoScope(f authorizationFixture, e domain.ChangeEvent) repositories.RiskScope {
	return repositories.RiskScope{ActorID: f.approver, DeviceID: f.device, EventID: e.ID}
}
func riskRepoAt(f authorizationFixture) time.Time { return f.now.Add(6 * time.Hour) }
func riskRepoCount(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}
func riskRepoEvent(t *testing.T, f authorizationFixture, n int) domain.ChangeEvent {
	t.Helper()
	ctx := context.Background()
	at := f.now.Add(2 * time.Hour)
	from := eventBackup(f.device, "running", at.Add(-time.Hour))
	to := eventBackup(f.device, "running", at)
	require.NoError(t, f.s.CreateBackup(ctx, from))
	e := eventBetween(from, to)
	e.AnalysisVersion = 1
	e.AnalyzedAt = &at
	e.ItemCount = n
	items := []domain.ChangeEventItem{}
	for i := 0; i < n; i++ {
		command := fmt.Sprintf("description item-%d", i)
		path := []string{"interface Gi1"}
		c := changes.Classify(path, command)
		items = append(items, domain.ChangeEventItem{ID: uuid.NewString(), ChangeEventID: e.ID, Ordinal: i, Operation: "add", Command: command, ContextPath: path, CreatedAt: at, ClassificationVersion: 1, Category: c.Category, Subcategory: c.Subcategory, ClassificationRule: c.RuleID, ClassifiedAt: &at})
	}
	require.NoError(t, f.s.CreateBackupWithChangeAnalysis(ctx, to, &e, items))
	return e
}

func TestRiskRepositoryValidMaterialization(t *testing.T) {
	for _, n := range []int{0, 1, 301} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			f := authorizationStore(t)
			e := riskRepoEvent(t, f, n)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			preview, err := f.s.PreviewRisk(ctx, scope, riskRepoAt(f))
			require.NoError(t, err)
			require.True(t, preview.Eligibility.Persistable)
			require.Equal(t, n, preview.Result.TotalItems)
			require.Zero(t, riskRepoCount(t, f.s, "change_risk_assessments"))
			require.Zero(t, riskRepoCount(t, f.s, "change_risk_definitions"))
			stored, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.NoError(t, err)
			require.True(t, stored.Created)
			require.Equal(t, 1, stored.Assessment.Revision)
			require.Equal(t, preview.Result, stored.Assessment.Result)
			if n == 0 {
				require.Equal(t, "unknown", stored.Assessment.Result.TechnicalImpact)
				require.Equal(t, "partial", stored.Assessment.Result.EvidenceCompleteness)
			} else {
				require.Equal(t, "low", stored.Assessment.Result.TechnicalImpact)
			}
			got, err := f.s.GetRisk(ctx, scope, stored.Assessment.ID)
			require.NoError(t, err)
			require.Equal(t, stored.Assessment, got)
			history, total, err := f.s.ListRisks(ctx, scope, repositories.Page{Limit: 50})
			require.NoError(t, err)
			require.Equal(t, 1, total)
			require.Equal(t, []domain.ChangeRiskAssessment{got}, history)
			page, err := f.s.ListRiskFindings(ctx, scope, "", repositories.Page{Limit: 1})
			require.NoError(t, err)
			require.Equal(t, got.ID, page.AssessmentID)
			require.Equal(t, len(got.Result.Findings), page.Total)
			require.Equal(t, got.Result.Findings[:1], page.Items)
			page, err = f.s.ListRiskFindings(ctx, scope, got.ID, repositories.Page{Limit: 1, Offset: 10000})
			require.NoError(t, err)
			require.Empty(t, page.Items)
			require.Equal(t, 1, riskRepoCount(t, f.s, "audit_logs"))
			require.Equal(t, 1, riskRepoCount(t, f.s, "change_risk_definitions"))
		})
	}
}

func TestRiskRepositoryRealRevisionChainAndHistoricalReplay(t *testing.T) {
	for _, policyFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(policyFirst), func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			at := riskRepoAt(f)
			a := f.approve(t, f.request(t))
			p := f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(time.Hour))
			r1, err := f.s.RecordRisk(ctx, scope, nil, at)
			require.NoError(t, err)
			retry, err := f.s.RecordRisk(ctx, scope, nil, at.Add(time.Second))
			require.NoError(t, err)
			require.False(t, retry.Created)
			require.Equal(t, r1.Assessment, retry.Assessment)
			link := f.link(a, e)
			link.LinkedAt = at.Add(time.Minute)
			policy := func(at time.Time) {
				_, err := f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, at)
				require.NoError(t, err)
			}
			auth := func(at time.Time) {
				link.LinkedAt = at
				require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
			}
			if policyFirst {
				policy(at.Add(time.Minute))
			} else {
				auth(at.Add(time.Minute))
			}
			_, err = f.s.RecordRisk(ctx, scope, nil, at.Add(90*time.Second))
			require.ErrorIs(t, err, domain.ErrConflict)
			r2, err := f.s.RecordRisk(ctx, scope, &r1.Assessment.ID, at.Add(90*time.Second))
			require.NoError(t, err)
			require.True(t, r2.Created)
			require.Equal(t, 2, r2.Assessment.Revision)
			retry, err = f.s.RecordRisk(ctx, scope, &r1.Assessment.ID, at.Add(95*time.Second))
			require.NoError(t, err)
			require.Equal(t, r2.Assessment, retry.Assessment)
			require.False(t, retry.Created)
			if policyFirst {
				auth(at.Add(2 * time.Minute))
			} else {
				policy(at.Add(2 * time.Minute))
			}
			_, err = f.s.RecordRisk(ctx, scope, &r1.Assessment.ID, at.Add(150*time.Second))
			require.ErrorIs(t, err, domain.ErrConflict)
			r3, err := f.s.RecordRisk(ctx, scope, &r2.Assessment.ID, at.Add(150*time.Second))
			require.NoError(t, err)
			require.Equal(t, 3, r3.Assessment.Revision)
			_, err = f.s.RecordRisk(ctx, scope, &r1.Assessment.ID, at.Add(155*time.Second))
			require.ErrorIs(t, err, domain.ErrConflict)
			retry, err = f.s.RecordRisk(ctx, scope, &r2.Assessment.ID, at.Add(160*time.Second))
			require.NoError(t, err)
			require.Equal(t, r3.Assessment, retry.Assessment)
			_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "revoked", at.Add(3*time.Minute))
			require.NoError(t, err)
			r4, err := f.s.RecordRisk(ctx, scope, &r3.Assessment.ID, at.Add(4*time.Minute))
			require.NoError(t, err)
			require.Equal(t, 4, r4.Assessment.Revision)
			require.Equal(t, "urgent", r4.Assessment.Result.ReviewPriority)
			_, err = f.s.RetirePolicySet(ctx, f.device, p.ID, f.approver, at.Add(5*time.Minute))
			require.NoError(t, err)
			for _, r := range []repositories.RiskRecord{r1, r2, r3, r4} {
				got, err := f.s.GetRisk(ctx, scope, r.Assessment.ID)
				require.NoError(t, err)
				require.Equal(t, r.Assessment, got)
			}
			list, total, err := f.s.ListRisks(ctx, scope, repositories.Page{Limit: 2, Offset: 1})
			require.NoError(t, err)
			require.Equal(t, 4, total)
			require.Equal(t, []domain.ChangeRiskAssessment{r3.Assessment, r2.Assessment}, list)
			var raw string
			require.NoError(t, f.s.db.QueryRow(`SELECT evidence_json FROM change_risk_assessments WHERE id=?`, r4.Assessment.ID).Scan(&raw))
			require.Contains(t, raw, `"status":"revoked"`)
			require.Contains(t, raw, `"recorded_status":"pass"`)
			require.Equal(t, 7, riskRepoCount(t, f.s, "audit_logs"))
			t.Log("R1 -> valid late workflow R2 -> other late workflow R3 -> real revocation R4; all old assessments replay unchanged. No fabricated A->B->A governance transition.")
		})
	}
}

func TestRiskRepositoryAtomicAudit(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprint(retry), func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			var original repositories.RiskRecord
			if retry {
				var err error
				original, err = f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
				require.NoError(t, err)
			}
			before := riskAdversarialSnapshot(t, f.s)
			_, err := f.s.db.Exec(`CREATE TRIGGER risk_test_audit_failure BEFORE INSERT ON audit_logs WHEN NEW.action='change_risk.evaluate' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`)
			require.NoError(t, err)
			got, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.Error(t, err)
			require.Zero(t, got)
			require.Equal(t, repositories.ErrRiskUnavailable.Error(), err.Error())
			require.Equal(t, before, riskAdversarialSnapshot(t, f.s))
			_, err = f.s.db.Exec(`DROP TRIGGER risk_test_audit_failure`)
			require.NoError(t, err)
			got, err = f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.NoError(t, err)
			require.Equal(t, !retry, got.Created)
			if retry {
				require.Equal(t, original.Assessment, got.Assessment)
			}
		})
	}
}

func TestRiskRepositoryScopeAndLiveGrants(t *testing.T) {
	for _, mutation := range []string{"disabled", "risk grant", "backup grant", "authorization grant", "policy grant", "device grant", "group", "wrong device", "wrong event", "foreign assessment"} {
		t.Run(mutation, func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			if mutation == "group" {
				_, err := f.s.db.Exec(`DELETE FROM user_roles WHERE user_id=?`, scope.ActorID)
				require.NoError(t, err)
				require.NoError(t, f.s.AssignRole(ctx, scope.ActorID, "network_administrator"))
				_, err = f.s.db.Exec(`UPDATE devices SET device_group='campus' WHERE id=?`, f.device)
				require.NoError(t, err)
				_, err = f.s.db.Exec(`INSERT INTO user_device_groups(user_id,device_group) VALUES(?,'campus')`, scope.ActorID)
				require.NoError(t, err)
			}
			original, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.NoError(t, err)
			want := domain.ErrForbidden
			id := original.Assessment.ID
			switch mutation {
			case "disabled":
				_, err = f.s.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, scope.ActorID)
			case "group":
				_, err = f.s.db.Exec(`UPDATE user_device_groups SET device_group='elsewhere' WHERE user_id=?`, scope.ActorID)
			case "wrong device":
				scope.DeviceID = f.otherDevice
				want = domain.ErrNotFound
			case "wrong event":
				scope.EventID = uuid.NewString()
				want = domain.ErrNotFound
			case "foreign assessment":
				other := authorizationEvent(t, f.s, f.device)
				scope.EventID = other.ID
				want = domain.ErrNotFound
			default:
				permission := map[string]string{"risk grant": domain.PermissionRiskRead, "backup grant": "backups.read", "authorization grant": domain.PermissionAuthorizationRead, "policy grant": domain.PermissionPolicyRead, "device grant": "devices.read"}[mutation]
				_, err = f.s.db.Exec(`DELETE FROM role_permissions WHERE permission_id=(SELECT id FROM permissions WHERE name=?)`, permission)
			}
			require.NoError(t, err)
			before := riskAdversarialSnapshot(t, f.s)
			_, err = f.s.GetRisk(ctx, scope, id)
			require.ErrorIs(t, err, want)
			_, _, err = f.s.ListRisks(ctx, scope, repositories.Page{Limit: 20})
			if mutation != "foreign assessment" {
				require.ErrorIs(t, err, want)
			} else {
				require.NoError(t, err)
			}
			_, err = f.s.ListRiskFindings(ctx, scope, id, repositories.Page{Limit: 20})
			require.ErrorIs(t, err, want)
			if mutation != "foreign assessment" {
				_, err = f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
				require.ErrorIs(t, err, want)
			}
			require.Equal(t, before, riskAdversarialSnapshot(t, f.s))
		})
	}
	// Evaluate permission is checked afresh even on a verified retry.
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	_, err := f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.NoError(t, err)
	_, err = f.s.db.Exec(`DELETE FROM role_permissions WHERE permission_id=(SELECT id FROM permissions WHERE name=?)`, domain.PermissionRiskEvaluate)
	require.NoError(t, err)
	_, err = f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.Equal(t, 1, riskRepoCount(t, f.s, "audit_logs"))
}

func riskRepoAssertLiveGrants(t *testing.T, s *Store, actor string, read, evaluate bool) {
	t.Helper()
	ctx := context.Background()
	u, err := s.GetUser(ctx, actor)
	require.NoError(t, err)
	require.True(t, u.Enabled)
	p, err := s.GetPrincipal(ctx, actor)
	require.NoError(t, err)
	for _, permission := range []string{"devices.read", "backups.read", domain.PermissionAuthorizationRead, domain.PermissionPolicyRead} {
		require.True(t, p.HasPermission(permission), "retain live %s", permission)
	}
	require.Equal(t, read, p.HasPermission(domain.PermissionRiskRead))
	require.Equal(t, evaluate, p.HasPermission(domain.PermissionRiskEvaluate))
}

func riskRepoRevokeGrant(t *testing.T, s *Store, actor, permission string) {
	t.Helper()
	result, err := s.db.Exec(`DELETE FROM role_permissions
        WHERE permission_id=(SELECT id FROM permissions WHERE name=?)
        AND role_id IN (SELECT role_id FROM user_roles WHERE user_id=?)`, permission, actor)
	require.NoError(t, err)
	n, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "remove only the selected live grant")
}

func riskRepoSuccessAudits(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='change_risk.evaluate' AND succeeded=1`).Scan(&n))
	return n
}

func riskRepoAssertNoEvidence(t *testing.T, s *Store) {
	t.Helper()
	for _, table := range []string{"change_risk_assessments", "change_risk_findings", "change_risk_definitions"} {
		require.Zero(t, riskRepoCount(t, s, table), table)
	}
	require.Zero(t, riskRepoSuccessAudits(t, s))
}

func TestRiskRepositoryRBACMissingReadBeforeRecord(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	ctx := context.Background()
	cached, err := f.s.GetPrincipal(ctx, scope.ActorID)
	require.NoError(t, err)
	require.True(t, cached.HasPermission(domain.PermissionRiskRead))
	riskRepoRevokeGrant(t, f.s, scope.ActorID, domain.PermissionRiskRead)
	riskRepoAssertLiveGrants(t, f.s, scope.ActorID, false, true)
	before := riskAdversarialSnapshot(t, f.s)
	_, err = f.s.GetRisk(ctx, scope, "")
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = f.s.PreviewRisk(ctx, scope, riskRepoAt(f))
	require.ErrorIs(t, err, domain.ErrForbidden)
	got, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.Zero(t, got)
	service := services.NewChangeRiskService(f.s, func() time.Time { return riskRepoAt(f) })
	for i := 0; i < 2; i++ {
		got, err = service.Record(ctx, cached, scope.DeviceID, scope.EventID, nil)
		require.ErrorIs(t, err, domain.ErrForbidden)
		require.Zero(t, got, "no success-ready result, even with cached grants")
	}
	riskRepoAssertNoEvidence(t, f.s)
	require.Equal(t, before, riskAdversarialSnapshot(t, f.s))
	t.Log("evaluate-only actor denied new record and repeated service request; zero headers, findings, model definitions and success audits")
}

func TestRiskRepositoryRBACMissingReadBeforeRetry(t *testing.T) {
	for _, successor := range []bool{false, true} {
		t.Run(fmt.Sprintf("successor=%t", successor), func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			newEvent := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			at := riskRepoAt(f)
			cached, err := f.s.GetPrincipal(ctx, scope.ActorID)
			require.NoError(t, err)
			var approval domain.ChangeAuthorization
			if successor {
				approval = f.approve(t, f.request(t))
			}
			r1, err := f.s.RecordRisk(ctx, scope, nil, at)
			require.NoError(t, err)
			require.True(t, r1.Created)
			require.Equal(t, 1, r1.Assessment.Revision)
			require.Equal(t, 1, riskRepoSuccessAudits(t, f.s))
			latest := r1
			expectations := []*string{&r1.Assessment.ID, nil}
			if successor {
				link := f.link(approval, e)
				link.LinkedAt = at.Add(time.Minute)
				require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, scope.DeviceID, link))
				at = at.Add(2 * time.Minute)
				latest, err = f.s.RecordRisk(ctx, scope, &r1.Assessment.ID, at)
				require.NoError(t, err)
				require.True(t, latest.Created)
				require.Equal(t, 2, latest.Assessment.Revision)
				expectations = []*string{&latest.Assessment.ID, &r1.Assessment.ID}
			}
			// These expectations really are accepted before revocation, including
			// the root nil / immediate-predecessor lost-response retry variants.
			for _, expected := range expectations {
				retry, err := f.s.RecordRisk(ctx, scope, expected, at)
				require.NoError(t, err)
				require.False(t, retry.Created)
				require.Equal(t, latest.Assessment, retry.Assessment)
			}
			audits := riskRepoSuccessAudits(t, f.s)
			riskRepoRevokeGrant(t, f.s, scope.ActorID, domain.PermissionRiskRead)
			riskRepoAssertLiveGrants(t, f.s, scope.ActorID, false, true)
			before := riskAdversarialSnapshot(t, f.s)
			service := services.NewChangeRiskService(f.s, func() time.Time { return at })
			for _, expected := range expectations {
				got, err := f.s.RecordRisk(ctx, scope, expected, at)
				require.ErrorIs(t, err, domain.ErrForbidden)
				require.Zero(t, got)
				got, err = service.Record(ctx, cached, scope.DeviceID, scope.EventID, expected)
				require.ErrorIs(t, err, domain.ErrForbidden)
				require.Zero(t, got)
			}
			got, err := service.Record(ctx, cached, scope.DeviceID, newEvent.ID, nil)
			require.ErrorIs(t, err, domain.ErrForbidden)
			require.Zero(t, got)
			require.Equal(t, latest.Assessment.Revision, riskRepoCount(t, f.s, "change_risk_assessments"))
			require.Equal(t, audits, riskRepoSuccessAudits(t, f.s))
			require.Equal(t, before, riskAdversarialSnapshot(t, f.s), "all historical headers, findings, registry and audits unchanged")
			t.Log("formerly valid latest and lost-response retries now forbidden; previously successful cached actor also denied a new event record")
		})
	}
}

func TestRiskRepositoryRBACReadWithoutEvaluate(t *testing.T) {
	for _, operator := range []bool{false, true} {
		t.Run(fmt.Sprintf("network_operator=%t", operator), func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			newEvent := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			r1, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.NoError(t, err)
			if operator {
				_, err = f.s.db.Exec(`DELETE FROM user_roles WHERE user_id=?`, scope.ActorID)
				require.NoError(t, err)
				require.NoError(t, f.s.AssignRole(ctx, scope.ActorID, "network_operator"))
			} else {
				riskRepoRevokeGrant(t, f.s, scope.ActorID, domain.PermissionRiskEvaluate)
			}
			riskRepoAssertLiveGrants(t, f.s, scope.ActorID, true, false)
			before := riskAdversarialSnapshot(t, f.s)
			p, err := f.s.GetPrincipal(ctx, scope.ActorID)
			require.NoError(t, err)
			service := services.NewChangeRiskService(f.s, func() time.Time { return riskRepoAt(f) })
			preview, err := service.Preview(ctx, p, scope.DeviceID, scope.EventID)
			require.NoError(t, err)
			require.Equal(t, r1.Assessment.Result, preview.Result)
			read, err := service.Get(ctx, p, scope.DeviceID, scope.EventID, r1.Assessment.ID)
			require.NoError(t, err)
			require.Equal(t, r1.Assessment, read)
			_, err = service.Preview(ctx, p, scope.DeviceID, newEvent.ID)
			require.NoError(t, err)
			got, err := service.Record(ctx, p, scope.DeviceID, newEvent.ID, nil)
			require.ErrorIs(t, err, domain.ErrForbidden)
			require.Zero(t, got)
			got, err = service.Record(ctx, p, scope.DeviceID, scope.EventID, &r1.Assessment.ID)
			require.ErrorIs(t, err, domain.ErrForbidden)
			require.Zero(t, got)
			require.Equal(t, 1, riskRepoSuccessAudits(t, f.s))
			require.Equal(t, before, riskAdversarialSnapshot(t, f.s))
			t.Log("live read and all prerequisites retained: previews and historical replay succeed; new record and verified retry denied without writes")
		})
	}
}

func TestRiskRepositoryRBACBothPermissions(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	ctx := context.Background()
	riskRepoAssertLiveGrants(t, f.s, scope.ActorID, true, true)
	service := services.NewChangeRiskService(f.s, func() time.Time { return riskRepoAt(f) })
	p := domain.Principal{UserID: scope.ActorID}
	r1, err := service.Record(ctx, p, scope.DeviceID, scope.EventID, nil)
	require.NoError(t, err)
	require.True(t, r1.Created)
	require.Equal(t, 1, r1.Assessment.Revision)
	require.Equal(t, 1, riskRepoSuccessAudits(t, f.s))
	headers := riskAdversarialRows(t, f.s.db, "change_risk_assessments")
	findings := riskAdversarialRows(t, f.s.db, "change_risk_findings")
	definitions := riskAdversarialRows(t, f.s.db, "change_risk_definitions")
	retry, err := service.Record(ctx, p, scope.DeviceID, scope.EventID, &r1.Assessment.ID)
	require.NoError(t, err)
	require.False(t, retry.Created)
	require.Equal(t, r1.Assessment, retry.Assessment)
	require.Equal(t, 2, riskRepoSuccessAudits(t, f.s))
	require.Equal(t, headers, riskAdversarialRows(t, f.s.db, "change_risk_assessments"))
	require.Equal(t, findings, riskAdversarialRows(t, f.s.db, "change_risk_findings"))
	require.Equal(t, definitions, riskAdversarialRows(t, f.s.db, "change_risk_definitions"))
	read, err := service.Get(ctx, p, scope.DeviceID, scope.EventID, r1.Assessment.ID)
	require.NoError(t, err)
	require.Equal(t, r1.Assessment, read)
	t.Log("both live grants: one sealed R1, immutable replay, one atomic success audit per authorized create/retry")
}

func TestRiskRepositoryRBACConcurrentReadRevocation(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var path string
	require.NoError(t, f.s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
	other, err := Open(ctx, path)
	require.NoError(t, err)
	defer other.Close()
	require.NotSame(t, f.s, other)
	ready := make(chan struct{}, 1)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	riskRepoBarrier(t, f.s, ready, release)
	type outcome struct {
		record repositories.RiskRecord
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		got, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
		done <- outcome{got, err}
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// A second real SQLite connection commits revocation after authorization
	// established the first transaction's old WAL snapshot, before its write.
	riskRepoRevokeGrant(t, other, scope.ActorID, domain.PermissionRiskRead)
	riskRepoAssertLiveGrants(t, other, scope.ActorID, false, true)
	before := riskAdversarialSnapshot(t, other)
	close(release)
	select {
	case result := <-done:
		require.ErrorIs(t, result.err, domain.ErrConflict)
		require.Zero(t, result.record)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	riskRepoAssertNoEvidence(t, f.s)
	got, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.Zero(t, got)
	require.Equal(t, before, riskAdversarialSnapshot(t, f.s))
	t.Log("independent Stores: stale WAL write conflicts, fresh write sees revoked risk.read; no evidence, registry writes or success audits")
}

func TestRiskRepositorySourceIntegrityAndLimits(t *testing.T) {
	cases := []struct {
		name, sql string
		kind      error
	}{
		{"missing item", `DELETE FROM change_event_items WHERE ordinal=1`, risk.ErrIntegrity},
		{"wrong count", `UPDATE change_events SET item_count=item_count+1`, risk.ErrIntegrity},
		{"noncontiguous ordinal", `UPDATE change_event_items SET ordinal=ordinal+5`, risk.ErrIntegrity},
		{"forged tuple", `UPDATE change_event_items SET category='interface',subcategory='description',classification_rule='interface.description' WHERE ordinal=1`, risk.ErrIntegrity},
		{"raw secret", `UPDATE change_event_items SET command='enable secret PHASE_C_SENTINEL_SECRET' WHERE ordinal=1`, risk.ErrIntegrity},
		{"raw context secret", `UPDATE change_event_items SET context_path_json='["username lab secret PHASE_C_SENTINEL_SECRET"]' WHERE ordinal=1`, risk.ErrIntegrity},
		{"source bytes", `UPDATE change_event_items SET command=printf('%.*c',5242881,'x') WHERE ordinal=1`, risk.ErrLimit},
		{"future capture source", `UPDATE change_event_items SET classified_at='2026-10-05T13:00:00Z'`, risk.ErrIntegrity},
		{"malformed raw time", `UPDATE change_event_items SET created_at='2026-10-04T13:00:00.0000000001Z'`, risk.ErrIntegrity},
		{"backup chronology", `UPDATE config_backups SET created_at='2026-10-05T13:00:00Z'`, risk.ErrIntegrity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			_, err := f.s.db.Exec(c.sql)
			require.NoError(t, err, "fixture must reach materializer, not fail in setup")
			before := riskAdversarialSnapshot(t, f.s)
			got, err := f.s.RecordRisk(context.Background(), riskRepoScope(f, e), nil, riskRepoAt(f))
			require.ErrorIs(t, err, c.kind)
			require.Zero(t, got)
			require.NotContains(t, err.Error(), "PHASE_C_SENTINEL_SECRET")
			require.Equal(t, before, riskAdversarialSnapshot(t, f.s))
		})
	}
}

func TestRiskRepositoryUnsupportedPreview(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			var err error
			if version == 0 {
				_, err = f.s.db.Exec(`UPDATE change_event_items SET classification_version=0,category='unclassified',subcategory='unclassified',classification_rule='',classified_at=NULL WHERE ordinal=0`)
			} else {
				_, err = f.s.db.Exec(`UPDATE change_event_items SET classification_version=2 WHERE ordinal=0`)
			}
			require.NoError(t, err)
			preview, err := f.s.PreviewRisk(context.Background(), riskRepoScope(f, e), riskRepoAt(f))
			require.NoError(t, err)
			require.False(t, preview.Eligibility.Persistable)
			require.Equal(t, "unknown", preview.Result.TechnicalImpact)
			require.Equal(t, riskString("high"), preview.Result.KnownImpactFloor)
			_, err = f.s.RecordRisk(context.Background(), riskRepoScope(f, e), nil, riskRepoAt(f))
			require.ErrorIs(t, err, risk.ErrIneligible)
			require.Zero(t, riskRepoCount(t, f.s, "change_risk_assessments"))
		})
	}
}

// Malicious/corrupt archives are assembled through SQL in disposable DBs only.
// Dropping immutability triggers is attacker setup, never production behavior.
func riskRepoCorrupt(t *testing.T, s *Store, table, statement string) {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name=?`, table)
	require.NoError(t, err)
	names := []string{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	for _, name := range names {
		_, err = s.db.Exec(`DROP TRIGGER "` + strings.ReplaceAll(name, `"`, `""`) + `"`)
		require.NoError(t, err)
	}
	_, err = s.db.Exec(statement)
	require.NoError(t, err)
}

func TestRiskRepositoryArchiveTampering(t *testing.T) {
	for _, c := range []struct{ name, table, query string }{
		{"evidence hash", "change_risk_assessments", `UPDATE change_risk_assessments SET evidence_hash=printf('%064d',0)`},
		{"result hash", "change_risk_assessments", `UPDATE change_risk_assessments SET result_hash=printf('%064d',0)`},
		{"typed result", "change_risk_assessments", `UPDATE change_risk_assessments SET technical_impact='low',known_impact_floor='low'`},
		{"definition bytes", "change_risk_definitions", `UPDATE change_risk_definitions SET definition_json=json_set(definition_json,'$.aggregation.default_review_priority','urgent')`},
		{"definition hash", "change_risk_definitions", `UPDATE change_risk_definitions SET definition_hash=printf('%064d',0)`},
		{"ordered findings", "change_risk_findings", `UPDATE change_risk_findings SET priority_floor='urgent' WHERE ordinal=0`},
		{"missing finding", "change_risk_findings", `DELETE FROM change_risk_findings WHERE ordinal=0`},
		{"raw binding nanosecond", "change_risk_assessments", `UPDATE change_risk_assessments SET source_bindings_json=json_set(source_bindings_json,'$.event.detected_at','2026-10-04T13:00:00.000000001Z')`},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := authorizationStore(t)
			e := authorizationEvent(t, f.s, f.device)
			scope := riskRepoScope(f, e)
			ctx := context.Background()
			original, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.NoError(t, err)
			// Disable FK only on isolated attacker setup for the registry-hash corruption;
			// turn it back on before invoking any trusted repository operation.
			if c.name == "definition hash" {
				_, err = f.s.db.Exec(`PRAGMA foreign_keys=OFF`)
				require.NoError(t, err)
			}
			riskRepoCorrupt(t, f.s, c.table, c.query)
			if c.name == "definition hash" {
				_, err = f.s.db.Exec(`PRAGMA foreign_keys=ON`)
				require.NoError(t, err)
			}
			got, err := f.s.GetRisk(ctx, scope, original.Assessment.ID)
			require.ErrorIs(t, err, risk.ErrIntegrity)
			require.Zero(t, got)
			result, err := f.s.RecordRisk(ctx, scope, nil, riskRepoAt(f))
			require.ErrorIs(t, err, risk.ErrIntegrity)
			require.Zero(t, result)
			require.Equal(t, 1, riskRepoCount(t, f.s, "audit_logs"))
		})
	}
}

func TestRiskRepositorySecretNonLeakage(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	result, err := f.s.RecordRisk(context.Background(), scope, nil, riskRepoAt(f))
	require.NoError(t, err)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, table := range []string{"change_risk_assessments", "change_risk_findings", "change_risk_definitions", "audit_logs"} {
		raw = append(raw, []byte(strings.Join(riskAdversarialRows(t, f.s.db, table), "\n"))...)
	}
	for _, forbidden := range []string{"description old", "interface Gi1", "enable secret", "<redacted>", "context_path", "raw_command", "RawCommand"} {
		require.NotContains(t, string(raw), forbidden)
	}
}

func TestRiskRepositoryTwoIndependentStores(t *testing.T) {
	f := authorizationStore(t)
	e := authorizationEvent(t, f.s, f.device)
	scope := riskRepoScope(f, e)
	ctx := context.Background()
	var path string
	require.NoError(t, f.s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
	second, err := Open(ctx, path)
	require.NoError(t, err)
	defer second.Close()
	require.NotSame(t, f.s, second)
	// Both transactions deliberately retain the SAME old snapshot before either
	// performs a write. Exactly one audit+header may commit; the other is BUSY.
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, s := range []*Store{f.s, second} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			results <- s.riskTransaction(ctx, func(tx *sql.Tx) error {
				if err := riskAuthorize(ctx, tx, scope, true); err != nil {
					return err
				}
				m, err := riskMaterialize(ctx, tx, scope, riskRepoAt(f).Format(risk.UTC9))
				if err != nil {
					return err
				}
				id, err := riskLatest(ctx, tx, scope)
				if err != nil {
					return err
				}
				if id != "" {
					return errors.New("unexpected fixture latest")
				}
				ready <- struct{}{}
				<-start
				captured := riskRepoAt(f).Format(risk.UTC9)
				if err = riskRegistry(ctx, tx, true, captured); err != nil {
					return err
				}
				_, hash := risk.CanonicalDefinitionV1()
				a := domain.ChangeRiskAssessment{ID: uuid.NewString(), EventID: e.ID, DeviceID: f.device, ConfigType: "running", Revision: 1, ModelVersion: 1, EvidenceSchemaVersion: 1, DefinitionHash: hash, EvidenceHash: m.evidenceHash, ResultHash: m.resultHash, CapturedAt: captured, AssessedBy: f.approver}
				if err = riskInsert(ctx, tx, a, m); err != nil {
					return err
				}
				return riskActionAudit(ctx, tx, scope, a.ID, captured)
			})
		}(s)
	}
	<-ready
	<-ready
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, domain.ErrConflict)
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	require.Equal(t, 1, riskRepoCount(t, f.s, "change_risk_assessments"))
	require.Equal(t, 1, riskRepoCount(t, f.s, "audit_logs"))
	t.Log("two independent Stores pinned same WAL snapshot: one commit, one conflict, one sealed root and one audit")
}
