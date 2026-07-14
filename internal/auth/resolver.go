package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrUnauthenticated is the sentinel error for authentication failures.
// The middleware translates this to HTTP 401 with a Spanish message.
var ErrUnauthenticated = errors.New("unauthenticated")

// authError wraps ErrUnauthenticated with a user-facing Spanish message.
// Error() returns the Spanish message (never stack traces or internal details).
type authError struct {
	msg   string
	inner error
}

func (e *authError) Error() string { return e.msg }
func (e *authError) Unwrap() error { return e.inner }

// Spanish messages for authentication failures.
const (
	msgNotRecognized = "no te reconozco. Por favor regístrate primero."
	msgDisabled      = "tu cuenta está deshabilitada. Contacta al administrador."
)

// CallerResolver resolves a caller ID to a Caller by querying accounts and clients.
// It executes at most 2 queries per resolution (per ADR-0011).
type CallerResolver struct {
	db *sql.DB
}

// NewCallerResolver creates a resolver with an already-open *sql.DB.
func NewCallerResolver(db *sql.DB) *CallerResolver {
	return &CallerResolver{db: db}
}

// Resolve looks up the caller by ID. Algorithm (≤ 2 queries):
//  1. SELECT from accounts WHERE id = ?
//     - Row with is_active=1 → continue to step 2
//     - Row with is_active=0 → ErrUnauthenticated (disabled), no clients query
//     - No row → go to step 3
//  2. SELECT from clients WHERE id = ? (only if step 1 found active account)
//     - Row found → Caller with ClientID = &id
//     - No row → Caller with ClientID = nil
//  3. SELECT from clients WHERE id = ? (only if step 1 found no account)
//     - Row found → Caller{Role: RoleClient, ClientID: &id}
//     - No row → ErrUnauthenticated (not recognized)
func (r *CallerResolver) Resolve(ctx context.Context, id string) (Caller, error) {
	// Step 1: query accounts
	var role string
	var profID *string
	var isActive int

	err := r.db.QueryRowContext(ctx,
		"SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?", id,
	).Scan(&id, &role, &profID, &isActive)

	switch {
	case err == nil:
		// Account found
		if isActive == 0 {
			return Caller{}, &authError{msg: msgDisabled, inner: ErrUnauthenticated}
		}

		// Active account — check if also a client (ADR-0011)
		caller := Caller{ID: id, Role: role, ProfessionalID: profID}
		var clientID string
		err := r.db.QueryRowContext(ctx,
			"SELECT id FROM clients WHERE id = ?", id,
		).Scan(&clientID)
		switch {
		case err == nil:
			caller.ClientID = &clientID
			return caller, nil
		case errors.Is(err, sql.ErrNoRows):
			// Not also a client; valid resolution, ClientID stays nil.
			return caller, nil
		default:
			// Real DB failure mid-resolution: do NOT mask as a successful
			// (caller, nil) — return the error so the middleware responds 500.
			return Caller{}, fmt.Errorf("resolve caller %q: %w", id, err)
		}

	case errors.Is(err, sql.ErrNoRows):
		// No account — check clients
		var clientID string
		err := r.db.QueryRowContext(ctx,
			"SELECT id FROM clients WHERE id = ?", id,
		).Scan(&clientID)
		if err == nil {
			return Caller{ID: id, Role: RoleClient, ClientID: &clientID}, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			return Caller{}, &authError{msg: msgNotRecognized, inner: ErrUnauthenticated}
		}
		return Caller{}, fmt.Errorf("resolve caller %q: %w", id, err)

	default:
		return Caller{}, fmt.Errorf("resolve caller %q: %w", id, err)
	}
}
