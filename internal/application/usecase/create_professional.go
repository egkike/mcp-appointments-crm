package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// defaultProfessionalStatus is the status applied when the payload omits it.
// ProfessionalsRepo.Save has the same fallback, but setting it here keeps the
// use case self-contained (and lets the tool echo the persisted status).
const defaultProfessionalStatus = "active"

// CreateProfessionalUseCase creates a staff member.
// Access is restricted to the owner role (ADR-0015 Decision 2).
type CreateProfessionalUseCase struct {
	professionals domainrepo.ProfessionalsRepo
	logger        *slog.Logger
}

// NewCreateProfessionalUseCase constructs the use case. A nil logger falls back
// to slog.Default() (see maintenanceLogger).
func NewCreateProfessionalUseCase(professionals domainrepo.ProfessionalsRepo, logger *slog.Logger) *CreateProfessionalUseCase {
	return &CreateProfessionalUseCase{professionals: professionals, logger: maintenanceLogger(logger)}
}

// Execute encodes the specialties, assembles the entity and persists it.
//
// The ID is assigned by ProfessionalsRepo.Save (UUID v4), so the returned
// entity carries the persisted ID. Name/status rules and the existence of every
// referenced service are enforced by the repository (entity.Validate +
// validateSpecialtiesExist) — a missing service surfaces as domain.ErrNotFound
// and is mapped to an actionable message instead of an internal error.
func (uc *CreateProfessionalUseCase) Execute(ctx context.Context, input dto.CreateProfessionalInput) (*entity.Professional, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	specialties, err := encodeServiceIDs(input.Specialties)
	if err != nil {
		return nil, fmt.Errorf("create_professional: %w", err)
	}

	professional := &entity.Professional{
		Name:          input.Name,
		RoleSpecialty: input.RoleSpecialty,
		Email:         input.Email,
		Phone:         input.Phone,
		Specialties:   specialties,
		Status:        defaultProfessionalStatus,
	}
	if input.Status != nil {
		professional.Status = *input.Status
	}

	if err := uc.professionals.Save(ctx, professional); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "los datos del profesional no son válidos: revisá el nombre y el estado (active o inactive)",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrNotFound):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "uno de los servicios indicados en las especialidades no existe",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrConflict):
			return nil, &domain.SemanticError{Code: domain.ErrCodeConflict, Message: "ya existe un profesional con esos datos", Cause: err}
		}
		return nil, fmt.Errorf("create_professional: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "create_professional", "professional", professional.ID, "create")
	return professional, nil
}

// encodeServiceIDs marshals service IDs into the JSON array stored in
// professionals.specialties. A nil slice stays nil so the column keeps NULL
// (meaning "no specialties declared"); a non-nil slice is replaced wholesale,
// including the empty slice. Shared by the create and update use cases.
func encodeServiceIDs(ids []string) (*string, error) {
	if ids == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("codificar especialidades: %w", err)
	}
	value := string(encoded)
	return &value, nil
}
