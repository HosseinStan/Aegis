package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"company.local/aegis/apps/aegis-api/internal/api"
	"company.local/aegis/apps/aegis-api/internal/auth"
	"company.local/aegis/apps/aegis-api/internal/config"
	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/connectors/cisco_ios_ssh"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"company.local/aegis/apps/aegis-api/internal/security"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Aegis API stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := observability.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := sqlite.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	defer store.Close()
	if (cfg.BootstrapAdminUsername == "") != (cfg.BootstrapAdminPassword == "") {
		return errors.New("BOOTSTRAP_ADMIN_USERNAME and BOOTSTRAP_ADMIN_PASSWORD must be provided together")
	}
	if cfg.BootstrapAdminUsername != "" {
		hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
		if err != nil {
			return err
		}
		if err := store.BootstrapAdmin(ctx, uuid.NewString(), cfg.BootstrapAdminUsername, hash); err != nil {
			return err
		}
		logger.Info("bootstrap administrator ensured", "username", cfg.BootstrapAdminUsername)
	}

	credentialStore, err := credentials.NewStore(store, cfg.CredentialMasterKey)
	if err != nil {
		return err
	}
	authService, err := auth.NewService(store, cfg.JWTIssuer, cfg.JWTAudience, cfg.JWTSigningKey, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	if err != nil {
		return err
	}
	registry := connectors.NewRegistry()
	cisco, err := cisco_ios_ssh.New(cfg.KnownHostsFile, cfg.SSHConnectTimeout, cfg.SSHHandshakeTimeout, cfg.DeviceCommandTimeout, cfg.MaxOutputBytes)
	if err != nil {
		return err
	}
	if err := registry.Register("cisco", "ios", "ssh", cisco); err != nil {
		return err
	}
	if err := registry.Register("cisco", "ios-xe", "ssh", cisco); err != nil {
		return err
	}
	metrics := &observability.Metrics{}
	deviceService := services.NewDeviceService(store, credentialStore, registry, metrics)
	configService := services.NewConfigService(store, credentialStore, registry, cfg.ConfigCacheTTL, cfg.FactsCacheTTL, cfg.InterfaceCacheTTL, cfg.MaxConcurrentConnections, cfg.MaxConnectionsPerDevice, metrics)
	backupService, err := services.NewBackupService(store, configService, cfg.BackupStoragePath, cfg.MaxOutputBytes, metrics)
	if err != nil {
		return err
	}
	trustedProxies, err := security.ParseCIDRs(cfg.TrustedProxyCIDRs)
	if err != nil {
		return err
	}
	handler := &api.Handler{Auth: authService, Credentials: credentialStore, Devices: deviceService, Configs: configService, Backups: backupService, Audits: store, Health: store, Logger: logger, OpenAPIPath: cfg.OpenAPIPath, TrustedProxies: trustedProxies}
	handler.ChangeAuthorizations = services.NewChangeAuthorizationService(store, store, store, store, store)
	handler.ChangePolicies = services.NewChangePolicyService(store, store, store)
	router := api.Routes(handler, metrics, api.RouterConfig{AllowedOrigins: cfg.CORSAllowedOrigins, RequestTimeout: cfg.RequestTimeout, MetricsEnabled: cfg.MetricsEnabled, TestConsoleEnabled: cfg.LocalTestConsoleEnabled, TrustedProxyCIDRs: cfg.TrustedProxyCIDRs})
	server := &http.Server{Addr: cfg.ListenAddress, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.RequestTimeout, WriteTimeout: cfg.RequestTimeout + 5*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20, ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError)}

	errChannel := make(chan error, 1)
	go func() {
		logger.Info("Aegis API listening", "address", cfg.ListenAddress, "environment", cfg.Environment)
		errChannel <- server.ListenAndServe()
	}()
	select {
	case err := <-errChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	logger.Info("Aegis API stopped gracefully")
	return nil
}
