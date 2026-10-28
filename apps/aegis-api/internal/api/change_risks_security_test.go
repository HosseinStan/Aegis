package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Test-only admission barrier; authorization/materialization/persistence always
// delegate to the actual SQLite repository, never mocked permission outcomes.
type riskHTTPBarrier struct {
	repositories.ChangeRiskRepository
	before func(context.Context) error
}

func (b *riskHTTPBarrier) RecordRisk(ctx context.Context, s repositories.RiskScope, expected *string, at time.Time) (repositories.RiskRecord, error) {
	if err := b.before(ctx); err != nil {
		return repositories.RiskRecord{}, err
	}
	return b.ChangeRiskRepository.RecordRisk(ctx, s, expected, at)
}
func (b *riskHTTPBarrier) PreviewRisk(ctx context.Context, s repositories.RiskScope, at time.Time) (repositories.RiskPreview, error) {
	if err := b.before(ctx); err != nil {
		return repositories.RiskPreview{}, err
	}
	return b.ChangeRiskRepository.PreviewRisk(ctx, s, at)
}

func TestRiskHTTPRevocationAfterAdmission(t *testing.T) {
	for _, mutation := range []string{"read", "evaluate", "disabled", "group"} {
		for _, retry := range []bool{false, true} {
			t.Run(mutation+map[bool]string{true: "/retry", false: "/new"}[retry], func(t *testing.T) {
				f := newRiskHTTP(t)
				body := "{}"
				if retry {
					r := f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 201)
					body = `{"expected_latest_assessment_id":"` + nestedString(t, r, "data", "assessment", "id") + `"}`
				}
				ready, release := make(chan struct{}, 1), make(chan struct{})
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				barrier := &riskHTTPBarrier{ChangeRiskRepository: f.store, before: func(ctx context.Context) error {
					ready <- struct{}{}
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}}
				f.h.ChangeRisks = services.NewChangeRiskService(barrier, func() time.Time { return f.now })
				done := make(chan struct {
					status int
					body   string
				}, 1)
				go func() {
					w := f.serve(ctx, "POST", f.base()+"/risk-assessment", "network_administrator", body, "application/json")
					done <- struct {
						status int
						body   string
					}{w.Code, w.Body.String()}
				}()
				select {
				case <-ready:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				switch mutation {
				case "read":
					f.revoke(t, "network_administrator", domain.PermissionRiskRead)
				case "evaluate":
					f.revoke(t, "network_administrator", domain.PermissionRiskEvaluate)
				case "disabled":
					_, err := f.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, f.users["network_administrator"])
					require.NoError(t, err)
				case "group":
					_, err := f.db.Exec(`INSERT INTO user_device_groups(user_id,device_group) VALUES(?,'elsewhere')`, f.users["network_administrator"])
					require.NoError(t, err)
				}
				before := f.evidence(t)
				close(release)
				select {
				case got := <-done:
					require.Equal(t, 403, got.status, got.body)
					require.Contains(t, got.body, `"code":"FORBIDDEN"`)
					require.NotContains(t, got.body, `"data"`)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				require.Equal(t, before, f.evidence(t))
				t.Log("real authenticated HTTP request passed admission; changed live DB grant/user/group still forbids recording/retry without success action or evidence")
			})
		}
	}
}

