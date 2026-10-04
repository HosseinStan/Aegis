package sqlite

import (
	"context"
	"slices"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

func (s *Store) CreateBackup(ctx context.Context, b domain.ConfigBackup) error {
	return s.CreateBackupWithChangeAnalysis(ctx, b, nil, nil)
}

func (s *Store) CreateBackupWithChangeEvent(ctx context.Context, b domain.ConfigBackup, event *domain.ChangeEvent) error {
	return s.CreateBackupWithChangeAnalysis(ctx, b, event, nil)
}

// CreateBackupWithChangeAnalysis commits the snapshot, event, and all items as
// one unit. Legacy callers may still persist an unanalyzed, version-zero event.
func (s *Store) CreateBackupWithChangeAnalysis(ctx context.Context, b domain.ConfigBackup, event *domain.ChangeEvent, items []domain.ChangeEventItem) error {
	if err := validateChangeAnalysis(b, event, items); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return repositoryError(err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO config_backups
        (id, device_id, config_type, storage_path, content_hash, size_bytes,
         trigger_type, triggered_by, retrieved_at, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, b.ID, b.DeviceID, b.ConfigType,
		b.StoragePath, b.ContentHash, b.SizeBytes, b.TriggerType, nullable(b.TriggeredBy),
		timestamp(b.RetrievedAt), timestamp(b.CreatedAt))
	if err != nil {
		return repositoryError(mapWriteError(err))
	}
	if event != nil {
		if err := insertChangeEvent(ctx, tx, *event); err != nil {
			return err
		}
	}
	for _, item := range items {
		if err := insertChangeEventItem(ctx, tx, item); err != nil {
			return err
		}
	}
	return repositoryError(mapWriteError(tx.Commit()))
}

func validateChangeAnalysis(b domain.ConfigBackup, event *domain.ChangeEvent, items []domain.ChangeEventItem) error {
	if event == nil {
		if len(items) != 0 {
			return domain.ErrValidation
		}
		return nil
	}
	if event.ToBackupID != b.ID || event.ItemCount != len(items) {
		return domain.ErrValidation
	}
	switch event.AnalysisVersion {
	case 0:
		if event.AnalyzedAt != nil || len(items) != 0 {
			return domain.ErrValidation
		}
	case domain.ChangeAnalysisVersion:
		if event.AnalyzedAt == nil {
			return domain.ErrValidation
		}
	default:
		return domain.ErrValidation
	}
	for ordinal, item := range items {
		if item.ChangeEventID != event.ID || item.Ordinal != ordinal ||
			(item.Operation != domain.ChangeOperationAdd && item.Operation != domain.ChangeOperationRemove) ||
			strings.TrimSpace(item.Command) == "" {
			return domain.ErrValidation
		}
		// Reject unsafe input rather than silently repairing it. This also guards
		// future callers that bypass the service's sanitizer.
		command, path, sensitive := changes.Sanitize(item.Command, item.ContextPath)
		if command != item.Command || !slices.Equal(path, item.ContextPath) || (sensitive && !item.Sensitive) {
			return domain.ErrValidation
		}
	}
	return nil
}

func (s *Store) GetBackup(ctx context.Context, deviceID, id string) (domain.ConfigBackup, error) {
	return scanBackup(s.db.QueryRowContext(ctx, `SELECT id, device_id, config_type,
        storage_path, content_hash, size_bytes, trigger_type, triggered_by,
        retrieved_at, created_at FROM config_backups WHERE device_id = ? AND id = ?`, deviceID, id).Scan)
}

func (s *Store) LatestBackup(ctx context.Context, deviceID, configType string) (domain.ConfigBackup, error) {
	return scanBackup(s.db.QueryRowContext(ctx, `SELECT id, device_id, config_type,
        storage_path, content_hash, size_bytes, trigger_type, triggered_by,
        retrieved_at, created_at FROM config_backups WHERE device_id = ? AND
        config_type = ? ORDER BY created_at DESC LIMIT 1`, deviceID, configType).Scan)
}

func (s *Store) ListBackups(ctx context.Context, deviceID string, page repositories.Page) ([]domain.ConfigBackup, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, config_type,
        storage_path, content_hash, size_bytes, trigger_type, triggered_by,
        retrieved_at, created_at, COUNT(*) OVER() FROM config_backups
        WHERE device_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, deviceID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]domain.ConfigBackup, 0)
	total := 0
	for rows.Next() {
		b, err := scanBackup(func(dest ...any) error { return rows.Scan(append(dest, &total)...) })
		if err != nil {
			return nil, 0, err
		}
		result = append(result, b)
	}
	return result, total, rows.Err()
}

func scanBackup(scan scanFunc) (domain.ConfigBackup, error) {
	var b domain.ConfigBackup
	var triggeredBy *string
	var retrievedAt, createdAt string
	err := scan(&b.ID, &b.DeviceID, &b.ConfigType, &b.StoragePath, &b.ContentHash,
		&b.SizeBytes, &b.TriggerType, &triggeredBy, &retrievedAt, &createdAt)
	if err != nil {
		return b, repositoryError(err)
	}
	if triggeredBy != nil {
		b.TriggeredBy = *triggeredBy
	}
	b.RetrievedAt, err = parseTime(retrievedAt)
	if err != nil {
		return b, err
	}
	b.CreatedAt, err = parseTime(createdAt)
	return b, err
}
