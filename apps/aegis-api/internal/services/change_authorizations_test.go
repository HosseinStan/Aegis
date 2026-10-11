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

func TestChangeAuthorizationServiceLiveActorsAndScope(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "service.db")
	s, err := sqlite.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	service := NewChangeAuthorizationService(s, s, s, s, s, func() time.Time { return now })
	credentialID := uuid.NewString()
	require.NoError(t, s.CreateCredentialRecord(ctx, repositories.CredentialRecord{ID: credentialID, Name: "lab", AuthType: "password", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}))
	device := domain.Device{ID: uuid.NewString(), DisplayName: "lab", ManagementIP: "192.0.2.50", ManagementPort: 22, Vendor: "cisco", Platform: "ios", Protocol: "ssh", CredentialID: credentialID, DeviceGroup: "group-A", Enabled: true, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, s.CreateDevice(ctx, device))
	actors := map[string]domain.Principal{}
	for _, role := range []string{"viewer", "network_operator", "network_administrator", "system_administrator"} {
		user := domain.User{ID: uuid.NewString(), Username: role, Enabled: true, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, s.CreateUser(ctx, user))
		require.NoError(t, s.AssignRole(ctx, user.ID, role))
		actors[role] = domain.Principal{UserID: user.ID, Permissions: []string{"*"}, DeviceGroups: []string{"*"}}
	}
	input := domain.ChangeAuthorizationInput{Reference: " chg-service ", ConfigType: "running", AllowedCategories: []string{"SYSTEM", " interface "}, WindowStart: now.Add(-time.Hour), WindowEnd: now.Add(time.Hour)}
	_, err = service.Create(ctx, domain.Principal{}, device.ID, input)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	_, err = service.Create(ctx, domain.Principal{UserID: uuid.NewString(), Permissions: []string{"*"}}, device.ID, input)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Create(ctx, actors["viewer"], device.ID, input)
	require.ErrorIs(t, err, domain.ErrForbidden)
	a, err := service.Create(ctx, actors["network_operator"], device.ID, input)
	require.NoError(t, err)
	require.Equal(t, "CHG-SERVICE", a.Reference)
	require.Equal(t, []string{"interface", "system"}, a.AllowedCategories)
	require.Equal(t, now, a.RequestedAt)
	require.Equal(t, a.RequestedAt, a.CreatedAt)
	require.Equal(t, actors["network_operator"].UserID, a.RequestedBy)
	_, err = service.Create(ctx, actors["network_operator"], device.ID, input)
	require.ErrorIs(t, err, domain.ErrConflict)
	for _, role := range []string{"viewer", "network_operator", "network_administrator"} {
		_, err = service.Approve(ctx, actors[role], device.ID, a.ID)
		require.ErrorIs(t, err, domain.ErrForbidden)
	}
	_, err = service.Reconcile(ctx, actors["network_operator"], device.ID, "missing", a.ID, domain.ChangeAuthorizationAttestation)
	require.ErrorIs(t, err, domain.ErrForbidden)
	approved, err := service.Approve(ctx, actors["system_administrator"], device.ID, a.ID)
	require.NoError(t, err)
	require.Equal(t, now, *approved.ApprovedAt)
	input.Reference = "CHG-SELF"
	self, err := service.Create(ctx, actors["system_administrator"], device.ID, input)
	require.NoError(t, err)
	_, err = service.Approve(ctx, actors["system_administrator"], device.ID, self.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Create(ctx, actors["network_operator"], uuid.NewString(), input)
	require.ErrorIs(t, err, domain.ErrNotFound)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`INSERT INTO user_device_groups VALUES (?,?)`, actors["network_operator"].UserID, "group-B")
	require.NoError(t, err)
	p := actors["network_operator"]
	_, err = service.Get(ctx, p, device.ID, a.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, _, err = service.List(ctx, p, device.ID, repositories.ChangeAuthorizationFilter{}, repositories.Page{Limit: 10})
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Create(ctx, p, device.ID, input)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.GetEvent(ctx, p, device.ID, "missing")
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, _, err = service.ListEvents(ctx, p, device.ID, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, _, err = service.ListItems(ctx, p, device.ID, "missing", repositories.Page{Limit: 10})
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Evidence(ctx, p, device.ID, "missing")
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, _, err = service.History(ctx, p, device.ID, a.ID, repositories.Page{Limit: 10})
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, actors["system_administrator"].UserID)
	require.NoError(t, err)
	_, err = service.Approve(ctx, actors["system_administrator"], device.ID, self.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = service.Create(ctx, actors["system_administrator"], device.ID, input)
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestChangeAuthorizationServiceRejectsInvalidFiltersAndPagination(t *testing.T) {
	require.False(t, authorizationPageValid(repositories.Page{Limit: 0}))
	require.False(t, authorizationPageValid(repositories.Page{Limit: 201}))
	require.False(t, authorizationPageValid(repositories.Page{Limit: 10, Offset: -1}))
	require.False(t, authorizationStatusValid("expired"))
	require.False(t, authorizationConfigValid("candidate"))
}
