package repository

import "errors"

// Sentinel errors for the repository layer.
// Callers use errors.Is to check for specific error conditions.
var (
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound = errors.New("no encontrado")

	// ErrConflict indicates a uniqueness or invariant violation.
	ErrConflict = errors.New("conflicto")

	// ErrInvalidInput indicates the provided input failed validation.
	ErrInvalidInput = errors.New("entrada inválida")
)
