package monitor

import (
	"errors"
	"fmt"
)

const (
	// ErrorCodeInternal is an internal error code.
	ErrorCodeInternal = "internal"
)

// Error represents an error within the context of volume-pdax-monitor service.
type Error struct {
	// Code is a machine-readable code.
	Code string `json:"code"`
	// Message is a human-readable message.
	Message string `json:"message"`
	// Inner is a wrapped error that is never shown to API consumers.
	Inner error `json:"-"`
}

func (e Error) Error() string {
	if e.Inner != nil {
		return fmt.Sprintf("%s %s: %v", e.Code, e.Message, e.Inner)
	}

	return fmt.Sprintf("%s %s", e.Code, e.Message)
}

// Unwrap the error returning the error's reason.
func (e Error) Unwrap() error {
	return e.Inner
}

// ErrorCode returns the code of the error, if available.
func ErrorCode(err error) string {
	var e Error
	if errors.As(err, &e) {
		return e.Code
	}

	return ""
}