func TestRiskHTTPHistoricalGovernanceAndLatestCAS(t *testing.T) {
	f := newRiskHTTP(t)
	ctx := context.Background()
	at := f.now.Add(-6 * time.Hour)
	a := domain.ChangeAuthorization{ID: uuid.NewString(), Reference: "CHG-RISK-HTTP", DeviceID: f.device, ConfigType: "running", AllowedCategories: []string{"aaa", "interface"}, WindowStart: at, WindowEnd: at.Add(4 * time.Hour), Status: "pending", RequestedBy: f.users["network_operator"], RequestedAt: at, CreatedAt: at, UpdatedAt: at}
	require.NoError(t, f.store.CreateChangeAuthorization(ctx, a))
	_, err := f.store.TransitionChangeAuthorization(ctx, f.device, a.ID, f.users["system_administrator"], "approved", at.Add(time.Hour))
	require.NoError(t, err)
	require.NoError(t, f.store.LinkChangeEventAuthorization(ctx, f.device, domain.ChangeEventAuthorization{EventID: f.eventID, AuthorizationID: a.ID, LinkedBy: f.users["network_administrator"], LinkedAt: at.Add(3 * time.Hour), Attestation: domain.ChangeAuthorizationAttestation}))
	r1 := f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 201)
	id := nestedString(t, r1, "data", "assessment", "id")
	historical := r1["data"].(map[string]any)["assessment"]
	_, err = f.store.TransitionChangeAuthorization(ctx, f.device, a.ID, f.users["system_administrator"], "revoked", f.now.Add(time.Minute))
	require.NoError(t, err)
	f.now = f.now.Add(2 * time.Minute)
	read := f.call(t, "GET", "/risk-assessments/"+id, "network_operator", "", 200)
	require.Equal(t, historical, read["data"], "historical approval snapshot is not rewritten by live revocation")
	preview := f.call(t, "POST", "/risk-assessment/preview", "network_operator", "{}", 200)
	require.Equal(t, "urgent", nestedString(t, preview, "data", "result", "review_priority"))
	before := f.evidence(t)
	f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 409)
	require.Equal(t, before, f.evidence(t))
	expected := `{"expected_latest_assessment_id":"` + id + `"}`
	r2 := f.call(t, "POST", "/risk-assessment", "network_administrator", expected, 201)
	r2id := nestedString(t, r2, "data", "assessment", "id")
	require.NotEqual(t, id, r2id)
	require.Equal(t, id, nestedString(t, r2, "data", "assessment", "previous_assessment_id"))
	page := f.call(t, "GET", "/risk-assessment/findings", "network_operator", "", 200)
	require.Equal(t, r2id, nestedString(t, page, "meta", "assessment_id"))
	page = f.call(t, "GET", "/risk-assessments/"+id+"/findings", "network_operator", "", 200)
	require.Equal(t, id, nestedString(t, page, "meta", "assessment_id"))
	lost := f.call(t, "POST", "/risk-assessment", "network_administrator", expected, 200)
	require.Equal(t, r2id, nestedString(t, lost, "data", "assessment", "id"))
	read = f.call(t, "GET", "/risk-assessments/"+id, "network_operator", "", 200)
	require.Equal(t, historical, read["data"])
}

func TestRiskHTTPAtomicAuditFailure(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "verified retry"}[retry], func(t *testing.T) {
			f := newRiskHTTP(t)
			if retry {
				f.call(t, "POST", "/risk-assessment", "system_administrator", "{}", 201)
			}
			before := f.evidence(t)
			_, err := f.db.Exec(`CREATE TRIGGER risk_http_audit_fail BEFORE INSERT ON audit_logs WHEN NEW.action='change_risk.evaluate' BEGIN SELECT RAISE(ABORT,'RBAC_HTTP_SECRET SQL diagnostic'); END`)
			require.NoError(t, err)
			res := f.call(t, "POST", "/risk-assessment", "system_administrator", "{}", 503)
			require.Equal(t, "RISK_UNAVAILABLE", nestedString(t, res, "error", "code"))
			require.Equal(t, before, f.evidence(t), "no committed result or new durable success audit")
			require.NotContains(t, f.logs.String(), "RBAC_HTTP_SECRET")
		})
	}
}

