package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

const (
	// minTopN is the smallest allowed value for top_n after clamping.
	minTopN = 1
	// maxTopN is the largest allowed value for top_n after clamping.
	maxTopN = 100
	// defaultTopN is used when top_n is omitted.
	defaultTopN = 10
)

// validPeriods lists the accepted period values for get_loyalty_report.
var validPeriods = []string{"last_week", "last_month", "last_quarter", "last_year", "all_time"}

// validPeriodSet is the allowlist used for validation; it is derived from
// validPeriods to keep the error message and the switch in sync.
var validPeriodSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(validPeriods))
	for _, p := range validPeriods {
		m[p] = struct{}{}
	}
	return m
}()

// GetLoyaltyReportUseCase returns the most frequent clients in a period.
// Access is restricted to owner and admin roles because rows expose phone PII.
type GetLoyaltyReportUseCase struct {
	bookings domainrepo.BookingsRepo
	clock    Clock
}

// NewGetLoyaltyReportUseCase constructs the use case.
func NewGetLoyaltyReportUseCase(bookings domainrepo.BookingsRepo) *GetLoyaltyReportUseCase {
	return &GetLoyaltyReportUseCase{bookings: bookings, clock: systemClock{}}
}

// SetClock replaces the clock. It is intended for tests.
func (uc *GetLoyaltyReportUseCase) SetClock(clock Clock) {
	uc.clock = clock
}

// Execute validates the period and top_n, computes the UTC window, and returns
// the ranked aggregation. Invalid periods return a semantic error listing the
// five valid values.
func (uc *GetLoyaltyReportUseCase) Execute(ctx context.Context, input dto.GetLoyaltyReportInput) (*dto.GetLoyaltyReportResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	ctx = auth.WithCaller(ctx, input.Caller)
	if _, err := auth.RequireRole(ctx, auth.RoleOwner, auth.RoleAdmin); err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	period := input.Period
	if period == "" {
		period = "last_month"
	}
	if _, ok := validPeriodSet[period]; !ok {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("periodo inválido. Valores válidos: %s", joinPeriods()),
			Cause:   domain.ErrInvalidInput,
		}
	}

	var start time.Time
	now := uc.clock.Now().UTC()
	switch period {
	case "last_week":
		start = now.AddDate(0, 0, -7)
	case "last_month":
		start = now.AddDate(0, -1, 0)
	case "last_quarter":
		start = now.AddDate(0, -3, 0)
	case "last_year":
		start = now.AddDate(-1, 0, 0)
	case "all_time":
		start = time.Time{}
	}

	limit := defaultTopN
	if input.TopN != nil {
		limit = *input.TopN
		if limit < minTopN {
			limit = minTopN
		}
		if limit > maxTopN {
			limit = maxTopN
		}
	}

	end := now
	counts, err := uc.bookings.AggregateByClient(ctx, start, end, limit)
	if err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo generar el reporte de fidelización, intente nuevamente",
			Cause:   err,
		}
	}

	results := make([]dto.LoyaltyReportEntry, 0, len(counts))
	for _, c := range counts {
		results = append(results, dto.LoyaltyReportEntry{
			ClientID:     c.ClientID,
			Name:         c.Name,
			Phone:        c.Phone,
			BookingCount: c.BookingCount,
		})
	}
	return &dto.GetLoyaltyReportResult{Results: results}, nil
}

// joinPeriods returns the comma-separated list of valid periods.
func joinPeriods() string {
	return strings.Join(validPeriods, ", ")
}
