package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/services"
)

type envelope struct {
	Data any `json:"data,omitempty"`
	Meta any `json:"meta,omitempty"`
}
type errorEnvelope struct {
	Error errorBody `json:"error"`
}
type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func WriteData(w http.ResponseWriter, r *http.Request, status int, data any) {
	WriteJSON(w, status, envelope{Data: data, Meta: map[string]any{"request_id": RequestID(r.Context())}})
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "The request could not be completed."
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		status, message = http.StatusUnauthorized, "Authentication is required."
	case errors.Is(err, domain.ErrForbidden):
		status, message = http.StatusForbidden, "You do not have permission to perform this action."
	case errors.Is(err, domain.ErrValidation):
		status, message = http.StatusBadRequest, "The request contains invalid data."
	case errors.Is(err, domain.ErrNotFound):
		status, message = http.StatusNotFound, "The requested resource was not found."
	case errors.Is(err, domain.ErrConflict):
		status, message = http.StatusConflict, "The requested change conflicts with existing data."
	case errors.Is(err, domain.ErrDeviceDisabled):
		status, message = http.StatusConflict, "The device is disabled."
	case errors.Is(err, domain.ErrDeviceAuth):
		status, message = http.StatusBadGateway, "The device rejected Aegis authentication."
	case errors.Is(err, domain.ErrHostKey):
		status, message = http.StatusBadGateway, "The device SSH identity could not be verified."
	case errors.Is(err, domain.ErrTimeout):
		status, message = http.StatusGatewayTimeout, "The device operation timed out."
	case errors.Is(err, domain.ErrOutputTooLarge):
		status, message = http.StatusBadGateway, "The device output exceeded the configured limit."
	case errors.Is(err, domain.ErrConnection):
		status, message = http.StatusBadGateway, "The device operation failed."
	case errors.Is(err, domain.ErrUnsupported):
		status, message = http.StatusUnprocessableEntity, "The requested operation is not supported by this connector."
	case errors.Is(err, domain.ErrRateLimited):
		status, message = http.StatusTooManyRequests, "Too many requests. Try again later."
	}
	WriteJSON(w, status, errorEnvelope{Error: errorBody{Code: services.ErrorCode(err), Message: message, RequestID: RequestID(r.Context())}})
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return errors.Join(domain.ErrValidation, err)
	}
	return nil
}
