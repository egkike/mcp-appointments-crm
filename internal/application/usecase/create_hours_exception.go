package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// CreateHoursExceptionUseCase adds a date-specific override of the weekly
// business hours (holiday, vacation, special window).
// Access is restricted to the owner role: an exception can suppress bookings,
// so it is narrower than the repository's admin+owner guard (design D5,
// ADR-0015 Decision 2).
type CreateHoursExceptionUseCase struct {
	exceptions domainrepo.BusinessHoursExceptionRepo
	logger     *slog.Logger
}

// NewCreateHoursExceptionUseCase constructs the use case. A nil logger falls
// back to slog.Default() (see maintenanceLogger).
func NewCreateHoursExceptionUseCase(exceptions domainrepo.BusinessHoursExceptionRepo, logger *slog.Logger) *CreateHoursExceptionUseCase {
	return &CreateHoursExceptionUseCase{exceptions: exceptions, logger: maintenanceLogger(logger)}
}

// Execute persists the exception through the repository (the single validator:
// date format/calendar, is_closed↔times consistency, HH:MM, open<close,
// uniqueness) and returns the stored row.
//
// Create returns only an error and never assigns the SQLite AUTOINCREMENT id,
// so the stored row is read back by its unique date — a single indexed lookup
// (design §1.1). A nil read-back is an infrastructure failure (fail closed, no
// half-empty payload).
func (uc *CreateHoursExceptionUseCase) Execute(ctx context.Context, input dto.CreateHoursExceptionInput) (*entity.BusinessHoursException, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	exception := &entity.BusinessHoursException{
		ExceptionDate: input.ExceptionDate,
		IsClosed:      input.IsClosed,
		OpenTime:      input.OpenTime,
		CloseTime:     input.CloseTime,
		Reason:        input.Reason,
	}

	if err := uc.exceptions.Create(ctx, exception); err != nil {
		var sem *domain.SemanticError
		switch {
		// The repository's date validator already emits a clean Spanish
		// SemanticError; pass it through unchanged (design §4).
		case errors.As(err, &sem):
			return nil, err
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la excepción no es válida. Si el negocio está cerrado, no envíes open_time/close_time; si está abierto, ambos son obligatorios en formato HH:MM y open_time debe ser anterior a close_time.",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrConflict):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeConflict,
				Message: "ya existe una excepción para esa fecha. Borrala y creala de nuevo si querés cambiar el horario de ese día.",
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("create_hours_exception: %w", err)
	}

	date, err := time.Parse("2006-01-02", input.ExceptionDate)
	if err != nil {
		return nil, fmt.Errorf("create_hours_exception: fecha persistida inválida %q: %w", input.ExceptionDate, err)
	}
	stored, err := uc.exceptions.Get(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("create_hours_exception: leer excepción creada: %w", err)
	}
	if stored == nil {
		return nil, fmt.Errorf("create_hours_exception: la excepción creada para %s no se encontró", input.ExceptionDate)
	}

	logMaintenanceMutation(uc.logger, caller, "create_hours_exception", "business_hours_exception", strconv.Itoa(stored.ID), "create")
	return stored, nil
}
