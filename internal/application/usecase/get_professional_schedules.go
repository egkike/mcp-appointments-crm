package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// GetProfessionalSchedulesUseCase reads one professional's weekly agenda
// (feat-owner-chat S7b). It closes the read-gap the conversational delete gate
// had (feature doc L4): before this read, `delete_schedule` could only repeat the
// professional+day pair the owner declared, never verify it.
//
// Role: OWNER, ADMIN or STAFF, enforced in THREE layers (feat-staff-self-read S3,
// issue #276): the composition-root ToolRBAC entry, auth.RequireRole below, and
// the SchedulesRepo.FindByProfessional guard. The agenda is
// business-operational data, so it stays closed to the client (which already
// resolves availability server-side through check_availability). Staff is
// own-only: the scope comes from the ONE shared predicate
// auth.AuthorizeProfessionalRead, so a foreign id is refused with the same
// FORBIDDEN answer as the record read (S2) instead of leaking through a
// repository lookup.
type GetProfessionalSchedulesUseCase struct {
	professionals domainrepo.ProfessionalsRepo
	schedules     domainrepo.SchedulesRepo
}

// NewGetProfessionalSchedulesUseCase constructs the use case.
func NewGetProfessionalSchedulesUseCase(
	professionals domainrepo.ProfessionalsRepo,
	schedules domainrepo.SchedulesRepo,
) *GetProfessionalSchedulesUseCase {
	return &GetProfessionalSchedulesUseCase{professionals: professionals, schedules: schedules}
}

// Execute returns the professional identity plus the stored weekly slots, ordered
// by day_of_week (0=Sunday .. 6=Saturday).
//
// Semantics (design §8.3):
//   - the professional must exist → semantic NOT_FOUND otherwise; the gate needs
//     "ese id no existe" to be distinguishable from "no tiene agenda cargada";
//   - an existing professional with no slot → empty (non-nil) list, never an
//     error;
//   - inactive professionals ARE returned with their status: the gate must see
//     against whom it confirms, and hiding the row would leave the owner
//     verifying an agenda of a professional the tool refused to name.
func (uc *GetProfessionalSchedulesUseCase) Execute(ctx context.Context, input dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	// Double enforcement, layer 2: pass auth.RequireRole's own semantic error
	// through unchanged (same mold as delete_schedule.go), keeping the
	// 401/403 distinction this layer's contract documents.
	caller, err := auth.RequireRole(ctx, auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff)
	if err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: auth.MsgForbiddenTool, Cause: domain.ErrForbidden}
	}

	// Scope, decided on the REQUEST and before any read (feat-staff-self-read
	// S2/S3): the shared predicate answers "may this caller read this
	// professional?", so staff is own-only here with the same FORBIDDEN answer the
	// record read gives, and owner/admin keep the whole roster.
	professionalID, err := auth.AuthorizeProfessionalRead(*caller, input.ProfessionalID)
	if err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, fmt.Errorf("get_professional_schedules: %w", err)
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
		return nil, fmt.Errorf("get_professional_schedules: %w", err)
	}

	schedules, err := uc.schedules.FindByProfessional(ctx, professionalID)
	if err != nil {
		// A disappearing professional between the two reads is still "no existe"
		// for the caller: the sentinel is the repository's, not subject to a
		// different message.
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "el profesional indicado no existe",
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("get_professional_schedules: %w", err)
	}
	if schedules == nil {
		schedules = []*entity.Schedule{}
	}

	return &dto.GetProfessionalSchedulesResult{
		Professional: dto.ProfessionalScheduleSubject{
			ID:     professional.ID,
			Name:   professional.Name,
			Status: professional.Status,
		},
		Schedules: schedules,
	}, nil
}
