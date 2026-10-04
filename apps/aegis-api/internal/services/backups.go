package services

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
	"github.com/pmezard/go-difflib/difflib"
)

type BackupService struct {
	repository repositories.BackupRepository
	configs    *ConfigService
	root       string
	maxBytes   int64
	metrics    *observability.Metrics
	locksMu    sync.Mutex
	locks      map[string]*sync.Mutex
}

type BackupResult struct {
	Backup  domain.ConfigBackup `json:"backup"`
	Created bool                `json:"created"`
}
type DiffResult struct {
	LeftBackupID  string `json:"left_backup_id"`
	RightBackupID string `json:"right_backup_id"`
	Changed       bool   `json:"changed"`
	AddedLines    int    `json:"added_lines"`
	RemovedLines  int    `json:"removed_lines"`
	Diff          string `json:"diff"`
}

func NewBackupService(repository repositories.BackupRepository, configs *ConfigService, root string, maxBytes int64, metricSet ...*observability.Metrics) (*BackupService, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("create backup root: %w", err)
	}
	var metrics *observability.Metrics
	if len(metricSet) > 0 {
		metrics = metricSet[0]
	}
	return &BackupService{repository: repository, configs: configs, root: absolute, maxBytes: maxBytes, metrics: metrics, locks: make(map[string]*sync.Mutex)}, nil
}

func (s *BackupService) Create(ctx context.Context, principal domain.Principal, deviceID, configType string) (BackupResult, error) {
	lock := s.backupLock(deviceID + ":" + configType)
	lock.Lock()
	defer lock.Unlock()
	if s.metrics != nil {
		s.metrics.BackupJobs.Add(1)
	}
	result, err := s.configs.GetConfig(ctx, principal, deviceID, configType, true)
	if err != nil {
		if s.metrics != nil {
			s.metrics.BackupFailures.Add(1)
		}
		return BackupResult{}, err
	}
	latest, err := s.repository.LatestBackup(ctx, deviceID, configType)
	hasPrevious := err == nil
	if err == nil && latest.ContentHash == result.ContentHash {
		return BackupResult{Backup: latest, Created: false}, nil
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return BackupResult{}, err
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	var event *domain.ChangeEvent
	if hasPrevious {
		_, previous, err := s.Content(ctx, deviceID, latest.ID)
		if err != nil {
			return BackupResult{}, err
		}
		diff, err := configDiff(previous, result.Config, latest.ID, id)
		if err != nil {
			return BackupResult{}, err
		}
		event = &domain.ChangeEvent{
			ID: uuid.NewString(), DeviceID: deviceID, ConfigType: configType,
			FromBackupID: latest.ID, ToBackupID: id, DetectedAt: now,
			AddedLines: diff.AddedLines, RemovedLines: diff.RemovedLines,
			AuthorizationStatus: domain.AuthorizationStatusUnaccounted,
			PolicyStatus:        domain.PolicyStatusNotEvaluated, CreatedAt: now, UpdatedAt: now,
		}
	}
	relative := filepath.Join(deviceID, now.Format("2006"), now.Format("01"), now.Format("20060102T150405.000000000Z")+"_"+configType+"_"+id+".txt")
	path, err := s.safePath(relative)
	if err != nil {
		return BackupResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return BackupResult{}, fmt.Errorf("create backup directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return BackupResult{}, fmt.Errorf("create backup file: %w", err)
	}
	_, writeErr := io.WriteString(file, result.Config)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return BackupResult{}, writeErr
		}
		return BackupResult{}, closeErr
	}
	backup := domain.ConfigBackup{ID: id, DeviceID: deviceID, ConfigType: configType, StoragePath: relative, ContentHash: result.ContentHash, SizeBytes: int64(len(result.Config)), TriggerType: "manual", TriggeredBy: principal.UserID, RetrievedAt: result.RetrievedAt, CreatedAt: now}
	if err := s.repository.CreateBackupWithChangeEvent(ctx, backup, event); err != nil {
		_ = os.Remove(path)
		return BackupResult{}, err
	}
	return BackupResult{Backup: backup, Created: true}, nil
}

func (s *BackupService) backupLock(key string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	lock := s.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.locks[key] = lock
	}
	return lock
}

func (s *BackupService) List(ctx context.Context, deviceID string, page repositories.Page) ([]domain.ConfigBackup, int, error) {
	return s.repository.ListBackups(ctx, deviceID, page)
}
func (s *BackupService) Get(ctx context.Context, deviceID, id string) (domain.ConfigBackup, error) {
	backup, err := s.repository.GetBackup(ctx, deviceID, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ConfigBackup{}, domain.ErrBackupNotFound
	}
	return backup, err
}

func (s *BackupService) Content(ctx context.Context, deviceID, id string) (domain.ConfigBackup, string, error) {
	backup, err := s.repository.GetBackup(ctx, deviceID, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ConfigBackup{}, "", domain.ErrBackupNotFound
		}
		return domain.ConfigBackup{}, "", err
	}
	path, err := s.safePath(backup.StoragePath)
	if err != nil {
		return domain.ConfigBackup{}, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return domain.ConfigBackup{}, "", fmt.Errorf("read backup: %w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, s.maxBytes+1))
	if err != nil {
		return domain.ConfigBackup{}, "", err
	}
	if int64(len(raw)) > s.maxBytes {
		return domain.ConfigBackup{}, "", domain.ErrOutputTooLarge
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	if hash != backup.ContentHash {
		return domain.ConfigBackup{}, "", errors.New("backup content integrity check failed")
	}
	return backup, string(raw), nil
}

func (s *BackupService) Diff(ctx context.Context, deviceID, leftID, rightID string) (DiffResult, error) {
	_, left, err := s.Content(ctx, deviceID, leftID)
	if err != nil {
		return DiffResult{}, err
	}
	_, right, err := s.Content(ctx, deviceID, rightID)
	if err != nil {
		return DiffResult{}, err
	}
	return configDiff(left, right, leftID, rightID)
}

func configDiff(left, right, leftID, rightID string) (DiffResult, error) {
	text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: difflib.SplitLines(left), B: difflib.SplitLines(right), FromFile: leftID, ToFile: rightID, Context: 3})
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{LeftBackupID: leftID, RightBackupID: rightID, Changed: left != right, Diff: text}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			result.AddedLines++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			result.RemovedLines++
		}
	}
	return result, nil
}

func (s *BackupService) safePath(relative string) (string, error) {
	candidate := filepath.Join(s.root, filepath.Clean(relative))
	rel, err := filepath.Rel(s.root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid backup storage path")
	}
	return candidate, nil
}
