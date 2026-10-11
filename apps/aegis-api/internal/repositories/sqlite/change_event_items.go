package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

var _ repositories.ChangeEventItemRepository = (*Store)(nil)

const changeEventItemColumns = `id, change_event_id, ordinal, operation,
    context_path_json, command, sensitive, created_at,
    classification_version, category, subcategory, classification_rule, classified_at`

func insertChangeEventItem(ctx context.Context, tx *sql.Tx, item domain.ChangeEventItem) error {
	path := item.ContextPath
	if path == nil {
		path = []string{}
	}
	encoded, err := json.Marshal(path)
	if err != nil {
		return domain.ErrValidation
	}
	var classifiedAt any
	if item.ClassifiedAt != nil {
		classifiedAt = timestamp(*item.ClassifiedAt)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO change_event_items (`+changeEventItemColumns+`)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.ChangeEventID, item.Ordinal,
		item.Operation, string(encoded), item.Command, item.Sensitive, timestamp(item.CreatedAt),
		item.ClassificationVersion, item.Category, item.Subcategory, item.ClassificationRule, classifiedAt)
	return repositoryError(mapWriteError(err))
}

func (s *Store) ListChangeEventItems(ctx context.Context, eventID string, page repositories.Page) ([]domain.ChangeEventItem, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_event_items WHERE change_event_id = ?`, eventID).Scan(&total); err != nil {
		return nil, 0, repositoryError(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+changeEventItemColumns+` FROM change_event_items
        WHERE change_event_id = ? ORDER BY ordinal ASC LIMIT ? OFFSET ?`, eventID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, repositoryError(err)
	}
	defer rows.Close()
	items := make([]domain.ChangeEventItem, 0)
	for rows.Next() {
		var item domain.ChangeEventItem
		var encoded, createdAt string
		var classifiedAt sql.NullString
		if err := rows.Scan(&item.ID, &item.ChangeEventID, &item.Ordinal, &item.Operation,
			&encoded, &item.Command, &item.Sensitive, &createdAt, &item.ClassificationVersion,
			&item.Category, &item.Subcategory, &item.ClassificationRule, &classifiedAt); err != nil {
			return nil, 0, repositoryError(err)
		}
		if err := json.Unmarshal([]byte(encoded), &item.ContextPath); err != nil {
			return nil, 0, domain.ErrValidation
		}
		if item.ContextPath == nil {
			item.ContextPath = []string{}
		}
		if item.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, 0, err
		}
		if item.ClassifiedAt, err = parseNullableTime(classifiedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, repositoryError(rows.Err())
}
