package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// RegistrationLookup is the narrow, auth-free read port the anonymous
// registration use case (get_or_create_client) consumes before any caller
// identity exists.
//
// Why auth-free: the WhatsApp/Telegram gateway registers an unknown sender
// before a Caller can be resolved, so the registration path runs with no
// caller and no role in the context. The existing ClientsRepo.FindByPhone
// (admin/owner) and AccountsRepo.FindByID (authenticated caller) therefore
// cannot serve it, and weakening their guards is not an option. This port is
// the registration-internal counterpart: it exposes ONLY the two reads the
// anonymous path needs, so the authorization surface stays explicit.
//
// The registration use case is the authority for these reads (phone
// validation, rate limiting and audit land in Phase 2B); the composition root
// wires this port ONLY into the registration use case, never into an
// authenticated flow. The server remains loopback-only, so no network caller
// can reach these reads directly.
//
// Implementations MUST NOT add role/caller checks; the whole point of the port
// is to be callable with the anonymous registration context.
type RegistrationLookup interface {
	// AccountExistsByID reports whether an accounts row with the given id
	// exists. It answers existence only, regardless of role or is_active:
	// the registration path rejects a phone owned by ANY account (active or
	// inactive), exactly like the authenticated collision check it replaces.
	// A missing row is (false, nil), never domain.ErrNotFound.
	AccountExistsByID(ctx context.Context, id string) (bool, error)

	// FindClientByPhoneAny returns the client whose phone column equals phone,
	// whatever its id: the registration use case distinguishes an existing
	// id == phone row (idempotent no-op) from a legacy id != phone row
	// (explicit conflict). A missing row returns domain.ErrNotFound, matching
	// ClientsRepo.FindByPhone semantics.
	FindClientByPhoneAny(ctx context.Context, phone string) (*entity.Client, error)
}
