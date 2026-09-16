package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Sentinels of the ownership transfer flow. Each one wraps domain.ErrConflict
// so callers branch on either sentinel without parsing prose (mirrors
// ErrOwnerAlreadyExists / ErrStaffAlreadyExists / ErrLastActiveOwner).
var (
	// ErrNoActiveOwner reports that the flow found no ACTIVE owner to transfer
	// from. The seed gateway owns that state; the transfer can never create it.
	ErrNoActiveOwner = fmt.Errorf("no hay un owner activo: %w", domain.ErrConflict)

	// ErrMultipleActiveOwners reports an installation that violates the
	// single-owner invariant (the SQLite triggers forbid it; only manual SQL or
	// corruption can produce it). Refusing is the fail-secure choice: swapping
	// from one of two active owners would leave two active owners behind.
	ErrMultipleActiveOwners = fmt.Errorf("hay más de un owner activo: %w", domain.ErrConflict)

	// ErrSuccessorNotTransferable reports a successor that is not an INACTIVE
	// owner-role account.
	ErrSuccessorNotTransferable = fmt.Errorf("el sucesor no es una cuenta de owner inactiva: %w", domain.ErrConflict)

	// ErrSuccessorPhoneTaken reports that the phone of a brand-new successor
	// already belongs to an account.
	ErrSuccessorPhoneTaken = fmt.Errorf("el teléfono del sucesor ya pertenece a una cuenta: %w", domain.ErrConflict)
)

// AccountsTransfer is the slice of repository.AccountsRepo that the ownership
// transfer flow needs: the snapshot that resolves the active owner and the
// successor candidates, the creation of a fresh successor row (step 1), the
// promotion of an existing staff row (step 1), and the transactional swap
// (step 2). *repository.AccountsRepo satisfies it unchanged; the narrow port
// keeps this package testable without a database (mirrors AccountsAdmin).
type AccountsTransfer interface {
	List(ctx context.Context) ([]*entity.Account, error)
	Create(ctx context.Context, a *entity.Account) error
	Update(ctx context.Context, a *entity.Account) error
	TransferOwnership(ctx context.Context, fromID, toID string) error
}

// SuccessorKind identifies how the operator named the successor. The three
// kinds mirror the three options the console offers (ADR-0016 Decision 1).
type SuccessorKind string

const (
	// SuccessorInactiveOwner reuses an existing role=owner, is_active=0 row.
	// Nothing is written: the row is already prepared.
	SuccessorInactiveOwner SuccessorKind = "inactive_owner"

	// SuccessorStaff promotes an existing staff account. The row is switched to
	// role=owner with is_active=0 (never active): the single-owner triggers
	// count only ACTIVE owners, so the promotion coexists with the current
	// owner.
	SuccessorStaff SuccessorKind = "staff"

	// SuccessorNewPhone creates a role=owner, is_active=0 row for a phone that
	// has no account yet.
	SuccessorNewPhone SuccessorKind = "new_phone"
)

// TransferSuccessor names the successor the operator picked: ID for the two
// "existing account" kinds, Phone plus DisplayName for a brand-new one.
type TransferSuccessor struct {
	Kind        SuccessorKind
	ID          string
	Phone       string
	DisplayName string
}

// TransferOutcome reports the two accounts the swap touched, projected onto the
// operator view so the console can name them.
type TransferOutcome struct {
	From AccountView
	To   AccountView
}

// ActiveOwner returns the single ACTIVE owner of the installation, projected
// onto the operator view. A missing owner fails with ErrNoActiveOwner and an
// installation with two active owners with ErrMultipleActiveOwners: this flow
// never guesses which one to deactivate.
func ActiveOwner(ctx context.Context, accounts AccountsTransfer) (AccountView, error) {
	rows, err := fetchTransferAccounts(ctx, accounts)
	if err != nil {
		return AccountView{}, err
	}

	owner, err := activeOwnerOf(rows)
	if err != nil {
		return AccountView{}, err
	}
	return viewOfAccount(owner), nil
}

