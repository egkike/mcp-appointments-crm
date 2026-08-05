// Package repository provides the data-access layer for the application.
//
// Error handling contract:
//   - Sentinel errors (domain.ErrNotFound, domain.ErrConflict, domain.ErrInvalidInput)
//     for CRUD control flow, usable with errors.Is.
//   - domain.SemanticError for business-domain errors (e.g., the 5-step
//     check_availability chain), usable with errors.As.
package repository
