package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

var _ repositories.ChangeEventRepository = (*Store)(nil)

const changeEventColumns = `id, device_id, config_type, from_backup_id,
    to_backup_id, detected_at, added_lines, removed_lines,
    authorization_status, policy_status, created_at, updated_at,
    analysis_version, analyzed_at, item_count`

func insertChangeEvent(ctx context.Context, tx *sql.Tx, e domain.ChangeEvent) error {
	// Fixed fractional precision keeps TEXT ordering chronological even when
	// a detection lands on an exact second. parseTime accepts this RFC3339 form.
	detectedAt := e.DetectedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")
	var analyzedAt any
	if e.AnalyzedAt != nil {
		analyzedAt = timestamp(*e.AnalyzedAt)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO change_events (`+changeEventColumns+`)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.DeviceID, e.ConfigType, e.FromBackupID, e.ToBackupID,
		detectedAt, e.AddedLines, e.RemovedLines,
		e.AuthorizationStatus, e.PolicyStatus, timestamp(e.CreatedAt), timestamp(e.UpdatedAt),
		e.AnalysisVersion, analyzedAt, e.ItemCount)
	return repositoryError(mapWriteError(err))
}

func (s *Store) GetChangeEvent(ctx context.Context, id string) (domain.ChangeEvent, error) {
	return scanChangeEvent(s.db.QueryRowContext(ctx,
		`SELECT `+changeEventColumns+` FROM change_events WHERE id = ?`, id).Scan)
}

func (s *Store) ListChangeEvents(ctx context.Context, filter repositories.ChangeEventFilter, page repositories.Page) ([]domain.ChangeEvent, int, error) {
	predicates := []string{"1 = 1"}
	args := []any{}
	for _, field := range []struct{ column, value string }{
		{"device_id", filter.DeviceID}, {"config_type", filter.ConfigType},
		{"authorization_status", filter.AuthorizationStatus}, {"policy_status", filter.PolicyStatus},
	} {
		if field.value != "" {
			predicates = append(predicates, field.column+" = ?")
			args = append(args, field.value)
		}
	}
	where := " WHERE " + strings.Join(predicates, " AND ")
	// Count separately so pages beyond the end still return the correct total.
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_events`+where, args...).Scan(&total); err != nil {
		return nil, 0, repositoryError(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+changeEventColumns+` FROM change_events`+where+
		` ORDER BY detected_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, repositoryError(err)
	}
	defer rows.Close()
	items := make([]domain.ChangeEvent, 0)
	for rows.Next() {
		item, err := scanChangeEvent(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, repositoryError(rows.Err())
}

func scanChangeEvent(scan scanFunc) (domain.ChangeEvent, error) {
	var e domain.ChangeEvent
	var detectedAt, createdAt, updatedAt string
	var analyzedAt sql.NullString
	err := scan(&e.ID, &e.DeviceID, &e.ConfigType, &e.FromBackupID, &e.ToBackupID,
		&detectedAt, &e.AddedLines, &e.RemovedLines, &e.AuthorizationStatus,
		&e.PolicyStatus, &createdAt, &updatedAt, &e.AnalysisVersion, &analyzedAt, &e.ItemCount)
	if err != nil {
		return e, repositoryError(err)
	}
	if e.DetectedAt, err = parseTime(detectedAt); err != nil {
		return e, err
	}
	if e.CreatedAt, err = parseTime(createdAt); err != nil {
		return e, err
	}
	if e.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return e, err
	}
	e.AnalyzedAt, err = parseNullableTime(analyzedAt)
	return e, err
}
