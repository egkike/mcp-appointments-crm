package admin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// TUICallerID is the synthetic audit identity every admin TUI operation runs
// under. It never comes from the HTTP transport, so it can never be forged by
// a remote caller.
const TUICallerID = "admin-tui"

// ErrOwnerAlreadyExists reports that the seed found (or raced into) an existing
// active owner. It wraps domain.ErrConflict so callers can branch on either
// sentinel without parsing prose.
var ErrOwnerAlreadyExists = fmt.Errorf("ya existe un owner activo: %w", domain.ErrConflict)

// TUIContext returns ctx carrying the fabricated owner Caller the TUI uses.
//
// ADR-0016 Decision 3.1 resolves the first-owner chicken-and-egg: every
// AccountsRepo mutation requires an auth.Caller in the context, but the first
// owner cannot come from the HTTP middleware. The TUI is the local privileged
// path — the OS administrator is the gatekeeper (ADR-0010) — and is the only
// non-HTTP construction site of auth.Caller. The SQLite single-owner triggers
// keep enforcing the invariant regardless of the call path.
func TUIContext(ctx context.Context) context.Context {
	return auth.WithCaller(ctx, auth.Caller{ID: TUICallerID, Role: auth.RoleOwner})
}

// AccountsReader is the read slice of repository.AccountsRepo that the seed
// decision needs. A narrow interface keeps this package testable without a DB.
type AccountsReader interface {
	GetByRole(ctx context.Context, role entity.AccountRole) ([]*entity.Account, error)
}

// AccountsCreator is the write slice of repository.AccountsRepo that the seed
// flow needs.
type AccountsCreator interface {
	Create(ctx context.Context, a *entity.Account) error
}

// NeedsSeed reports whether the first-boot owner gateway must run: true when no
// ACTIVE owner account exists. Deactivated owners do not count — the seed
// gateway creates the first owner, and re-activation is the transfer flow
// (T5), which owns the single-owner invariant.
func NeedsSeed(ctx context.Context, accounts AccountsReader) (bool, error) {
	owners, err := accounts.GetByRole(TUIContext(ctx), entity.RoleOwner)
	if err != nil {
		return false, fmt.Errorf("verificar owner activo: %w", err)
	}
	for _, a := range owners {
		if a.Active {
			return false, nil
		}
	}
	return true, nil
}

// EnsureCallerID closes the partial-seed wedge of finding R4-partial-seed-wedge:
// Seed can commit the owner and then the console flow fails to write the
// caller-id file, so the next run sees an active owner, exits clean, and never
// repairs the missing file — leaving Hermes without a resolvable caller id.
// When the file already exists it returns (false, nil) and touches nothing.
// Otherwise it resolves the active owner's caller id (the account id IS the
// phone, fact 7) through the narrow AccountsReader port and writes it with
// WriteCallerID. written is true only when the file was written.
func EnsureCallerID(ctx context.Context, accounts AccountsReader) (written bool, err error) {
	path, err := CallerIDPath()
	if err != nil {
		return false, err
	}

	switch _, statErr := os.Stat(path); {
	case statErr == nil:
		return false, nil
	case !errors.Is(statErr, os.ErrNotExist):
		return false, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo verificar el archivo caller-id",
			Cause:   statErr,
		}
	}

	owners, err := accounts.GetByRole(TUIContext(ctx), entity.RoleOwner)
	if err != nil {
		return false, fmt.Errorf("resolver el owner para reparar el caller id: %w", err)
	}

	for _, owner := range owners {
		if !owner.Active {
			continue
		}
		if _, err := WriteCallerID(owner.ID); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, &domain.SemanticError{
		Code:    domain.ErrCodeConflict,
		Message: "no hay un owner activo cuyo caller id se pueda reparar",
	}
}

// AccountsActivator is the slice of repository.AccountsRepo that the deadend
// recovery needs: the owner snapshot plus the update that flips an inactive
// owner row back to active. *repository.AccountsRepo satisfies it unchanged.
type AccountsActivator interface {
	AccountsReader
	Update(ctx context.Context, a *entity.Account) error
}

// ErrOwnerNotReactivatable reports an account that cannot be reactivated by the
// deadend recovery: it is not an inactive owner row.
var ErrOwnerNotReactivatable = fmt.Errorf("la cuenta no es un owner inactivo reactivable: %w", domain.ErrConflict)

// InactiveOwners returns the owner-role accounts that are currently INACTIVE,
// in repository order, projected onto the operator view.
//
// This is the accessor of finding R4-deactivated-owner-seed-deadend: NeedsSeed
// keeps its contract unchanged (it answers "is there an ACTIVE owner?"), while
// the console asks this second question to tell a clean install apart from an
// installation whose owner was deactivated. Without it the seed gateway would
// blindly INSERT the same phone and hit accounts.id PRIMARY KEY.
func InactiveOwners(ctx context.Context, accounts AccountsReader) ([]AccountView, error) {
	owners, err := accounts.GetByRole(TUIContext(ctx), entity.RoleOwner)
	if err != nil {
		return nil, fmt.Errorf("listar owners inactivos: %w", err)
	}

	inactive := make([]AccountView, 0, len(owners))
	for _, owner := range owners {
		if owner.Active {
			continue
		}
		inactive = append(inactive, viewOfAccount(owner))
	}
	return inactive, nil
}

