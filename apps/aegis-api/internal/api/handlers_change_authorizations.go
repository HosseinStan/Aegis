package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/go-chi/chi/v5"
)

func decodeAuthorizationJSON(w http.ResponseWriter, r *http.Request, value any, allowEmpty bool) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := d.Decode(&raw); err != nil {
		if allowEmpty && errors.Is(err, io.EOF) {
			return nil
		}
		return domain.ErrValidation
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return domain.ErrValidation
	}
	object := json.NewDecoder(bytes.NewReader(raw))
	object.DisallowUnknownFields()
	if err := object.Decode(value); err != nil {
		return domain.ErrValidation
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.ErrValidation
	}
	return nil
}

func (h *Handler) CreateChangeAuthorization(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	p, _ := Principal(r.Context())
	deviceID := chi.URLParam(r, "deviceID")
	var in domain.ChangeAuthorizationInput
	err := decodeAuthorizationJSON(w, r, &in, false)
	var a domain.ChangeAuthorization
	if err == nil {
		a, err = h.ChangeAuthorizations.Create(r.Context(), p, deviceID, in)
	}
	h.audit(r, p, "change_authorization.request", "change_authorization", a.ID, deviceID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusCreated, a)
}

func (h *Handler) ChangeAuthorizationAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		p, _ := Principal(r.Context())
		deviceID, id := chi.URLParam(r, "deviceID"), chi.URLParam(r, "authorizationID")
		var empty struct{}
		err := decodeAuthorizationJSON(w, r, &empty, true)
		var a domain.ChangeAuthorization
		if err == nil {
			switch action {
			case "approve":
				a, err = h.ChangeAuthorizations.Approve(r.Context(), p, deviceID, id)
			case "reject":
				a, err = h.ChangeAuthorizations.Reject(r.Context(), p, deviceID, id)
			case "revoke":
				a, err = h.ChangeAuthorizations.Revoke(r.Context(), p, deviceID, id)
			default:
				err = domain.ErrValidation
			}
		}
		h.audit(r, p, "change_authorization."+action, "change_authorization", id, deviceID, err, started)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		WriteData(w, r, http.StatusOK, a)
	}
}

func (h *Handler) AuthorizeChangeEvent(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	p, _ := Principal(r.Context())
	deviceID, eventID := chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID")
	var in struct {
		AuthorizationID string `json:"authorization_id"`
		Attestation     string `json:"attestation"`
	}
	err := decodeAuthorizationJSON(w, r, &in, false)
	var link domain.ChangeEventAuthorization
	if err == nil {
		link, err = h.ChangeAuthorizations.Reconcile(r.Context(), p, deviceID, eventID, in.AuthorizationID, in.Attestation)
	}
	h.audit(r, p, "change_authorization.link", "change_event", eventID, deviceID, err, started)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, link)
}

func (h *Handler) GetChangeAuthorization(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	a, err := h.ChangeAuthorizations.Get(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "authorizationID"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, a)
}
func (h *Handler) ListChangeAuthorizations(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.ChangeAuthorizations.List(r.Context(), p, chi.URLParam(r, "deviceID"), repositories.ChangeAuthorizationFilter{Status: r.URL.Query().Get("status"), ConfigType: r.URL.Query().Get("config_type")}, page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}
func (h *Handler) ListChangeEvents(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.ChangeAuthorizations.ListEvents(r.Context(), p, chi.URLParam(r, "deviceID"), repositories.ChangeEventFilter{ConfigType: r.URL.Query().Get("config_type"), AuthorizationStatus: r.URL.Query().Get("authorization_status"), PolicyStatus: r.URL.Query().Get("policy_status")}, page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}
func (h *Handler) GetChangeEvent(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	e, err := h.ChangeAuthorizations.GetEvent(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, e)
}
func (h *Handler) ListChangeEventItems(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.ChangeAuthorizations.ListItems(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"), page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}
func (h *Handler) ChangeAuthorizationEvidence(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	e, err := h.ChangeAuthorizations.Evidence(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, e)
}
func (h *Handler) ChangeAuthorizationHistory(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.ChangeAuthorizations.History(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "authorizationID"), page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}
