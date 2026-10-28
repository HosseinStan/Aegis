package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/auth"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/go-chi/chi/v5"
)

type HealthStore interface{ Ping(context.Context) error }

type Handler struct {
	Auth                 *auth.Service
	Credentials          *credentials.Store
	Devices              *services.DeviceService
	Configs              *services.ConfigService
	Backups              *services.BackupService
	ChangeAuthorizations *services.ChangeAuthorizationService
	ChangePolicies       *services.ChangePolicyService
	ChangeRisks          *services.ChangeRiskService
	Audits               repositories.AuditRepository
	Health               HealthStore
	Logger               *slog.Logger
	OpenAPIPath          string
	TrustedProxies       []*net.IPNet
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	WriteData(w, r, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.Health.Ping(r.Context()); err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	tokens, principal, err := h.Auth.Login(r.Context(), input.Username, input.Password)
	h.audit(r, principal, "auth.login", "user", principal.UserID, "", err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	setDocsCookie(w, r, tokens.AccessToken, int(tokens.ExpiresIn))
	WriteData(w, r, http.StatusOK, map[string]any{"tokens": tokens, "user": principal})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	tokens, err := h.Auth.Refresh(r.Context(), input.RefreshToken)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, tokens)
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	principal, _ := Principal(r.Context())
	h.audit(r, principal, "auth.logout", "user", principal.UserID, "", nil, time.Now())
	setDocsCookie(w, r, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateDocsSession(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if raw == "" {
		WriteError(w, r, domain.ErrUnauthorized)
		return
	}
	setDocsCookie(w, r, raw, 15*60)
	w.WriteHeader(http.StatusNoContent)
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setDocsCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "aegis_docs_token", Value: value, Path: "/docs", MaxAge: maxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: requestIsHTTPS(r)})
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal, _ := Principal(r.Context())
	WriteData(w, r, http.StatusOK, principal)
}

func (h *Handler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	var input struct {
		Name         string `json:"name"`
		AuthType     string `json:"auth_type"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		PrivateKey   string `json:"private_key"`
		EnableSecret string `json:"enable_secret"`
	}
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	created, err := h.Credentials.Create(r.Context(), domain.Credential{Name: input.Name, AuthType: input.AuthType, Username: input.Username, Password: input.Password, PrivateKey: []byte(input.PrivateKey), EnableSecret: input.EnableSecret})
	h.audit(r, principal, "credentials.create", "credential", created.ID, "", err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	created.Username, created.Password, created.PrivateKey, created.EnableSecret = "", "", nil, ""
	WriteData(w, r, http.StatusCreated, created)
}

func (h *Handler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.Credentials.List(r.Context(), page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}

func (h *Handler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	id := chi.URLParam(r, "credentialID")
	err := h.Credentials.Delete(r.Context(), id)
	h.audit(r, principal, "credentials.delete", "credential", id, "", err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type deviceRequest struct {
	DisplayName    string `json:"display_name"`
	Hostname       string `json:"hostname"`
	ManagementIP   string `json:"management_ip"`
	ManagementPort int    `json:"management_port"`
	Vendor         string `json:"vendor"`
	Platform       string `json:"platform"`
	Protocol       string `json:"protocol"`
	CredentialID   string `json:"credential_id"`
	DeviceGroup    string `json:"device_group"`
	Description    string `json:"description"`
	Enabled        *bool  `json:"enabled,omitempty"`
}

type devicePatchRequest struct {
	DisplayName    *string `json:"display_name"`
	Hostname       *string `json:"hostname"`
	ManagementIP   *string `json:"management_ip"`
	ManagementPort *int    `json:"management_port"`
	Vendor         *string `json:"vendor"`
	Platform       *string `json:"platform"`
	Protocol       *string `json:"protocol"`
	CredentialID   *string `json:"credential_id"`
	DeviceGroup    *string `json:"device_group"`
	Description    *string `json:"description"`
	Enabled        *bool   `json:"enabled"`
}

func (i devicePatchRequest) empty() bool {
	return i.DisplayName == nil && i.Hostname == nil && i.ManagementIP == nil && i.ManagementPort == nil && i.Vendor == nil && i.Platform == nil && i.Protocol == nil && i.CredentialID == nil && i.DeviceGroup == nil && i.Description == nil && i.Enabled == nil
}

func (i deviceRequest) device() domain.Device {
	enabled := true
	if i.Enabled != nil {
		enabled = *i.Enabled
	}
	port := i.ManagementPort
	if port == 0 {
		port = 22
	}
	return domain.Device{DisplayName: i.DisplayName, Hostname: i.Hostname, ManagementIP: i.ManagementIP, ManagementPort: port, Vendor: strings.ToLower(i.Vendor), Platform: strings.ToLower(i.Platform), Protocol: strings.ToLower(i.Protocol), CredentialID: i.CredentialID, DeviceGroup: i.DeviceGroup, Description: i.Description, Enabled: enabled}
}

func (h *Handler) CreateDevice(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	var input deviceRequest
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	created, err := h.Devices.Create(r.Context(), input.device())
	h.audit(r, principal, "devices.create", "device", created.ID, created.ID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusCreated, created)
}

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	principal, _ := Principal(r.Context())
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var filter repositories.DeviceFilter
	filter.Group, filter.Search = r.URL.Query().Get("group"), r.URL.Query().Get("search")
	if raw := r.URL.Query().Get("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			WriteError(w, r, domain.ErrValidation)
			return
		}
		filter.Enabled = &value
	}
	items, total, err := h.Devices.List(r.Context(), principal, filter, page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}

func (h *Handler) GetDevice(w http.ResponseWriter, r *http.Request) {
	principal, _ := Principal(r.Context())
	item, err := h.Devices.Get(r.Context(), principal, chi.URLParam(r, "deviceID"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, item)
}

func (h *Handler) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	id := chi.URLParam(r, "deviceID")
	existing, err := h.Devices.GetInternal(r.Context(), principal, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var input devicePatchRequest
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	if input.empty() {
		WriteError(w, r, domain.ErrValidation)
		return
	}
	updated := existing
	if input.DisplayName != nil {
		updated.DisplayName = *input.DisplayName
	}
	if input.Hostname != nil {
		updated.Hostname = *input.Hostname
	}
	if input.ManagementIP != nil {
		updated.ManagementIP = *input.ManagementIP
	}
	if input.ManagementPort != nil {
		updated.ManagementPort = *input.ManagementPort
	}
	if input.Vendor != nil {
		updated.Vendor = strings.ToLower(*input.Vendor)
	}
	if input.Platform != nil {
		updated.Platform = strings.ToLower(*input.Platform)
	}
	if input.Protocol != nil {
		updated.Protocol = strings.ToLower(*input.Protocol)
	}
	if input.CredentialID != nil {
		updated.CredentialID = *input.CredentialID
	}
	if input.DeviceGroup != nil {
		updated.DeviceGroup = *input.DeviceGroup
	}
	if input.Description != nil {
		updated.Description = *input.Description
	}
	if input.Enabled != nil {
		updated.Enabled = *input.Enabled
	}
	updated, err = h.Devices.Update(r.Context(), updated)
	if err == nil {
		h.Configs.Invalidate(id)
	}
	h.audit(r, principal, "devices.update", "device", id, id, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, updated)
}

func (h *Handler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	id := chi.URLParam(r, "deviceID")
	_, err := h.Devices.GetInternal(r.Context(), principal, id)
	if err == nil {
		err = h.Devices.Delete(r.Context(), id)
	}
	if err == nil {
		h.Configs.Invalidate(id)
	}
	h.audit(r, principal, "devices.delete", "device", id, id, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TestConnection(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	id := chi.URLParam(r, "deviceID")
	err := h.Devices.TestConnection(r.Context(), principal, id)
	h.audit(r, principal, "devices.test_connection", "device", id, id, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, map[string]bool{"connected": true})
}

func (h *Handler) Config(kind string, download bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		principal, _ := Principal(r.Context())
		id := chi.URLParam(r, "deviceID")
		bypass := r.URL.Query().Get("refresh") == "true" && principal.HasPermission("devices.manage")
		result, err := h.Configs.GetConfig(r.Context(), principal, id, kind, bypass)
		h.audit(r, principal, "configs.read_"+kind, "device", id, id, err, started)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if download {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.txt"`, safeFilename(result.Hostname, id), kind))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(result.Config))
			return
		}
		WriteData(w, r, http.StatusOK, result)
	}
}

func (h *Handler) Facts(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	id := chi.URLParam(r, "deviceID")
	value, cached, err := h.Configs.GetFacts(r.Context(), principal, id, false)
	h.audit(r, principal, "configs.read_facts", "device", id, id, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, map[string]any{"device_id": id, "from_cache": cached, "facts": value})
}
func (h *Handler) Interfaces(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	id := chi.URLParam(r, "deviceID")
	value, cached, err := h.Configs.GetInterfaces(r.Context(), principal, id, false)
	h.audit(r, principal, "configs.read_interfaces", "device", id, id, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, map[string]any{"device_id": id, "from_cache": cached, "interfaces": value})
}
