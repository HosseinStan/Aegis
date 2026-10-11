package services

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/connectors/mock"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func backupFixture(t *testing.T) (*BackupService, *sqlite.Store, *mock.Connector, domain.Principal, string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "aegis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	vault, err := credentials.NewStore(store, bytes.Repeat([]byte{4}, 32))
	require.NoError(t, err)
	credential, err := vault.Create(ctx, domain.Credential{Name: "lab", AuthType: "password", Username: "test", Password: "test-password"})
	require.NoError(t, err)
	now := time.Now().UTC()
	device := domain.Device{ID: uuid.NewString(), DisplayName: "Lab switch", ManagementIP: "192.0.2.10",
		ManagementPort: 22, Vendor: "cisco", Platform: "ios", Protocol: "ssh", CredentialID: credential.ID,
		Enabled: true, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, store.CreateDevice(ctx, device))
	connector := &mock.Connector{Running: "hostname LAB\ninterface Gi1/0/1\n description old\n", Startup: "hostname LAB\n"}
	registry := connectors.NewRegistry()
	require.NoError(t, registry.Register("cisco", "ios", "ssh", connector))
	configs := NewConfigService(store, vault, registry, time.Minute, time.Minute, time.Minute, 5, 1)
	service, err := NewBackupService(store, configs, t.TempDir(), 1024*1024)
	require.NoError(t, err)
	return service, store, connector, domain.Principal{UserID: uuid.NewString(), Username: "admin", Permissions: []string{"*"}}, device.ID
}

func backupFiles(t *testing.T, root string) []string {
	t.Helper()
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	require.NoError(t, err)
	return files
}

