package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
)

type Store struct {
	repository repositories.CredentialRepository
	aead       cipher.AEAD
}

// TODO SECURITY: Add an atomic re-encryption workflow before supporting online master-key rotation.

func NewStore(repository repositories.CredentialRepository, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("credential master key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Store{repository: repository, aead: aead}, nil
}

func (s *Store) Create(ctx context.Context, credential domain.Credential) (domain.Credential, error) {
	credential.Name = strings.TrimSpace(credential.Name)
	credential.Username = strings.TrimSpace(credential.Username)
	credential.AuthType = strings.ToLower(strings.TrimSpace(credential.AuthType))
	if credential.Name == "" || credential.Username == "" {
		return domain.Credential{}, fmt.Errorf("%w: name and username are required", domain.ErrValidation)
	}
	if credential.AuthType != "password" && credential.AuthType != "private_key" {
		return domain.Credential{}, fmt.Errorf("%w: auth_type must be password or private_key", domain.ErrValidation)
	}
	if credential.AuthType == "password" && credential.Password == "" {
		return domain.Credential{}, fmt.Errorf("%w: password is required", domain.ErrValidation)
	}
	if credential.AuthType == "private_key" && len(credential.PrivateKey) == 0 {
		return domain.Credential{}, fmt.Errorf("%w: private_key is required", domain.ErrValidation)
	}
	credential.ID = uuid.NewString()
	credential.CreatedAt = time.Now().UTC()
	credential.UpdatedAt = credential.CreatedAt
	record := repositories.CredentialRecord{ID: credential.ID, Name: credential.Name, AuthType: credential.AuthType, CreatedAt: credential.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: credential.UpdatedAt.Format(time.RFC3339Nano)}
	var err error
	if record.UsernameEncrypted, err = s.encrypt(credential.ID+":username", []byte(credential.Username)); err != nil {
		return domain.Credential{}, err
	}
	if record.PasswordEncrypted, err = s.encryptOptional(credential.ID+":password", []byte(credential.Password)); err != nil {
		return domain.Credential{}, err
	}
	if record.PrivateKeyEncrypted, err = s.encryptOptional(credential.ID+":private_key", credential.PrivateKey); err != nil {
		return domain.Credential{}, err
	}
	if record.EnableSecretEncrypted, err = s.encryptOptional(credential.ID+":enable_secret", []byte(credential.EnableSecret)); err != nil {
		return domain.Credential{}, err
	}
	if err := s.repository.CreateCredentialRecord(ctx, record); err != nil {
		return domain.Credential{}, err
	}
	return credential, nil
}

func (s *Store) Get(ctx context.Context, id string) (domain.Credential, error) {
	r, err := s.repository.GetCredentialRecord(ctx, id)
	if err != nil {
		return domain.Credential{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return domain.Credential{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	if err != nil {
		return domain.Credential{}, err
	}
	c := domain.Credential{ID: r.ID, Name: r.Name, AuthType: r.AuthType, CreatedAt: createdAt, UpdatedAt: updatedAt}
	var raw []byte
	if raw, err = s.decrypt(r.ID+":username", r.UsernameEncrypted); err != nil {
		return domain.Credential{}, err
	}
	c.Username = string(raw)
	if raw, err = s.decryptOptional(r.ID+":password", r.PasswordEncrypted); err != nil {
		return domain.Credential{}, err
	}
	c.Password = string(raw)
	if c.PrivateKey, err = s.decryptOptional(r.ID+":private_key", r.PrivateKeyEncrypted); err != nil {
		return domain.Credential{}, err
	}
	if raw, err = s.decryptOptional(r.ID+":enable_secret", r.EnableSecretEncrypted); err != nil {
		return domain.Credential{}, err
	}
	c.EnableSecret = string(raw)
	return c, nil
}

func (s *Store) List(ctx context.Context, page repositories.Page) ([]domain.Credential, int, error) {
	records, total, err := s.repository.ListCredentialRecords(ctx, page)
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.Credential, 0, len(records))
	for _, r := range records {
		createdAt, _ := time.Parse(time.RFC3339Nano, r.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339Nano, r.UpdatedAt)
		result = append(result, domain.Credential{ID: r.ID, Name: r.Name, AuthType: r.AuthType, CreatedAt: createdAt, UpdatedAt: updatedAt})
	}
	return result, total, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	return s.repository.DeleteCredentialRecord(ctx, id)
}

func (s *Store) encryptOptional(aad string, plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	return s.encrypt(aad, plaintext)
}
func (s *Store) decryptOptional(aad string, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	return s.decrypt(aad, ciphertext)
}

func (s *Store) encrypt(aad string, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	return s.aead.Seal(nonce, nonce, plaintext, []byte(aad)), nil
}

func (s *Store) decrypt(aad string, value []byte) ([]byte, error) {
	if len(value) < s.aead.NonceSize() {
		return nil, errors.New("encrypted credential is truncated")
	}
	nonce, ciphertext := value[:s.aead.NonceSize()], value[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return nil, errors.New("credential decryption failed")
	}
	return plaintext, nil
}
