package api

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/auth"
	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/connectors/mock"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type authorizationAPI struct {
	server                    *httptest.Server
	store                     *sqlite.Store
	connector                 *mock.Connector
	tokens, users             map[string]string
	device, otherDevice, path string
	now                       *time.Time
}

func newAuthorizationAPI(t *testing.T) authorizationAPI {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "api.db")
	store, err := sqlite.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	now := time.Now().UTC()
	a := authorizationAPI{store: store, tokens: map[string]string{}, users: map[string]string{}, path: path, now: &now}
	authService, err := auth.NewService(store, "aegis-api", "test", bytes.Repeat([]byte{8}, 32), 15*time.Minute, time.Hour)
	require.NoError(t, err)
	password := "long test fixture password"
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	for _, role := range []string{"viewer", "network_operator", "network_administrator", "system_administrator", "independent_admin"} {
		id := uuid.NewString()
		a.users[role] = id
		require.NoError(t, store.CreateUser(ctx, domain.User{ID: id, Username: role, PasswordHash: hash, Enabled: true, CreatedAt: now, UpdatedAt: now}))
		grant := role
		if role == "independent_admin" {
			grant = "system_administrator"
		}
		require.NoError(t, store.AssignRole(ctx, id, grant))
		tokens, _, err := authService.Login(ctx, role, password)
		require.NoError(t, err)
		a.tokens[role] = tokens.AccessToken
	}
	credentialStore, err := credentials.NewStore(store, bytes.Repeat([]byte{4}, 32))
	require.NoError(t, err)
	credential, err := credentialStore.Create(ctx, domain.Credential{Name: "lab", AuthType: "password", Username: "test", Password: "device-secret"})
	require.NoError(t, err)
	for index, target := range []*string{&a.device, &a.otherDevice} {
		*target = uuid.NewString()
		ip := []string{"192.0.2.60", "192.0.2.61"}[index]
		require.NoError(t, store.CreateDevice(ctx, domain.Device{ID: *target, DisplayName: "lab", ManagementIP: ip, ManagementPort: 22, Vendor: "cisco", Platform: "ios", Protocol: "ssh", CredentialID: credential.ID, DeviceGroup: "group-A", Enabled: true, CreatedAt: now, UpdatedAt: now}))
	}
	registry := connectors.NewRegistry()
	a.connector = &mock.Connector{Running: "hostname LAB\n"}
	require.NoError(t, registry.Register("cisco", "ios", "ssh", a.connector))
	devices := services.NewDeviceService(store, credentialStore, registry)
	configs := services.NewConfigService(store, credentialStore, registry, time.Minute, time.Minute, time.Minute, 5, 1)
	backups, err := services.NewBackupService(store, configs, filepath.Join(t.TempDir(), "backups"), 1<<20)
	require.NoError(t, err)
	h := &Handler{Auth: authService, Credentials: credentialStore, Devices: devices, Configs: configs, Backups: backups, Audits: store, Health: store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.ChangeAuthorizations = services.NewChangeAuthorizationService(store, store, store, store, store, func() time.Time { return *a.now })
	h.ChangePolicies = services.NewChangePolicyService(store, store, store, func() time.Time { return *a.now })
	a.server = httptest.NewServer(Routes(h, &observability.Metrics{}, RouterConfig{RequestTimeout: 10 * time.Second}))
	t.Cleanup(a.server.Close)
	return a
}

func (a authorizationAPI) call(t *testing.T, method, path, role string, body any) jsonResponse {
	t.Helper()
	return requestJSON(t, a.server.Client(), method, a.server.URL+"/api/v1/devices/"+a.device+path, a.tokens[role], body)
}
func (a authorizationAPI) input(reference string) map[string]any {
	return map[string]any{"reference": reference, "config_type": "running", "allowed_categories": []string{"interface", "aaa"}, "window_start": a.now.Add(-time.Hour).Format(time.RFC3339Nano), "window_end": a.now.Add(time.Hour).Format(time.RFC3339Nano)}
}

