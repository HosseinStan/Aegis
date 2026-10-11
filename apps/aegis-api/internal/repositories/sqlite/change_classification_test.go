package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/migrations"
	"github.com/stretchr/testify/require"
)

func classifiedAnalysisFixture(t *testing.T, s *Store, deviceID string) (domain.ConfigBackup, domain.ChangeEvent, []domain.ChangeEventItem) {
	t.Helper()
	to, event, items := analysisFixture(t, s, deviceID)
	for i := range items {
		result := changes.Classify(items[i].ContextPath, items[i].Command)
		items[i].ClassificationVersion = domain.ChangeClassificationVersion
		items[i].Category, items[i].Subcategory, items[i].ClassificationRule = result.Category, result.Subcategory, result.RuleID
		items[i].ClassifiedAt = &to.CreatedAt
	}
	return to, event, items
}

func TestClassificationMigrationPreservesTaskTwoItemsWithoutBackfill(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "task-two.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	for _, name := range []string{"001_initial.sql", "002_seed_rbac.sql", "003_change_events.sql", "004_change_event_items.sql"} {
		body, err := migrations.Files.ReadFile(name)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(body))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?, ?)`, name, timestamp(time.Now()))
		require.NoError(t, err)
	}
	devices := changeEventDevices(t, s)
	to, event, legacy := analysisFixture(t, s, devices[0].ID)
	require.NoError(t, s.CreateBackup(ctx, to))
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, insertChangeEvent(ctx, tx, event))
	for _, item := range legacy {
		encoded, err := json.Marshal(item.ContextPath)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO change_event_items
            (id, change_event_id, ordinal, operation, context_path_json, command, sensitive, created_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.ChangeEventID, item.Ordinal, item.Operation,
			string(encoded), item.Command, item.Sensitive, timestamp(item.CreatedAt))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	var oldRoot, oldIndexes int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT rootpage FROM sqlite_master WHERE name = 'change_event_items'`).Scan(&oldRoot))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = 'change_event_items'`).Scan(&oldIndexes))
	require.NoError(t, s.Close())

	s, err = Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.Migrate(ctx)) // Schema-only, idempotent, no reclassification.
	loaded, total, err := s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, legacy, loaded)
	for _, item := range loaded {
		require.Zero(t, item.ClassificationVersion)
		require.Equal(t, "unclassified", item.Category)
		require.Equal(t, "unclassified", item.Subcategory)
		require.Empty(t, item.ClassificationRule)
		require.Nil(t, item.ClassifiedAt)
	}
	loadedEvent, err := s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, event, loadedEvent)
	loadedBackup, err := s.GetBackup(ctx, to.DeviceID, to.ID)
	require.NoError(t, err)
	require.Equal(t, to, loadedBackup)
	var root, indexes, count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT rootpage FROM sqlite_master WHERE name = 'change_event_items'`).Scan(&root))
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = 'change_event_items'`).Scan(&indexes))
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	require.Equal(t, oldRoot, root)       // No table rebuild.
	require.Equal(t, oldIndexes, indexes) // No speculative indexes.
	require.Equal(t, 5, count)
}

func TestClassifiedItemsRoundTripOnFreshDatabase(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	to, event, items := classifiedAnalysisFixture(t, s, devices[0].ID)
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, to, &event, items))
	loaded, total, err := s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, items, loaded)
	for _, item := range loaded {
		require.Equal(t, domain.ChangeClassificationVersion, item.ClassificationVersion)
		require.NotNil(t, item.ClassifiedAt)
		require.Equal(t, time.UTC, item.ClassifiedAt.Location())
	}
	page, total, err := s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, items[1:], page)
	page, total, err = s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 1, Offset: 10})
	require.NoError(t, err)
	require.Empty(t, page)
	require.Equal(t, 2, total)
	var migrations int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrations))
	require.Equal(t, 5, migrations)
}

func TestClassificationValidationRejectsSpoofedOrNonLegacyMetadata(t *testing.T) {
	for _, test := range []struct {
		name   string
		legacy bool
		edit   func(*domain.ChangeEventItem)
	}{
		{"missing time", false, func(i *domain.ChangeEventItem) { i.ClassifiedAt = nil }},
		{"wrong category", false, func(i *domain.ChangeEventItem) { i.Category = "switching" }},
		{"wrong subcategory", false, func(i *domain.ChangeEventItem) { i.Subcategory = "generic" }},
		{"wrong rule", false, func(i *domain.ChangeEventItem) { i.ClassificationRule = "spoofed.rule" }},
		{"unknown version", false, func(i *domain.ChangeEventItem) { i.ClassificationVersion = 2 }},
		{"negative version", false, func(i *domain.ChangeEventItem) { i.ClassificationVersion = -1 }},
		{"version one cannot be unclassified", false, func(i *domain.ChangeEventItem) { i.Category = "unclassified" }},
		{"changed command", false, func(i *domain.ChangeEventItem) { i.Command = "switchport access vlan 20" }},
		{"changed context", false, func(i *domain.ChangeEventItem) { i.ContextPath = []string{"router bgp 65000"} }},
		{"legacy wrong category", true, func(i *domain.ChangeEventItem) { i.Category = "interface" }},
		{"legacy wrong subcategory", true, func(i *domain.ChangeEventItem) { i.Subcategory = "description" }},
		{"legacy rule supplied", true, func(i *domain.ChangeEventItem) { i.ClassificationRule = "interface.description" }},
		{"legacy time supplied", true, func(i *domain.ChangeEventItem) { i.ClassifiedAt = &i.CreatedAt }},
		{"legacy empty category", true, func(i *domain.ChangeEventItem) { i.Category = "" }},
		{"legacy empty subcategory", true, func(i *domain.ChangeEventItem) { i.Subcategory = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, devices := changeEventStore(t)
			var to domain.ConfigBackup
			var event domain.ChangeEvent
			var items []domain.ChangeEventItem
			if test.legacy {
				to, event, items = analysisFixture(t, s, devices[0].ID)
			} else {
				to, event, items = classifiedAnalysisFixture(t, s, devices[0].ID)
			}
			test.edit(&items[0])
			before := items[0]
			require.ErrorIs(t, s.CreateBackupWithChangeAnalysis(context.Background(), to, &event, items), domain.ErrValidation)
			require.Equal(t, before, items[0]) // Invalid metadata is not silently repaired.
			assertAnalysisAbsent(t, s, to, event.ID)
		})
	}
}

func TestValidLegacyClassificationMetadataRemainsSupported(t *testing.T) {
	s, devices := changeEventStore(t)
	to, event, items := analysisFixture(t, s, devices[0].ID)
	require.NoError(t, s.CreateBackupWithChangeAnalysis(context.Background(), to, &event, items))
	loaded, _, err := s.ListChangeEventItems(context.Background(), event.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, items, loaded)
}

func TestClassificationDatabaseConstraints(t *testing.T) {
	s, devices := changeEventStore(t)
	ctx := context.Background()
	to, event, items := classifiedAnalysisFixture(t, s, devices[0].ID)
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, to, &event, items))
	for _, test := range []struct {
		column string
		value  any
	}{
		{"classification_version", -1}, {"classification_version", nil},
		{"category", ""}, {"category", nil}, {"subcategory", ""}, {"subcategory", nil},
		{"classification_rule", nil},
	} {
		_, err := s.db.ExecContext(ctx, `UPDATE change_event_items SET `+test.column+` = ? WHERE id = ?`, test.value, items[0].ID)
		require.Error(t, err)
	}
}
