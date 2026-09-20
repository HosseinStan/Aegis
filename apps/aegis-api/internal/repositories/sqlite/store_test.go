package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMigrationsAndDeviceRepository(t *testing.T) {
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "aegis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	now := time.Now().UTC()
	credentialID := uuid.NewString()
	require.NoError(t, store.CreateCredentialRecord(context.Background(), repositories.CredentialRecord{ID: credentialID, Name: "test", AuthType: "password", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}))
	d := domain.Device{ID: uuid.NewString(), DisplayName: "Office switch", ManagementIP: "192.0.2.10", ManagementPort: 22, Vendor: "cisco", Platform: "ios", Protocol: "ssh", CredentialID: credentialID, Enabled: true, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, store.CreateDevice(context.Background(), d))
	loaded, err := store.GetDevice(context.Background(), d.ID)
	require.NoError(t, err)
	require.Equal(t, d.ManagementIP, loaded.ManagementIP)
	items, total, err := store.ListDevices(context.Background(), repositories.DeviceFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
}

func TestRBACSeeds(t *testing.T) {
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "aegis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	now := time.Now().UTC()
	userID := uuid.NewString()
	require.NoError(t, store.CreateUser(context.Background(), domain.User{ID: userID, Username: "admin", PasswordHash: []byte("hash"), Enabled: true, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, store.AssignRole(context.Background(), userID, "system_administrator"))
	principal, err := store.GetPrincipal(context.Background(), userID)
	require.NoError(t, err)
	require.True(t, principal.HasPermission("credentials.manage"))
	require.True(t, principal.HasPermission("system.manage"))
}