func TestBackupChangeDetectionAndAdjacentChain(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	filter := repositories.ChangeEventFilter{DeviceID: deviceID, ConfigType: "running"}
	page := repositories.Page{Limit: 10}
	first, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.True(t, first.Created)
	events, total, err := store.ListChangeEvents(ctx, filter, page)
	require.NoError(t, err)
	require.Empty(t, events)
	require.Zero(t, total)
	files := backupFiles(t, s.root)
	duplicate, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.False(t, duplicate.Created)
	require.Equal(t, first.Backup, duplicate.Backup)
	require.Equal(t, files, backupFiles(t, s.root))
	events, total, err = store.ListChangeEvents(ctx, filter, page)
	require.NoError(t, err)
	require.Empty(t, events)
	require.Zero(t, total)

	connector.SetRunning("hostname LAB\ninterface Gi1/0/1\n description new\n shutdown\n")
	second, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.True(t, second.Created)
	events, total, err = store.ListChangeEvents(ctx, filter, page)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, first.Backup.ID, event.FromBackupID)
	require.Equal(t, second.Backup.ID, event.ToBackupID)
	require.Equal(t, deviceID, event.DeviceID)
	require.Equal(t, "running", event.ConfigType)
	require.Equal(t, domain.AuthorizationStatusUnaccounted, event.AuthorizationStatus)
	require.Equal(t, domain.PolicyStatusNotEvaluated, event.PolicyStatus)
	require.Equal(t, 2, event.AddedLines)
	require.Equal(t, 1, event.RemovedLines)
	require.Equal(t, second.Backup.CreatedAt, event.DetectedAt)
	require.Equal(t, event.DetectedAt, event.CreatedAt)
	require.Equal(t, event.CreatedAt, event.UpdatedAt)
	require.Equal(t, time.UTC, event.DetectedAt.Location())
	require.Equal(t, domain.ChangeAnalysisVersion, event.AnalysisVersion)
	require.NotNil(t, event.AnalyzedAt)
	require.Equal(t, event.DetectedAt, *event.AnalyzedAt)
	items, itemTotal, err := store.ListChangeEventItems(ctx, event.ID, page)
	require.NoError(t, err)
	require.Equal(t, 3, itemTotal)
	require.Equal(t, itemTotal, event.ItemCount)
	for ordinal, item := range items {
		requireClassifiedItem(t, item)
		require.Equal(t, event.ID, item.ChangeEventID)
		require.Equal(t, ordinal, item.Ordinal)
		require.Equal(t, []string{"interface Gi1/0/1"}, item.ContextPath)
		require.Equal(t, []string{"remove", "add", "add"}[ordinal], item.Operation)
		require.Equal(t, []string{"description old", "description new", "shutdown"}[ordinal], item.Command)
		require.Equal(t, event.DetectedAt, item.CreatedAt)
		require.Equal(t, time.UTC, item.CreatedAt.Location())
		require.False(t, item.Sensitive)
		_, err := uuid.Parse(item.ID)
		require.NoError(t, err)
	}
	_, err = uuid.Parse(event.ID)
	require.NoError(t, err)
	diff, err := s.Diff(ctx, deviceID, first.Backup.ID, second.Backup.ID)
	require.NoError(t, err)
	require.True(t, diff.Changed)
	require.Equal(t, event.AddedLines, diff.AddedLines)
	require.Equal(t, event.RemovedLines, diff.RemovedLines)
	require.Equal(t, "--- "+first.Backup.ID+"\n+++ "+second.Backup.ID+
		"\n@@ -1,4 +1,5 @@\n hostname LAB\n interface Gi1/0/1\n- description old\n+ description new\n+ shutdown\n \n", diff.Diff)

	connector.SetRunning("hostname LAB\ninterface Gi1/0/1\n description third\n shutdown\n")
	third, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.True(t, third.Created)
	events, total, err = store.ListChangeEvents(ctx, filter, page)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, events, 2)
	require.Equal(t, second.Backup.ID, events[0].FromBackupID)
	require.Equal(t, third.Backup.ID, events[0].ToBackupID)
	require.Equal(t, first.Backup.ID, events[1].FromBackupID)
	require.Equal(t, second.Backup.ID, events[1].ToBackupID)
	require.Equal(t, 1, events[0].AddedLines)
	require.Equal(t, 1, events[0].RemovedLines)
	backups, backupTotal, err := store.ListBackups(ctx, deviceID, page)
	require.NoError(t, err)
	require.Equal(t, 3, backupTotal)
	require.Len(t, backups, 3)
	require.Len(t, backupFiles(t, s.root), 3)
	unchanged, err := s.Diff(ctx, deviceID, third.Backup.ID, third.Backup.ID)
	require.NoError(t, err)
	require.False(t, unchanged.Changed)
	require.Empty(t, unchanged.Diff)
	require.Zero(t, unchanged.AddedLines)
	require.Zero(t, unchanged.RemovedLines)

	// Startup forms an independent snapshot chain for the same device.
	startup, err := s.Create(ctx, principal, deviceID, "startup")
	require.NoError(t, err)
	require.True(t, startup.Created)
	connector.Startup = "hostname STARTUP\n"
	startupChanged, err := s.Create(ctx, principal, deviceID, "startup")
	require.NoError(t, err)
	require.True(t, startupChanged.Created)
	events, total, err = store.ListChangeEvents(ctx, repositories.ChangeEventFilter{ConfigType: "startup"}, page)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, events, 1)
	require.Equal(t, startup.Backup.ID, events[0].FromBackupID)
	require.Equal(t, startupChanged.Backup.ID, events[0].ToBackupID)
}

