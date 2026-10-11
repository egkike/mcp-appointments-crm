package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// DeleteHoursExceptionUseCase removes a business-hours exception by id.
// Access is restricted to the owner role (design D5, ADR-0015 Decision 2);
// the repository's admin+owner guard remains the inner barrier.
type DeleteHoursExceptionUseCase struct {
	exceptions domainrepo.BusinessHoursExceptionRepo
	logger     *slog.Logger
}

// NewDeleteHoursExceptionUseCase constructs the use case. A nil logger falls
// back to slog.Default() (see maintenanceLogger).
func NewDeleteHoursExceptionUseCase(exceptions domainrepo.BusinessHoursExceptionRepo, logger *slog.Logger) *DeleteHoursExceptionUseCase {
	return &DeleteHoursExceptionUseCase{exceptions: exceptions, logger: maintenanceLogger(logger)}
}

// Execute deletes the exception identified by input.ExceptionID. The repository
// maps a zero-row delete to domain.ErrNotFound; this use case turns that
// sentinel into a message the LLM can act on.
func (uc *DeleteHoursExceptionUseCase) Execute(ctx context.Context, input dto.DeleteHoursExceptionInput) (*dto.DeleteHoursExceptionResult, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	if err := uc.exceptions.Delete(ctx, input.ExceptionID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "no existe una excepción con ese id.", Cause: err}
		}
		return nil, fmt.Errorf("delete_hours_exception: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "delete_hours_exception", "business_hours_exception", strconv.Itoa(input.ExceptionID), "delete")
	return &dto.DeleteHoursExceptionResult{ExceptionID: input.ExceptionID, Status: "deleted"}, nil
}
