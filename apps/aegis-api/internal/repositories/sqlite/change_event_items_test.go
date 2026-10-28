package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func analysisFixture(t *testing.T, s *Store, deviceID string) (domain.ConfigBackup, domain.ChangeEvent, []domain.ChangeEventItem) {
	t.Helper()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	from := eventBackup(deviceID, "running", now)
	require.NoError(t, s.CreateBackup(context.Background(), from))
	to := eventBackup(deviceID, "running", now.Add(time.Hour))
	event := eventBetween(from, to)
	event.AnalysisVersion, event.AnalyzedAt, event.ItemCount = domain.ChangeAnalysisVersion, &to.CreatedAt, 2
	items := []domain.ChangeEventItem{
		{ID: "item-z", ChangeEventID: event.ID, Ordinal: 0, Operation: domain.ChangeOperationRemove, ContextPath: []string{"interface \"A\"", "sub\\mode"}, Command: "description old", CreatedAt: to.CreatedAt},
		{ID: "item-a", ChangeEventID: event.ID, Ordinal: 1, Operation: domain.ChangeOperationAdd, ContextPath: []string{}, Command: "enable secret <redacted>", Sensitive: true, CreatedAt: to.CreatedAt},
	}
	for i := range items {
		items[i].Category, items[i].Subcategory = domain.ChangeCategoryUnclassified, "unclassified"
	}
	return to, event, items
}

func TestChangeEventItemsMigrationPreservesUnanalyzedEvents(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	for _, name := range []string{"001_initial.sql", "002_seed_rbac.sql", "003_change_events.sql"} {
		body, err := migrations.Files.ReadFile(name)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(body))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?, ?)`, name, timestamp(time.Now()))
		require.NoError(t, err)
	}
	devices := changeEventDevices(t, s)
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	from := eventBackup(devices[0].ID, "running", now)
	to := eventBackup(devices[0].ID, "running", now.Add(time.Hour))
	require.NoError(t, s.CreateBackup(ctx, from))
	require.NoError(t, s.CreateBackup(ctx, to))
	event := eventBetween(from, to)
	_, err = db.ExecContext(ctx, `INSERT INTO change_events
        (id, device_id, config_type, from_backup_id, to_backup_id, detected_at,
         added_lines, removed_lines, authorization_status, policy_status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.DeviceID, event.ConfigType,
		event.FromBackupID, event.ToBackupID, timestamp(event.DetectedAt), event.AddedLines,
		event.RemovedLines, event.AuthorizationStatus, event.PolicyStatus, timestamp(event.CreatedAt), timestamp(event.UpdatedAt))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	s, err = Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.Migrate(ctx))
	loaded, err := s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, event, loaded)
	require.Zero(t, loaded.AnalysisVersion)
	require.Nil(t, loaded.AnalyzedAt)
	require.Zero(t, loaded.ItemCount)
	items, total, err := s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total)
	backup, err := s.GetBackup(ctx, to.DeviceID, to.ID)
	require.NoError(t, err)
	require.Equal(t, to, backup)
	assertApprovedTaskSixMigrations(t, s.db)
}

func TestChangeEventItemsRepositoryPagingAndMetadata(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	to, event, items := analysisFixture(t, s, devices[0].ID)
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, to, &event, items))
	loaded, err := s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, event, loaded)
	for _, test := range []struct {
		name  string
		id    string
		page  repositories.Page
		want  []domain.ChangeEventItem
		total int
	}{
		{"ordinal not ID order", event.ID, repositories.Page{Limit: 10}, items, 2},
		{"page", event.ID, repositories.Page{Limit: 1, Offset: 1}, items[1:], 2},
		{"past end", event.ID, repositories.Page{Limit: 1, Offset: 9}, []domain.ChangeEventItem{}, 2},
		{"empty limit", event.ID, repositories.Page{Limit: 0}, []domain.ChangeEventItem{}, 2},
		{"absent event", "absent", repositories.Page{Limit: 10}, []domain.ChangeEventItem{}, 0},
		{"parameterized ID", "' OR 1=1 --", repositories.Page{Limit: 10}, []domain.ChangeEventItem{}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, total, err := s.ListChangeEventItems(ctx, test.id, test.page)
			require.NoError(t, err)
			require.Equal(t, test.total, total)
			require.Equal(t, test.want, got)
		})
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM change_events WHERE id = ?`, event.ID)
	require.NoError(t, err)
	_, total, err := s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total) // Foreign-key cascade removes the child facts.
}

func TestChangeEventItemDatabaseConstraints(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	to, event, items := analysisFixture(t, s, devices[0].ID)
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, to, &event, items))
	for _, test := range []struct {
		name               string
		eventID, op, text  string
		ordinal, sensitive int
	}{
		{"foreign key", "absent", "add", "safe", 2, 0},
		{"duplicate ordinal", event.ID, "add", "safe", 0, 0},
		{"negative ordinal", event.ID, "add", "safe", -1, 0},
		{"invalid operation", event.ID, "replace", "safe", 2, 0},
		{"empty command", event.ID, "add", "", 2, 0},
		{"invalid sensitivity", event.ID, "add", "safe", 2, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := s.db.ExecContext(ctx, `INSERT INTO change_event_items
                (id, change_event_id, ordinal, operation, command, sensitive, created_at)
                VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), test.eventID, test.ordinal, test.op, test.text, test.sensitive, timestamp(to.CreatedAt))
			require.Error(t, err)
		})
	}
	for _, column := range []string{"analysis_version", "item_count"} {
		_, err := s.db.ExecContext(ctx, `UPDATE change_events SET `+column+` = -1 WHERE id = ?`, event.ID)
		require.Error(t, err)
	}
}

