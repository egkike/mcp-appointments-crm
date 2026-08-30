package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestSearchServicesAdvancedUseCase(t *testing.T) {
	t.Run("owner allowed and results mapped", func(t *testing.T) {
		repo := &mockServicesRepo{SearchFTSFn: func(ctx context.Context, query string) ([]*entity.Service, error) {
			if query != "corte" {
				t.Errorf("query = %q, want corte", query)
			}
			return []*entity.Service{
				{ID: "s1", Name: "Corte", Description: ptr("Clásico"), DurationMinutes: 30, Price: 500.0, Active: true},
			}, nil
		}}
		uc := NewSearchServicesAdvancedUseCase(repo)
		result, err := uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{
			Caller:    auth.Caller{ID: "owner-1", Role: auth.RoleOwner},
			QueryText: "corte",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 {
			t.Fatalf("got %d results, want 1", len(result.Results))
		}
		entry := result.Results[0]
		if entry.ID != "s1" || entry.Name != "Corte" || entry.DurationMinutes != 30 || entry.Price != 500.0 || !entry.IsActive {
			t.Errorf("got %+v", entry)
		}
	})

	t.Run("staff rejected", func(t *testing.T) {
		uc := NewSearchServicesAdvancedUseCase(&mockServicesRepo{})
		_, err := uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{
			Caller:    auth.Caller{ID: "staff-1", Role: auth.RoleStaff, ProfessionalID: ptr("p1")},
			QueryText: "corte",
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("client rejected", func(t *testing.T) {
		uc := NewSearchServicesAdvancedUseCase(&mockServicesRepo{})
		_, err := uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{
			Caller:    auth.Caller{ID: "client-1", Role: auth.RoleClient, ClientID: ptr("c1")},
			QueryText: "corte",
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("unauthenticated rejected", func(t *testing.T) {
		uc := NewSearchServicesAdvancedUseCase(&mockServicesRepo{})
		_, err := uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{QueryText: "corte"})
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("got %v, want ErrUnauthenticated", err)
		}
	})
}
