package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	appmiddleware "company.local/aegis/apps/aegis-api/internal/api/middleware"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
	"company.local/aegis/apps/aegis-api/internal/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func changeRiskRoutes(r chi.Router, h *Handler) {
	e := "/api/v1/devices/{deviceID}/change-events/{eventID}"
	with := func(action string, record bool) []func(http.Handler) http.Handler {
		m := []func(http.Handler) http.Handler{h.riskAudit(action)}
		for _, p := range []string{"devices.read", "backups.read", domain.PermissionAuthorizationRead, domain.PermissionPolicyRead, domain.PermissionRiskRead} {
			m = append(m, appmiddleware.Require(p))
		}
		if record {
			m = append(m, appmiddleware.Require(domain.PermissionRiskEvaluate))
		}
		return m
	}
	r.With(with("preview", false)...).Post(e+"/risk-assessment/preview", h.PreviewChangeRisk)
	r.With(with("evaluate_request", true)...).Post(e+"/risk-assessment", h.RecordChangeRisk)
	r.With(with("read", false)...).Get(e+"/risk-assessment", h.GetChangeRisk)
	r.With(with("read", false)...).Get(e+"/risk-assessment/findings", h.ListChangeRiskFindings)
	r.With(with("list", false)...).Get(e+"/risk-assessments", h.ListChangeRisks)
	r.With(with("read", false)...).Get(e+"/risk-assessments/{assessmentID}", h.GetChangeRisk)
	r.With(with("read", false)...).Get(e+"/risk-assessments/{assessmentID}/findings", h.ListChangeRiskFindings)
}

