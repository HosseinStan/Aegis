package credentials

import (
	"bytes"
	"context"
	"testing"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/stretchr/testify/require"
)

type memoryRepository struct{ record repositories.CredentialRecord }

func (m *memoryRepository) CreateCredentialRecord(_ context.Context, record repositories.CredentialRecord) error {
	m.record = record
	return nil
}
func (m *memoryRepository) GetCredentialRecord(context.Context, string) (repositories.CredentialRecord, error) {
	return m.record, nil
}
func (m *memoryRepository) ListCredentialRecords(context.Context, repositories.Page) ([]repositories.CredentialRecord, int, error) {
	return []repositories.CredentialRecord{m.record}, 1, nil
}
func (m *memoryRepository) DeleteCredentialRecord(context.Context, string) error { return nil }

func TestStoreEncryptsAndDecryptsCredentialFields(t *testing.T) {
	repository := &memoryRepository{}
	store, err := NewStore(repository, bytes.Repeat([]byte{7}, 32))
	require.NoError(t, err)
	created, err := store.Create(context.Background(), domain.Credential{Name: "office switches", AuthType: "password", Username: "aegis", Password: "secret password", EnableSecret: "enable secret"})
	require.NoError(t, err)
	require.NotEqual(t, []byte("aegis"), repository.record.UsernameEncrypted)
	require.NotContains(t, string(repository.record.PasswordEncrypted), "secret password")
	retrieved, err := store.Get(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "aegis", retrieved.Username)
	require.Equal(t, "secret password", retrieved.Password)
	require.Equal(t, "enable secret", retrieved.EnableSecret)
}

func TestStoreRejectsInvalidCredential(t *testing.T) {
	store, err := NewStore(&memoryRepository{}, bytes.Repeat([]byte{3}, 32))
	require.NoError(t, err)
	_, err = store.Create(context.Background(), domain.Credential{Name: "missing", AuthType: "password", Username: "user"})
	require.ErrorIs(t, err, domain.ErrValidation)
}