// ReactivateOwner flips an existing INACTIVE owner-role account back to ACTIVE.
// It is the sanctioned recovery of the deadend state (R4-deactivated-owner-seed-deadend):
// the installation has no active owner but its owner row still exists, so the
// operator reactivates that row instead of creating a second one that the
// PRIMARY KEY would reject anyway.
//
// The guards, in order:
//
//  1. a blank id is rejected before any port call;
//  2. the id must match an owner-role row of the snapshot;
//  3. that row must be INACTIVE — an already-active account has nothing to
//     reactivate, and the operator reads a business message;
//  4. no OTHER active owner may exist: changing owners of a healthy
//     installation is TransferOwnership, never a blind activation. This mirrors
//     the accounts_single_owner_update trigger, as a semantic message.
//
// The display name is preserved as-is: reactivating is not an edit.
func ReactivateOwner(ctx context.Context, accounts AccountsActivator, id string) (AccountView, error) {
	accountID := strings.TrimSpace(id)
	if accountID == "" {
		return AccountView{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el teléfono de la cuenta no puede estar vacío",
		}
	}

	owners, err := accounts.GetByRole(TUIContext(ctx), entity.RoleOwner)
	if err != nil {
		return AccountView{}, fmt.Errorf("listar owners: %w", err)
	}

	var target *entity.Account
	for _, owner := range owners {
		if owner.Active {
			return AccountView{}, &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "ya existe un owner activo: para cambiar de owner usá la transferencia de propiedad",
				Cause:   ErrOwnerAlreadyExists,
			}
		}
		if owner.ID == accountID {
			target = owner
		}
	}

	if target == nil {
		return AccountView{}, &domain.SemanticError{
			Code:    domain.ErrCodeNotFound,
			Message: "no existe una cuenta de owner inactiva con ese teléfono",
		}
	}

	reactivated := &entity.Account{
		ID:          target.ID,
		Role:        entity.RoleOwner,
		DisplayName: target.DisplayName,
		Active:      true,
	}
	if err := accounts.Update(TUIContext(ctx), reactivated); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return AccountView{}, &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "ya existe un owner activo: no se reactivó ninguna cuenta",
				Cause:   ErrOwnerAlreadyExists,
			}
		}
		return AccountView{}, fmt.Errorf("reactivar la cuenta de owner: %w", err)
	}
	return viewOfAccount(reactivated), nil
}

// SeedInput holds the operator-provided fields of the first owner. The
// professional picker is T3: an owner has no professional_id, so there is no
// field for it here.
type SeedInput struct {
	Phone       string
	DisplayName string
}

// ValidatePhone reuses entity.Client.HasValidPhone — the single phone
// validation point of the codebase (fact 7: accounts has no phone column, the
// account id IS the phone). No second regex lives here on purpose.
func ValidatePhone(phone string) error {
	c := entity.Client{Phone: phone}
	if !c.HasValidPhone() {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el teléfono no es válido: usá de 4 a 15 dígitos con un '+' opcional al inicio",
		}
	}
	return nil
}

// ValidateDisplayName rejects the blank display names that the operator can
// produce by pressing Enter on the prompt.
func ValidateDisplayName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el nombre para mostrar no puede estar vacío",
		}
	}
	return nil
}

// ValidateSeedInput validates every operator-provided field before the write.
func ValidateSeedInput(input SeedInput) error {
	if err := ValidatePhone(input.Phone); err != nil {
		return err
	}
	return ValidateDisplayName(input.DisplayName)
}

// Seed creates the first owner account through AccountsRepo.Create, running
// under the fabricated owner Caller (TUIContext).
//
// A second-active-owner conflict — caught either by the repo pre-check or by
// the SQLite trigger — is reported as ErrOwnerAlreadyExists wrapped in a
// *domain.SemanticError, never as a driver dump. The underlying cause is
// preserved so `errors.Is(err, domain.ErrConflict)` keeps working.
//
// Writing the caller-id file is a separate step (WriteCallerID) owned by the
// console flow, which only runs it after Seed succeeds: the file must never
// advertise a caller id for an owner that was not created.
func Seed(ctx context.Context, accounts AccountsCreator, input SeedInput) error {
	if err := ValidateSeedInput(input); err != nil {
		return err
	}

	account := &entity.Account{
		ID:          input.Phone,
		Role:        entity.RoleOwner,
		DisplayName: strings.TrimSpace(input.DisplayName),
		Active:      true,
	}

	if err := accounts.Create(TUIContext(ctx), account); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "ya existe un owner activo o una cuenta con ese teléfono; no se creó ninguna cuenta nueva",
				Cause:   ErrOwnerAlreadyExists,
			}
		}
		return fmt.Errorf("crear owner: %w", err)
	}
	return nil
}
