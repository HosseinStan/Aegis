package api

import (
	"errors"
	"net/http"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// This route-local audit wrapper also records permission middleware failures.
// It records action/IDs and safe outcome codes, never request bodies or items.
func (h *Handler) policyAudit(action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &policyStatusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			var err error
			switch recorder.status {
			case http.StatusBadRequest:
				err = domain.ErrValidation
			case http.StatusUnauthorized:
				err = domain.ErrUnauthorized
			case http.StatusForbidden:
				err = domain.ErrForbidden
			case http.StatusNotFound:
				err = domain.ErrNotFound
			case http.StatusConflict:
				err = domain.ErrConflict
			default:
				if recorder.status >= 500 {
					err = errors.New("policy request failed")
				}
			}
			p, _ := Principal(r.Context())
			id := chi.URLParam(r, "policyID")
			if id == "" {
				id = chi.URLParam(r, "eventID")
			}
			if id == "" {
				id = recorder.resourceID
			}
			h.audit(r, p, "change_policy."+action, "change_policy", policyAuditID(id), policyAuditID(chi.URLParam(r, "deviceID")), err, started)
		})
	}
}

// Malformed path IDs are not copied verbatim into audit evidence: a rejected
// request can otherwise smuggle arbitrary secret-bearing text into logs.
func policyAuditID(value string) string {
	id, err := uuid.Parse(value)
	if err != nil {
		return ""
	}
	return id.String()
}

type policyStatusWriter struct {
	http.ResponseWriter
	status     int
	resourceID string
}

func (w *policyStatusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *policyStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (h *Handler) CreateChangePolicy(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	var input domain.PolicySetInput
	err := decodeAuthorizationJSON(w, r, &input, false)
	var set domain.PolicySet
	if err == nil {
		set, err = h.ChangePolicies.Create(r.Context(), p, chi.URLParam(r, "deviceID"), input)
	}
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if recorder, ok := w.(*policyStatusWriter); ok {
		recorder.resourceID = set.ID
	}
	WriteData(w, r, http.StatusCreated, set)
}

func (h *Handler) GetChangePolicy(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	set, err := h.ChangePolicies.Get(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "policyID"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, set)
}
func (h *Handler) ListChangePolicies(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	sets, total, err := h.ChangePolicies.List(r.Context(), p, chi.URLParam(r, "deviceID"), repositories.PolicySetFilter{
		Status: r.URL.Query().Get("status"), ConfigType: r.URL.Query().Get("config_type")}, page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: sets, Meta: pageMeta(r, page, total)})
}
func (h *Handler) ChangePolicyAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := Principal(r.Context())
		var empty struct{}
		err := decodeAuthorizationJSON(w, r, &empty, true)
		var set domain.PolicySet
		if err == nil {
			switch action {
			case "activate":
				set, err = h.ChangePolicies.Activate(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "policyID"))
			case "retire":
				set, err = h.ChangePolicies.Retire(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "policyID"))
			default:
				err = domain.ErrValidation
			}
		}
		if err != nil {
			WriteError(w, r, err)
			return
		}
		WriteData(w, r, http.StatusOK, set)
	}
}
func (h *Handler) EvaluateChangePolicy(preview bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := Principal(r.Context())
		var empty struct{}
		err := decodeAuthorizationJSON(w, r, &empty, true)
		var result any
		if err == nil {
			if preview {
				result, err = h.ChangePolicies.Preview(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"))
			} else {
				result, err = h.ChangePolicies.Evaluate(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"))
			}
		}
		if err != nil {
			WriteError(w, r, err)
			return
		}
		WriteData(w, r, http.StatusOK, result)
	}
}
func (h *Handler) ChangePolicyEvidence(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	evidence, err := h.ChangePolicies.Evidence(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, evidence)
}
func (h *Handler) ChangePolicyEvidenceItems(w http.ResponseWriter, r *http.Request) {
	p, _ := Principal(r.Context())
	page, err := pagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items, total, err := h.ChangePolicies.Items(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"), page)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: pageMeta(r, page, total)})
}
