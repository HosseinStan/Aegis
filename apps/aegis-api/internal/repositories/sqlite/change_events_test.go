package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func changeEventStore(t *testing.T) (*Store, []domain.Device) {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "aegis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s, changeEventDevices(t, s)
}

func changeEventDevices(t *testing.T, s *Store) []domain.Device {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	credentialID := uuid.NewString()
	require.NoError(t, s.CreateCredentialRecord(ctx, repositories.CredentialRecord{
		ID: credentialID, Name: "lab", AuthType: "password", CreatedAt: timestamp(now), UpdatedAt: timestamp(now),
	}))
	devices := []domain.Device{
		{ID: uuid.NewString(), DisplayName: "Lab A", ManagementIP: "192.0.2.10"},
		{ID: uuid.NewString(), DisplayName: "Lab B", ManagementIP: "192.0.2.11"},
	}
	for i := range devices {
		d := &devices[i]
		d.ManagementPort, d.Vendor, d.Platform, d.Protocol = 22, "cisco", "ios", "ssh"
		d.CredentialID, d.Enabled, d.CreatedAt, d.UpdatedAt = credentialID, true, now, now
		require.NoError(t, s.CreateDevice(ctx, *d))
	}
	return devices
}

func eventBackup(deviceID, kind string, now time.Time) domain.ConfigBackup {
	id := uuid.NewString()
	return domain.ConfigBackup{ID: id, DeviceID: deviceID, ConfigType: kind,
		StoragePath: id + ".txt", ContentHash: "test-hash", SizeBytes: 1,
		TriggerType: "manual", RetrievedAt: now, CreatedAt: now}
}

func eventBetween(from, to domain.ConfigBackup) domain.ChangeEvent {
	return domain.ChangeEvent{ID: uuid.NewString(), DeviceID: to.DeviceID, ConfigType: to.ConfigType,
		FromBackupID: from.ID, ToBackupID: to.ID, DetectedAt: to.CreatedAt,
		AddedLines: 2, RemovedLines: 1, AuthorizationStatus: domain.AuthorizationStatusUnaccounted,
		PolicyStatus: domain.PolicyStatusNotEvaluated, CreatedAt: to.CreatedAt, UpdatedAt: to.CreatedAt}
}

