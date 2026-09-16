package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ErrLastActiveOwner reports the refusal to soft-delete the only ACTIVE owner.
// It wraps domain.ErrConflict so callers can branch on either sentinel without
// parsing prose (mirrors ErrOwnerAlreadyExists / ErrStaffAlreadyExists).
//
// The check is a pre-check in this flow (defense in depth): the SQLite
// single-owner triggers keep enforcing the invariant regardless of the call
// path, but the operator must read a business message, never a driver dump.
var ErrLastActiveOwner = fmt.Errorf("no se puede desactivar al único owner activo: %w", domain.ErrConflict)

// AccountsAdmin is the slice of repository.AccountsRepo that the Deactivate
// flow and the list views need: the full list (deactivate pre-check and the
// "all accounts" view), the role-filtered list, and the soft delete.
// *repository.AccountsRepo satisfies it unchanged; the narrow port keeps this
// package testable without a database (mirrors AccountsReader/AccountsCreator).
//
// Hard delete is deliberately absent: ADR-0016 Decision 1 fixes soft delete
// only, and the repository exposes no delete method to call.
type AccountsAdmin interface {
	List(ctx context.Context) ([]*entity.Account, error)
	GetByRole(ctx context.Context, role entity.AccountRole) ([]*entity.Account, error)
	Deactivate(ctx context.Context, id string) error
}

// AccountView is the operator-facing projection of one account row: exactly the
// columns the list views and the deactivate picker render (ADR-0016 Decision 1,
// "List views"). Nothing else leaves the port, so the console cannot print
// unrelated fields by accident (mirrors PickableProfessional).
type AccountView struct {
	ID             string
	Role           entity.AccountRole
	ProfessionalID string // "" when the account has no professional (owner/admin)
	Active         bool
	DisplayName    string
}

// ListFilter selects the rows of a list view. Role == "" means every role;
// IncludeInactive == false keeps only ACTIVE accounts, because a soft-deleted
// account is history and the operator has to opt in to see it.
type ListFilter struct {
	Role            entity.AccountRole
	IncludeInactive bool
}

// ListAccounts returns the accounts selected by filter, in repository order
// (created_at ASC: List and GetByRole share that ORDER BY), projected onto
// AccountView.
//
// The filter is validated before any port call, so an unknown role never
// reaches the repository and the operator reads the business message instead of
// the repo's input error.
func ListAccounts(ctx context.Context, accounts AccountsAdmin, filter ListFilter) ([]AccountView, error) {
	if err := validateAccountRole(filter.Role); err != nil {
		return nil, err
	}

	rows, err := fetchAccounts(ctx, accounts, filter.Role)
	if err != nil {
		return nil, err
	}
	return mapAccountViews(rows, filter.IncludeInactive), nil
}

// validateAccountRole accepts the three domain roles plus the empty role that
// means "every role". Any other value is an operator-level input error.
func validateAccountRole(role entity.AccountRole) error {
	switch role {
	case "", entity.RoleOwner, entity.RoleAdmin, entity.RoleStaff:
		return nil
	default:
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("el rol %q no es válido: usá owner, admin o staff", role),
		}
	}
}

// fetchAccounts reads the rows through the narrow port under the fabricated
// owner Caller (TUIContext), choosing the repository method that matches the
// filter. Every caller of this package's flows needs the same error context, so
// the wrap lives here.
func fetchAccounts(ctx context.Context, accounts AccountsAdmin, role entity.AccountRole) ([]*entity.Account, error) {
	if role == "" {
		rows, err := accounts.List(TUIContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("listar cuentas: %w", err)
		}
		return rows, nil
	}

	rows, err := accounts.GetByRole(TUIContext(ctx), role)
	if err != nil {
		return nil, fmt.Errorf("listar cuentas por rol: %w", err)
	}
	return rows, nil
}

