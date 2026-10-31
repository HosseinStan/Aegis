// inventory-demo is a development-only, loopback Inventory API.
// It deliberately does not load production config or import any SSH connector.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"company.local/aegis/apps/aegis-api/internal/api"
	"company.local/aegis/apps/aegis-api/internal/auth"
	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/connectors/mock"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
)

const demoSource = "Aegis Go API — Isolated Demo"

type bootstrap struct {
	RunParent string `json:"run_parent"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

func main() {
	if err := run(); err != nil {
		// Never include bootstrap input, credentials, tokens or database contents.
		fmt.Fprintln(os.Stderr, "Isolated Inventory demo stopped:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 1 {
		return errors.New("bootstrap is accepted only through private stdin")
	}
	var input bootstrap
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return errors.New("invalid private demo bootstrap")
	}
	if !strings.HasPrefix(input.Username, "demo-viewer-") || len(input.Username) > 80 || len(input.Password) < 32 || len(input.Password) > 64 {
		return errors.New("an ephemeral demo viewer identity is required")
	}
	if !filepath.IsAbs(input.RunParent) {
		return errors.New("run_parent must be an absolute external directory")
	}
	// Never choose a DSN, an existing database, or a production configuration.
	for dir := filepath.Clean(input.RunParent); ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return errors.New("disposable runs must be outside Git")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	if err := os.MkdirAll(input.RunParent, 0700); err != nil {
		return err
	}
	runDir, err := os.MkdirTemp(input.RunParent, "aegis-inventory-demo-")
	if err != nil {
		return err
	}
	// Retain this disposable directory on shutdown for local inspection; never clean/restore it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := sqlite.Open(ctx, filepath.Join(runDir, "inventory-demo.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	now := time.Now().UTC()
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return err
	}
	userID := uuid.NewString()
	if err := store.CreateUser(ctx, domain.User{ID: userID, Username: input.Username, DisplayName: "Isolated DEMO viewer", PasswordHash: hash, Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		return err
	}
	if err := store.AssignRole(ctx, userID, "viewer"); err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	authService, err := auth.NewService(store, "aegis-isolated-demo", "aegis-local-preview", key, 10*time.Minute, 20*time.Minute)
	if err != nil {
		return err
	}
	input.Password = "" // Signing key and all bearer tokens stay in process memory.
	registry := connectors.NewRegistry()
	if err := registry.Register("mock", "synthetic", "mock", &mock.Connector{}); err != nil {
		return err
	}

	// The existing schema requires a credential FK. This inert record has NO
	// username/password/key blobs, is not a usable credential, and is never read
	// by a connector. No credential store or device-operation service is created.
	placeholderID := uuid.NewString()
	if err := store.CreateCredentialRecord(ctx, repositories.CredentialRecord{ID: placeholderID, Name: "DEMO inert schema reference", AuthType: "mock-disabled", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}); err != nil {
		return err
	}
	seeds := []struct{ id, name, ip, group string }{
		{"core-sw-01", "CORE-SW-01", "192.0.2.10", "northstar-hq"},
		{"core-sw-02", "CORE-SW-02", "192.0.2.11", "northstar-hq"},
		{"edge-rtr-01", "EDGE-RTR-01", "198.51.100.20", "harbor-dc"},
		{"access-sw-01", "ACCESS-SW-01", "203.0.113.30", "cedar-branch"},
	}
	ranges := []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24")}
	for _, seed := range seeds {
		addr, err := netip.ParseAddr(seed.ip)
		if err != nil {
			return err
		}
		allowed := false
		for _, prefix := range ranges {
			allowed = allowed || prefix.Contains(addr)
		}
		if !allowed {
			return errors.New("demo seeds must use documentation ranges")
		}
		if err := store.CreateDevice(ctx, domain.Device{ID: seed.id, DisplayName: seed.name, Hostname: seed.name, ManagementIP: seed.ip, ManagementPort: 22, Vendor: "mock", Platform: "synthetic", Protocol: "mock", CredentialID: placeholderID, DeviceGroup: seed.group, Description: "SYNTHETIC DEMO — never a live device", Enabled: false, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	handler := &api.Handler{Auth: authService, Devices: services.NewDeviceService(store, nil, registry), Audits: store, Health: store, Logger: logger}
	router := api.Routes(handler, &observability.Metrics{}, api.RouterConfig{RequestTimeout: 10 * time.Second})
	leaf := regexp.MustCompile("^/api/v1/devices/[a-zA-Z0-9-]+$")
	restricted := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Aegis-Data-Source", "aegis-go-isolated-demo")
		w.Header().Set("X-Aegis-Demo-Mode", "mock-inventory-v1")
		w.Header().Set("Cache-Control", "no-store")
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || (host != "127.0.0.1" && host != "localhost") {
			http.Error(w, "Loopback demo only", http.StatusForbidden)
			return
		}
		allowedGet := r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/api/v1/auth/me" || r.URL.Path == "/api/v1/devices" || leaf.MatchString(r.URL.Path)
		allowedLogin := r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login"
		if !(r.Method == http.MethodGet && allowedGet) && !allowedLogin {
			http.Error(w, "This demo exposes Inventory reads only", http.StatusNotFound)
			return
		}
		router.ServeHTTP(w, r)
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: restricted, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 12 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10}
	result := map[string]any{"url": "http://" + listener.Addr().String(), "pid": os.Getpid(), "run_directory": runDir, "source": demoSource, "connector": "mock", "devices": len(seeds)}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return err
	}
	failures := make(chan error, 1)
	go func() { failures <- server.Serve(listener) }()
	select {
	case err := <-failures:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
	return nil
}
