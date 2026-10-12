package services

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestChangePolicyServiceLiveActorsScopeAndServerMetadata(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "policy-service.db")
	s, err := sqlite.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	now := time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC)
	service := NewChangePolicyService(s, s, s, func() time.Time { return now })
	credentialID := uuid.NewString()
	require.NoError(t, s.CreateCredentialRecord(ctx, repositories.CredentialRecord{ID: credentialID, Name: "lab", AuthType: "password", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}))
	device := domain.Device{ID: uuid.NewString(), DisplayName: "lab", ManagementIP: "192.0.2.70", ManagementPort: 22, Vendor: "cisco", Platform: "ios", Protocol: "ssh", CredentialID: credentialID, DeviceGroup: "group-A", Enabled: true, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, s.CreateDevice(ctx, device))
	actors := map[string]domain.Principal{}
	for _, role := range []string{"viewer", "network_operator", "network_administrator", "system_administrator"} {
		id := uuid.NewString()
		require.NoError(t, s.CreateUser(ctx, domain.User{ID: id, Username: role, Enabled: true, CreatedAt: now, UpdatedAt: now}))
		require.NoError(t, s.AssignRole(ctx, id, role))
		// Deliberately forged cached claims must never grant live access.
		actors[role] = domain.Principal{UserID: id, Permissions: []string{"*"}, DeviceGroups: []string{"*"}}
	}
	in := domain.PolicySetInput{Reference: " pol-service ", ConfigType: "running", Rules: []domain.PolicyRule{{Category: " SYSTEM ", Operation: "add", Decision: "allow"}}}
	_, err = service.Create(ctx, domain.Principal{}, device.ID, in)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	_, err = service.Create(ctx, domain.Principal{UserID: uuid.NewString(), Permissions: []string{"*"}}, device.ID, in)
	require.ErrorIs(t, err, domain.ErrForbidden)
	for _, role := range []string{"viewer", "network_operator", "network_administrator"} {
		_, err = service.Create(ctx, actors[role], device.ID, in)
		require.ErrorIs(t, err, domain.ErrForbidden)
	}
	p, err := service.Create(ctx, actors["system_administrator"], device.ID, in)
	require.NoError(t, err)
	require.Equal(t, "POL-SERVICE", p.Reference)
	require.Equal(t, now, p.CreatedAt)
	require.Equal(t, now, p.UpdatedAt)
	require.Equal(t, actors["system_administrator"].UserID, p.CreatedBy)
	require.Nil(t, p.ActivatedAt)
	require.Equal(t, "draft", p.Status)
	_, _, err = service.List(ctx, actors["network_operator"], device.ID, repositories.PolicySetFilter{}, repositories.Page{Limit: 0})
	require.ErrorIs(t, err, domain.ErrValidation)
	_, err = service.Get(ctx, actors["viewer"], device.ID, p.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Get(ctx, actors["network_operator"], uuid.NewString(), p.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	for _, role := range []string{"network_operator", "network_administrator"} {
		_, err = service.Activate(ctx, actors[role], device.ID, p.ID)
		require.ErrorIs(t, err, domain.ErrForbidden)
	}
	now = now.Add(time.Hour)
	p, err = service.Activate(ctx, actors["system_administrator"], device.ID, p.ID)
	require.NoError(t, err)
	require.Equal(t, now, *p.ActivatedAt)
	_, err = service.Preview(ctx, actors["network_operator"], device.ID, "missing")
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Evaluate(ctx, actors["network_operator"], device.ID, "missing")
	require.ErrorIs(t, err, domain.ErrForbidden)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`INSERT INTO user_device_groups VALUES (?,'group-B')`, actors["network_administrator"].UserID)
	require.NoError(t, err)
	for _, call := range []func() error{
		func() error { _, err := service.Get(ctx, actors["network_administrator"], device.ID, p.ID); return err },
		func() error {
			_, _, err := service.List(ctx, actors["network_administrator"], device.ID, repositories.PolicySetFilter{}, repositories.Page{Limit: 20})
			return err
		},
		func() error {
			_, err := service.Preview(ctx, actors["network_administrator"], device.ID, "missing")
			return err
		},
		func() error {
			_, err := service.Evaluate(ctx, actors["network_administrator"], device.ID, "missing")
			return err
		},
		func() error {
			_, err := service.Evidence(ctx, actors["network_administrator"], device.ID, "missing")
			return err
		},
		func() error {
			_, _, err := service.Items(ctx, actors["network_administrator"], device.ID, "missing", repositories.Page{Limit: 20})
			return err
		},
	} {
		require.ErrorIs(t, call(), domain.ErrForbidden)
	}
	_, err = db.Exec(`DELETE FROM user_roles WHERE user_id=?`, actors["system_administrator"].UserID)
	require.NoError(t, err)
	_, err = service.Retire(ctx, actors["system_administrator"], device.ID, p.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Get(ctx, actors["system_administrator"], device.ID, p.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, actors["network_operator"].UserID)
	require.NoError(t, err)
	_, err = service.Get(ctx, actors["network_operator"], device.ID, p.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
}
