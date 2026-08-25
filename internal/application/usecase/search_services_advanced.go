package usecase

import (
	"context"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// SearchServicesAdvancedUseCase performs an owner/admin-scoped FTS search on services.
type SearchServicesAdvancedUseCase struct {
	services domainrepo.ServicesRepo
}

// NewSearchServicesAdvancedUseCase constructs the use case.
func NewSearchServicesAdvancedUseCase(services domainrepo.ServicesRepo) *SearchServicesAdvancedUseCase {
	return &SearchServicesAdvancedUseCase{services: services}
}

// Execute runs the FTS search. Only owner/admin callers are allowed; other
// authenticated roles receive a semantic forbidden error.
func (uc *SearchServicesAdvancedUseCase) Execute(ctx context.Context, input dto.SearchServicesAdvancedInput) (*dto.SearchServicesAdvancedResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	ctx = auth.WithCaller(ctx, input.Caller)
	if _, err := auth.RequireRole(ctx, auth.RoleOwner, auth.RoleAdmin); err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}
	matches, err := uc.services.SearchFTS(ctx, input.QueryText)
	if err != nil {
		return nil, fmt.Errorf("search_services_advanced: %w", err)
	}
	results := make([]dto.ServiceSearchEntry, 0, len(matches))
	for _, s := range matches {
		results = append(results, dto.ServiceSearchEntry{
			ID:              s.ID,
			Name:            s.Name,
			Description:     s.Description,
			DurationMinutes: s.DurationMinutes,
			Price:           s.Price,
			IsActive:        s.Active,
		})
	}
	return &dto.SearchServicesAdvancedResult{Results: results}, nil
}