func TestBackupChangeDetectionRejectsUnreadablePreviousContent(t *testing.T) {
	for _, failure := range []string{"missing", "corrupt", "too large"} {
		t.Run(failure, func(t *testing.T) {
			s, store, connector, principal, deviceID := backupFixture(t)
			ctx := context.Background()
			first, err := s.Create(ctx, principal, deviceID, "running")
			require.NoError(t, err)
			path := filepath.Join(s.root, first.Backup.StoragePath)
			switch failure {
			case "missing":
				require.NoError(t, os.Remove(path))
			case "corrupt":
				require.NoError(t, os.WriteFile(path, []byte("corrupted\n"), 0o600))
			case "too large":
				s.maxBytes = 1
			}
			files := backupFiles(t, s.root)
			connector.SetRunning("hostname CHANGED\n")
			_, err = s.Create(ctx, principal, deviceID, "running")
			require.Error(t, err)
			require.Equal(t, files, backupFiles(t, s.root))
			backups, total, err := store.ListBackups(ctx, deviceID, repositories.Page{Limit: 10})
			require.NoError(t, err)
			require.Equal(t, 1, total)
			require.Len(t, backups, 1)
			_, total, err = store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
			require.NoError(t, err)
			require.Zero(t, total)
		})
	}
}

type invalidEventRepository struct{ repositories.BackupRepository }

func (r invalidEventRepository) CreateBackupWithChangeAnalysis(ctx context.Context, b domain.ConfigBackup, event *domain.ChangeEvent, items []domain.ChangeEventItem) error {
	invalid := *event
	invalid.PolicyStatus = "invalid"
	return r.BackupRepository.CreateBackupWithChangeAnalysis(ctx, b, &invalid, items)
}

func TestBackupChangeEventDatabaseFailureRemovesNewFile(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	_, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	files := backupFiles(t, s.root)
	s.repository = invalidEventRepository{store}
	connector.SetRunning("hostname CHANGED\n")
	_, err = s.Create(ctx, principal, deviceID, "running")
	require.Error(t, err)
	require.Equal(t, files, backupFiles(t, s.root))
	_, total, err := store.ListBackups(ctx, deviceID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	_, total, err = store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total)
}

func TestConcurrentBackupsCreateOneEvent(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	_, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	connector.SetRunning("hostname CHANGED\n")
	results := make([]BackupResult, 5)
	errors := make([]error, len(results))
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errors[i] = s.Create(ctx, principal, deviceID, "running")
		}()
	}
	wg.Wait()
	created := 0
	for i, result := range results {
		require.NoError(t, errors[i])
		if result.Created {
			created++
		}
	}
	require.Equal(t, 1, created)
	events, total, err := store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	items, itemTotal, err := store.ListChangeEventItems(ctx, events[0].ID, repositories.Page{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, 4, itemTotal)
	require.Equal(t, events[0].ItemCount, itemTotal)
	for ordinal, item := range items {
		require.Equal(t, ordinal, item.Ordinal)
		requireClassifiedItem(t, item)
	}
	require.Len(t, backupFiles(t, s.root), 2)
}

func TestBackupCommentOnlyChangeHasAnalyzedEventWithoutItems(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	connector.SetRunning("hostname LAB\n! old comment\n")
	first, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	connector.SetRunning("hostname LAB\n! new comment\n\n")
	second, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.True(t, second.Created)
	events, total, err := store.ListChangeEvents(ctx, repositories.ChangeEventFilter{DeviceID: deviceID}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	event := events[0]
	require.Equal(t, domain.ChangeAnalysisVersion, event.AnalysisVersion)
	require.NotNil(t, event.AnalyzedAt)
	require.Equal(t, time.UTC, event.AnalyzedAt.Location())
	require.Zero(t, event.ItemCount)
	items, itemTotal, err := store.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, itemTotal)
	diff, err := s.Diff(ctx, deviceID, first.Backup.ID, second.Backup.ID)
	require.NoError(t, err)
	require.True(t, diff.Changed)
	require.Positive(t, diff.AddedLines)
	require.Positive(t, diff.RemovedLines)
	require.Equal(t, diff.AddedLines, event.AddedLines)
	require.Equal(t, diff.RemovedLines, event.RemovedLines)
}

