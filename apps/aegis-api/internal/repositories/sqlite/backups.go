package sqlite

import (
	"context"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

func (s *Store) CreateBackup(ctx context.Context, b domain.ConfigBackup) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO config_backups
        (id, device_id, config_type, storage_path, content_hash, size_bytes,
         trigger_type, triggered_by, retrieved_at, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, b.ID, b.DeviceID, b.ConfigType,
		b.StoragePath, b.ContentHash, b.SizeBytes, b.TriggerType, nullable(b.TriggeredBy),
		timestamp(b.RetrievedAt), timestamp(b.CreatedAt))
	return repositoryError(mapWriteError(err))
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