// PrepareSuccessor runs the first step of the transfer: it guarantees that an
// INACTIVE owner-role row exists for the successor the operator picked, and
// returns it projected onto the operator view. The swap itself is
// TransferOwnership.
//
// An existing inactive owner row is validated and reused; a staff row is
// promoted; a fresh phone gets a new owner row created inactive. Every kind
// leaves the single-owner invariant untouched: at most one ACTIVE owner exists
// before, during and after this step.
//
// A failure here never left an ACTIVE account behind, so a partially prepared
// successor is a retryable inactive owner row, not a broken installation.
func PrepareSuccessor(ctx context.Context, accounts AccountsTransfer, successor TransferSuccessor) (AccountView, error) {
	switch successor.Kind {
	case SuccessorInactiveOwner:
		return prepareInactiveOwnerSuccessor(ctx, accounts, successor.ID)
	case SuccessorStaff:
		return promoteStaffSuccessor(ctx, accounts, successor.ID)
	case SuccessorNewPhone:
		return createOwnerSuccessor(ctx, accounts, successor)
	default:
		return AccountView{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("el tipo de sucesor %q no es válido: usá una cuenta existente o un teléfono nuevo", successor.Kind),
		}
	}
}

// TransferOwnership runs the second and last step: it validates fromID is the
// ACTIVE owner and toID an existing INACTIVE owner-role account, then calls the
// transactional swap of the repository under the fabricated TUI caller.
//
// The checks here are a pre-check in this flow (defense in depth): the
// repository re-reads both rows inside the transaction and the SQLite trigger
// keeps the invariant regardless of the call path — but the operator must read
// a business message, never a driver dump.
func TransferOwnership(ctx context.Context, accounts AccountsTransfer, fromID, toID string) (TransferOutcome, error) {
	rows, err := fetchTransferAccounts(ctx, accounts)
	if err != nil {
		return TransferOutcome{}, err
	}

	from, err := activeOwnerOf(rows)
	if err != nil {
		return TransferOutcome{}, err
	}
	if from.ID != strings.TrimSpace(fromID) {
		return TransferOutcome{}, &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: fmt.Sprintf("la cuenta %q no es el owner activo: el owner activo es %q", fromID, from.ID),
			Cause:   ErrNoActiveOwner,
		}
	}

	to, err := inactiveOwnerOf(rows, toID)
	if err != nil {
		return TransferOutcome{}, err
	}

	if err := accounts.TransferOwnership(TUIContext(ctx), from.ID, to.ID); err != nil {
		return TransferOutcome{}, fmt.Errorf("transferir la propiedad: %w", err)
	}

	// The outcome reports the COMMITTED state, not the pre-swap snapshot: the
	// console names the previous owner and the new one from it.
	fromView := viewOfAccount(from)
	fromView.Active = false
	to.Active = true
	return TransferOutcome{From: fromView, To: to}, nil
}

// fetchTransferAccounts reads the account snapshot through the narrow port
// under the fabricated owner Caller (TUIContext). Every flow of this file needs
// the same error context, so the wrap lives here.
func fetchTransferAccounts(ctx context.Context, accounts AccountsTransfer) ([]*entity.Account, error) {
	rows, err := accounts.List(TUIContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listar cuentas: %w", err)
	}
	return rows, nil
}

// activeOwnerOf locates the single ACTIVE owner of the snapshot. Two of them
// means a corrupted installation, so it refuses instead of picking one.
func activeOwnerOf(rows []*entity.Account) (*entity.Account, error) {
	var owner *entity.Account
	for _, account := range rows {
		if account.Role != entity.RoleOwner || !account.Active {
			continue
		}
		if owner != nil {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "hay más de un owner activo: la instalación está inconsistente, no se transfirió nada",
				Cause:   ErrMultipleActiveOwners,
			}
		}
		owner = account
	}
	if owner == nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: "no hay un owner activo al que transferirle la propiedad",
			Cause:   ErrNoActiveOwner,
		}
	}
	return owner, nil
}

// inactiveOwnerOf resolves one snapshot row into a valid transfer target: it
// must exist, have role=owner and be INACTIVE. Used by both the pre-check of
// TransferOwnership and the "existing successor" preparation, so the rule lives
// in exactly one place.
func inactiveOwnerOf(rows []*entity.Account, id string) (AccountView, error) {
	accountID := strings.TrimSpace(id)
	if accountID == "" {
		return AccountView{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el teléfono del sucesor no puede estar vacío",
		}
	}

	for _, account := range rows {
		if account.ID != accountID {
			continue
		}
		if account.Role != entity.RoleOwner {
			return AccountView{}, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la cuenta elegida no tiene rol owner: promovela a owner inactivo antes de transferirle la propiedad",
			}
		}
		if account.Active {
			return AccountView{}, &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "la cuenta elegida ya es un owner activo: no hay nada que transferirle",
				Cause:   ErrSuccessorNotTransferable,
			}
		}
		return viewOfAccount(account), nil
	}

	return AccountView{}, &domain.SemanticError{
		Code:    domain.ErrCodeNotFound,
		Message: "no existe una cuenta de owner inactiva con ese teléfono",
	}
}

