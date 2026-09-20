package sqlite

import (
	"context"
	"database/sql"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

func (s *Store) CreateCredentialRecord(ctx context.Context, r repositories.CredentialRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO credentials
        (id, name, auth_type, username_encrypted, password_encrypted,
         private_key_encrypted, enable_secret_encrypted, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, r.Name, r.AuthType,
		r.UsernameEncrypted, r.PasswordEncrypted, r.PrivateKeyEncrypted,
		r.EnableSecretEncrypted, r.CreatedAt, r.UpdatedAt)
	return repositoryError(mapWriteError(err))
}

func (s *Store) GetCredentialRecord(ctx context.Context, id string) (repositories.CredentialRecord, error) {
	var r repositories.CredentialRecord
	err := s.db.QueryRowContext(ctx, `SELECT id, name, auth_type,
        username_encrypted, password_encrypted, private_key_encrypted,
        enable_secret_encrypted, created_at, updated_at FROM credentials WHERE id = ?`, id).Scan(
		&r.ID, &r.Name, &r.AuthType, &r.UsernameEncrypted, &r.PasswordEncrypted,
		&r.PrivateKeyEncrypted, &r.EnableSecretEncrypted, &r.CreatedAt, &r.UpdatedAt)
	return r, repositoryError(err)
}

func (s *Store) ListCredentialRecords(ctx context.Context, page repositories.Page) ([]repositories.CredentialRecord, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, auth_type, created_at,
        updated_at, COUNT(*) OVER() FROM credentials ORDER BY name LIMIT ? OFFSET ?`, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	records := make([]repositories.CredentialRecord, 0)
	total := 0
	for rows.Next() {
		var r repositories.CredentialRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.AuthType, &r.CreatedAt, &r.UpdatedAt, &total); err != nil {
			return nil, 0, err
		}
		records = append(records, r)
	}
	return records, total, rows.Err()
}

func (s *Store) DeleteCredentialRecord(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	if err != nil {
		if isForeignKeyError(err) {
			return domain.ErrConflict
		}
		return err
	}
	return affected(result)
}

func isForeignKeyError(err error) bool {
	return err != nil && (err == sql.ErrNoRows || containsFold(err.Error(), "foreign key"))
}
