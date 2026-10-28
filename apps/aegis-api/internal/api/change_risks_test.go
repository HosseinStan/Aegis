package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/auth"
	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type riskHTTPFixture struct {
	store                        *sqlite.Store
	db                           *sql.DB
	h                            *Handler
	router                       http.Handler
	device, otherDevice, eventID string
	users, tokens                map[string]string
	now                          time.Time
	requests                     atomic.Uint32
	logs                         bytes.Buffer
}

func newRiskHTTP(t *testing.T) *riskHTTPFixture {
	t.Helper()
	f := &riskHTTPFixture{users: map[string]string{}, tokens: map[string]string{}, now: time.Date(2026, 10, 4, 17, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "risk-http.db")
	var err error
	f.store, err = sqlite.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.store.Close()) })
	// Separate real SQL connection is disposable attack/setup instrumentation,
	// not a replacement for production Store.Open or backend authorization.
	f.db, err = sql.Open("sqlite", path)
	require.NoError(t, err)
	f.db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, f.db.Close()) })
	_, err = f.db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000`)
	require.NoError(t, err)
	a, err := auth.NewService(f.store, "risk-http", "test", bytes.Repeat([]byte{17}, 32), time.Hour, 2*time.Hour)
	require.NoError(t, err)
	password := "disposable local test password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	for _, role := range []string{"viewer", "network_operator", "network_administrator", "system_administrator"} {
		id := uuid.NewString()
		f.users[role] = id
		require.NoError(t, f.store.CreateUser(ctx, domain.User{ID: id, Username: role, PasswordHash: hash, Enabled: true, CreatedAt: f.now, UpdatedAt: f.now}))
		require.NoError(t, f.store.AssignRole(ctx, id, role))
		tokens, _, err := a.Login(ctx, role, password)
		require.NoError(t, err)
		f.tokens[role] = tokens.AccessToken
	}
	credential := uuid.NewString()
	require.NoError(t, f.store.CreateCredentialRecord(ctx, repositories.CredentialRecord{ID: credential, Name: "lab", AuthType: "password", CreatedAt: f.now.Format(time.RFC3339Nano), UpdatedAt: f.now.Format(time.RFC3339Nano)}))
	for i, target := range []*string{&f.device, &f.otherDevice} {
		*target = uuid.NewString()
		require.NoError(t, f.store.CreateDevice(ctx, domain.Device{ID: *target, DisplayName: "lab", ManagementIP: fmt.Sprintf("192.0.2.%d", i+10), ManagementPort: 22, Vendor: "cisco", Platform: "ios", Protocol: "ssh", CredentialID: credential, DeviceGroup: "campus", Enabled: true, CreatedAt: f.now, UpdatedAt: f.now}))
	}
	f.eventID = f.event(t, f.device)
	f.h = &Handler{Auth: a, Audits: f.store, Health: f.store, Logger: slog.New(slog.NewTextHandler(&f.logs, nil)), ChangeRisks: services.NewChangeRiskService(f.store, func() time.Time { return f.now })}
	f.router = Routes(f.h, &observability.Metrics{}, RouterConfig{RequestTimeout: 15 * time.Second})
	return f
}

func (f *riskHTTPFixture) event(t *testing.T, device string) string {
	t.Helper()
	ctx := context.Background()
	at := f.now.Add(-4 * time.Hour)
	backup := func(at time.Time) domain.ConfigBackup {
		id := uuid.NewString()
		return domain.ConfigBackup{ID: id, DeviceID: device, ConfigType: "running", StoragePath: id + ".txt", ContentHash: id, SizeBytes: 1, TriggerType: "manual", RetrievedAt: at, CreatedAt: at}
	}
	from, to := backup(at.Add(-time.Hour)), backup(at)
	require.NoError(t, f.store.CreateBackup(ctx, from))
	e := domain.ChangeEvent{ID: uuid.NewString(), DeviceID: device, ConfigType: "running", FromBackupID: from.ID, ToBackupID: to.ID, DetectedAt: at, AddedLines: 1, RemovedLines: 1, AuthorizationStatus: "unaccounted", PolicyStatus: "not_evaluated", AnalysisVersion: 1, AnalyzedAt: &at, ItemCount: 2, CreatedAt: at, UpdatedAt: at}
	items := []domain.ChangeEventItem{
		{ID: uuid.NewString(), ChangeEventID: e.ID, Ordinal: 0, Operation: "remove", Command: "description old", ContextPath: []string{"interface Gi1"}, CreatedAt: at},
		{ID: uuid.NewString(), ChangeEventID: e.ID, Ordinal: 1, Operation: "add", Command: "enable secret <redacted>", ContextPath: []string{}, Sensitive: true, CreatedAt: at},
	}
	for i := range items {
		c := changes.Classify(items[i].ContextPath, items[i].Command)
		items[i].Category, items[i].Subcategory, items[i].ClassificationRule = c.Category, c.Subcategory, c.RuleID
		items[i].ClassificationVersion, items[i].ClassifiedAt = 1, &at
	}
	require.NoError(t, f.store.CreateBackupWithChangeAnalysis(ctx, to, &e, items))
	return e.ID
}

func (f *riskHTTPFixture) base() string {
	return "/api/v1/devices/" + f.device + "/change-events/" + f.eventID
}

func (f *riskHTTPFixture) serve(ctx context.Context, method, path, role, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", f.requests.Add(1)%250+1)
	if role != "" {
		r.Header.Set("Authorization", "Bearer "+f.tokens[role])
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

func (f *riskHTTPFixture) call(t *testing.T, method, suffix, role, body string, want int) map[string]any {
	t.Helper()
	w := f.serve(context.Background(), method, f.base()+suffix, role, body, "application/json")
	require.Equal(t, want, w.Code, w.Body.String())
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	if want >= 400 {
		require.Nil(t, result["data"])
		require.NotEmpty(t, nestedString(t, result, "error", "code"))
		require.NotEmpty(t, nestedString(t, result, "error", "message"))
		require.True(t, riskHTTPID(nestedString(t, result, "error", "request_id")))
	} else {
		require.NotNil(t, result["data"])
		require.True(t, riskHTTPID(nestedString(t, result, "meta", "request_id")))
	}
	for _, forbidden := range []string{"RBAC_HTTP_SECRET", "source_bindings", "evidence_json", "result_json", "context_path", "\"command\"", "enable secret", "description old", "storage_path", "SQL logic error"} {
		require.NotContains(t, w.Body.String(), forbidden)
	}
	return result
}

// Compare all risk evidence plus successful durable actions, excluding only
// intentionally separate best-effort HTTP attempt/read audit rows.
func (f *riskHTTPFixture) evidence(t *testing.T) map[string][]string {
	t.Helper()
	snapshot := map[string][]string{}
	for _, table := range []string{"change_risk_assessments", "change_risk_findings", "change_risk_definitions", "audit_logs"} {
		query := "SELECT * FROM " + table
		if table == "audit_logs" {
			query += " WHERE action='change_risk.evaluate' AND succeeded=1"
		}
		rows, err := f.db.Query(query)
		require.NoError(t, err)
		columns, err := rows.Columns()
		require.NoError(t, err)
		values := []string{}
		for rows.Next() {
			v, pointers := make([]any, len(columns)), make([]any, len(columns))
			for i := range v {
				pointers[i] = &v[i]
			}
			require.NoError(t, rows.Scan(pointers...))
			raw, err := json.Marshal(v)
			require.NoError(t, err)
			values = append(values, string(raw))
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		sort.Strings(values)
		snapshot[table] = values
	}
	return snapshot
}

func (f *riskHTTPFixture) revoke(t *testing.T, role, permission string) {
	t.Helper()
	r, err := f.db.Exec(`DELETE FROM role_permissions WHERE permission_id=(SELECT id FROM permissions WHERE name=?) AND role_id IN (SELECT role_id FROM user_roles WHERE user_id=?)`, permission, f.users[role])
	require.NoError(t, err)
	n, err := r.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	p, err := f.store.GetPrincipal(context.Background(), f.users[role])
	require.NoError(t, err)
	require.False(t, p.HasPermission(permission))
}

func TestRiskHTTPSevenRoutesAndCAS(t *testing.T) {
	f := newRiskHTTP(t)
	before := f.evidence(t)
	preview := f.call(t, "POST", "/risk-assessment/preview", "network_operator", "{}", 200)
	require.False(t, preview["data"].(map[string]any)["historical"].(bool))
	require.Equal(t, before, f.evidence(t))
	f.call(t, "GET", "/risk-assessment", "network_operator", "", 404)
	created := f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 201)
	require.True(t, created["data"].(map[string]any)["created"].(bool))
	a := created["data"].(map[string]any)["assessment"].(map[string]any)
	id := a["id"].(string)
	require.True(t, a["historical"].(bool))
	require.Equal(t, float64(1), a["revision"])
	require.Nil(t, a["result"].(map[string]any)["findings"])
	require.NotEmpty(t, a["result"].(map[string]any)["governance_findings"])
	require.Len(t, f.evidence(t)["audit_logs"], 1)
	for _, suffix := range []string{"/risk-assessment", "/risk-assessments/" + id} {
		read := f.call(t, "GET", suffix, "network_operator", "", 200)
		require.Equal(t, a, read["data"])
	}
	for _, suffix := range []string{"/risk-assessment/findings", "/risk-assessments/" + id + "/findings"} {
		page := f.call(t, "GET", suffix+"?limit=1&offset=1", "network_operator", "", 200)
		require.Len(t, page["data"].([]any), 1)
		require.Equal(t, float64(1), page["data"].([]any)[0].(map[string]any)["ordinal"])
		require.Equal(t, id, nestedString(t, page, "meta", "assessment_id"))
		require.True(t, page["meta"].(map[string]any)["historical"].(bool))
	}
	history := f.call(t, "GET", "/risk-assessments", "network_operator", "", 200)
	require.Equal(t, []any{a}, history["data"])
	require.Equal(t, float64(50), history["meta"].(map[string]any)["pagination"].(map[string]any)["limit"])
	retry := f.call(t, "POST", "/risk-assessment", "system_administrator", `{"expected_latest_assessment_id":"`+id+`"}`, 200)
	require.False(t, retry["data"].(map[string]any)["created"].(bool))
	require.Equal(t, a, retry["data"].(map[string]any)["assessment"])
	f.call(t, "POST", "/risk-assessment", "system_administrator", `{"expected_latest_assessment_id":null}`, 200)
	before = f.evidence(t)
	f.call(t, "POST", "/risk-assessment", "system_administrator", `{"expected_latest_assessment_id":"`+uuid.NewString()+`"}`, 409)
	require.Equal(t, before, f.evidence(t))
	require.Len(t, before["audit_logs"], 3, "exactly one durable action per committed create/retry, never a handler duplicate")
}

func TestRiskHTTPAuthenticationRoleMatrix(t *testing.T) {
	f := newRiskHTTP(t)
	root := f.call(t, "POST", "/risk-assessment", "system_administrator", "{}", 201)
	id := nestedString(t, root, "data", "assessment", "id")
	for _, role := range []string{"", "viewer", "network_operator", "network_administrator", "system_administrator"} {
		for _, tc := range []struct{ method, suffix string }{
			{"POST", "/risk-assessment/preview"}, {"POST", "/risk-assessment"}, {"GET", "/risk-assessment"}, {"GET", "/risk-assessment/findings"}, {"GET", "/risk-assessments"}, {"GET", "/risk-assessments/" + id}, {"GET", "/risk-assessments/" + id + "/findings"},
		} {
			want := 200
			if role == "" {
				want = 401
			} else if role == "viewer" || (role == "network_operator" && tc.method == "POST" && tc.suffix == "/risk-assessment") {
				want = 403
			}
			before := f.evidence(t)
			f.call(t, tc.method, tc.suffix, role, "", want)
			if want != 200 {
				require.Equal(t, before, f.evidence(t))
			}
		}
	}
}

func TestRiskHTTPReadAndEvaluateRevocation(t *testing.T) {
	for _, recorded := range []bool{false, true} {
		for _, permission := range []string{domain.PermissionRiskRead, domain.PermissionRiskEvaluate, "devices.read", "backups.read", domain.PermissionAuthorizationRead, domain.PermissionPolicyRead} {
			t.Run(fmt.Sprintf("recorded=%t/%s", recorded, permission), func(t *testing.T) {
				f := newRiskHTTP(t)
				id := ""
				if recorded {
					res := f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 201)
					id = nestedString(t, res, "data", "assessment", "id")
				}
				f.revoke(t, "network_administrator", permission)
				p, err := f.store.GetPrincipal(context.Background(), f.users["network_administrator"])
				require.NoError(t, err)
				for _, keep := range []string{domain.PermissionRiskRead, domain.PermissionRiskEvaluate, "devices.read", "backups.read", domain.PermissionAuthorizationRead, domain.PermissionPolicyRead} {
					require.Equal(t, keep != permission, p.HasPermission(keep), keep)
				}
				before := f.evidence(t)
				body := "{}"
				if recorded {
					body = `{"expected_latest_assessment_id":"` + id + `"}`
				}
				f.call(t, "POST", "/risk-assessment", "network_administrator", body, 403)
				f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 403)
				readStatus := 403
				if permission == domain.PermissionRiskEvaluate {
					readStatus = 200
				}
				f.call(t, "POST", "/risk-assessment/preview", "network_administrator", "{}", readStatus)
				if recorded {
					for _, suffix := range []string{"/risk-assessment", "/risk-assessment/findings", "/risk-assessments", "/risk-assessments/" + id, "/risk-assessments/" + id + "/findings"} {
						f.call(t, "GET", suffix, "network_administrator", "", readStatus)
					}
				}
				require.Equal(t, before, f.evidence(t), "denial never writes evidence, registry or a successful action")
			})
		}
	}
}

func TestRiskHTTPStrictRequestsAndScope(t *testing.T) {
	f := newRiskHTTP(t)
	before := f.evidence(t)
	for i, tc := range []struct{ body, content string }{
		{"null", "application/json"}, {"[]", "application/json"}, {"42", "application/json"}, {" ", "application/json"}, {"{", "application/json"},
		{"{} {}", "application/json"}, {"{} garbage", "application/json"}, {`{"expected_latest_assessment_id":""}`, "application/json"},
		{`{"expected_latest_assessment_id":3}`, "application/json"}, {`{"expected_latest_assessment_id":{}}`, "application/json"},
		{`{"expected_latest_assessment_id":null,"expected_latest_assessment_id":null}`, "application/json"},
		{`{"expected_latest_assessment_id":null,"expected_latest_assessment_\u0069d":null}`, "application/json"},
		{`{"actor_id":"RBAC_HTTP_SECRET"}`, "application/json"}, {`{"result":{}}`, "application/json"}, {`{"evidence":{}}`, "application/json"},
		{`{"impact":"low","priority":"normal"}`, "application/json"}, {"{}", "text/plain"}, {"{}", ""}, {"{}", "application/json; charset=iso-8859-1"},
		{"{}" + strings.Repeat(" ", 4095), "application/json"}, {"{\"x\":\"" + string([]byte{255}) + "\"}", "application/json"},
	} {
		t.Run(fmt.Sprintf("strict_body_%02d", i), func(t *testing.T) {
			w := f.serve(context.Background(), "POST", f.base()+"/risk-assessment", "system_administrator", tc.body, tc.content)
			want, code := 400, "VALIDATION_FAILED"
			if tc.content == "text/plain" {
				want, code = 415, "UNSUPPORTED_MEDIA_TYPE"
			}
			require.Equal(t, want, w.Code, w.Body.String())
			require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, code, nestedString(t, body, "error", "code"))
			require.Equal(t, w.Header().Get("X-Request-ID"), nestedString(t, body, "error", "request_id"))
			require.True(t, riskHTTPID(nestedString(t, body, "error", "request_id")))
			require.NotContains(t, w.Body.String(), "RBAC_HTTP_SECRET")
		})
	}
	f.call(t, "POST", "/risk-assessment/preview", "system_administrator", `{"expected_latest_assessment_id":null}`, 400)
	for _, query := range []string{"limit=0", "limit=-1", "limit=201", "limit=", "limit=999999999999999999999999", "offset=-1", "offset=10001", "offset=99999999999999999999999", "offset=1.0", "limit=1&limit=2", "limit=%ZZ", "offset=+1", "score=0", "limit=2;offset=1"} {
		for _, suffix := range []string{"/risk-assessments", "/risk-assessment/findings"} {
			f.call(t, "GET", suffix+"?"+query, "system_administrator", "", 400)
		}
	}
	f.call(t, "POST", "/risk-assessment/preview?limit=1", "system_administrator", "{}", 400)
	f.call(t, "GET", "/risk-assessment?offset=0", "system_administrator", "", 400)
	require.Equal(t, before, f.evidence(t))
	created := f.call(t, "POST", "/risk-assessment", "system_administrator", "{}"+strings.Repeat(" ", 4094), 201)
	id := nestedString(t, created, "data", "assessment", "id")
	otherEvent := f.event(t, f.device)
	foreignDeviceEvent := f.event(t, f.otherDevice)
	for _, suffix := range []string{"/risk-assessments/" + uuid.NewString(), "/risk-assessments/" + uuid.NewString() + "/findings"} {
		f.call(t, "GET", suffix, "system_administrator", "", 404)
	}
	for _, path := range []string{
		"/api/v1/devices/" + f.otherDevice + "/change-events/" + f.eventID + "/risk-assessment",
		"/api/v1/devices/" + f.device + "/change-events/" + foreignDeviceEvent + "/risk-assessment",
		"/api/v1/devices/" + f.device + "/change-events/" + otherEvent + "/risk-assessments/" + id,
		"/api/v1/devices/" + f.device + "/change-events/" + otherEvent + "/risk-assessments/" + id + "/findings",
	} {
		w := f.serve(context.Background(), "GET", path, "system_administrator", "", "")
		require.Equal(t, 404, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"code":"NOT_FOUND"`)
		require.NotContains(t, w.Body.String(), id)
	}
	f.call(t, "GET", "/risk-assessments?limit=200&offset=10000", "system_administrator", "", 200)
	f.call(t, "GET", "/risk-assessments/not-a-uuid", "system_administrator", "", 400)
}
