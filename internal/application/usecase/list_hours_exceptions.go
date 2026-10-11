package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// ListHoursExceptionsUseCase reads the business-hours exceptions within an
// inclusive [from, to] range, ordered by exception_date ascending.
//
// The MCP surface is owner-only (design D5); the repository's own List guard
// stays open to any authenticated caller (REQ-BHE-AUTH-002) because availability
// is a hot path. List is a read: it has no logger dependency and therefore emits
// no maintenance audit record.
type ListHoursExceptionsUseCase struct {
	exceptions domainrepo.BusinessHoursExceptionRepo
}

// NewListHoursExceptionsUseCase constructs the read-only use case.
func NewListHoursExceptionsUseCase(exceptions domainrepo.BusinessHoursExceptionRepo) *ListHoursExceptionsUseCase {
	return &ListHoursExceptionsUseCase{exceptions: exceptions}
}

// Execute validates the owner gate and the two date bounds, then streams the
// repository result. An empty range yields a non-nil empty slice ([] on the
// wire, never null) and is not an error.
func (uc *ListHoursExceptionsUseCase) Execute(ctx context.Context, input dto.ListHoursExceptionsInput) ([]*entity.BusinessHoursException, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	if _, err := auth.RequireRole(ctx, auth.RoleOwner); err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	from, err := parseExceptionDateBound(input.From)
	if err != nil {
		return nil, err
	}
	to, err := parseExceptionDateBound(input.To)
	if err != nil {
		return nil, err
	}

	rows, err := uc.exceptions.List(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("list_hours_exceptions: %w", err)
	}
	if rows == nil {
		rows = []*entity.BusinessHoursException{}
	}
	return rows, nil
}

// parseExceptionDateBound applies the repository's YYYY-MM-DD rule to a range
// bound at the use-case edge (format-only gate, design §3): a malformed or
// calendar-invalid bound becomes a Spanish semantic error before the repository
// is reached, so it can never be silently widened or dropped by the SQL filter.
func parseExceptionDateBound(bound string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", bound)
	if err != nil || date.Format("2006-01-02") != bound {
		return time.Time{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "las fechas del rango deben tener formato YYYY-MM-DD.",
			Cause:   fmt.Errorf("la fecha %q del rango no tiene formato YYYY-MM-DD: %w", bound, domain.ErrInvalidInput),
		}
	}
	return date, nil
}
