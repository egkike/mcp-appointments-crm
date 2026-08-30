package usecase

import (
	"context"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// SearchClientsAdvancedUseCase performs a role-scoped FTS search on clients.
// Admin/owner see all matches; client sees only its own row; staff sees clients
// linked by any of its bookings.
type SearchClientsAdvancedUseCase struct {
	clients domainrepo.ClientsRepo
}

// NewSearchClientsAdvancedUseCase constructs the use case.
func NewSearchClientsAdvancedUseCase(clients domainrepo.ClientsRepo) *SearchClientsAdvancedUseCase {
	return &SearchClientsAdvancedUseCase{clients: clients}
}

// Execute runs the FTS search using the caller from the input DTO. The caller
// must be authenticated; repository scoping enforces the role-based filter.
func (uc *SearchClientsAdvancedUseCase) Execute(ctx context.Context, input dto.SearchClientsAdvancedInput) (*dto.SearchClientsAdvancedResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	matches, err := uc.clients.SearchFTS(auth.WithCaller(ctx, input.Caller), input.QueryText)
	if err != nil {
		return nil, fmt.Errorf("search_clients_advanced: %w", err)
	}
	results := make([]dto.ClientSearchEntry, 0, len(matches))
	for _, c := range matches {
		results = append(results, dto.ClientSearchEntry{
			ID:          c.ID,
			Name:        c.Name,
			Phone:       c.Phone,
			Preferences: c.Preferences,
		})
	}
	return &dto.SearchClientsAdvancedResult{Results: results}, nil
}
