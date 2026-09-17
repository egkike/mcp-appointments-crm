package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// DeleteServiceUseCase removes a service from the catalog by ID.
// Access is restricted to the owner role (ADR-0015 Decision 2).
type DeleteServiceUseCase struct {
	services domainrepo.ServicesRepo
	logger   *slog.Logger
}

// NewDeleteServiceUseCase constructs the use case. A nil logger falls back to
// slog.Default() (see maintenanceLogger).
func NewDeleteServiceUseCase(services domainrepo.ServicesRepo, logger *slog.Logger) *DeleteServiceUseCase {
	return &DeleteServiceUseCase{services: services, logger: maintenanceLogger(logger)}
}

// Execute deletes the service identified by input.ServiceID.
//
// A delete blocked by existing bookings fails at the SQLite layer:
// bookings.service_id is declared ON DELETE RESTRICT (internal/db/schema.go),
// so SQLite rejects the parent delete with a foreign-key constraint violation
// that internal/repository/sqlite_errors.go classifies as domain.ErrConflict.
// The ErrConflict branch below is the only place that turns it into an
// LLM-actionable Spanish message; the application layer deliberately does NOT
// inspect driver result codes.
func (uc *DeleteServiceUseCase) Execute(ctx context.Context, input dto.DeleteServiceInput) (*dto.DeleteServiceResult, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	if err := uc.services.Delete(ctx, input.ServiceID); err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el servicio no existe", Cause: err}
		case errors.Is(err, domain.ErrConflict):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "no se puede eliminar el servicio porque tiene reservas asociadas",
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("delete_service: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "delete_service", "service", input.ServiceID, "delete")
	return &dto.DeleteServiceResult{ServiceID: input.ServiceID, Status: "deleted"}, nil
}