func TestRiskHTTPCorruptHistoricalEvidence(t *testing.T) {
	for _, attack := range []string{"result hash", "missing finding", "missing model"} {
		t.Run(attack, func(t *testing.T) {
			f := newRiskHTTP(t)
			r := f.call(t, "POST", "/risk-assessment", "system_administrator", "{}", 201)
			id := nestedString(t, r, "data", "assessment", "id")
			// Privileged corruption setup in this disposable database only.
			rows, err := f.db.Query(`SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name IN ('change_risk_assessments','change_risk_findings','change_risk_definitions')`)
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
				_, err = f.db.Exec(`DROP TRIGGER "` + strings.ReplaceAll(name, `"`, `""`) + `"`)
				require.NoError(t, err)
			}
			query := map[string]string{"result hash": `UPDATE change_risk_assessments SET result_hash=printf('%064d',0)`, "missing finding": `DELETE FROM change_risk_findings WHERE ordinal=0`, "missing model": `DELETE FROM change_risk_definitions`}[attack]
			_, err = f.db.Exec(`PRAGMA foreign_keys=OFF`)
			require.NoError(t, err)
			_, err = f.db.Exec(query)
			require.NoError(t, err)
			_, err = f.db.Exec(`PRAGMA foreign_keys=ON`)
			require.NoError(t, err)
			before := f.evidence(t)
			for _, suffix := range []string{"/risk-assessment", "/risk-assessments/" + id, "/risk-assessments", "/risk-assessment/findings", "/risk-assessments/" + id + "/findings"} {
				r := f.call(t, "GET", suffix, "system_administrator", "", 500)
				require.Equal(t, "RISK_INTEGRITY_FAILURE", nestedString(t, r, "error", "code"))
			}
			require.Equal(t, before, f.evidence(t))
		})
	}
}

func TestRiskHTTPIneligibleAndUnavailable(t *testing.T) {
	for _, mode := range []string{"legacy", "closed", "busy"} {
		t.Run(mode, func(t *testing.T) {
			f := newRiskHTTP(t)
			if mode == "legacy" {
				_, err := f.db.Exec(`UPDATE change_event_items SET classification_version=0,category='unclassified',subcategory='unclassified',classification_rule='',classified_at=NULL WHERE ordinal=0`)
				require.NoError(t, err)
				preview := f.call(t, "POST", "/risk-assessment/preview", "network_operator", "{}", 200)
				require.False(t, preview["data"].(map[string]any)["eligibility"].(map[string]any)["persistable"].(bool))
				require.Equal(t, "unknown", nestedString(t, preview, "data", "result", "technical_impact"))
				f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 409)
			} else if mode == "closed" {
				f.h.ChangeRisks = services.NewChangeRiskService(&riskHTTPBarrier{ChangeRiskRepository: f.store, before: func(context.Context) error { return f.store.Close() }}, func() time.Time { return f.now })
				f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 503)
			} else {
				// A distinct SQLite writer holds the file; no Go correctness mutex.
				_, err := f.db.Exec(`BEGIN IMMEDIATE`)
				require.NoError(t, err)
				defer func() { _, err := f.db.Exec(`ROLLBACK`); require.NoError(t, err) }()
				f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 503)
			}
			for table, rows := range f.evidence(t) {
				require.Empty(t, rows, table)
			}
		})
	}
}

type riskHTTPFailAudit struct{ repositories.AuditRepository }

func (a riskHTTPFailAudit) CreateAudit(ctx context.Context, _ domain.AuditLog) error {
	if _, ok := ctx.Deadline(); !ok {
		panic("risk audit must have a deadline")
	}
	return errors.New("RBAC_HTTP_SECRET private database diagnostic")
}

func TestRiskHTTPBestEffortAuditSafety(t *testing.T) {
	f := newRiskHTTP(t)
	f.h.Audits = riskHTTPFailAudit{f.store}
	f.call(t, "POST", "/risk-assessment/preview", "network_operator", "{}", 200)
	f.call(t, "POST", "/risk-assessment", "viewer", "{}", 403)
	require.Contains(t, f.logs.String(), "risk request audit unavailable")
	require.NotContains(t, f.logs.String(), "RBAC_HTTP_SECRET")
	for table, rows := range f.evidence(t) {
		require.Empty(t, rows, table)
	}
}

// Completion notification occurs after the real detached audit returns. Risk
// service admission and repository cleanup have already completed at this point.
type riskHTTPAuditCompletion struct {
	repositories.AuditRepository
	done chan<- error
}

func (a riskHTTPAuditCompletion) CreateAudit(ctx context.Context, record domain.AuditLog) error {
	err := a.AuditRepository.CreateAudit(ctx, record)
	select {
	case a.done <- err:
	default:
	}
	return err
}

type riskHTTPOutcome struct {
	status            int
	contentType, body string
}

