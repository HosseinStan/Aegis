package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/auth"
	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/connectors/mock"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAPIWorkflow(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "aegis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	hash, err := auth.HashPassword("correct horse battery staple")
	require.NoError(t, err)
	now := time.Now().UTC()
	userID := uuid.NewString()
	require.NoError(t, store.CreateUser(ctx, domain.User{ID: userID, Username: "admin", PasswordHash: hash, Enabled: true, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, store.AssignRole(ctx, userID, "system_administrator"))
	viewerHash, err := auth.HashPassword("viewer password is long")
	require.NoError(t, err)
	viewerID := uuid.NewString()
	require.NoError(t, store.CreateUser(ctx, domain.User{ID: viewerID, Username: "viewer", PasswordHash: viewerHash, Enabled: true, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, store.AssignRole(ctx, viewerID, "viewer"))

	credentialStore, err := credentials.NewStore(store, bytes.Repeat([]byte{4}, 32))
	require.NoError(t, err)
	authService, err := auth.NewService(store, "aegis-api", "office-vue-app", bytes.Repeat([]byte{8}, 32), 15*time.Minute, time.Hour)
	require.NoError(t, err)
	registry := connectors.NewRegistry()
	deviceConnector := &mock.Connector{Running: "version 17.9\nhostname OFFICE-SW1\n", Startup: "version 17.9\nhostname OFFICE-SW1\n", Facts: domain.DeviceFacts{Hostname: "OFFICE-SW1", Model: "C9300"}, Interfaces: []domain.InterfaceStatus{{Name: "Gi1/0/1", Status: "connected"}}}
	require.NoError(t, registry.Register("cisco", "ios", "ssh", deviceConnector))
	deviceService := services.NewDeviceService(store, credentialStore, registry)
	configService := services.NewConfigService(store, credentialStore, registry, time.Minute, time.Minute, time.Minute, 5, 1)
	backupService, err := services.NewBackupService(store, configService, filepath.Join(t.TempDir(), "backups"), 1024*1024)
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := &Handler{Auth: authService, Credentials: credentialStore, Devices: deviceService, Configs: configService, Backups: backupService, Audits: store, Health: store, Logger: logger, OpenAPIPath: filepath.Join("..", "..", "..", "..", "packages", "shared-contracts", "openapi", "aegis-api.yaml")}
	server := httptest.NewServer(Routes(handler, &observability.Metrics{}, RouterConfig{RequestTimeout: 10 * time.Second}))
	t.Cleanup(server.Close)

	unauthorized := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/devices", "", nil)
	require.Equal(t, http.StatusUnauthorized, unauthorized.StatusCode)
	unauthorized.Body.Close()

	login := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]any{"username": "admin", "password": "correct horse battery staple"})
	require.Equal(t, http.StatusOK, login.status)
	token := nestedString(t, login.body, "data", "tokens", "access_token")
	docsSession := request(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/docs-session", token, nil)
	require.Equal(t, http.StatusNoContent, docsSession.StatusCode)
	docsCookies := docsSession.Cookies()
	docsSession.Body.Close()
	require.NotEmpty(t, docsCookies)
	docsRequest, err := http.NewRequest(http.MethodGet, server.URL+"/docs/", nil)
	require.NoError(t, err)
	docsRequest.AddCookie(docsCookies[0])
	docsResponse, err := server.Client().Do(docsRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, docsResponse.StatusCode)
	docsResponse.Body.Close()
	viewerLogin := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]any{"username": "viewer", "password": "viewer password is long"})
	require.Equal(t, http.StatusOK, viewerLogin.status)
	viewerToken := nestedString(t, viewerLogin.body, "data", "tokens", "access_token")
	forbidden := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/credentials", viewerToken, map[string]any{"name": "denied", "auth_type": "password", "username": "x", "password": "x"})
	require.Equal(t, http.StatusForbidden, forbidden.status)

	credential := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/credentials", token, map[string]any{"name": "switch credential", "auth_type": "password", "username": "aegis", "password": "device-password"})
	require.Equal(t, http.StatusCreated, credential.status, credential.raw)
	credentialID := nestedString(t, credential.body, "data", "id")
	require.NotContains(t, credential.raw, "device-password")

	device := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/devices", token, map[string]any{"display_name": "Office SW1", "hostname": "OFFICE-SW1", "management_ip": "192.0.2.10", "management_port": 22, "vendor": "cisco", "platform": "ios", "protocol": "ssh", "credential_id": credentialID, "device_group": "branch-office"})
	require.Equal(t, http.StatusCreated, device.status, device.raw)
	deviceID := nestedString(t, device.body, "data", "id")
	missing := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/devices/"+uuid.NewString(), token, nil)
	require.Equal(t, http.StatusNotFound, missing.status)
	require.Equal(t, "DEVICE_NOT_FOUND", nestedString(t, missing.body, "error", "code"))

	config := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/devices/"+deviceID+"/config/running", token, nil)
	require.Equal(t, http.StatusOK, config.status, config.raw)
	require.Contains(t, nestedString(t, config.body, "data", "config"), "hostname OFFICE-SW1")
	callsAfterLiveRead := deviceConnector.CallCount()
	cachedConfig := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/devices/"+deviceID+"/config/running", token, nil)
	require.Equal(t, http.StatusOK, cachedConfig.status)
	require.True(t, nestedBool(t, cachedConfig.body, "data", "from_cache"))
	require.Equal(t, callsAfterLiveRead, deviceConnector.CallCount())
	deviceConnector.SetError(domain.ErrConnection)
	failed := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/devices/"+deviceID+"/config/running?refresh=true", token, nil)
	require.Equal(t, http.StatusBadGateway, failed.status)
	require.Equal(t, "DEVICE_CONNECTION_FAILED", nestedString(t, failed.body, "error", "code"))
	require.NotContains(t, failed.raw, "SSH")
	deviceConnector.SetError(nil)

	first := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/devices/"+deviceID+"/backups", token, map[string]any{"config_type": "running"})
	require.Equal(t, http.StatusCreated, first.status, first.raw)
	firstID := nestedString(t, first.body, "data", "backup", "id")
	duplicate := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/devices/"+deviceID+"/backups", token, map[string]any{"config_type": "running"})
	require.Equal(t, http.StatusOK, duplicate.status, duplicate.raw)

	deviceConnector.SetRunning("version 17.9\nhostname OFFICE-SW1\ninterface Gi1/0/1\n description uplink\n")
	second := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/devices/"+deviceID+"/backups", token, map[string]any{"config_type": "running"})
	require.Equal(t, http.StatusCreated, second.status, second.raw)
	secondID := nestedString(t, second.body, "data", "backup", "id")
	diff := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/devices/"+deviceID+"/backups/"+firstID+"/diff/"+secondID, token, nil)
	require.Equal(t, http.StatusOK, diff.status, diff.raw)
	require.True(t, nestedBool(t, diff.body, "data", "changed"))

	audits := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/audit-logs", token, nil)
	require.Equal(t, http.StatusOK, audits.status, audits.raw)
	require.NotEmpty(t, audits.body["data"])
}