func TestBackupChangeItemsNeverPersistCommandOrContextSecrets(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	const oldSecret = "SENTINEL_SERVICE_OLD_PRIVATE_VALUE"
	const newSecret = "SENTINEL_SERVICE_NEW_PRIVATE_VALUE"
	connector.SetRunning("enable secret 9 " + oldSecret + "\nunknown password " + oldSecret + "\n child old\nradius server LAB\n key " + oldSecret + "\n")
	_, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	connector.SetRunning("enable secret 9 " + newSecret + "\nunknown password " + newSecret + "\n child new\nradius server LAB\n key " + newSecret + "\ncrypto key generate rsa modulus 2048\n")
	second, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	events, total, err := store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	event := events[0]
	items, itemTotal, err := store.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, 9, itemTotal)
	require.Equal(t, event.ItemCount, itemTotal)
	sensitive, sensitiveContext, ordinaryKey := 0, false, false
	for ordinal, item := range items {
		require.Equal(t, ordinal, item.Ordinal)
		require.Equal(t, event.ID, item.ChangeEventID)
		require.False(t, strings.Contains(item.Command, oldSecret) || strings.Contains(item.Command, newSecret))
		for _, parent := range item.ContextPath {
			require.False(t, strings.Contains(parent, oldSecret) || strings.Contains(parent, newSecret))
			if strings.Contains(parent, "<redacted>") {
				sensitiveContext = true
				require.True(t, item.Sensitive)
			}
		}
		if item.Sensitive {
			sensitive++
		}
		if item.Command == "crypto key generate rsa modulus 2048" {
			ordinaryKey = true
			require.False(t, item.Sensitive)
		}
	}
	require.Equal(t, 8, sensitive)
	require.True(t, sensitiveContext)
	require.True(t, ordinaryKey)
	// Raw secrets remain only in the existing protected snapshot, not items.
	_, content, err := s.Content(ctx, deviceID, second.Backup.ID)
	require.NoError(t, err)
	require.True(t, strings.Contains(content, newSecret))
	files := backupFiles(t, s.root)
	duplicate, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.False(t, duplicate.Created)
	require.Equal(t, files, backupFiles(t, s.root))
	_, total, err = store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	_, total, err = store.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, itemTotal, total)
}

type invalidItemRepository struct {
	repositories.BackupRepository
	backup  domain.ConfigBackup
	eventID string
}

func (r *invalidItemRepository) CreateBackupWithChangeAnalysis(ctx context.Context, b domain.ConfigBackup, event *domain.ChangeEvent, items []domain.ChangeEventItem) error {
	r.backup, r.eventID = b, event.ID
	invalid := append([]domain.ChangeEventItem{}, items...)
	invalid[1].ID = invalid[0].ID // Fail SQL after the first valid item is inserted.
	return r.BackupRepository.CreateBackupWithChangeAnalysis(ctx, b, event, invalid)
}

func TestBackupItemInsertionFailureRollsBackAndRemovesNewFile(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	first, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	files := backupFiles(t, s.root)
	failing := &invalidItemRepository{BackupRepository: store}
	s.repository = failing
	connector.SetRunning("hostname CHANGED\n")
	_, err = s.Create(ctx, principal, deviceID, "running")
	require.ErrorIs(t, err, domain.ErrConflict)
	require.Equal(t, files, backupFiles(t, s.root))
	_, err = os.Stat(filepath.Join(s.root, first.Backup.StoragePath))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(s.root, failing.backup.StoragePath))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = store.GetBackup(ctx, deviceID, first.Backup.ID)
	require.NoError(t, err)
	_, err = store.GetBackup(ctx, deviceID, failing.backup.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = store.GetChangeEvent(ctx, failing.eventID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, total, err := store.ListChangeEventItems(ctx, failing.eventID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total)
	_, total, err = store.ListBackups(ctx, deviceID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	_, total, err = store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total)
	// The previous snapshot still reads correctly, and a subsequent retry works.
	_, _, err = s.Content(ctx, deviceID, first.Backup.ID)
	require.NoError(t, err)
	s.repository = store
	result, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.True(t, result.Created)
	require.Len(t, backupFiles(t, s.root), 2)
}
