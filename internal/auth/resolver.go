package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// authError wraps domain.ErrUnauthenticated with a user-facing Spanish message.
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
// A nil *sql.DB panics at wiring time (fail fast, same as
// NewAuthMiddleware): a per-request nil deref in the middle of the auth
// chain would kill the connection instead of producing a controlled 500.
func NewCallerResolver(db *sql.DB) *CallerResolver {
	if db == nil {
		panic("auth: NewCallerResolver requires a non-nil *sql.DB")
	}
	return &CallerResolver{db: db}
}

// Resolve looks up the caller by ID. Algorithm (≤ 2 queries):
//  1. SELECT from accounts WHERE id = ?
//     - Row with is_active=1 → continue to step 2
//     - Row with is_active=0 → domain.ErrUnauthenticated (disabled), no clients query
//     - No row → go to step 3
//  2. SELECT from clients WHERE id = ? (only if step 1 found active account)
//     - Row found → Caller with ClientID = &id
//     - No row → Caller with ClientID = nil
//  3. SELECT from clients WHERE id = ? (only if step 1 found no account)
//     - Row found → Caller{Role: RoleClient, ClientID: &id}
//     - No row → domain.ErrUnauthenticated (not recognized)
func (r *CallerResolver) Resolve(ctx context.Context, id string) (Caller, error) {
	// Step 1: query accounts
	var role string
	var profID *string
	var isActive int

	err := r.db.QueryRowContext(ctx,
		"SELECT role, professional_id, is_active FROM accounts WHERE id = ?", id,
	).Scan(&role, &profID, &isActive)

	switch {
	case err == nil:
		// Account found
		if isActive == 0 {
			return Caller{}, &authError{msg: msgDisabled, inner: domain.ErrUnauthenticated}
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
			// The caller ID is intentionally NOT embedded here: it is PII and
			// would leak into server logs; the middleware logs a hashed
			// caller reference for correlation instead.
			return Caller{}, fmt.Errorf("resolve caller: %w", err)
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
			return Caller{}, &authError{msg: msgNotRecognized, inner: domain.ErrUnauthenticated}
		}
		// DB failure — no PII in the error text (see note in step 2).
		return Caller{}, fmt.Errorf("resolve caller: %w", err)

	default:
		// DB failure — no PII in the error text (see note in step 2).
		return Caller{}, fmt.Errorf("resolve caller: %w", err)
	}
}
