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

func TestSearchClientsAdvancedUseCase(t *testing.T) {
	t.Run("unauthenticated caller rejected", func(t *testing.T) {
		uc := NewSearchClientsAdvancedUseCase(&mockClientsRepo{})
		_, err := uc.Execute(context.Background(), dto.SearchClientsAdvancedInput{
			Caller: auth.Caller{},
		})
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("got %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("admin receives mapped results", func(t *testing.T) {
		repo := &mockClientsRepo{SearchFTSFn: func(ctx context.Context, query string) ([]*entity.Client, error) {
			if query != "juan" {
				t.Errorf("query = %q, want juan", query)
			}
			return []*entity.Client{
				{ID: "c1", Name: "Juan", Phone: "+5491112345678", Preferences: ptr("alergia")},
			}, nil
		}}
		uc := NewSearchClientsAdvancedUseCase(repo)
		result, err := uc.Execute(context.Background(), dto.SearchClientsAdvancedInput{
			Caller:    auth.Caller{ID: "admin-1", Role: auth.RoleAdmin},
			QueryText: "juan",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 {
			t.Fatalf("got %d results, want 1", len(result.Results))
		}
		entry := result.Results[0]
		if entry.ID != "c1" || entry.Name != "Juan" || entry.Phone != "+5491112345678" || *entry.Preferences != "alergia" {
			t.Errorf("got %+v", entry)
		}
	})

	t.Run("empty result returns empty slice not nil", func(t *testing.T) {
		repo := &mockClientsRepo{SearchFTSFn: func(ctx context.Context, query string) ([]*entity.Client, error) {
			return []*entity.Client{}, nil
		}}
		uc := NewSearchClientsAdvancedUseCase(repo)
		result, err := uc.Execute(context.Background(), dto.SearchClientsAdvancedInput{
			Caller:    auth.Caller{ID: "admin-1", Role: auth.RoleAdmin},
			QueryText: "xyz",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Results == nil || len(result.Results) != 0 {
			t.Errorf("got %v, want empty non-nil slice", result.Results)
		}
	})

	t.Run("repo error propagates", func(t *testing.T) {
		repo := &mockClientsRepo{SearchFTSFn: func(ctx context.Context, query string) ([]*entity.Client, error) {
			return nil, domain.ErrInvalidInput
		}}
		uc := NewSearchClientsAdvancedUseCase(repo)
		_, err := uc.Execute(context.Background(), dto.SearchClientsAdvancedInput{
			Caller:    auth.Caller{ID: "admin-1", Role: auth.RoleAdmin},
			QueryText: "*",
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
}