// mapAccountViews projects the entity rows onto the operator view and applies
// the active filter. It is pure (no DB, no context), so the projection and the
// opt-in rule are unit-testable. The result is never nil: an empty view is an
// empty slice, matching the repository contract.
func mapAccountViews(rows []*entity.Account, includeInactive bool) []AccountView {
	views := make([]AccountView, 0, len(rows))
	for _, account := range rows {
		if !includeInactive && !account.Active {
			continue
		}
		views = append(views, viewOfAccount(account))
	}
	return views
}

// viewOfAccount maps one entity row onto the operator view, flattening the
// nullable professional id into the empty string the console renders as "-".
func viewOfAccount(account *entity.Account) AccountView {
	view := AccountView{
		ID:          account.ID,
		Role:        account.Role,
		Active:      account.Active,
		DisplayName: account.DisplayName,
	}
	if account.ProfessionalID != nil {
		view.ProfessionalID = *account.ProfessionalID
	}
	return view
}

// DeactivateOutcome reports what a soft delete actually did, so the console can
// tell "desactivada" from "ya estaba inactiva" without re-reading the row.
type DeactivateOutcome struct {
	Account         AccountView
	AlreadyInactive bool
}

// DeactivateAccount soft-deletes the account id through the narrow port,
// running under the fabricated owner Caller (TUIContext).
//
// Order of the guards matters:
//
//  1. a blank id is rejected before any port call;
//  2. the account must exist in the List snapshot — otherwise the operator gets
//     a semantic not-found instead of the repo's wrapped error;
//  3. an already INACTIVE account is an outcome, not an error and not a write:
//     the operator asked for a state the account is already in, and nothing is
//     changed (the repository's own Deactivate is idempotent as well);
//  4. the LAST active owner is refused before the write. Ownership transfer is
//     T5's flow; the TUI must never leave the system without an owner.
//
// Only then is Deactivate called. A port failure is wrapped with the flow
// context, never turned into a business message.
func DeactivateAccount(ctx context.Context, accounts AccountsAdmin, id string) (DeactivateOutcome, error) {
	accountID := strings.TrimSpace(id)
	if accountID == "" {
		return DeactivateOutcome{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el teléfono de la cuenta no puede estar vacío",
		}
	}

	// One snapshot answers all three pre-checks: the target row, its active
	// state and how many active owners exist. The accounts table holds the
	// operator and a handful of staff accounts, so the full list is cheap.
	rows, err := accounts.List(TUIContext(ctx))
	if err != nil {
		return DeactivateOutcome{}, fmt.Errorf("listar cuentas: %w", err)
	}

	target, activeOwners, found := findAccount(rows, accountID)
	if !found {
		return DeactivateOutcome{}, &domain.SemanticError{
			Code:    domain.ErrCodeNotFound,
			Message: "no existe una cuenta con ese teléfono",
		}
	}

	if !target.Active {
		return DeactivateOutcome{Account: viewOfAccount(target), AlreadyInactive: true}, nil
	}

	if target.Role == entity.RoleOwner && activeOwners == 1 {
		return DeactivateOutcome{}, &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: "no se puede desactivar al único owner activo: el sistema quedaría sin owner",
			Cause:   ErrLastActiveOwner,
		}
	}

	if err := accounts.Deactivate(TUIContext(ctx), accountID); err != nil {
		return DeactivateOutcome{}, fmt.Errorf("desactivar la cuenta: %w", err)
	}
	return DeactivateOutcome{Account: viewOfAccount(target)}, nil
}

// findAccount locates one row by id and counts the ACTIVE owners in the same
// pass. found is false when the id is not in the snapshot; activeOwners counts
// every active owner of the snapshot, including target itself.
func findAccount(rows []*entity.Account, id string) (target *entity.Account, activeOwners int, found bool) {
	for _, account := range rows {
		if account.Role == entity.RoleOwner && account.Active {
			activeOwners++
		}
		if account.ID == id {
			target = account
		}
	}
	return target, activeOwners, target != nil
}
