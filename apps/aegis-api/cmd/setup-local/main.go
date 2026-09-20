package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "local setup failed:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat("go.mod"); err != nil {
		return errors.New("run this command from apps/aegis-api")
	}
	if err := os.MkdirAll(filepath.Join("data", "backups"), 0o750); err != nil {
		return err
	}
	knownHosts := filepath.Join("data", "known_hosts")
	if file, err := os.OpenFile(knownHosts, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600); err == nil {
		_ = file.Close()
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	jwtKey, err := randomBase64(32)
	if err != nil {
		return err
	}
	masterKey, err := randomBase64(32)
	if err != nil {
		return err
	}
	adminPassword, err := randomURLSafe(24)
	if err != nil {
		return err
	}
	appEnv := fmt.Sprintf(`APP_ENV=development
HTTP_LISTEN_ADDRESS=127.0.0.1:8080
PUBLIC_BASE_URL=http://127.0.0.1:8080
DATABASE_DRIVER=sqlite
DATABASE_DSN=./data/aegis.db
AUTH_MODE=local_jwt
JWT_ISSUER=aegis-api
JWT_AUDIENCE=office-vue-app
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_SIGNING_KEY=%s
CREDENTIAL_MASTER_KEY=%s
SSH_KNOWN_HOSTS_FILE=./data/known_hosts
SSH_CONNECT_TIMEOUT=5s
SSH_HANDSHAKE_TIMEOUT=10s
DEVICE_COMMAND_TIMEOUT=30s
REQUEST_TIMEOUT=45s
MAX_OUTPUT_BYTES=5242880
CONFIG_CACHE_TTL=15s
FACTS_CACHE_TTL=5m
INTERFACE_CACHE_TTL=30s
MAX_CONCURRENT_DEVICE_CONNECTIONS=10
MAX_CONNECTIONS_PER_DEVICE=1
BACKUP_STORAGE_PATH=./data/backups
BACKUP_ENCRYPTION_ENABLED=false
CORS_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
TRUSTED_PROXY_CIDRS=
LOG_LEVEL=info
METRICS_ENABLED=true
LOCAL_TEST_CONSOLE_ENABLED=true
OPENAPI_SPEC_PATH=../../packages/shared-contracts/openapi/aegis-api.yaml
BOOTSTRAP_ADMIN_USERNAME=aegis-admin
BOOTSTRAP_ADMIN_PASSWORD=%s
`, jwtKey, masterKey, adminPassword)
	if err := writeExclusive(".env", []byte(appEnv), 0o600); err != nil {
		return err
	}
	rootEnv := fmt.Sprintf(`AEGIS_HOST=localhost
AEGIS_HTTP_PORT=8081
AEGIS_HTTPS_PORT=8443
AEGIS_PUBLIC_BASE_URL=https://localhost:8443
JWT_SIGNING_KEY=%s
CREDENTIAL_MASTER_KEY=%s
CORS_ALLOWED_ORIGINS=https://localhost:8443,http://localhost:5173
BOOTSTRAP_ADMIN_USERNAME=aegis-admin
BOOTSTRAP_ADMIN_PASSWORD=%s
LOG_LEVEL=info
`, jwtKey, masterKey, adminPassword)
	rootPath := filepath.Clean(filepath.Join("..", "..", ".env"))
	if err := writeExclusive(rootPath, []byte(rootEnv), 0o600); err != nil {
		return err
	}
	fmt.Println("Local runtime configuration created without displaying secrets.")
	fmt.Println("Administrator username: aegis-admin")
	fmt.Println("Administrator password: apps/aegis-api/.env -> BOOTSTRAP_ADMIN_PASSWORD")
	return nil
}

func randomBase64(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}
func randomURLSafe(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func writeExclusive(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists; refusing to overwrite local secrets", path)
	}
	if err != nil {
		return err
	}
	if _, err = file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