func riskHTTPID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// The body has one optional scalar; decode its object tokens explicitly so
// duplicate (including escaped) keys cannot be silently overwritten by JSON.
func decodeRiskRequest(w http.ResponseWriter, r *http.Request, record bool) (*string, error) {
	if r.Body == nil {
		return nil, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	raw, err := io.ReadAll(r.Body)
	if err != nil || !utf8.Valid(raw) {
		return nil, domain.ErrValidation
	}
	if len(raw) == 0 {
		return nil, nil
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		return nil, domain.ErrValidation
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, domain.ErrValidation
	}
	var expected *string
	seen := false
	for d.More() {
		key, err := d.Token()
		if err != nil || key != "expected_latest_assessment_id" || !record || seen {
			return nil, domain.ErrValidation
		}
		seen = true
		if err := d.Decode(&expected); err != nil || (expected != nil && !riskHTTPID(*expected)) {
			return nil, domain.ErrValidation
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return nil, domain.ErrValidation
	}
	if _, err = d.Token(); !errors.Is(err, io.EOF) {
		return nil, domain.ErrValidation
	}
	return expected, nil
}

func riskHTTPPage(r *http.Request, paged bool) (repositories.Page, error) {
	page := repositories.Page{Limit: 50}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return page, domain.ErrValidation
	}
	for key, values := range query {
		if !paged || (key != "limit" && key != "offset") || len(values) != 1 || values[0] == "" {
			return page, domain.ErrValidation
		}
		for _, c := range values[0] {
			if c < '0' || c > '9' {
				return page, domain.ErrValidation
			}
		}
		n, err := strconv.ParseUint(values[0], 10, 32)
		if err != nil || (key == "limit" && (n < 1 || n > 200)) || (key == "offset" && n > 10000) {
			return page, domain.ErrValidation
		}
		if key == "limit" {
			page.Limit = int(n)
		} else {
			page.Offset = int(n)
		}
	}
	return page, nil
}

func (h *Handler) riskRequest(r *http.Request, paged bool) (domain.Principal, repositories.Page, error) {
	p, ok := Principal(r.Context())
	if !ok {
		return p, repositories.Page{}, domain.ErrUnauthorized
	}
	for _, name := range []string{"deviceID", "eventID", "assessmentID"} {
		id := chi.URLParam(r, name)
		if (name != "assessmentID" || id != "") && !riskHTTPID(id) {
			return p, repositories.Page{}, domain.ErrValidation
		}
	}
	page, err := riskHTTPPage(r, paged)
	if err == nil && h.ChangeRisks == nil {
		err = repositories.ErrRiskUnavailable
	}
	return p, page, err
}

func (h *Handler) PreviewChangeRisk(w http.ResponseWriter, r *http.Request) {
	p, _, err := h.riskRequest(r, false)
	if err == nil {
		_, err = decodeRiskRequest(w, r, false)
	}
	var result repositories.RiskPreview
	if err == nil {
		result, err = h.ChangeRisks.Preview(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"))
	}
	if err != nil {
		writeRiskError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, result)
}

func (h *Handler) RecordChangeRisk(w http.ResponseWriter, r *http.Request) {
	p, _, err := h.riskRequest(r, false)
	var expected *string
	if err == nil {
		expected, err = decodeRiskRequest(w, r, true)
	}
	var result repositories.RiskRecord
	if err == nil {
		result, err = h.ChangeRisks.Record(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"), expected)
	}
	if err != nil {
		writeRiskError(w, r, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	WriteData(w, r, status, riskRecordDTO{Created: result.Created, Assessment: riskAssessmentResponse(result.Assessment)})
}

func (h *Handler) GetChangeRisk(w http.ResponseWriter, r *http.Request) {
	p, _, err := h.riskRequest(r, false)
	var result domain.ChangeRiskAssessment
	if err == nil {
		result, err = h.ChangeRisks.Get(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"), chi.URLParam(r, "assessmentID"))
	}
	if err != nil {
		writeRiskError(w, r, err)
		return
	}
	WriteData(w, r, http.StatusOK, riskAssessmentResponse(result))
}

func (h *Handler) ListChangeRisks(w http.ResponseWriter, r *http.Request) {
	p, page, err := h.riskRequest(r, true)
	var records []domain.ChangeRiskAssessment
	var total int
	if err == nil {
		records, total, err = h.ChangeRisks.List(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"), page)
	}
	if err != nil {
		writeRiskError(w, r, err)
		return
	}
	items := make([]riskAssessmentDTO, 0, len(records))
	for _, record := range records {
		items = append(items, riskAssessmentResponse(record))
	}
	meta := pageMeta(r, page, total)
	meta["historical"] = true
	WriteJSON(w, http.StatusOK, envelope{Data: items, Meta: meta})
}

func (h *Handler) ListChangeRiskFindings(w http.ResponseWriter, r *http.Request) {
	p, page, err := h.riskRequest(r, true)
	var result repositories.RiskFindingPage
	if err == nil {
		result, err = h.ChangeRisks.Findings(r.Context(), p, chi.URLParam(r, "deviceID"), chi.URLParam(r, "eventID"), chi.URLParam(r, "assessmentID"), page)
	}
	if err != nil {
		writeRiskError(w, r, err)
		return
	}
	meta := pageMeta(r, page, result.Total)
	meta["assessment_id"], meta["historical"] = result.AssessmentID, true
	WriteJSON(w, http.StatusOK, envelope{Data: result.Items, Meta: meta})
}

// Risk-only errors: preserve prior API behavior and never serialize causes.
func writeRiskError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusServiceUnavailable, "RISK_UNAVAILABLE", "Risk assessment is temporarily unavailable."
	var engine interface{ Code() int }
	busy := errors.As(err, &engine) && (engine.Code()&255 == 5 || engine.Code()&255 == 6)
	switch {
	case busy:
	case errors.Is(err, domain.ErrUnauthorized):
		status, code, message = 401, "UNAUTHORIZED", "Authentication is required."
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = 403, "FORBIDDEN", "You do not have permission to perform this action."
	case errors.Is(err, domain.ErrValidation):
		status, code, message = 400, "VALIDATION_FAILED", "The request contains invalid data."
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = 404, "NOT_FOUND", "The requested resource was not found."
	case errors.Is(err, risk.ErrIntegrity):
		status, code, message = 500, "RISK_INTEGRITY_FAILURE", "Risk evidence could not be verified."
	case errors.Is(err, risk.ErrLimit):
		status, code, message = 409, "RISK_ASSESSMENT_LIMIT", "Risk evidence exceeds the supported assessment limits."
	case errors.Is(err, risk.ErrIneligible):
		status, code, message = 409, "RISK_EVIDENCE_UNSUPPORTED", "Risk evidence is not eligible for recording."
	case errors.Is(err, domain.ErrConflict):
		status, code, message = 409, "CONFLICT", "The requested change conflicts with existing data."
	}
	WriteJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message, RequestID: RequestID(r.Context())}})
}

type riskStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *riskStatusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *riskStatusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *riskStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (h *Handler) riskAudit(action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			writer := &riskStatusWriter{ResponseWriter: w}
			next.ServeHTTP(writer, r)
			// All service transactions/admission are released before best-effort
			// audit re-enters the Store. Successful evaluate is NEVER duplicated.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			p, _ := Principal(r.Context())
			id := chi.URLParam(r, "assessmentID")
			if id == "" {
				id = chi.URLParam(r, "eventID")
			}
			safeID := func(value string) string {
				if riskHTTPID(value) {
					return value
				}
				return ""
			}
			ip := net.ParseIP(security.ClientIP(r, h.TrustedProxies))
			source := ""
			if ip != nil {
				source = ip.String()
			}
			record := domain.AuditLog{ID: uuid.NewString(), RequestID: safeID(RequestID(r.Context())), UserID: safeID(p.UserID), SourceIP: source, DeviceID: safeID(chi.URLParam(r, "deviceID")), Action: "change_risk." + action, ResourceType: "change_risk_assessment", ResourceID: safeID(id), Succeeded: writer.status >= 200 && writer.status < 300, DurationMS: time.Since(started).Milliseconds(), CreatedAt: time.Now().UTC()}
			if !record.Succeeded {
				record.ErrorCode = "HTTP_" + strconv.Itoa(writer.status)
			}
			if h.Audits == nil || h.Audits.CreateAudit(ctx, record) != nil {
				if h.Logger != nil {
					h.Logger.Warn("risk request audit unavailable", "request_id", record.RequestID, "action", record.Action)
				}
			}
		})
	}
}
