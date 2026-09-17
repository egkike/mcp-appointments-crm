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
	"github.com/egkike/mcp-appointments-crm/internal/idgen"
)

// CreateServiceUseCase creates a bookable service in the catalog.
// Access is restricted to the owner role (ADR-0015 Decision 2).
type CreateServiceUseCase struct {
	services domainrepo.ServicesRepo
	logger   *slog.Logger
}

// NewCreateServiceUseCase constructs the use case. A nil logger falls back to
// slog.Default() (see maintenanceLogger).
func NewCreateServiceUseCase(services domainrepo.ServicesRepo, logger *slog.Logger) *CreateServiceUseCase {
	return &CreateServiceUseCase{services: services, logger: maintenanceLogger(logger)}
}

// Execute builds the entity, assigns its ID and persists it.
//
// ServicesRepo.Save neither generates the primary key nor validates beyond
// entity.Service.Validate(), so the ID is minted here with idgen (UUID v4) —
// mirroring the booking use case. Name/duration/price rules are enforced by the
// repository-side Validate(); this use case only assembles the entity.
func (uc *CreateServiceUseCase) Execute(ctx context.Context, input dto.CreateServiceInput) (*entity.Service, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	serviceID, err := idgen.New()
	if err != nil {
		return nil, fmt.Errorf("create_service: generar id: %w", err)
	}

	// An omitted flag creates an active service: the catalog is meant to be
	// bookable, and the Go zero value (false) would silently hide it.
	active := true
	if input.Active != nil {
		active = *input.Active
	}

	service := &entity.Service{
		ID:              serviceID,
		Name:            input.Name,
		Description:     input.Description,
		DurationMinutes: input.DurationMinutes,
		Price:           input.Price,
		Active:          active,
	}

	if err := uc.services.Save(ctx, service); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "los datos del servicio no son válidos: revisá el nombre, la duración en minutos y el precio",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrConflict):
			return nil, &domain.SemanticError{Code: domain.ErrCodeConflict, Message: "ya existe un servicio con esos datos", Cause: err}
		case errors.Is(err, domain.ErrNotFound):
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el servicio no existe", Cause: err}
		}
		return nil, fmt.Errorf("create_service: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "create_service", "service", service.ID, "create")
	return service, nil
}