func TestChangeAuthorizationHTTPFullWorkflow(t *testing.T) {
	a := newAuthorizationAPI(t)
	first := a.call(t, "POST", "/backups", "network_operator", map[string]any{"config_type": "running"})
	require.Equal(t, 201, first.status, first.raw)
	created := a.call(t, "POST", "/change-authorizations", "network_operator", a.input(" chg-api-001 "))
	require.Equal(t, 201, created.status, created.raw)
	id := nestedString(t, created.body, "data", "id")
	require.Equal(t, a.users["network_operator"], nestedString(t, created.body, "data", "requested_by"))
	duplicate := a.call(t, "POST", "/change-authorizations", "network_operator", a.input("CHG-API-001"))
	require.Equal(t, 409, duplicate.status)
	approved := a.call(t, "POST", "/change-authorizations/"+id+"/approve", "system_administrator", nil)
	require.Equal(t, 200, approved.status, approved.raw)
	a.connector.SetRunning("hostname LAB\ninterface Gi1\n description approved-uplink\nusername ops secret 0 secret-should-never-appear\n")
	second := a.call(t, "POST", "/backups", "network_operator", map[string]any{"config_type": "running"})
	require.Equal(t, 201, second.status, second.raw)
	events := a.call(t, "GET", "/change-events?limit=1&authorization_status=unaccounted", "network_operator", nil)
	require.Equal(t, 200, events.status, events.raw)
	list := events.body["data"].([]any)
	require.Len(t, list, 1)
	eventID := list[0].(map[string]any)["id"].(string)
	event, err := a.store.GetChangeEvent(context.Background(), eventID)
	require.NoError(t, err)
	require.Equal(t, "unaccounted", event.AuthorizationStatus)
	require.Equal(t, "not_evaluated", event.PolicyStatus)
	*a.now = event.DetectedAt.Add(time.Second)
	items := a.call(t, "GET", "/change-events/"+eventID+"/items", "network_operator", nil)
	require.Equal(t, 200, items.status, items.raw)
	require.NotContains(t, items.raw, "secret-should-never-appear")
	require.Contains(t, items.raw, "redacted")
	require.Contains(t, items.raw, "classification_version")
	require.NotContains(t, items.raw, "storage_path")
	get := a.call(t, "GET", "/change-events/"+eventID, "network_operator", nil)
	require.Equal(t, 200, get.status)
	require.NotContains(t, get.raw, "raw_diff")
	before := a.call(t, "GET", "/change-events/"+eventID+"/authorization", "network_operator", nil)
	require.Equal(t, 404, before.status)
	calls := a.connector.CallCount()
	bad := a.call(t, "POST", "/change-events/"+eventID+"/authorize", "network_administrator", map[string]any{"authorization_id": id})
	require.Equal(t, 400, bad.status)
	linked := a.call(t, "POST", "/change-events/"+eventID+"/authorize", "network_administrator", map[string]any{"authorization_id": id, "attestation": domain.ChangeAuthorizationAttestation})
	require.Equal(t, 200, linked.status, linked.raw)
	dup := a.call(t, "POST", "/change-events/"+eventID+"/authorize", "network_administrator", map[string]any{"authorization_id": id, "attestation": domain.ChangeAuthorizationAttestation})
	require.Equal(t, 409, dup.status)
	get = a.call(t, "GET", "/change-events/"+eventID, "network_operator", nil)
	require.Equal(t, "authorized", nestedString(t, get.body, "data", "authorization_status"))
	require.Equal(t, "not_evaluated", nestedString(t, get.body, "data", "policy_status"))
	evidence := a.call(t, "GET", "/change-events/"+eventID+"/authorization", "network_operator", nil)
	require.Equal(t, 200, evidence.status)
	require.Equal(t, id, nestedString(t, evidence.body, "data", "authorization_id"))
	revoked := a.call(t, "POST", "/change-authorizations/"+id+"/revoke", "system_administrator", nil)
	require.Equal(t, 200, revoked.status)
	evidence = a.call(t, "GET", "/change-events/"+eventID+"/authorization", "network_operator", nil)
	require.Equal(t, 200, evidence.status)
	history := a.call(t, "GET", "/change-authorizations/"+id+"/history", "network_operator", nil)
	require.Equal(t, 200, history.status, history.raw)
	entries := history.body["data"].([]any)
	require.Len(t, entries, 4)
	for index, action := range []string{"requested", "approved", "linked", "revoked"} {
		require.Equal(t, action, entries[index].(map[string]any)["action"])
	}
	require.Equal(t, calls, a.connector.CallCount(), "authorization operations must not execute device commands")
	wrongDevice := requestJSON(t, a.server.Client(), "GET", a.server.URL+"/api/v1/devices/"+a.otherDevice+"/change-authorizations/"+id, a.tokens["network_operator"], nil)
	require.Equal(t, 404, wrongDevice.status)
	wrongEvent := requestJSON(t, a.server.Client(), "GET", a.server.URL+"/api/v1/devices/"+a.otherDevice+"/change-events/"+eventID+"/items", a.tokens["network_operator"], nil)
	require.Equal(t, 404, wrongEvent.status)
	audits, _, err := a.store.ListAudits(context.Background(), repositories.Page{Limit: 200})
	require.NoError(t, err)
	actions := map[string]bool{}
	for _, log := range audits {
		actions[log.Action] = true
	}
	for _, action := range []string{"change_authorization.request", "change_authorization.approve", "change_authorization.link", "change_authorization.revoke"} {
		require.True(t, actions[action], action)
	}
}

