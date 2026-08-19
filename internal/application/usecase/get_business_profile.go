package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// GetBusinessProfileUseCase retrieves the singleton business profile.
//
// Auth note: role restriction (owner/admin/staff) is enforced at the transport
// layer by the RBAC entry for get_business_profile in main.go; this use case
// has no Caller input by design (the profile is not tenant-scoped), matching
// the BusinessProfilePort contract in internal/mcp/ports.go.
type GetBusinessProfileUseCase struct {
	profiles repository.BusinessProfileRepo
}

// NewGetBusinessProfileUseCase constructs a GetBusinessProfileUseCase with the
// given dependencies.
func NewGetBusinessProfileUseCase(profiles repository.BusinessProfileRepo) *GetBusinessProfileUseCase {
	return &GetBusinessProfileUseCase{profiles: profiles}
}

// Execute returns the singleton business profile. A missing profile (repo
// contract: domain.ErrNotFound) is a semantic domain condition, so it maps to
// a SemanticError instead of the generic internal error; the MCP error mapper
// turns it into a -32002 response with a Spanish message.
func (uc *GetBusinessProfileUseCase) Execute(ctx context.Context) (*entity.BusinessProfile, error) {
	profile, err := uc.profiles.Get(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "perfil del negocio no encontrado",
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("obtener perfil del negocio: %w", err)
	}
	return profile, nil
}
