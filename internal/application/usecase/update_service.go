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

// UpdateServiceUseCase applies a partial merge to an existing service: only the
// fields carried by the input are overwritten.
// Access is restricted to the owner role (ADR-0015 Decision 2).
type UpdateServiceUseCase struct {
	services domainrepo.ServicesRepo
	logger   *slog.Logger
}

// NewUpdateServiceUseCase constructs the use case. A nil logger falls back to
// slog.Default() (see maintenanceLogger).
func NewUpdateServiceUseCase(services domainrepo.ServicesRepo, logger *slog.Logger) *UpdateServiceUseCase {
	return &UpdateServiceUseCase{services: services, logger: maintenanceLogger(logger)}
}

// Execute reads the stored service, merges the provided fields and persists it.
//
// The read-first flow distinguishes the two failures the repository collapses
// into domain.ErrNotFound: a missing service (mapped here, before the write)
// and a write that matched no row. Name/duration/price rules stay in
// entity.Service.Validate(), invoked by the repository.
func (uc *UpdateServiceUseCase) Execute(ctx context.Context, input dto.UpdateServiceInput) (*entity.Service, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	service, err := uc.services.FindByID(ctx, input.ServiceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el servicio no existe", Cause: err}
		}
		return nil, fmt.Errorf("update_service: %w", err)
	}

	if input.Name != nil {
		service.Name = *input.Name
	}
	if input.Description != nil {
		service.Description = input.Description
	}
	if input.DurationMinutes != nil {
		service.DurationMinutes = *input.DurationMinutes
	}
	if input.Price != nil {
		service.Price = *input.Price
	}
	if input.Active != nil {
		service.Active = *input.Active
	}

	if err := uc.services.Update(ctx, service); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "los datos del servicio no son válidos: revisá el nombre, la duración en minutos y el precio",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrNotFound):
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el servicio no existe", Cause: err}
		case errors.Is(err, domain.ErrConflict):
			return nil, &domain.SemanticError{Code: domain.ErrCodeConflict, Message: "conflicto al actualizar el servicio", Cause: err}
		}
		return nil, fmt.Errorf("update_service: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "update_service", "service", service.ID, "update")
	return service, nil
}
