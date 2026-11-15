package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/connectors/cisco_ios_ssh"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestLabPermissionsPersistenceAndStorageGate(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "lab.sqlite")
	key := randomBytes(32)
	const host = "127.0.0.1:9182"
	const origin = "http://" + host
	open := func() (*sqlite.Store, *credentials.Store, http.Handler) {
		store, err := sqlite.Open(context.Background(), db)
		require.NoError(t, err)
		vault, err := credentials.NewStore(store, key)
		require.NoError(t, err)
		connector, err := cisco_ios_ssh.NewPinned(nil, time.Second, time.Second, time.Second, maxConfigBytes)
		require.NoError(t, err)
		manager, err := newManager(store, vault, connector, filepath.Join(dir, "backups"))
		require.NoError(t, err)
		login, err := labLogin("synthetic operator fixture", "synthetic viewer fixture")
		require.NoError(t, err)
		return store, vault, newLabSite(host, login, manager, os.DirFS(dir)).routes()
	}
	store, _, site := open()
	request := func(method, path, requestOrigin string, body any, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		var raw io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			raw = bytes.NewReader(encoded)
		}
		req := httptest.NewRequest(method, origin+path, raw)
		req.RemoteAddr = "127.0.0.1:41001"
		req.Header.Set("Origin", requestOrigin)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		site.ServeHTTP(rec, req)
		require.Equal(t, status, rec.Code, "request %s %s", method, path)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		return rec
	}
	signIn := func(role string) *http.Cookie {
		result := request("POST", "/api/lab/session", origin, map[string]string{"username": role, "password": "synthetic " + role + " fixture"}, nil, 200)
		require.NotContains(t, result.Body.String(), "access_token")
		cookies := result.Result().Cookies()
		require.Len(t, cookies, 1)
		require.True(t, cookies[0].HttpOnly)
		require.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
		return cookies[0]
	}
	decode := func(rec *httptest.ResponseRecorder, value any) {
		var envelope struct{ Data json.RawMessage }
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
		require.NoError(t, json.Unmarshal(envelope.Data, value))
	}
	const devices = "/api/lab/devices"
	const secrets = "/api/lab/credentials"
	request("GET", devices, "", nil, nil, 401)
	operator, viewer := signIn("operator"), signIn("viewer")
	request("GET", secrets, "", nil, viewer, 403)
	request("POST", devices, "http://127.0.0.1:9999", nil, operator, 403)
	request("POST", devices, "", nil, operator, 403)
	request("POST", devices, origin, nil, viewer, 403)
	request("POST", secrets, origin, nil, viewer, 403)

	const password = "synthetic SSH fixture only"
	var credential domain.Credential
	created := request("POST", secrets, origin, map[string]string{"name": "Synthetic credential", "username": "synthetic-user", "auth_type": "password", "password": password}, operator, 201)
	decode(created, &credential)
	require.NotContains(t, created.Body.String(), password)
	require.NotContains(t, created.Body.String(), "synthetic-user")
	row, err := store.GetCredentialRecord(context.Background(), credential.ID)
	require.NoError(t, err)
	require.NotContains(t, string(row.PasswordEncrypted), password)
	require.NotContains(t, string(row.UsernameEncrypted), "synthetic-user")

	request("POST", secrets, origin, map[string]string{"name": "Malformed key", "username": "synthetic-user", "auth_type": "private_key", "private_key": "invalid"}, operator, 400)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyBlock, err := ssh.MarshalPrivateKey(privateKey, "synthetic test key")
	require.NoError(t, err)
	pemKey := string(pem.EncodeToMemory(keyBlock))
	savedKey := request("POST", secrets, origin, map[string]string{"name": "Synthetic private key", "username": "synthetic-user", "auth_type": "private_key", "private_key": pemKey}, operator, 201)
	require.NotContains(t, savedKey.Body.String(), "PRIVATE KEY")
	var keyRef domain.Credential
	decode(savedKey, &keyRef)
	keyRow, err := store.GetCredentialRecord(context.Background(), keyRef.ID)
	require.NoError(t, err)
	require.NotContains(t, string(keyRow.PrivateKeyEncrypted), "PRIVATE KEY")

	register := func(name, address string) labDevice {
		input := deviceInput{DisplayName: name, ManagementIP: address, ManagementPort: 22, Vendor: "cisco", Platform: "ios", CredentialID: credential.ID}
		var device labDevice
		decode(request("POST", devices, origin, input, operator, 201), &device)
		require.True(t, device.CredentialAssigned)
		require.False(t, device.Enabled || device.HostKeyPinned || device.TargetApproved || device.ConnectionReady)
		return device
	}
	first := register("Synthetic switch A", "192.0.2.1")
	second := register("Synthetic switch B", "192.0.2.2")
	require.NotEqual(t, first.ID, second.ID)
	update := deviceInput{DisplayName: "Synthetic switch A edited", ManagementIP: first.ManagementIP, ManagementPort: 22, Vendor: "cisco", Platform: "ios-xe", CredentialID: credential.ID}
	decode(request("PUT", devices+"/"+first.ID, origin, update, operator, 200), &first)
	require.Equal(t, update.DisplayName, first.DisplayName)
	require.False(t, first.Enabled)
	prefix := devices + "/" + first.ID
	request("GET", prefix+"/read-running", "", nil, operator, 405)
	request("POST", prefix+"/read-running", origin, nil, viewer, 403)
	request("POST", prefix+"/test-ssh", origin, nil, viewer, 403)
	for _, action := range []string{"/test-ssh", "/read-running", "/backups"} {
		blocked := request("POST", prefix+action, origin, nil, operator, 403)
		code := "TARGET_BLOCKED"
		if action == "/backups" {
			code = "BACKUP_STORAGE_LOCKED"
		}
		require.Contains(t, blocked.Body.String(), code)
	}
	require.NoFileExists(t, filepath.Join(dir, "backups", first.ID))

	// Only the test writes synthetic files. The browser has no backup writer.
	const saved = "hostname SYNTHETIC-LAB\nend\n"
	const updated = "hostname SYNTHETIC-LAB-UPDATED\nend\n"
	seed := func(id, body string, at time.Time) {
		sum := sha256.Sum256([]byte(body))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "backups", id+".txt"), []byte(body), 0o600))
		require.NoError(t, store.CreateBackup(context.Background(), domain.ConfigBackup{
			ID: id, DeviceID: first.ID, ConfigType: "running", StoragePath: id + ".txt", ContentHash: hex.EncodeToString(sum[:]), SizeBytes: int64(len(body)), TriggerType: "manual", CreatedAt: at, RetrievedAt: at,
		}))
	}
	now := time.Now().UTC()
	seed("test-backup-one", saved, now.Add(-time.Minute))
	seed("test-backup-two", updated, now)
	content := request("GET", prefix+"/backups/test-backup-one", "", nil, viewer, 200)
	require.Contains(t, content.Body.String(), "hostname SYNTHETIC-LAB")
	diff := request("GET", prefix+"/backups/test-backup-one/diff/test-backup-two", "", nil, viewer, 200)
	require.Contains(t, diff.Body.String(), "+hostname SYNTHETIC-LAB-UPDATED")
	request("GET", devices+"/"+second.ID+"/backups/test-backup-one", "", nil, viewer, 404)

	require.NoError(t, store.Close())
	store, vault, site := open()
	defer store.Close()
	request("GET", devices, "", nil, operator, 401)
	viewer = signIn("viewer")
	response := request("GET", devices, "", nil, viewer, 200)
	require.NotContains(t, response.Body.String(), "credential_id")
	var records []labDevice
	decode(response, &records)
	require.Len(t, records, 2)
	require.Equal(t, first.ID, records[0].ID)
	require.Equal(t, update.DisplayName, records[0].DisplayName)
	require.True(t, records[0].CredentialAssigned)
	require.NotNil(t, records[0].LastBackupAt)
	require.Nil(t, records[0].LastConnectionTest)
	c, err := vault.Get(context.Background(), credential.ID)
	require.NoError(t, err)
	require.Equal(t, password, c.Password)
	request("GET", prefix+"/backups/test-backup-two", "", nil, viewer, 200)
	request("DELETE", "/api/lab/session", origin, nil, viewer, 204)
	request("GET", devices, "", nil, viewer, 401)
}
