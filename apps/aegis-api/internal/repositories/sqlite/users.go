package sqlite

import (
	"context"
	"database/sql"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

func (s *Store) CreateUser(ctx context.Context, u domain.User) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users
        (id, external_subject, username, display_name, email, password_hash,
         enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, nullable(u.ExternalSubject), u.Username, nullable(u.DisplayName), nullable(u.Email),
		u.PasswordHash, u.Enabled, timestamp(u.CreatedAt), timestamp(u.UpdatedAt))
	return repositoryError(mapWriteError(err))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT id, external_subject,
        username, display_name, email, password_hash, enabled, created_at,
        updated_at FROM users WHERE username = ?`, username))
}

func (s *Store) GetUser(ctx context.Context, id string) (domain.User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT id, external_subject,
        username, display_name, email, password_hash, enabled, created_at,
        updated_at FROM users WHERE id = ?`, id))
}

func (s *Store) scanUser(row *sql.Row) (domain.User, error) {
	var u domain.User
	var subject, displayName, email sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&u.ID, &subject, &u.Username, &displayName, &email, &u.PasswordHash,
		&u.Enabled, &createdAt, &updatedAt)
	if err != nil {
		return u, repositoryError(err)
	}
	u.ExternalSubject, u.DisplayName, u.Email = subject.String, displayName.String, email.String
	u.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return u, err
	}
	u.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return u, err
	}
	return u, nil
}

func (s *Store) GetPrincipal(ctx context.Context, userID string) (domain.Principal, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return domain.Principal{}, err
	}
	p := domain.Principal{UserID: u.ID, Username: u.Username}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT p.name FROM permissions p
        JOIN role_permissions rp ON rp.permission_id = p.id
        JOIN user_roles ur ON ur.role_id = rp.role_id WHERE ur.user_id = ? ORDER BY p.name`, userID)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			rows.Close()
			return p, err
		}
		p.Permissions = append(p.Permissions, permission)
	}
	if err := rows.Close(); err != nil {
		return p, err
	}
	groupRows, err := s.db.QueryContext(ctx, `SELECT device_group FROM user_device_groups WHERE user_id = ? ORDER BY device_group`, userID)
	if err != nil {
		return p, err
	}
	defer groupRows.Close()
	for groupRows.Next() {
		var group string
		if err := groupRows.Scan(&group); err != nil {
			return p, err
		}
		p.DeviceGroups = append(p.DeviceGroups, group)
	}
	return p, groupRows.Err()
}

func (s *Store) AssignRole(ctx context.Context, userID, roleName string) error {
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO user_roles(user_id, role_id)
        SELECT ?, id FROM roles WHERE name = ?`, userID, roleName)
	if err != nil {
		return err
	}
	return affected(result)
}

func (s *Store) BootstrapAdmin(ctx context.Context, id, username string, passwordHash []byte) error {
	_, err := s.GetUserByUsername(ctx, username)
	if err == nil {
		return nil
	}
	if err != domain.ErrNotFound {
		return err
	}
	now := time.Now().UTC()
	if err := s.CreateUser(ctx, domain.User{ID: id, Username: username, DisplayName: "Bootstrap Administrator", PasswordHash: passwordHash, Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		return err
	}
	return s.AssignRole(ctx, id, "system_administrator")
}
