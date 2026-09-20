package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func setRequiredSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("j", 32))))
	t.Setenv("CREDENTIAL_MASTER_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 32))))
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("AUTH_MODE", "local_jwt")
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("BACKUP_ENCRYPTION_ENABLED", "false")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
}

func TestLoadValidDevelopmentConfiguration(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	configuration, err := Load()
	require.NoError(t, err)
	require.Equal(t, "sqlite", configuration.DatabaseDriver)
	require.Len(t, configuration.CredentialMasterKey, 32)
}

func TestLoadRequiresHTTPSInProduction(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "http://aegis.internal")
	_, err := Load()
	require.EqualError(t, err, "PUBLIC_BASE_URL must use HTTPS in production")
}

func TestLoadFailsClosedForUnimplementedBackupEncryption(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("BACKUP_ENCRYPTION_ENABLED", "true")
	_, err := Load()
	require.EqualError(t, err, "BACKUP_ENCRYPTION_ENABLED is not implemented in this milestone")
}

func TestLoadRejectsTestConsoleInProduction(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "https://aegis.internal")
	t.Setenv("LOCAL_TEST_CONSOLE_ENABLED", "true")
	_, err := Load()
	require.EqualError(t, err, "LOCAL_TEST_CONSOLE_ENABLED must be false in production")
}