func requireRiskHTTPServerError(t *testing.T, got riskHTTPOutcome, code string) {
	t.Helper()
	require.Equal(t, 503, got.status, got.body)
	require.Contains(t, got.contentType, "application/json")
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(got.body), &body), "a live-request 503 must never use the canceled-transport exception")
	require.Equal(t, code, nestedString(t, body, "error", "code"))
	require.NotEmpty(t, nestedString(t, body, "error", "message"))
	require.True(t, riskHTTPID(nestedString(t, body, "error", "request_id")))
	require.Nil(t, body["data"])
	for _, forbidden := range []string{"RBAC_HTTP_SECRET", "SQL logic error", "source_bindings", "context_path", "private database diagnostic"} {
		require.NotContains(t, got.body, forbidden)
	}
}

func waitRiskHTTPAudit(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		require.NoError(t, err, "real bounded detached audit completes after backend/admission release")
	case <-time.After(10 * time.Second):
		t.Fatal("backend completion/audit did not settle")
	}
}

func TestRiskHTTPCancellationAndOverload(t *testing.T) {
	for _, cancelRequest := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancellation", false: "bounded overload"}[cancelRequest], func(t *testing.T) {
			f := newRiskHTTP(t)
			before := f.evidence(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ready, release := make(chan struct{}, 2), make(chan struct{})
			audited := make(chan error, 11)
			f.h.Audits = riskHTTPAuditCompletion{f.store, audited}
			var entered atomic.Int32
			barrier := &riskHTTPBarrier{ChangeRiskRepository: f.store, before: func(ctx context.Context) error {
				if entered.Add(1) <= 2 {
					ready <- struct{}{}
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-release:
					return nil
				}
			}}
			f.h.ChangeRisks = services.NewChangeRiskService(barrier, func() time.Time { return f.now })
			n := 11
			if cancelRequest {
				n = 1
			}
			done := make(chan riskHTTPOutcome, n)
			for i := 0; i < n; i++ {
				go func() {
					w := f.serve(ctx, http.MethodPost, f.base()+"/risk-assessment/preview", "network_operator", "{}", "application/json")
					done <- riskHTTPOutcome{w.Code, w.Header().Get("Content-Type"), w.Body.String()}
				}()
			}
			for i := 0; i < min(n, 2); i++ {
				select {
				case <-ready:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if cancelRequest {
				require.Positive(t, entered.Load(), "original cancellation scenario reaches the real service")
				cancel()
				got := <-done
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				require.Equal(t, 503, got.status, got.body)
				t.Logf("parent canceled=%v service entered=%d status=%d content_type=%q raw_body=%q", ctx.Err(), entered.Load(), got.status, got.contentType, got.body)
				// Only a genuinely canceled parent permits a bodyless transport
				// outcome. If the inner safe JSON won the race, validate it fully.
				if got.body != "" {
					requireRiskHTTPServerError(t, got, "RISK_UNAVAILABLE")
				}
				waitRiskHTTPAudit(t, audited)
				require.Equal(t, before, f.evidence(t), "no risk evidence/model/success action after parent cancellation")
				f.h.ChangeRisks = services.NewChangeRiskService(f.store, func() time.Time { return f.now })
				f.call(t, "POST", "/risk-assessment/preview", "network_operator", "{}", 200)
			} else {
				select {
				case got := <-done:
					require.NoError(t, ctx.Err(), "overload response has a LIVE parent context")
					requireRiskHTTPServerError(t, got, "RISK_UNAVAILABLE")
					t.Logf("live-parent overload status=%d content_type=%q raw_body=%q", got.status, got.contentType, got.body)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				close(release)
				for i := 0; i < n-1; i++ {
					got := <-done
					require.Equal(t, 200, got.status, got.body)
					var body map[string]any
					require.NoError(t, json.Unmarshal([]byte(got.body), &body))
					require.NotNil(t, body["data"])
				}
				require.NoError(t, ctx.Err())
			}
			require.Equal(t, before, f.evidence(t))
			require.NotContains(t, f.logs.String(), "RBAC_HTTP_SECRET")
		})
	}
}

func TestRiskHTTPRecordCancellationNoCommit(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(map[bool]string{false: "new record", true: "verified retry"}[retry], func(t *testing.T) {
			f := newRiskHTTP(t)
			body := "{}"
			if retry {
				root := f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 201)
				body = "{\"expected_latest_assessment_id\":\"" + nestedString(t, root, "data", "assessment", "id") + "\"}"
			}
			before := f.evidence(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready, audited := make(chan struct{}, 1), make(chan error, 1)
			f.h.Audits = riskHTTPAuditCompletion{f.store, audited}
			barrier := &riskHTTPBarrier{ChangeRiskRepository: f.store, before: func(ctx context.Context) error {
				ready <- struct{}{}
				<-ctx.Done()
				// Exercise the real repository with the canceled operation
				// context; never detach a recording or fake an auth outcome.
				return nil
			}}
			f.h.ChangeRisks = services.NewChangeRiskService(barrier, func() time.Time { return f.now })
			done := make(chan riskHTTPOutcome, 1)
			go func() {
				w := f.serve(ctx, "POST", f.base()+"/risk-assessment", "network_administrator", body, "application/json")
				done <- riskHTTPOutcome{w.Code, w.Header().Get("Content-Type"), w.Body.String()}
			}()
			select {
			case <-ready:
			case <-time.After(10 * time.Second):
				t.Fatal("record did not reach service")
			}
			cancel()
			got := <-done
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Equal(t, 503, got.status)
			if got.body != "" {
				requireRiskHTTPServerError(t, got, "RISK_UNAVAILABLE")
			}
			waitRiskHTTPAudit(t, audited)
			require.Equal(t, before, f.evidence(t), "canceled record/retry adds no assessment/finding/model or successful action")
			require.NoError(t, f.store.Ping(context.Background()))
			f.h.ChangeRisks = services.NewChangeRiskService(f.store, func() time.Time { return f.now })
			f.call(t, "POST", "/risk-assessment/preview", "network_operator", "{}", 200)
			require.Equal(t, before, f.evidence(t))
			t.Logf("record retry=%t parent=%v status=%d content_type=%q raw_body=%q; real backend/audit settled, fresh preview succeeds", retry, ctx.Err(), got.status, got.contentType, got.body)
		})
	}
}

