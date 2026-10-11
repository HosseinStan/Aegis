package repositories

import (
	"context"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

type Page struct {
	Limit  int
	Offset int
}

type DeviceFilter struct {
	Enabled *bool
	Group   string
	Search  string
}

type DeviceRepository interface {
	CreateDevice(context.Context, domain.Device) error
	GetDevice(context.Context, string) (domain.Device, error)
	ListDevices(context.Context, DeviceFilter, Page) ([]domain.Device, int, error)
	UpdateDevice(context.Context, domain.Device) error
	DeleteDevice(context.Context, string) error
	RecordDeviceResult(context.Context, string, string) error
}

type CredentialRepository interface {
	CreateCredentialRecord(context.Context, CredentialRecord) error
	GetCredentialRecord(context.Context, string) (CredentialRecord, error)
	ListCredentialRecords(context.Context, Page) ([]CredentialRecord, int, error)
	DeleteCredentialRecord(context.Context, string) error
}

type CredentialRecord struct {
	ID                    string
	Name                  string
	AuthType              string
	UsernameEncrypted     []byte
	PasswordEncrypted     []byte
	PrivateKeyEncrypted   []byte
	EnableSecretEncrypted []byte
	CreatedAt             string
	UpdatedAt             string
}

type UserRepository interface {
	CreateUser(context.Context, domain.User) error
	GetUserByUsername(context.Context, string) (domain.User, error)
	GetUser(context.Context, string) (domain.User, error)
	GetPrincipal(context.Context, string) (domain.Principal, error)
	AssignRole(context.Context, string, string) error
}

type AuditRepository interface {
	CreateAudit(context.Context, domain.AuditLog) error
	ListAudits(context.Context, Page) ([]domain.AuditLog, int, error)
	GetAudit(context.Context, string) (domain.AuditLog, error)
}

type BackupRepository interface {
	CreateBackup(context.Context, domain.ConfigBackup) error
	CreateBackupWithChangeEvent(context.Context, domain.ConfigBackup, *domain.ChangeEvent) error
	CreateBackupWithChangeAnalysis(context.Context, domain.ConfigBackup, *domain.ChangeEvent, []domain.ChangeEventItem) error
	GetBackup(context.Context, string, string) (domain.ConfigBackup, error)
	LatestBackup(context.Context, string, string) (domain.ConfigBackup, error)
	ListBackups(context.Context, string, Page) ([]domain.ConfigBackup, int, error)
}

type ChangeEventFilter struct {
	DeviceID            string
	ConfigType          string
	AuthorizationStatus string
	PolicyStatus        string
}

type ChangeEventRepository interface {
	GetChangeEvent(context.Context, string) (domain.ChangeEvent, error)
	ListChangeEvents(context.Context, ChangeEventFilter, Page) ([]domain.ChangeEvent, int, error)
}

type ChangeEventItemRepository interface {
	ListChangeEventItems(context.Context, string, Page) ([]domain.ChangeEventItem, int, error)
}

type ChangeAuthorizationFilter struct {
	Status     string
	ConfigType string
}

// Writes take server-generated timestamps and the authenticated actor's ID.
// Implementations must recheck eligibility and persist history atomically.
type ChangeAuthorizationRepository interface {
	CreateChangeAuthorization(context.Context, domain.ChangeAuthorization) error
	GetChangeAuthorization(context.Context, string, string) (domain.ChangeAuthorization, error)
	ListChangeAuthorizations(context.Context, string, ChangeAuthorizationFilter, Page) ([]domain.ChangeAuthorization, int, error)
	TransitionChangeAuthorization(context.Context, string, string, string, string, time.Time) (domain.ChangeAuthorization, error)
	LinkChangeEventAuthorization(context.Context, string, domain.ChangeEventAuthorization) error
	GetChangeEventAuthorization(context.Context, string, string) (domain.ChangeEventAuthorization, error)
	ListChangeAuthorizationHistory(context.Context, string, string, Page) ([]domain.ChangeAuthorizationHistory, int, error)
}
