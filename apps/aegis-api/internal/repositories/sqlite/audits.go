package sqlite

import (
	"context"
	"database/sql"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

func (s *Store) CreateAudit(ctx context.Context, a domain.AuditLog) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs
        (id, request_id, user_id, username, source_ip, device_id, action,
         resource_type, resource_id, succeeded, error_code, duration_ms, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.ID, a.RequestID,
		nullable(a.UserID), nullable(a.Username), nullable(a.SourceIP), nullable(a.DeviceID),
		a.Action, nullable(a.ResourceType), nullable(a.ResourceID), a.Succeeded,
		nullable(a.ErrorCode), a.DurationMS, timestamp(a.CreatedAt))
	return err
}

func (s *Store) ListAudits(ctx context.Context, page repositories.Page) ([]domain.AuditLog, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, request_id, user_id, username,
        source_ip, device_id, action, resource_type, resource_id, succeeded,
        error_code, duration_ms, created_at, COUNT(*) OVER() FROM audit_logs
        ORDER BY created_at DESC LIMIT ? OFFSET ?`, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]domain.AuditLog, 0)
	total := 0
	for rows.Next() {
		a, err := scanAudit(func(dest ...any) error { return rows.Scan(append(dest, &total)...) })
		if err != nil {
			return nil, 0, err
		}
		result = append(result, a)
	}
	return result, total, rows.Err()
}

func (s *Store) GetAudit(ctx context.Context, id string) (domain.AuditLog, error) {
	return scanAudit(s.db.QueryRowContext(ctx, `SELECT id, request_id, user_id,
        username, source_ip, device_id, action, resource_type, resource_id,
        succeeded, error_code, duration_ms, created_at FROM audit_logs WHERE id = ?`, id).Scan)
}

func scanAudit(scan scanFunc) (domain.AuditLog, error) {
	var a domain.AuditLog
	var userID, username, sourceIP, deviceID, resourceType, resourceID, errorCode sql.NullString
	var createdAt string
	err := scan(&a.ID, &a.RequestID, &userID, &username, &sourceIP, &deviceID,
		&a.Action, &resourceType, &resourceID, &a.Succeeded, &errorCode, &a.DurationMS, &createdAt)
	if err != nil {
		return a, repositoryError(err)
	}
	a.UserID, a.Username, a.SourceIP, a.DeviceID = userID.String, username.String, sourceIP.String, deviceID.String
	a.ResourceType, a.ResourceID, a.ErrorCode = resourceType.String, resourceID.String, errorCode.String
	a.CreatedAt, err = parseTime(createdAt)
	return a, err
}
