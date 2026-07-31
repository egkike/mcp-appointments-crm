package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// CheckAvailabilityUseCase delegates to the domain AvailabilityService.
type CheckAvailabilityUseCase struct {
	svc  *service.AvailabilityService
	deps service.AvailabilityDeps
}

// NewCheckAvailabilityUseCase constructs a CheckAvailabilityUseCase with the
// given domain service and its pre-assembled dependency struct.
func NewCheckAvailabilityUseCase(
	svc *service.AvailabilityService,
	deps service.AvailabilityDeps,
) *CheckAvailabilityUseCase {
	return &CheckAvailabilityUseCase{svc: svc, deps: deps}
}

// Execute checks whether the requested datetime is available for booking.
//
// CheckAvailability is a non-authoritative preview tool (per design Decisión 11)
// and does NOT require authentication. The Caller field on the params DTO is
// preserved for consistency but not enforced.
//
// Domain service errors (business closed, professional not working, slot out of
// hours, overlap, past) propagate as-is.
func (uc *CheckAvailabilityUseCase) Execute(ctx context.Context, input dto.CheckAvailabilityInput) (*dto.CheckAvailabilityResult, error) {
	params := &service.CheckAvailabilityParams{
		ServiceID:      input.ServiceID,
		ProfessionalID: input.ProfessionalID,
		StartDatetime:  input.StartDatetime.Format(time.RFC3339),
	}
	result, err := uc.svc.CheckAvailability(ctx, params, uc.deps)
	if err != nil {
		return nil, fmt.Errorf("consultar disponibilidad: %w", err)
	}
	return &dto.CheckAvailabilityResult{Available: result.Available}, nil
}
