package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse database timestamp: %w", err)
	}
	return parsed, nil
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func repositoryError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if errors.Is(err, errConflict) {
		return domain.ErrConflict
	}
	return err
}
