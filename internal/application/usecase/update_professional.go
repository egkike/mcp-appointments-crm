package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// UpdateProfessionalUseCase applies a partial merge to an existing
// professional: only the fields carried by the input are overwritten, and the
// specialties list is replaced wholesale when it is provided.
// Access is restricted to the owner role (ADR-0015 Decision 2).
type UpdateProfessionalUseCase struct {
	professionals domainrepo.ProfessionalsRepo
	logger        *slog.Logger
}

// NewUpdateProfessionalUseCase constructs the use case. A nil logger falls back
// to slog.Default() (see maintenanceLogger).
func NewUpdateProfessionalUseCase(professionals domainrepo.ProfessionalsRepo, logger *slog.Logger) *UpdateProfessionalUseCase {
	return &UpdateProfessionalUseCase{professionals: professionals, logger: maintenanceLogger(logger)}
}

// Execute reads the stored professional, merges the provided fields and
// persists it.
//
// The read-first flow separates "the professional does not exist" (mapped here)
// from the domain.ErrNotFound the repository raises when a specialty references
// a service that does not exist. Name/status rules stay in
// entity.Professional.Validate(); specialty existence stays in
// validateSpecialtiesExist (both invoked by the repository).
func (uc *UpdateProfessionalUseCase) Execute(ctx context.Context, input dto.UpdateProfessionalInput) (*entity.Professional, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	professional, err := uc.professionals.FindByID(ctx, input.ProfessionalID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el profesional no existe", Cause: err}
		}
		return nil, fmt.Errorf("update_professional: %w", err)
	}

	if input.Name != nil {
		professional.Name = *input.Name
	}
	if input.RoleSpecialty != nil {
		professional.RoleSpecialty = input.RoleSpecialty
	}
	if input.Email != nil {
		professional.Email = input.Email
	}
	if input.Phone != nil {
		professional.Phone = input.Phone
	}
	if input.Status != nil {
		professional.Status = *input.Status
	}
	if input.Specialties != nil {
		specialties, err := encodeServiceIDs(*input.Specialties)
		if err != nil {
			return nil, fmt.Errorf("update_professional: %w", err)
		}
		professional.Specialties = specialties
	}

	if err := uc.professionals.Update(ctx, professional); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "los datos del profesional no son válidos: revisá el nombre y el estado (active o inactive)",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrNotFound):
			// Reached when a specialty references a service that does not
			// exist; a concurrent deletion of the professional would surface
			// with the same code.
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "uno de los servicios indicados en las especialidades no existe",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrConflict):
			return nil, &domain.SemanticError{Code: domain.ErrCodeConflict, Message: "conflicto al actualizar el profesional", Cause: err}
		}
		return nil, fmt.Errorf("update_professional: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "update_professional", "professional", professional.ID, "update")
	return professional, nil
}
