package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// SearchServicesAdvancedUseCase serves two views of the service catalog: the
// owner/admin maintenance search and the active-only booking catalog (issue #255).
type SearchServicesAdvancedUseCase struct {
	services domainrepo.ServicesRepo
}

// NewSearchServicesAdvancedUseCase constructs the use case.
func NewSearchServicesAdvancedUseCase(services domainrepo.ServicesRepo) *SearchServicesAdvancedUseCase {
	return &SearchServicesAdvancedUseCase{services: services}
}

// Execute returns the service catalog the caller may see.
//
// The two views are deliberately different (issue #255):
//   - owner/admin get the MAINTENANCE view: an FTS search over every service,
//     retired ones included, because they are the ones who manage them. This is
//     the behaviour that existed before the client catalog was opened.
//   - every other authenticated caller gets the BOOKING catalog: the ACTIVE
//     services only, filtered by the same term. A retired service must stay
//     unreachable from a booking flow even when searched by name, and an empty
//     term means the whole active catalog — what a client needs before it knows
//     any service name. SearchFTS rejects an empty term, so this branch
//     deliberately does not go through it.
func (uc *SearchServicesAdvancedUseCase) Execute(ctx context.Context, input dto.SearchServicesAdvancedInput) (*dto.SearchServicesAdvancedResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	ctx = auth.WithCaller(ctx, input.Caller)

	// No default case on purpose: owner/admin are the ONLY privileged roles, and
	// every other authenticated role is the booking view below. A future
	// privileged role must be added here explicitly — falling through would give it
	// the public catalog silently (advisory R2-switch-no-default, issue #257).
	switch input.Caller.Role {
	case auth.RoleOwner, auth.RoleAdmin:
		// Maintenance view: FTS over every service, retired ones included, because
		// they are the ones who manage them.
		matches, err := uc.services.SearchFTS(ctx, input.QueryText)
		if err != nil {
			return nil, fmt.Errorf("search_services_advanced: %w", err)
		}
		return &dto.SearchServicesAdvancedResult{Results: serviceSearchEntries(matches)}, nil
	}

	// Booking catalog for every other authenticated caller. FINDACTIVE is the
	// only source: its SQL restricts to is_active = 1 and orders by name, so a
	// retired service stays unreachable even when searched by name.
	//
	// The switch is explicit rather than driven by a swallowed RequireRole error
	// on purpose: falling through on ANY guard error would hand the public
	// catalog to a caller whose context broke (review advisories R2-001/R3-002).
	active, err := uc.services.FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("search_services_advanced: %w", err)
	}
	term := strings.ToLower(strings.TrimSpace(input.QueryText))
	matches := make([]*entity.Service, 0, len(active))
	for _, service := range active {
		if matchesBookingTerm(service, term) {
			matches = append(matches, service)
		}
	}
	return &dto.SearchServicesAdvancedResult{Results: serviceSearchEntries(matches)}, nil
}

// matchesBookingTerm reports whether a service matches the booking-catalog term.
// An empty term means "the whole active catalog": a client needs to see what it
// can book before it knows any service name. Matching is a case-insensitive
// substring on name and description — deliberately NOT the FTS ranking, whose
// index also covers retired services (see Execute). Kept in one named place so
// the rule reads as a rule instead of as loop-body logic.
func matchesBookingTerm(service *entity.Service, term string) bool {
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(service.Name), term) {
		return true
	}
	return service.Description != nil && strings.Contains(strings.ToLower(*service.Description), term)
}

// serviceSearchEntries projects entities onto the wire entry. One place, so the
// maintenance view and the booking catalog cannot drift in field handling.
func serviceSearchEntries(services []*entity.Service) []dto.ServiceSearchEntry {
	entries := make([]dto.ServiceSearchEntry, 0, len(services))
	for _, s := range services {
		entries = append(entries, dto.ServiceSearchEntry{
			ID:              s.ID,
			Name:            s.Name,
			Description:     s.Description,
			DurationMinutes: s.DurationMinutes,
			Price:           s.Price,
			IsActive:        s.Active,
		})
	}
	return entries
}
