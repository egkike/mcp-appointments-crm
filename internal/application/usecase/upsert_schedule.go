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

// UpsertScheduleUseCase inserts or replaces a professional's weekly slot for a
// single day of the week. Access is restricted to the owner role
// (ADR-0015 Decision 2).
type UpsertScheduleUseCase struct {
	schedules domainrepo.SchedulesRepo
	logger    *slog.Logger
}

// NewUpsertScheduleUseCase constructs the use case. A nil logger falls back to
// slog.Default() (see maintenanceLogger).
func NewUpsertScheduleUseCase(schedules domainrepo.SchedulesRepo, logger *slog.Logger) *UpsertScheduleUseCase {
	return &UpsertScheduleUseCase{schedules: schedules, logger: maintenanceLogger(logger)}
}

// Execute writes the day slot and returns the row as stored.
//
// Day range (0=Sunday..6=Saturday), the HH:MM format and start<end are enforced
// by SchedulesRepo.Upsert; this use case only assembles the entity. The stored
// row is read back because the schedules primary key is assigned by SQLite, so
// the in-memory entity would come back without its ID.
//
// Foreign key: schedules.professional_id references professionals(id)
// (internal/db/schema.go). SchedulesRepo.Upsert classifies the resulting
// violation as domain.ErrConflict, and the only foreign key on this write path is
// that parent reference, so the conflict is answered with ErrCodeNotFound and a
// message naming the missing professional.
func (uc *UpsertScheduleUseCase) Execute(ctx context.Context, input dto.UpsertScheduleInput) (*entity.Schedule, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	schedule := &entity.Schedule{
		ProfessionalID: input.ProfessionalID,
		DayOfWeek:      input.DayOfWeek,
		StartTime:      input.StartTime,
		EndTime:        input.EndTime,
	}

	if err := uc.schedules.Upsert(ctx, schedule); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el horario no es válido: el día debe estar entre 0 (domingo) y 6 (sábado) y las horas en formato HH:MM con inicio anterior al fin",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrNotFound):
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el profesional no existe", Cause: err}
		case errors.Is(err, domain.ErrConflict):
			// Repository-classified foreign-key failure: schedules.professional_id
			// has no matching parent row, so the professional indicated in the
			// input does not exist (NOT_FOUND, not a generic conflict).
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el profesional indicado no existe", Cause: err}
		}
		return nil, fmt.Errorf("upsert_schedule: %w", err)
	}

	stored, err := uc.schedules.FindByProfessionalAndDay(ctx, input.ProfessionalID, input.DayOfWeek)
	if err != nil {
		return nil, fmt.Errorf("upsert_schedule: leer horario guardado: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "upsert_schedule", "schedule", maintenanceScheduleID(input.ProfessionalID, input.DayOfWeek), "upsert")
	return stored, nil
}
