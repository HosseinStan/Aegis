package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

func (s *Store) CreateDevice(ctx context.Context, d domain.Device) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO devices
        (id, display_name, hostname, management_ip, management_port, vendor,
         platform, protocol, credential_id, device_group, description, enabled,
         created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.DisplayName, nullable(d.Hostname), d.ManagementIP, d.ManagementPort,
		d.Vendor, d.Platform, d.Protocol, d.CredentialID, nullable(d.DeviceGroup),
		nullable(d.Description), d.Enabled, timestamp(d.CreatedAt), timestamp(d.UpdatedAt))
	return repositoryError(mapWriteError(err))
}

func (s *Store) GetDevice(ctx context.Context, id string) (domain.Device, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, display_name, hostname,
        management_ip, management_port, vendor, platform, protocol,
        credential_id, device_group, description, enabled, created_at,
        updated_at, last_success_at, last_failure_at, last_error_code
        FROM devices WHERE id = ?`, id)
	d, err := scanDevice(row.Scan)
	return d, repositoryError(err)
}

func (s *Store) ListDevices(ctx context.Context, filter repositories.DeviceFilter, page repositories.Page) ([]domain.Device, int, error) {
	conditions := []string{"1=1"}
	var args []any
	if filter.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, *filter.Enabled)
	}
	if filter.Group != "" {
		conditions = append(conditions, "device_group = ?")
		args = append(args, filter.Group)
	}
	if filter.Search != "" {
		conditions = append(conditions, "(display_name LIKE ? OR hostname LIKE ? OR management_ip LIKE ?)")
		value := "%" + filter.Search + "%"
		args = append(args, value, value, value)
	}
	query := `SELECT id, display_name, hostname, management_ip, management_port,
        vendor, platform, protocol, credential_id, device_group, description,
        enabled, created_at, updated_at, last_success_at, last_failure_at,
        last_error_code, COUNT(*) OVER() FROM devices WHERE ` + strings.Join(conditions, " AND ") + ` ORDER BY display_name LIMIT ? OFFSET ?`
	args = append(args, page.Limit, page.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	devices := make([]domain.Device, 0)
	total := 0
	for rows.Next() {
		d, err := scanDevice(func(dest ...any) error { return rows.Scan(append(dest, &total)...) })
		if err != nil {
			return nil, 0, err
		}
		devices = append(devices, d)
	}
	return devices, total, rows.Err()
}

func (s *Store) UpdateDevice(ctx context.Context, d domain.Device) error {
	result, err := s.db.ExecContext(ctx, `UPDATE devices SET display_name = ?,
        hostname = ?, management_ip = ?, management_port = ?, vendor = ?,
        platform = ?, protocol = ?, credential_id = ?, device_group = ?,
        description = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		d.DisplayName, nullable(d.Hostname), d.ManagementIP, d.ManagementPort, d.Vendor,
		d.Platform, d.Protocol, d.CredentialID, nullable(d.DeviceGroup), nullable(d.Description),
		d.Enabled, timestamp(d.UpdatedAt), d.ID)
	if err != nil {
		return repositoryError(mapWriteError(err))
	}
	return affected(result)
}

func (s *Store) DeleteDevice(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		return repositoryError(mapWriteError(err))
	}
	return affected(result)
}

func (s *Store) RecordDeviceResult(ctx context.Context, id, errorCode string) error {
	now := timestamp(time.Now())
	var result sql.Result
	var err error
	if errorCode == "" {
		result, err = s.db.ExecContext(ctx, `UPDATE devices SET last_success_at = ?, last_error_code = NULL WHERE id = ?`, now, id)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE devices SET last_failure_at = ?, last_error_code = ? WHERE id = ?`, now, errorCode, id)
	}
	if err != nil {
		return err
	}
	return affected(result)
}

type scanFunc func(...any) error

func scanDevice(scan scanFunc) (domain.Device, error) {
	var d domain.Device
	var hostname, group, description, successAt, failureAt, errorCode sql.NullString
	var createdAt, updatedAt string
	err := scan(&d.ID, &d.DisplayName, &hostname, &d.ManagementIP, &d.ManagementPort,
		&d.Vendor, &d.Platform, &d.Protocol, &d.CredentialID, &group, &description,
		&d.Enabled, &createdAt, &updatedAt, &successAt, &failureAt, &errorCode)
	if err != nil {
		return d, err
	}
	d.Hostname, d.DeviceGroup, d.Description, d.LastErrorCode = hostname.String, group.String, description.String, errorCode.String
	if d.CreatedAt, err = parseTime(createdAt); err != nil {
		return d, err
	}
	if d.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return d, err
	}
	if d.LastSuccessAt, err = parseNullableTime(successAt); err != nil {
		return d, err
	}
	if d.LastFailureAt, err = parseNullableTime(failureAt); err != nil {
		return d, err
	}
	return d, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func affected(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	return nil
}
