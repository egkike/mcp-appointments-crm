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
