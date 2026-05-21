// core/domain/errors.go
package domain

import "errors"

// Sentinel domain errors. Adapters and use cases should wrap these so callers
// can detect failure categories via errors.Is.
var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidState = errors.New("invalid state transition")
	ErrInvalidInput = errors.New("invalid input")
)
