package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ErrClientPhoneTaken reports that the operator's phone already belongs to a
// DIFFERENT clients row, so the double role of ADR-0011 cannot be created from
// this account. It wraps domain.ErrConflict so callers can branch on either
// sentinel without parsing prose (mirrors ErrOwnerAlreadyExists /
// ErrStaffAlreadyExists / ErrLastActiveOwner).
var ErrClientPhoneTaken = fmt.Errorf("el teléfono ya está registrado como cliente: %w", domain.ErrConflict)

// AccountsByIDReader is the read slice of repository.AccountsRepo that the
// add-self flow needs: the account row whose id is the caller id. The flow
// validates that id against the accounts table instead of trusting console
// input (ADR-0016 Decision 3.1: the TUI is the local privileged path, but it
// still validates its own inputs). *repository.AccountsRepo satisfies it
// unchanged.
type AccountsByIDReader interface {
	FindByID(ctx context.Context, id string) (*entity.Account, error)
}

// ClientsSelfService is the client-side slice of repository.ClientsRepo that the
// add-self flow needs: the read that answers "is this caller id already a
// client?" and the write that creates the row. *repository.ClientsRepo
// satisfies it unchanged; the narrow port keeps this package testable without a
// database (mirrors AccountsReader/AccountsCreator).
//
// GetOrCreate is deliberately absent: it inserts a UUID id, and the discovery
// of ADR-0011 (verified fact 6) matches clients.id against the caller id — a
// UUID row is invisible to CallerResolver step 2, so it would leave ClientID
// nil while pretending to have registered the client.
type ClientsSelfService interface {
	FindByID(ctx context.Context, id string) (*entity.Client, error)
	Create(ctx context.Context, c *entity.Client) error
}

// AddSelfOutcome reports what the add-self flow did, so the console can tell a
// fresh registration from an idempotent one without re-reading the row.
type AddSelfOutcome struct {
	ClientID          string
	AlreadyRegistered bool
}

// AddSelfAsClient registers the operator's own account as a client of the
// business: it creates a clients row whose id EQUALS the caller's account id
// (the phone) and whose phone column holds the same phone.
//
// That double role is what gives the caller a ClientID (ADR-0011: owner, admin
// and staff are clients of their own business). CallerResolver step 2 discovers
// it with `SELECT id FROM clients WHERE id = <caller id>` (verified fact 6), so
// any other id — the UUID of GetOrCreate, for instance — leaves the resolution
// with ClientID nil.
//
// Guards, in order:
//
//  1. the caller id must be a valid phone (verified fact 7: accounts has no
//     phone column, the account id IS the phone) and the display name
//     non-blank;
//  2. the caller id must belong to an EXISTING, ACTIVE account, validated
//     through the accounts port, never trusted from the console;
//  3. an existing clients row carrying that id is an OUTCOME, not an error:
//     the operator asked for a state the system is already in, so nothing is
//     written (idempotent) and the outcome is reported;
//  4. only then is the row created. A duplicate — clients.phone is UNIQUE, and
//     clients.id is the PRIMARY KEY when the row appears between the read and
//     the write — surfaces as a semantic conflict, never as a driver dump.
func AddSelfAsClient(ctx context.Context, accounts AccountsByIDReader, clients ClientsSelfService, callerID, name string) (AddSelfOutcome, error) {
	phone := strings.TrimSpace(callerID)
	if phone == "" {
		return AddSelfOutcome{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el teléfono de la cuenta del operador no puede estar vacío",
		}
	}
	// The caller id is the phone, so the single phone validation point applies
	// to it (verified fact 7): no second regex lives here.
	if err := ValidatePhone(phone); err != nil {
		return AddSelfOutcome{}, err
	}

	displayName := strings.TrimSpace(name)
	if err := ValidateDisplayName(displayName); err != nil {
		return AddSelfOutcome{}, err
	}

	account, err := accounts.FindByID(TUIContext(ctx), phone)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return AddSelfOutcome{}, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: fmt.Sprintf("no existe una cuenta con el teléfono %s: creá primero la cuenta y después agregala como cliente", phone),
			}
		}
		return AddSelfOutcome{}, fmt.Errorf("verificar la cuenta del operador: %w", err)
	}
	if !account.Active {
		// A disabled account is rejected by CallerResolver step 1 before it ever
		// looks at the clients table, so registering it would be a phantom
		// success: the operator would never be able to call with that id.
		return AddSelfOutcome{}, &domain.SemanticError{
			Code:    domain.ErrCodeForbidden,
			Message: "tu cuenta está deshabilitada: no se puede registrar como cliente",
			Cause:   domain.ErrForbidden,
		}
	}

	existing, err := clients.FindByID(TUIContext(ctx), phone)
	switch {
	case err == nil:
		// The row already carries exactly the id the discovery matches on.
		return AddSelfOutcome{ClientID: existing.ID, AlreadyRegistered: true}, nil
	case errors.Is(err, domain.ErrNotFound):
		// No clients row for this id: create it below.
	default:
		return AddSelfOutcome{}, fmt.Errorf("verificar el cliente del operador: %w", err)
	}

	// The id and the phone are the same value on purpose: the id is what
	// CallerResolver step 2 matches, the phone is the UNIQUE business key.
	client := &entity.Client{ID: phone, Name: displayName, Phone: phone, Active: true}
	if err := clients.Create(TUIContext(ctx), client); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return AddSelfOutcome{}, &domain.SemanticError{
				Code: domain.ErrCodeConflict,
				Message: "el teléfono ya está registrado como cliente con otra ficha: revisá esa ficha de cliente " +
					"antes de volver a intentarlo",
				Cause: ErrClientPhoneTaken,
			}
		}
		return AddSelfOutcome{}, fmt.Errorf("registrar el cliente del operador: %w", err)
	}
	return AddSelfOutcome{ClientID: client.ID}, nil
}