func TestChangeEventMigrationUpgradePreservesHistoricalBackups(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "existing.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	s := &Store{db: db}
	db.SetMaxOpenConns(1)
	for _, name := range []string{"001_initial.sql", "002_seed_rbac.sql"} {
		body, err := migrations.Files.ReadFile(name)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(body))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?, ?)`, name, timestamp(time.Now()))
		require.NoError(t, err)
	}
	devices := changeEventDevices(t, s)
	before := eventBackup(devices[0].ID, "running", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	second := eventBackup(devices[0].ID, "running", before.CreatedAt.Add(time.Hour))
	require.NoError(t, s.CreateBackup(ctx, before))
	require.NoError(t, s.CreateBackup(ctx, second))
	require.NoError(t, s.Close())

	s, err = Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.Migrate(ctx)) // Reopening/reapplying is safe.
	loaded, err := s.GetBackup(ctx, before.DeviceID, before.ID)
	require.NoError(t, err)
	require.Equal(t, before, loaded)
	items, total, err := s.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total) // Structure only; historical backups are not backfilled.
	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	require.Equal(t, 7, count)
}

func TestChangeEventRepositoryReadsAndFilters(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	events := make([]domain.ChangeEvent, 3)
	for i := range events {
		device, kind := devices[0], "running"
		if i == 2 {
			device, kind = devices[1], "startup"
		}
		from := eventBackup(device.ID, kind, now.Add(-time.Hour))
		// Exercise both exact-second/fractional ordering and equal-time ID ties.
		to := eventBackup(device.ID, kind, now.Add(time.Duration(i/2)*time.Nanosecond))
		events[i] = eventBetween(from, to)
		events[i].ID = []string{"event-a", "event-b", "event-c"}[i]
		if i == 2 {
			events[i].AuthorizationStatus = domain.AuthorizationStatusAuthorized
			events[i].PolicyStatus = domain.PolicyStatusViolation
		}
		require.NoError(t, s.CreateBackup(ctx, from))
		require.NoError(t, s.CreateBackupWithChangeEvent(ctx, to, &events[i]))
		loaded, err := s.GetChangeEvent(ctx, events[i].ID)
		require.NoError(t, err)
		require.Equal(t, events[i], loaded)
	}
	_, err := s.GetChangeEvent(ctx, "absent")
	require.ErrorIs(t, err, domain.ErrNotFound)

	for _, test := range []struct {
		name   string
		filter repositories.ChangeEventFilter
		page   repositories.Page
		ids    []string
		total  int
	}{
		{"all and tie ordering", repositories.ChangeEventFilter{}, repositories.Page{Limit: 10}, []string{"event-c", "event-b", "event-a"}, 3},
		{"page", repositories.ChangeEventFilter{}, repositories.Page{Limit: 1, Offset: 1}, []string{"event-b"}, 3},
		{"past end", repositories.ChangeEventFilter{}, repositories.Page{Limit: 1, Offset: 9}, []string{}, 3},
		{"device", repositories.ChangeEventFilter{DeviceID: devices[0].ID}, repositories.Page{Limit: 10}, []string{"event-b", "event-a"}, 2},
		{"type", repositories.ChangeEventFilter{ConfigType: "startup"}, repositories.Page{Limit: 10}, []string{"event-c"}, 1},
		{"authorization", repositories.ChangeEventFilter{AuthorizationStatus: domain.AuthorizationStatusAuthorized}, repositories.Page{Limit: 10}, []string{"event-c"}, 1},
		{"policy", repositories.ChangeEventFilter{PolicyStatus: domain.PolicyStatusNotEvaluated}, repositories.Page{Limit: 10}, []string{"event-b", "event-a"}, 2},
		{"combined", repositories.ChangeEventFilter{DeviceID: devices[1].ID, ConfigType: "startup", AuthorizationStatus: domain.AuthorizationStatusAuthorized, PolicyStatus: domain.PolicyStatusViolation}, repositories.Page{Limit: 10}, []string{"event-c"}, 1},
		{"parameterized filter", repositories.ChangeEventFilter{DeviceID: "' OR 1=1 --"}, repositories.Page{Limit: 10}, []string{}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			items, total, err := s.ListChangeEvents(ctx, test.filter, test.page)
			require.NoError(t, err)
			require.Equal(t, test.total, total)
			ids := make([]string, 0, len(items))
			for _, item := range items {
				ids = append(ids, item.ID)
			}
			require.Equal(t, test.ids, ids)
		})
	}
}

func TestChangeEventDatabaseConstraints(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	from := eventBackup(devices[0].ID, "running", now)
	to := eventBackup(devices[0].ID, "running", now.Add(time.Hour))
	otherDevice := eventBackup(devices[1].ID, "running", now)
	otherType := eventBackup(devices[0].ID, "startup", now)
	for _, b := range []domain.ConfigBackup{from, to, otherDevice, otherType} {
		require.NoError(t, s.CreateBackup(ctx, b))
	}
	valid := eventBetween(from, to)
	for _, test := range []struct {
		name string
		edit func(*domain.ChangeEvent)
	}{
		{"self reference", func(e *domain.ChangeEvent) { e.FromBackupID = e.ToBackupID }},
		{"missing source", func(e *domain.ChangeEvent) { e.FromBackupID = uuid.NewString() }},
		{"missing target", func(e *domain.ChangeEvent) { e.ToBackupID = uuid.NewString() }},
		{"missing device", func(e *domain.ChangeEvent) { e.DeviceID = uuid.NewString() }},
		{"wrong event device", func(e *domain.ChangeEvent) { e.DeviceID = devices[1].ID }},
		{"wrong event type", func(e *domain.ChangeEvent) { e.ConfigType = "startup" }},
		{"source device mismatch", func(e *domain.ChangeEvent) { e.FromBackupID = otherDevice.ID }},
		{"target device mismatch", func(e *domain.ChangeEvent) { e.ToBackupID = otherDevice.ID }},
		{"source type mismatch", func(e *domain.ChangeEvent) { e.FromBackupID = otherType.ID }},
		{"target type mismatch", func(e *domain.ChangeEvent) { e.ToBackupID = otherType.ID }},
		{"invalid type", func(e *domain.ChangeEvent) { e.ConfigType = "other" }},
		{"negative added", func(e *domain.ChangeEvent) { e.AddedLines = -1 }},
		{"negative removed", func(e *domain.ChangeEvent) { e.RemovedLines = -1 }},
		{"invalid authorization", func(e *domain.ChangeEvent) { e.AuthorizationStatus = "unknown" }},
		{"invalid policy", func(e *domain.ChangeEvent) { e.PolicyStatus = "unknown" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := valid
			test.edit(&e)
			tx, err := s.db.BeginTx(ctx, nil)
			require.NoError(t, err)
			require.Error(t, insertChangeEvent(ctx, tx, e))
			require.NoError(t, tx.Rollback())
		})
	}
	// Omitting statistics/status columns exercises the schema's default values.
	_, err := s.db.ExecContext(ctx, `INSERT INTO change_events
        (id, device_id, config_type, from_backup_id, to_backup_id, detected_at, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, valid.ID, valid.DeviceID, valid.ConfigType,
		valid.FromBackupID, valid.ToBackupID, timestamp(now), timestamp(now), timestamp(now))
	require.NoError(t, err)
	loaded, err := s.GetChangeEvent(ctx, valid.ID)
	require.NoError(t, err)
	require.Zero(t, loaded.AddedLines)
	require.Zero(t, loaded.RemovedLines)
	require.Equal(t, domain.AuthorizationStatusUnaccounted, loaded.AuthorizationStatus)
	require.Equal(t, domain.PolicyStatusNotEvaluated, loaded.PolicyStatus)
	duplicate := valid
	duplicate.ID = uuid.NewString()
	tx, err := s.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.ErrorIs(t, insertChangeEvent(ctx, tx, duplicate), domain.ErrConflict)
	require.NoError(t, tx.Rollback())
	_, err = s.db.ExecContext(ctx, `DELETE FROM config_backups WHERE id = ?`, from.ID)
	require.Error(t, err)
	_, err = s.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, valid.DeviceID)
	require.Error(t, err)
}

func TestBackupChangeEventTransactionRollsBack(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	from := eventBackup(devices[0].ID, "running", now)
	require.NoError(t, s.CreateBackup(ctx, from))
	to := eventBackup(devices[0].ID, "running", now.Add(time.Hour))
	event := eventBetween(from, to)
	event.PolicyStatus = "invalid"
	require.Error(t, s.CreateBackupWithChangeEvent(ctx, to, &event))
	_, err := s.GetBackup(ctx, to.DeviceID, to.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = s.GetChangeEvent(ctx, event.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	// A subsequent valid transaction proves rollback released the connection.
	event.PolicyStatus = domain.PolicyStatusNotEvaluated
	require.NoError(t, s.CreateBackupWithChangeEvent(ctx, to, &event))
	_, err = s.GetBackup(ctx, to.DeviceID, to.ID)
	require.NoError(t, err)
	_, err = s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
}
