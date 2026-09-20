package services

import (
	"errors"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, domain.ErrUnauthorized):
		return "UNAUTHORIZED"
	case errors.Is(err, domain.ErrForbidden):
		return "FORBIDDEN"
	case errors.Is(err, domain.ErrValidation):
		return "VALIDATION_FAILED"
	case errors.Is(err, domain.ErrDeviceNotFound):
		return "DEVICE_NOT_FOUND"
	case errors.Is(err, domain.ErrBackupNotFound):
		return "BACKUP_NOT_FOUND"
	case errors.Is(err, domain.ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrDeviceDisabled):
		return "DEVICE_DISABLED"
	case errors.Is(err, domain.ErrDeviceAuth):
		return "DEVICE_AUTHENTICATION_FAILED"
	case errors.Is(err, domain.ErrHostKey):
		return "SSH_HOST_KEY_MISMATCH"
	case errors.Is(err, domain.ErrTimeout):
		return "COMMAND_TIMEOUT"
	case errors.Is(err, domain.ErrOutputTooLarge):
		return "OUTPUT_TOO_LARGE"
	case errors.Is(err, domain.ErrConnection):
		return "DEVICE_CONNECTION_FAILED"
	case errors.Is(err, domain.ErrUnsupported):
		return "CONNECTOR_UNSUPPORTED"
	case errors.Is(err, domain.ErrConflict):
		return "CONFLICT"
	case errors.Is(err, domain.ErrRateLimited):
		return "RATE_LIMITED"
	default:
		return "INTERNAL_ERROR"
	}
}