func TestRiskHTTPGlobalServerTimeout(t *testing.T) {
	f := newRiskHTTP(t)
	before := f.evidence(t)
	audited, release := make(chan error, 1), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	f.h.Audits = riskHTTPAuditCompletion{f.store, audited}
	var reached, deadline atomic.Bool
	barrier := &riskHTTPBarrier{ChangeRiskRepository: f.store, before: func(ctx context.Context) error {
		reached.Store(true)
		<-ctx.Done()
		deadline.Store(errors.Is(ctx.Err(), context.DeadlineExceeded))
		// Retain the test barrier until the outer middleware has delivered its
		// actual deadline response. No production timeout setting is changed.
		<-release
		return ctx.Err()
	}}
	f.h.ChangeRisks = services.NewChangeRiskService(barrier, func() time.Time { return f.now })
	f.router = Routes(f.h, &observability.Metrics{}, RouterConfig{RequestTimeout: time.Second})
	parent := context.Background()
	w := f.serve(parent, "POST", f.base()+"/risk-assessment/preview", "network_operator", "{}", "application/json")
	require.NoError(t, parent.Err())
	require.True(t, reached.Load())
	// The operation context is a descendant of this outer deadline.
	got := riskHTTPOutcome{w.Code, w.Header().Get("Content-Type"), w.Body.String()}
	requireRiskHTTPServerError(t, got, "COMMAND_TIMEOUT")
	require.NotContains(t, got.body, "RISK_UNAVAILABLE")
	close(release)
	waitRiskHTTPAudit(t, audited)
	require.True(t, deadline.Load())
	require.Equal(t, before, f.evidence(t))
	require.NoError(t, f.store.Ping(context.Background()))
	t.Logf("live parent, actual global server deadline: status=%d content_type=%q raw_body=%q; backend/audit settled", got.status, got.contentType, got.body)
}
