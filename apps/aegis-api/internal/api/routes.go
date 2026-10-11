package api

import (
	"net/http"
	"os"
	"time"

	appmiddleware "company.local/aegis/apps/aegis-api/internal/api/middleware"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"github.com/go-chi/chi/v5"
	"github.com/swaggest/swgui/v5emb"
)

type RouterConfig struct {
	AllowedOrigins     []string
	RequestTimeout     time.Duration
	MetricsEnabled     bool
	TestConsoleEnabled bool
	TrustedProxyCIDRs  []string
}

func Routes(handler *Handler, metrics *observability.Metrics, config RouterConfig) http.Handler {
	router := chi.NewRouter()
	router.Use(appmiddleware.RequestID)
	router.Use(appmiddleware.Recover(handler.Logger))
	router.Use(appmiddleware.SecurityHeaders)
	router.Use(appmiddleware.CORS(config.AllowedOrigins))
	router.Use(appmiddleware.NewRateLimiter(10, 30, config.TrustedProxyCIDRs...).Middleware)
	router.Use(appmiddleware.Logging(handler.Logger, metrics))
	router.Use(appmiddleware.Timeout(config.RequestTimeout))

	router.Get("/healthz", handler.Healthz)
	router.Get("/readyz", handler.Readyz)
	if config.TestConsoleEnabled {
		router.Mount("/test-console", TestConsole())
	}
	router.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/login", handler.Login)
		r.Post("/refresh", handler.Refresh)
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.Authenticate(handler.Auth))
			r.Post("/logout", handler.Logout)
			r.Get("/me", handler.Me)
			r.Post("/docs-session", handler.CreateDocsSession)
		})
	})
	router.Group(func(docs chi.Router) {
		docs.Use(appmiddleware.AuthenticateDocs(handler.Auth))
		docs.Get("/docs/openapi.yaml", handler.OpenAPI)
		docs.Mount("/docs", v5emb.New("Aegis API", "/docs/openapi.yaml", "/docs"))
	})

	router.Group(func(protected chi.Router) {
		protected.Use(appmiddleware.Authenticate(handler.Auth))
		protected.Get("/api/openapi.yaml", handler.OpenAPI)

		protected.With(appmiddleware.Require("credentials.manage")).Get("/api/v1/credentials", handler.ListCredentials)
		protected.With(appmiddleware.Require("credentials.manage")).Post("/api/v1/credentials", handler.CreateCredential)
		protected.With(appmiddleware.Require("credentials.manage")).Delete("/api/v1/credentials/{credentialID}", handler.DeleteCredential)
		protected.With(appmiddleware.Require("devices.read")).Get("/api/v1/devices", handler.ListDevices)
		protected.With(appmiddleware.Require("devices.manage")).Post("/api/v1/devices", handler.CreateDevice)
		protected.With(appmiddleware.Require("devices.read")).Get("/api/v1/devices/{deviceID}", handler.GetDevice)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("devices.manage")).Patch("/api/v1/devices/{deviceID}", handler.UpdateDevice)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("devices.manage")).Delete("/api/v1/devices/{deviceID}", handler.DeleteDevice)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("devices.manage")).Post("/api/v1/devices/{deviceID}/test-connection", handler.TestConnection)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("configs.read")).Get("/api/v1/devices/{deviceID}/config/running", handler.Config("running", false))
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("configs.read")).Get("/api/v1/devices/{deviceID}/config/startup", handler.Config("startup", false))
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("configs.read")).Get("/api/v1/devices/{deviceID}/config/running/download", handler.Config("running", true))
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("configs.read")).Get("/api/v1/devices/{deviceID}/config/startup/download", handler.Config("startup", true))
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("configs.read")).Get("/api/v1/devices/{deviceID}/facts", handler.Facts)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("configs.read")).Get("/api/v1/devices/{deviceID}/interfaces", handler.Interfaces)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read")).Get("/api/v1/devices/{deviceID}/backups", handler.ListBackups)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.create")).Post("/api/v1/devices/{deviceID}/backups", handler.CreateBackup)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read")).Get("/api/v1/devices/{deviceID}/backups/{backupID}", handler.GetBackup)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read")).Get("/api/v1/devices/{deviceID}/backups/{backupID}/download", handler.DownloadBackup)
		protected.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read")).Get("/api/v1/devices/{deviceID}/backups/{leftID}/diff/{rightID}", handler.DiffBackups)
		protected.With(appmiddleware.Require("audit.read")).Get("/api/v1/audit-logs", handler.ListAudits)
		protected.With(appmiddleware.Require("audit.read")).Get("/api/v1/audit-logs/{auditID}", handler.GetAudit)
		changeAuthorizationRoutes(protected, handler)
		if config.MetricsEnabled {
			protected.With(appmiddleware.Require("system.manage")).Handle("/metrics", metrics)
		}
	})

	router.NotFound(func(w http.ResponseWriter, r *http.Request) { WriteError(w, r, domain.ErrNotFound) })
	return router
}

func changeAuthorizationRoutes(r chi.Router, h *Handler) {
	requests := "/api/v1/devices/{deviceID}/change-authorizations"
	events := "/api/v1/devices/{deviceID}/change-events"
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require(domain.PermissionAuthorizationRead)).Get(requests, h.ListChangeAuthorizations)
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require(domain.PermissionAuthorizationRequest)).Post(requests, h.CreateChangeAuthorization)
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require(domain.PermissionAuthorizationRead)).Get(requests+"/{authorizationID}", h.GetChangeAuthorization)
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require(domain.PermissionAuthorizationRead)).Get(requests+"/{authorizationID}/history", h.ChangeAuthorizationHistory)
	for _, action := range []string{"approve", "reject", "revoke"} {
		r.With(appmiddleware.Require("devices.read"), appmiddleware.Require(domain.PermissionAuthorizationApprove)).Post(requests+"/{authorizationID}/"+action, h.ChangeAuthorizationAction(action))
	}
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read")).Get(events, h.ListChangeEvents)
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read")).Get(events+"/{eventID}", h.GetChangeEvent)
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read")).Get(events+"/{eventID}/items", h.ListChangeEventItems)
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require("backups.read"), appmiddleware.Require(domain.PermissionAuthorizationReconcile)).Post(events+"/{eventID}/authorize", h.AuthorizeChangeEvent)
	r.With(appmiddleware.Require("devices.read"), appmiddleware.Require(domain.PermissionAuthorizationRead)).Get(events+"/{eventID}/authorization", h.ChangeAuthorizationEvidence)
}

func (h *Handler) OpenAPI(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile(h.OpenAPIPath)
	if err != nil {
		h.Logger.Error("read OpenAPI contract", "path", h.OpenAPIPath, "error", err)
		WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(content)
}
