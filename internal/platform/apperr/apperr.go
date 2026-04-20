package apperr

import "errors"

// Detail describes one field-level validation problem.
type Detail struct {
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Error is the application-level error shape exposed to HTTP handlers.
type Error struct {
	Code    string
	Message string
	Status  int
	Details []Detail
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// New constructs an application error.
func New(status int, code string, message string, details ...Detail) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Status:  status,
		Details: append([]Detail(nil), details...),
	}
}

// Wrap constructs an application error that keeps the original cause.
func Wrap(err error, status int, code string, message string, details ...Detail) *Error {
	appErr := New(status, code, message, details...)
	appErr.Cause = err
	return appErr
}

// Field creates one field-level error detail entry.
func Field(field string, reason string) Detail {
	return Detail{
		Field:  field,
		Reason: reason,
	}
}

// As extracts an application error from the error chain.
func As(err error) (*Error, bool) {
	var appErr *Error
	if !errors.As(err, &appErr) {
		return nil, false
	}
	return appErr, true
}