func TestChangeAnalysisValidationRejectsMalformedInput(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*domain.ChangeEvent, []domain.ChangeEventItem)
	}{
		{"wrong parent", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { items[0].ChangeEventID = "other" }},
		{"wrong target", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { e.ToBackupID = "other" }},
		{"count mismatch", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { e.ItemCount++ }},
		{"unanalyzed items", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { e.AnalysisVersion = 0 }},
		{"unknown version", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { e.AnalysisVersion++ }},
		{"missing analysis time", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { e.AnalyzedAt = nil }},
		{"ordinal gap", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { items[1].Ordinal = 3 }},
		{"duplicate ordinal", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { items[1].Ordinal = 0 }},
		{"negative ordinal", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { items[0].Ordinal = -1 }},
		{"operation", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { items[0].Operation = "replace" }},
		{"empty command", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { items[0].Command = "  " }},
		{"raw command secret", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) {
			items[0].Command = "password SENTINEL_REJECT_PRIVATE_VALUE"
		}},
		{"raw context secret", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) {
			items[0].ContextPath = []string{"radius server LAB", "key SENTINEL_REJECT_PRIVATE_VALUE"}
		}},
		{"missing sensitive marker", func(e *domain.ChangeEvent, items []domain.ChangeEventItem) { items[1].Sensitive = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, devices := changeEventStore(t)
			to, event, items := analysisFixture(t, s, devices[0].ID)
			test.edit(&event, items)
			err := s.CreateBackupWithChangeAnalysis(context.Background(), to, &event, items)
			require.True(t, err != nil)
			require.False(t, strings.Contains(err.Error(), "SENTINEL_REJECT_PRIVATE_VALUE"))
			require.ErrorIs(t, err, domain.ErrValidation)
			assertAnalysisAbsent(t, s, to, event.ID)
		})
	}
	s, devices := changeEventStore(t)
	to, event, items := analysisFixture(t, s, devices[0].ID)
	require.ErrorIs(t, s.CreateBackupWithChangeAnalysis(context.Background(), to, nil, items), domain.ErrValidation)
	assertAnalysisAbsent(t, s, to, event.ID)
}

func assertAnalysisAbsent(t *testing.T, s *Store, to domain.ConfigBackup, eventID string) {
	t.Helper()
	ctx := context.Background()
	_, err := s.GetBackup(ctx, to.DeviceID, to.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = s.GetChangeEvent(ctx, eventID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, total, err := s.ListChangeEventItems(ctx, eventID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total)
}

func TestChangeAnalysisLateItemFailureRollsBack(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	to, event, items := analysisFixture(t, s, devices[0].ID)
	items[1].ID = items[0].ID // Ordinals valid: failure is the second SQL insert.
	require.ErrorIs(t, s.CreateBackupWithChangeAnalysis(ctx, to, &event, items), domain.ErrConflict)
	assertAnalysisAbsent(t, s, to, event.ID)
	_, total, err := s.ListBackups(ctx, to.DeviceID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	items[1].ID = uuid.NewString()
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, to, &event, items))
	loaded, total, err := s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, items, loaded)
}

func TestChangeAnalysisZeroItemsAndLegacyCompatibility(t *testing.T) {
	for _, analyzed := range []bool{false, true} {
		s, devices := changeEventStore(t)
		to, event, _ := analysisFixture(t, s, devices[0].ID)
		event.ItemCount = 0
		if !analyzed {
			event.AnalysisVersion, event.AnalyzedAt = 0, nil
		}
		require.NoError(t, s.CreateBackupWithChangeEvent(context.Background(), to, &event))
		loaded, err := s.GetChangeEvent(context.Background(), event.ID)
		require.NoError(t, err)
		require.Equal(t, event, loaded)
	}
}