func TestChangeAuthorizationHTTPRBACAndForgedInputs(t *testing.T) {
	a := newAuthorizationAPI(t)
	for _, test := range []struct {
		role, path, method string
		body               any
		status             int
	}{
		{"", "/change-authorizations", "GET", nil, 401},
		{"viewer", "/change-authorizations", "POST", a.input("CHG-VIEWER"), 403},
		{"viewer", "/change-authorizations", "GET", nil, 403},
		{"viewer", "/change-events", "GET", nil, 403},
		{"network_operator", "/change-authorizations/missing/approve", "POST", nil, 403},
		{"network_administrator", "/change-authorizations/missing/approve", "POST", nil, 403},
		{"network_operator", "/change-events/missing/authorize", "POST", map[string]any{}, 403},
		{"viewer", "/change-events/missing/authorize", "POST", map[string]any{}, 403},
	} {
		got := a.call(t, test.method, test.path, test.role, test.body)
		require.Equal(t, test.status, got.status, got.raw)
	}
	for _, field := range []string{"requested_by", "requested_at", "approved_by", "device_id"} {
		input := a.input("CHG-FORGE")
		input[field] = "forged"
		got := a.call(t, "POST", "/change-authorizations", "system_administrator", input)
		require.Equal(t, 400, got.status)
	}
	for _, value := range []any{"2026-10-12T08:00:00", "not-a-time", nil} {
		input := a.input("CHG-TIME")
		input["window_start"] = value
		got := a.call(t, "POST", "/change-authorizations", "system_administrator", input)
		require.Equal(t, 400, got.status)
	}
	self := a.call(t, "POST", "/change-authorizations", "system_administrator", a.input("CHG-SELF"))
	require.Equal(t, 201, self.status)
	id := nestedString(t, self.body, "data", "id")
	got := a.call(t, "POST", "/change-authorizations/"+id+"/approve", "system_administrator", nil)
	require.Equal(t, 403, got.status)
	got = a.call(t, "POST", "/change-authorizations/"+id+"/approve", "independent_admin", map[string]any{"approved_by": "forged"})
	require.Equal(t, 400, got.status)
	got = a.call(t, "POST", "/change-authorizations/"+id+"/approve", "independent_admin", nil)
	require.Equal(t, 200, got.status, got.raw)
	for _, path := range []string{"/change-authorizations?limit=201", "/change-authorizations?status=expired", "/change-events?offset=-1", "/change-events?config_type=candidate"} {
		got = a.call(t, "GET", path, "network_operator", nil)
		require.Equal(t, 400, got.status, got.raw)
	}
	got = a.call(t, "POST", "/change-authorizations", "system_administrator", a.input("CHG';DROP TABLE users;--"))
	require.Equal(t, 400, got.status)
	require.NotContains(t, got.raw, "SQL")
}

