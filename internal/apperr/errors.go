package apperr

import (
	"context"
	"errors"
	"fmt"
)

type Kind string

const (
	KindValidation      Kind = "validation"
	KindUnauthenticated Kind = "unauthenticated"
	KindForbidden       Kind = "forbidden"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindUnavailable     Kind = "unavailable"
	KindInternal        Kind = "internal"
)

type Error struct {
	Kind      Kind
	Code      string
	Message   string
	Operation string
	Cause     error
}

func (e *Error) Error() string {
	if e.Operation == "" {
		return e.Message
	}
	return e.Operation + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func New(kind Kind, code, message string) error {
	return &Error{Kind: kind, Code: code, Message: message}
}

func Wrap(err error, operation string) error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		clone := *typed
		clone.Operation = operation
		clone.Cause = err
		return &clone
	}
	return &Error{Kind: KindInternal, Code: "internal_error", Message: "internal operation failed", Operation: operation, Cause: err}
}

func Validation(code, message string) error      { return New(KindValidation, code, message) }
func Unauthenticated(code, message string) error { return New(KindUnauthenticated, code, message) }
func Forbidden(code, message string) error       { return New(KindForbidden, code, message) }
func NotFound(code, message string) error        { return New(KindNotFound, code, message) }
func Conflict(code, message string) error        { return New(KindConflict, code, message) }
func Unavailable(code, message string) error     { return New(KindUnavailable, code, message) }

func Classify(err error) (kind Kind, code, message string) {
	if err == nil {
		return "", "", ""
	}
	if errors.Is(err, context.Canceled) {
		return KindUnavailable, "request_cancelled", "request was cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return KindUnavailable, "deadline_exceeded", "request deadline was exceeded"
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Kind, typed.Code, typed.Message
	}
	return KindInternal, "internal_error", "an internal error occurred"
}

func IsKind(err error, kind Kind) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Kind == kind
}

func Detail(err error) string {
	if err == nil {
		return ""
	}
	var typed *Error
	if errors.As(err, &typed) && typed.Operation != "" {
		return fmt.Sprintf("%s: %v", typed.Operation, typed.Cause)
	}
	return err.Error()
}