type jsonResponse struct {
	status int
	body   map[string]any
	raw    string
}

func requestJSON(t *testing.T, client *http.Client, method, url, token string, body any) jsonResponse {
	t.Helper()
	response := request(t, client, method, url, token, body)
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	decoded := map[string]any{}
	if len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, &decoded), string(raw))
	}
	return jsonResponse{status: response.StatusCode, body: decoded, raw: string(raw)}
}

func request(t *testing.T, client *http.Client, method, url, token string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(req)
	require.NoError(t, err)
	return response
}

func nestedString(t *testing.T, root map[string]any, keys ...string) string {
	t.Helper()
	var current any = root
	for _, key := range keys {
		object, ok := current.(map[string]any)
		require.True(t, ok, "expected object before %s", key)
		current, ok = object[key]
		require.True(t, ok, "missing key %s", key)
	}
	value, ok := current.(string)
	require.True(t, ok, "expected string at %v", keys)
	return value
}
func nestedBool(t *testing.T, root map[string]any, keys ...string) bool {
	t.Helper()
	var current any = root
	for _, key := range keys {
		object, ok := current.(map[string]any)
		require.True(t, ok)
		current, ok = object[key]
		require.True(t, ok)
	}
	value, ok := current.(bool)
	require.True(t, ok)
	return value
}