func TestChangeAuthorizationHTTPCrossGroupAllEndpoints(t *testing.T) {
	a := newAuthorizationAPI(t)
	db, err := sql.Open("sqlite", a.path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	// A custom approver has all feature permissions but lacks system.manage,
	// so the existing device-group restriction remains applicable.
	_, err = db.Exec(`INSERT INTO role_permissions SELECT 'role-network-admin', id FROM permissions WHERE name='changes.authorization.approve'`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO user_device_groups VALUES (?,?)`, a.users["network_administrator"], "group-B")
	require.NoError(t, err)
	for _, test := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/change-authorizations", nil}, {"GET", "/change-authorizations/missing", nil}, {"GET", "/change-authorizations/missing/history", nil},
		{"POST", "/change-authorizations", a.input("CHG-GROUP")}, {"POST", "/change-authorizations/missing/approve", nil}, {"POST", "/change-authorizations/missing/reject", nil}, {"POST", "/change-authorizations/missing/revoke", nil},
		{"GET", "/change-events", nil}, {"GET", "/change-events/missing", nil}, {"GET", "/change-events/missing/items", nil}, {"GET", "/change-events/missing/authorization", nil},
		{"POST", "/change-events/missing/authorize", map[string]any{"authorization_id": "missing", "attestation": domain.ChangeAuthorizationAttestation}},
	} {
		got := a.call(t, test.method, test.path, "network_administrator", test.body)
		require.Equal(t, 403, got.status, got.raw)
		require.NotContains(t, got.raw, "group-A")
	}
}

func TestAuthorizationDecoderRejectsTrailingJSON(t *testing.T) {
	for _, body := range []string{`{} {}`, `{"approved_by":"forged"}`, `{"approved_at":"2026-10-12T08:00:00Z"}`, `{broken}`, `null`, `[]`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var input struct{}
		require.ErrorIs(t, decodeAuthorizationJSON(httptest.NewRecorder(), r, &input, true), domain.ErrValidation)
	}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	var input struct{}
	require.NoError(t, decodeAuthorizationJSON(httptest.NewRecorder(), r, &input, true))
}

func TestAuthorizationItemsHTTPFailsClosedOnCorruptedSecret(t *testing.T) {
	a := newAuthorizationAPI(t)
	first := a.call(t, "POST", "/backups", "network_operator", map[string]any{"config_type": "running"})
	require.Equal(t, 201, first.status)
	a.connector.SetRunning("hostname LAB\nusername ops secret 0 original-secret\n")
	second := a.call(t, "POST", "/backups", "network_operator", map[string]any{"config_type": "running"})
	require.Equal(t, 201, second.status)
	events, _, err := a.store.ListChangeEvents(context.Background(), repositories.ChangeEventFilter{DeviceID: a.device}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, events, 1)
	db, err := sql.Open("sqlite", a.path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`UPDATE change_event_items SET command='username ops secret 0 corrupted-secret' WHERE change_event_id=?`, events[0].ID)
	require.NoError(t, err)
	items := a.call(t, "GET", "/change-events/"+events[0].ID+"/items", "network_operator", nil)
	require.Equal(t, 400, items.status)
	require.NotContains(t, items.raw, "corrupted-secret")
	require.NotContains(t, items.raw, "original-secret")
}
