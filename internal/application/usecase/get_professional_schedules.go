package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
// Role: OWNER ONLY, enforced twice by design (design §8.3 D5): the
// composition-root ToolRBAC entry and auth.RequireRole(RoleOwner) below. The
// agenda is business-operational data; the client already resolves availability
// server-side through check_availability, so opening this read would widen the
// wire without a product need.
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
	if _, err := auth.RequireRole(ctx, auth.RoleOwner); err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	professionalID := strings.TrimSpace(input.ProfessionalID)

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
