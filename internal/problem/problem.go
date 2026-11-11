// Package problem defines the business errors that services return and the
// API turns into HTTP responses, each with a message ready for the user.
package problem

import (
	"errors"
	"fmt"
)

// Kind classifies a business error so the API can pick the status code.
type Kind int

// Error kinds, mapped to 400, 404 and 409 by the API.
const (
	KindInvalid Kind = iota + 1
	KindNotFound
	KindConflict
)

// Error is a business error with a stable code and a message in Portuguese.
type Error struct {
	Kind    Kind
	Code    string
	Message string
}

// Error returns the user message.
func (e *Error) Error() string { return e.Message }

// Is matches another *Error by kind and code, so sentinel errors work with errors.Is.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Kind == t.Kind && e.Code == t.Code
}

// Invalid returns an input validation error.
func Invalid(code, format string, args ...any) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: fmt.Sprintf(format, args...)}
}

// NotFound returns an error for a missing record.
func NotFound(code, message string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: message}
}

// Conflict returns an error for an action the current state does not allow.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// As returns the *Error inside err, if any.
func As(err error) (*Error, bool) {
	var p *Error
	ok := errors.As(err, &p)
	return p, ok
}
