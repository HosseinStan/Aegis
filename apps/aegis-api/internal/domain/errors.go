package domain

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict")
	ErrValidation     = errors.New("validation failed")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrDeviceDisabled = errors.New("device disabled")
	ErrConnection     = errors.New("device connection failed")
	ErrDeviceAuth     = errors.New("device authentication failed")
	ErrHostKey        = errors.New("ssh host key verification failed")
	ErrTimeout        = errors.New("operation timed out")
	ErrOutputTooLarge = errors.New("output too large")
	ErrUnsupported    = errors.New("unsupported connector")
	ErrRateLimited    = errors.New("rate limited")
	ErrDeviceNotFound = errors.Join(ErrNotFound, errors.New("device not found"))
	ErrBackupNotFound = errors.Join(ErrNotFound, errors.New("backup not found"))
)