// prepareInactiveOwnerSuccessor validates the reuse of an existing inactive
// owner row. It never writes: the row is already exactly what the swap needs.
func prepareInactiveOwnerSuccessor(ctx context.Context, accounts AccountsTransfer, id string) (AccountView, error) {
	rows, err := fetchTransferAccounts(ctx, accounts)
	if err != nil {
		return AccountView{}, err
	}
	return inactiveOwnerOf(rows, id)
}

// promoteStaffSuccessor turns an existing staff account into an INACTIVE owner
// account (step 1 of the transfer).
//
// The write is safe while the current owner is still active because the
// accounts_single_owner_update trigger only rejects a NEW ACTIVE owner: the row
// is written with is_active=0 and role=owner in one statement. The professional
// link is dropped on purpose — entity.Account documents professional_id as
// staff-only ("nil for admin/owner"), and an owner caller must not inherit a
// professional scope.
func promoteStaffSuccessor(ctx context.Context, accounts AccountsTransfer, id string) (AccountView, error) {
	accountID := strings.TrimSpace(id)
	if accountID == "" {
		return AccountView{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el teléfono de la cuenta a promover no puede estar vacío",
		}
	}

	rows, err := fetchTransferAccounts(ctx, accounts)
	if err != nil {
		return AccountView{}, err
	}

	var staff *entity.Account
	for _, account := range rows {
		if account.ID == accountID {
			staff = account
			break
		}
	}
	if staff == nil {
		return AccountView{}, &domain.SemanticError{
			Code:    domain.ErrCodeNotFound,
			Message: "no existe una cuenta con ese teléfono",
		}
	}
	if staff.Role != entity.RoleStaff {
		return AccountView{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "solo se puede promover a owner una cuenta de staff",
		}
	}

	promoted := &entity.Account{
		ID:          staff.ID,
		Role:        entity.RoleOwner,
		DisplayName: staff.DisplayName,
		Active:      false,
	}
	if err := accounts.Update(TUIContext(ctx), promoted); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return AccountView{}, &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "no se pudo promover la cuenta a owner inactivo: ya existe un owner activo con ese estado",
			}
		}
		return AccountView{}, fmt.Errorf("promover la cuenta de staff a owner: %w", err)
	}
	return viewOfAccount(promoted), nil
}

// createOwnerSuccessor creates the successor account for a phone that has no
// account yet (step 1 of the transfer).
//
// The row is created with is_active=0 on purpose: the single-owner triggers
// count only ACTIVE owners, so the prepared owner row coexists with the current
// owner until the swap. A phone that already belongs to an account — an
// inactive owner, or a staff account the operator should promote — surfaces as
// a semantic conflict with the guidance to pick that account instead.
func createOwnerSuccessor(ctx context.Context, accounts AccountsTransfer, successor TransferSuccessor) (AccountView, error) {
	phone := strings.TrimSpace(successor.Phone)
	displayName := strings.TrimSpace(successor.DisplayName)

	if err := ValidatePhone(phone); err != nil {
		return AccountView{}, err
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return AccountView{}, err
	}

	account := &entity.Account{
		ID:          phone,
		Role:        entity.RoleOwner,
		DisplayName: displayName,
		Active:      false,
	}
	if err := accounts.Create(TUIContext(ctx), account); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return AccountView{}, &domain.SemanticError{
				Code: domain.ErrCodeConflict,
				Message: "ese teléfono ya pertenece a una cuenta; si querés transferirle la propiedad a esa " +
					"persona, elegí esa cuenta de la lista de sucesores, o usá un teléfono nuevo",
				Cause: ErrSuccessorPhoneTaken,
			}
		}
		return AccountView{}, fmt.Errorf("crear la cuenta de owner sucesora: %w", err)
	}
	return viewOfAccount(account), nil
}
