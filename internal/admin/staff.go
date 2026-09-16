package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ErrStaffAlreadyExists reports that the staff phone already belongs to an
// account. It wraps domain.ErrConflict so callers can branch on either sentinel
// without parsing prose (mirrors ErrOwnerAlreadyExists).
var ErrStaffAlreadyExists = fmt.Errorf("ya existe una cuenta con ese teléfono: %w", domain.ErrConflict)

// ProfessionalsReader is the read slice of repository.ProfessionalsRepo that
// the staff picker needs. FindActive already enforces the active filter and the
// ORDER BY name, and restricts a staff caller to its own row; the TUI runs as
// owner, so it receives every active professional. A narrow port keeps this
// package testable without a database (mirrors AccountsReader).
type ProfessionalsReader interface {
	FindActive(ctx context.Context) ([]*entity.Professional, error)
}

// PickableProfessional is the picker view of one professional: the id the staff
// account points at, the name the operator recognises, and the phone used to
// prefill the phone prompt (ADR-0016 Decision 3.2). Only these three fields
// leave the port, so the console cannot render unrelated columns by accident.
type PickableProfessional struct {
	ID    string
	Name  string
	Phone string
}

// StaffInput holds the operator-provided fields of a new staff account.
type StaffInput struct {
	ProfessionalID string
	Phone          string
	DisplayName    string
}

// ListPickableProfessionals returns the ACTIVE professionals in repository
// order (name ascending) mapped to the picker view. It runs under the
// fabricated owner Caller, so FindActive returns every active row.
//
// The list is the only sanctioned source of professional ids for Add Staff:
// accounts.professional_id has no FK (fact 8), so a free-text id could create a
// dangling reference.
func ListPickableProfessionals(ctx context.Context, professionals ProfessionalsReader) ([]PickableProfessional, error) {
	active, err := fetchActiveProfessionals(ctx, professionals)
	if err != nil {
		return nil, err
	}
	return mapPickableProfessionals(active), nil
}

// mapPickableProfessionals projects the entity rows onto the picker view. It is
// pure (no DB, no context), so the id/name/phone mapping is unit-testable.
func mapPickableProfessionals(active []*entity.Professional) []PickableProfessional {
	pickable := make([]PickableProfessional, 0, len(active))
	for _, p := range active {
		item := PickableProfessional{ID: p.ID, Name: p.Name}
		if p.Phone != nil {
			item.Phone = *p.Phone
		}
		pickable = append(pickable, item)
	}
	return pickable
}

// fetchActiveProfessionals reads the active rows through the narrow port under
// the fabricated owner Caller. Every caller of this package's flows needs the
// same error context, so the wrap lives here.
func fetchActiveProfessionals(ctx context.Context, professionals ProfessionalsReader) ([]*entity.Professional, error) {
	active, err := professionals.FindActive(TUIContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listar profesionales activos: %w", err)
	}
	return active, nil
}

// errProfessionalUnavailable is the single semantic error for "the selected
// professional is not a valid active row". It is built by a function (not a
// shared pointer) so no caller can mutate a package-level error.
func errProfessionalUnavailable() error {
	return &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: "el profesional seleccionado no existe o no está activo",
	}
}

// ValidateStaffInput validates every operator-provided field before the write.
// A blank professional id is rejected here so an obviously invalid input never
// reaches the ports.
func ValidateStaffInput(input StaffInput) error {
	if strings.TrimSpace(input.ProfessionalID) == "" {
		return errProfessionalUnavailable()
	}
	if err := ValidatePhone(input.Phone); err != nil {
		return err
	}
	return ValidateDisplayName(input.DisplayName)
}

// ensureActiveProfessional re-validates the selected professional against the
// port. The picker lists active rows, but the row can change between the listing
// and the write; accounts.professional_id has no FK (fact 8), so this explicit
// check is the only guard against a dangling reference.
func ensureActiveProfessional(ctx context.Context, professionals ProfessionalsReader, professionalID string) error {
	active, err := fetchActiveProfessionals(ctx, professionals)
	if err != nil {
		return err
	}
	for _, p := range active {
		if p.ID == professionalID && p.IsActive() {
			return nil
		}
	}
	return errProfessionalUnavailable()
}

// AddStaff creates a staff account linked to an existing ACTIVE professional,
// running under the fabricated owner Caller (TUIContext).
//
// The phone (the account id, fact 7) must be unique across accounts; a
// uniqueness conflict — raised by the repo pre-check or by the SQLite UNIQUE
// constraint — is reported as ErrStaffAlreadyExists wrapped in a
// *domain.SemanticError, never as a driver dump. The cause is preserved so
// errors.Is(err, domain.ErrConflict) keeps working.
func AddStaff(ctx context.Context, professionals ProfessionalsReader, accounts AccountsCreator, input StaffInput) error {
	if err := ValidateStaffInput(input); err != nil {
		return err
	}

	if err := ensureActiveProfessional(ctx, professionals, input.ProfessionalID); err != nil {
		return err
	}

	professionalID := input.ProfessionalID
	account := &entity.Account{
		ID:             input.Phone,
		Role:           entity.RoleStaff,
		DisplayName:    strings.TrimSpace(input.DisplayName),
		ProfessionalID: &professionalID,
		Active:         true,
	}

	if err := accounts.Create(TUIContext(ctx), account); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "ya existe una cuenta con ese teléfono; no se creó ninguna cuenta nueva",
				Cause:   ErrStaffAlreadyExists,
			}
		}
		return fmt.Errorf("crear cuenta de staff: %w", err)
	}
	return nil
}
