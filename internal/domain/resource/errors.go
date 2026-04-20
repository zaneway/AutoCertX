package resource

import "errors"

var (
	ErrNotFound      = errors.New("resource not found")
	ErrConflict      = errors.New("resource conflict")
	ErrScopeMismatch = errors.New("resource scope mismatch")
	ErrValidation    = errors.New("resource validation failed")
	ErrUnavailable   = errors.New("resource unavailable")
)
