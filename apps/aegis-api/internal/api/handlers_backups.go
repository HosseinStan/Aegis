package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/security"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	deviceID := chi.URLParam(r, "deviceID")
	var input struct {
		ConfigType string `json:"config_type"`
	}
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	if input.ConfigType == "" {
		input.ConfigType = "running"
	}
	result, err := h.Backups.Create(r.Context(), principal, deviceID, input.ConfigType)
	h.audit(r, principal, "backups.create", "device", deviceID, deviceID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	WriteData(w, r, status, result)
}

func (h *Handler) ListBackups(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	deviceID := chi.URLParam(r, "deviceID")
	if _, err := h.Devices.Get(r.Context(), principal, deviceID); err != nil {
		WriteError(w, r, err)
		return
	}
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.Backups.List(r.Context(), deviceID, page)
	h.audit(r, principal, "backups.list", "device", deviceID, deviceID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}

func (h *Handler) GetBackup(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	deviceID := chi.URLParam(r, "deviceID")
	if _, err := h.Devices.Get(r.Context(), principal, deviceID); err != nil {
		WriteError(w, r, err)
		return
	}
	backup, content, err := h.Backups.Content(r.Context(), deviceID, chi.URLParam(r, "backupID"))
	h.audit(r, principal, "backups.read", "backup", chi.URLParam(r, "backupID"), deviceID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, map[string]any{"backup": backup, "config": content})
}

func (h *Handler) DownloadBackup(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	deviceID := chi.URLParam(r, "deviceID")
	if _, err := h.Devices.Get(r.Context(), principal, deviceID); err != nil {
		WriteError(w, r, err)
		return
	}
	backup, content, err := h.Backups.Content(r.Context(), deviceID, chi.URLParam(r, "backupID"))
	h.audit(r, principal, "backups.download", "backup", chi.URLParam(r, "backupID"), deviceID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeFilename(deviceID, deviceID)+`_`+backup.ConfigType+`.txt"`)
	_, _ = w.Write([]byte(content))
}

func (h *Handler) DiffBackups(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	principal, _ := Principal(r.Context())
	deviceID := chi.URLParam(r, "deviceID")
	if _, err := h.Devices.Get(r.Context(), principal, deviceID); err != nil {
		WriteError(w, r, err)
		return
	}
	result, err := h.Backups.Diff(r.Context(), deviceID, chi.URLParam(r, "leftID"), chi.URLParam(r, "rightID"))
	h.audit(r, principal, "backups.diff", "device", deviceID, deviceID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, result)
}

func (h *Handler) ListAudits(w http.ResponseWriter, r *http.Request) {
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.Audits.ListAudits(r.Context(), page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}
func (h *Handler) GetAudit(w http.ResponseWriter, r *http.Request) {
	item, err := h.Audits.GetAudit(r.Context(), chi.URLParam(r, "auditID"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, item)
}

func (h *Handler) audit(r *http.Request, principal domain.Principal, action, resourceType, resourceID, deviceID string, operationErr error, started time.Time) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
	defer cancel()
	host := security.ClientIP(r, h.TrustedProxies)
	record := domain.AuditLog{ID: uuid.NewString(), RequestID: RequestID(r.Context()), UserID: principal.UserID, Username: principal.Username, SourceIP: host, DeviceID: deviceID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Succeeded: operationErr == nil, ErrorCode: services.ErrorCode(operationErr), DurationMS: time.Since(started).Milliseconds(), CreatedAt: time.Now().UTC()}
	if err := h.Audits.CreateAudit(ctx, record); err != nil {
		h.Logger.Error("write audit record", "request_id", record.RequestID, "action", action, "error", err)
	}
}

func pagination(r *http.Request) (repositories.Page, error) {
	limit, offset := 50, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			return repositories.Page{}, domain.ErrValidation
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return repositories.Page{}, domain.ErrValidation
		}
	}
	return repositories.Page{Limit: limit, Offset: offset}, nil
}
func pageMeta(r *http.Request, page repositories.Page, total int) map[string]any {
	return map[string]any{"request_id": RequestID(r.Context()), "pagination": map[string]int{"limit": page.Limit, "offset": page.Offset, "total": total}}
}
func safeFilename(preferred, fallback string) string {
	value := preferred
	if value == "" {
		value = fallback
	}
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, value)
	return value
}
