package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.RegistrationLookup = (*RegistrationLookup)(nil)

// RegistrationLookup composes the two auth-free registration reads — one owned
// by accounts, one by clients — into the single domain port the anonymous
// registration use case consumes.
//
// It is a thin delegating adapter, not a new source of behavior: each method
// forwards to the concrete repo that owns its table, so SQL stays in the
// adapter that owns it and table ownership is preserved. Neither delegate
// performs a role/caller check (see the port's doc for why the registration
// path must be auth-free); every other repo method keeps its guards untouched.
//
// The composition root constructs it from the same handles it already builds
// and wires it ONLY into get_or_create_client — the server stays loopback-only.
type RegistrationLookup struct {
	accounts *AccountsRepo
	clients  *ClientsRepo
}

// NewRegistrationLookup builds the registration lookup adapter from the
// accounts and clients repos.
func NewRegistrationLookup(accounts *AccountsRepo, clients *ClientsRepo) *RegistrationLookup {
	return &RegistrationLookup{accounts: accounts, clients: clients}
}

// AccountExistsByID delegates to the auth-free accounts existence read.
func (r *RegistrationLookup) AccountExistsByID(ctx context.Context, id string) (bool, error) {
	return r.accounts.AccountExistsByID(ctx, id)
}

// FindClientByPhoneAny delegates to the auth-free phone lookup that returns the
// row whatever its id.
func (r *RegistrationLookup) FindClientByPhoneAny(ctx context.Context, phone string) (*entity.Client, error) {
	return r.clients.FindClientByPhoneAny(ctx, phone)
}
