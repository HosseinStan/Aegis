package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Environment              string
	ListenAddress            string
	PublicBaseURL            string
	DatabaseDSN              string
	DatabaseDriver           string
	AuthMode                 string
	JWTIssuer                string
	JWTAudience              string
	JWTSigningKey            []byte
	JWTAccessTTL             time.Duration
	JWTRefreshTTL            time.Duration
	CredentialMasterKey      []byte
	KnownHostsFile           string
	SSHConnectTimeout        time.Duration
	SSHHandshakeTimeout      time.Duration
	DeviceCommandTimeout     time.Duration
	RequestTimeout           time.Duration
	MaxOutputBytes           int64
	ConfigCacheTTL           time.Duration
	FactsCacheTTL            time.Duration
	InterfaceCacheTTL        time.Duration
	MaxConcurrentConnections int
	MaxConnectionsPerDevice  int
	BackupStoragePath        string
	BackupEncryptionEnabled  bool
	CORSAllowedOrigins       []string
	TrustedProxyCIDRs        []string
	LogLevel                 string
	MetricsEnabled           bool
	LocalTestConsoleEnabled  bool
	OpenAPIPath              string
	BootstrapAdminUsername   string
	BootstrapAdminPassword   string
}

func Load() (Config, error) {
	// Local development may use an uncommitted .env file. Production sets
	// APP_ENV explicitly and never loads files implicitly.
	if os.Getenv("APP_ENV") == "" {
		_ = godotenv.Load()
	}
	c := Config{
		Environment:            env("APP_ENV", "development"),
		ListenAddress:          env("HTTP_LISTEN_ADDRESS", ":8080"),
		PublicBaseURL:          env("PUBLIC_BASE_URL", "http://localhost:8080"),
		DatabaseDSN:            env("DATABASE_DSN", "./data/aegis.db"),
		DatabaseDriver:         env("DATABASE_DRIVER", "sqlite"),
		AuthMode:               env("AUTH_MODE", "local_jwt"),
		JWTIssuer:              env("JWT_ISSUER", "aegis-api"),
		JWTAudience:            env("JWT_AUDIENCE", "office-vue-app"),
		KnownHostsFile:         env("SSH_KNOWN_HOSTS_FILE", "./data/known_hosts"),
		BackupStoragePath:      env("BACKUP_STORAGE_PATH", "./data/backups"),
		CORSAllowedOrigins:     csv(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		TrustedProxyCIDRs:      csv(os.Getenv("TRUSTED_PROXY_CIDRS")),
		LogLevel:               env("LOG_LEVEL", "info"),
		OpenAPIPath:            env("OPENAPI_SPEC_PATH", "../../packages/shared-contracts/openapi/aegis-api.yaml"),
		BootstrapAdminUsername: os.Getenv("BOOTSTRAP_ADMIN_USERNAME"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}
	var err error
	if c.JWTAccessTTL, err = duration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if c.JWTRefreshTTL, err = duration("JWT_REFRESH_TOKEN_TTL", 7*24*time.Hour); err != nil {
		return Config{}, err
	}
	if c.SSHConnectTimeout, err = duration("SSH_CONNECT_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if c.SSHHandshakeTimeout, err = duration("SSH_HANDSHAKE_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if c.DeviceCommandTimeout, err = duration("DEVICE_COMMAND_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if c.RequestTimeout, err = duration("REQUEST_TIMEOUT", 45*time.Second); err != nil {
		return Config{}, err
	}
	if c.ConfigCacheTTL, err = duration("CONFIG_CACHE_TTL", 15*time.Second); err != nil {
		return Config{}, err
	}
	if c.FactsCacheTTL, err = duration("FACTS_CACHE_TTL", 5*time.Minute); err != nil {
		return Config{}, err
	}
	if c.InterfaceCacheTTL, err = duration("INTERFACE_CACHE_TTL", 30*time.Second); err != nil {
		return Config{}, err
	}
	if c.MaxOutputBytes, err = integer64("MAX_OUTPUT_BYTES", 5*1024*1024); err != nil {
		return Config{}, err
	}
	if c.MaxConcurrentConnections, err = integer("MAX_CONCURRENT_DEVICE_CONNECTIONS", 10); err != nil {
		return Config{}, err
	}
	if c.MaxConnectionsPerDevice, err = integer("MAX_CONNECTIONS_PER_DEVICE", 1); err != nil {
		return Config{}, err
	}
	if c.MetricsEnabled, err = boolean("METRICS_ENABLED", true); err != nil {
		return Config{}, err
	}
	if c.LocalTestConsoleEnabled, err = boolean("LOCAL_TEST_CONSOLE_ENABLED", c.Environment == "development"); err != nil {
		return Config{}, err
	}
	if c.BackupEncryptionEnabled, err = boolean("BACKUP_ENCRYPTION_ENABLED", false); err != nil {
		return Config{}, err
	}

	if c.JWTSigningKey, err = secret("JWT_SIGNING_KEY", 32); err != nil {
		return Config{}, err
	}
	if c.CredentialMasterKey, err = secret("CREDENTIAL_MASTER_KEY", 32); err != nil {
		return Config{}, err
	}
	if c.Environment == "production" && strings.HasPrefix(c.PublicBaseURL, "http://") {
		return Config{}, errors.New("PUBLIC_BASE_URL must use HTTPS in production")
	}
	if c.Environment == "production" && c.LocalTestConsoleEnabled {
		return Config{}, errors.New("LOCAL_TEST_CONSOLE_ENABLED must be false in production")
	}
	if c.AuthMode != "local_jwt" {
		return Config{}, fmt.Errorf("AUTH_MODE %q is not implemented; configure local_jwt", c.AuthMode)
	}
	if c.DatabaseDriver != "sqlite" {
		return Config{}, fmt.Errorf("DATABASE_DRIVER %q is not implemented; configure sqlite", c.DatabaseDriver)
	}
	// TODO SECURITY: Implement authenticated backup-file encryption before this setting can be enabled.
	if c.BackupEncryptionEnabled {
		return Config{}, errors.New("BACKUP_ENCRYPTION_ENABLED is not implemented in this milestone")
	}
	if _, err := url.ParseRequestURI(c.PublicBaseURL); err != nil {
		return Config{}, fmt.Errorf("PUBLIC_BASE_URL: %w", err)
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS contains %q: %w", cidr, err)
		}
	}
	return c, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func csv(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
func duration(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}
func integer(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}
func integer64(name string, fallback int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}
func boolean(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}
func secret(name string, expected int) ([]byte, error) {
	value := os.Getenv(name)
	if value == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil && len(decoded) == expected {
		return decoded, nil
	}
	raw := []byte(value)
	if len(raw) < expected {
		return nil, fmt.Errorf("%s must be at least %d raw bytes or exactly %d base64-decoded bytes", name, expected, expected)
	}
	return raw, nil
}
