package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// GetProfessionalUseCase reads ONE professional record (feat-staff-self-read S1,
// issue #276). It closes the gap the staff role had: a staff caller could not
// resolve its own profile from the chat, and the catalog read
// (search_professionals_advanced) is the wrong answer to "who am I" — it only
// lists ACTIVE professionals, drops the caller's own status and never carries
// contact data.
//
// Role: OWNER, ADMIN or STAFF, enforced twice by design (AGENTS.md boundary
// contract 1): the ToolRBAC row in the composition root and auth.RequireRole
// below. The client stays outside the surface entirely (S5): the conversational
// booking flow resolves professionals through the PII-free catalog.
//
// Scope and PII are TWO questions, both answered by the shared predicates in
// internal/auth and never re-derived here:
//   - auth.AuthorizeProfessionalRead decides WHICH record may be read (staff is
//     own-only, and the refusal is FORBIDDEN with the shipped message — S2);
//   - auth.CanReadProfessionalContact decides WHICH FIELDS cross (owner and the
//     professional itself only — S4).
type GetProfessionalUseCase struct {
	professionals domainrepo.ProfessionalsRepo
}

// NewGetProfessionalUseCase constructs the use case.
func NewGetProfessionalUseCase(professionals domainrepo.ProfessionalsRepo) *GetProfessionalUseCase {
	return &GetProfessionalUseCase{professionals: professionals}
}

// Execute returns the professional record plus the contact fields the caller is
// allowed to see.
//
// Semantics (S1/S2/S4):
//   - an empty professional_id asks for the caller's own record; a caller with
//     no professional link (owner, admin, client) gets a semantic INVALID_INPUT
//     naming the missing field instead of an empty answer;
//   - a staff caller naming a foreign id is refused BEFORE the read, so the
//     answer is FORBIDDEN and not the repository's UNAUTHENTICATED sentinel: the
//     caller is authenticated, its scope is what falls short;
//   - an unknown id is NOT_FOUND, distinct from "no specialties loaded";
//   - inactive professionals ARE returned with their status: the record is never
//     hidden from a caller allowed to read it.
func (uc *GetProfessionalUseCase) Execute(ctx context.Context, input dto.GetProfessionalInput) (*dto.GetProfessionalResult, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff)
	if err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: auth.MsgForbiddenTool, Cause: domain.ErrForbidden}
	}

	professionalID, err := auth.AuthorizeProfessionalRead(*caller, input.ProfessionalID)
	if err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, fmt.Errorf("get_professional: %w", err)
	}

	professional, err := uc.professionals.FindByID(ctx, professionalID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "el profesional indicado no existe",
				Cause:   err,
			}
		}
		// FindByID carries its own repository-level guard. It cannot fire on this
		// path (the scope above already admitted the id), but passing its semantic
		// error through unchanged keeps the surface honest if that ever changes,
		// instead of degrading a refusal into an internal error.
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, fmt.Errorf("get_professional: %w", err)
	}
	if professional == nil {
		// A port that reports success without a record is a bug, and a zero-value
		// professional would be a well-formed lie. Fail closed.
		return nil, fmt.Errorf("get_professional: el repositorio no devolvió el profesional %s", professionalID)
	}

	result := &dto.GetProfessionalResult{
		Professional: dto.ProfessionalDetail{
			ID:            professional.ID,
			Name:          professional.Name,
			RoleSpecialty: professional.RoleSpecialty,
			Status:        professional.Status,
			Specialties:   professionalSpecialtyIDs(professional),
		},
	}
	if auth.CanReadProfessionalContact(*caller, professional.ID) {
		result.Email = professional.Email
		result.Phone = professional.Phone
	}
	return result, nil
}
