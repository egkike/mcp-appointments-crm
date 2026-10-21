package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

// catalogProfessionals returns the fixture used by most subtests: two actives
// declaring one service each plus an active with an unreadable specialties value.
func catalogProfessionals() []*entity.Professional {
	return []*entity.Professional{
		{ID: "p-romina", Name: "Romina", RoleSpecialty: ptr("Estilista"), Status: "active", Specialties: ptr(`["s-lavado"]`)},
		{ID: "p-santiago", Name: "Santiago", RoleSpecialty: ptr("Barbero"), Status: "active", Specialties: ptr(`["s-corte"]`)},
	}
}

// newSearchProfessionalsUC wires the use case with the shared real query
// validator (repository.QueryValidator delegates to validateFTSQuery), so the
// tests below exercise the same text policy the FTS tools enforce. Test-only
// import of the persistence adapter, same shape as
// TestSearchServicesAdvancedBadFTSInputIsSemantic.
func newSearchProfessionalsUC(professionals *mockProfessionalsRepo, services *mockServicesRepo) *SearchProfessionalsAdvancedUseCase {
	return NewSearchProfessionalsAdvancedUseCase(professionals, services, repository.QueryValidator{})
}

func TestSearchProfessionalsAdvancedUseCase(t *testing.T) {
	t.Run("client caller is admitted and service_id filters by specialties", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindAllActiveFn: func(ctx context.Context) ([]*entity.Professional, error) {
			return catalogProfessionals(), nil
		}}
		services := &mockServicesRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Service, error) {
			if id != "s-lavado" {
				t.Errorf("FindByID(%q); want s-lavado", id)
			}
			return &entity.Service{ID: id, Name: "Lavado de Cabello", Active: true}, nil
		}}

		result, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), ServiceID: "s-lavado"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 {
			t.Fatalf("got %d results, want 1 (only Romina declares s-lavado): %+v", len(result.Results), result.Results)
		}
		entry := result.Results[0]
		if entry.ID != "p-romina" || entry.Name != "Romina" {
			t.Errorf("got %+v; want p-romina/Romina", entry)
		}
		if entry.RoleSpecialty == nil || *entry.RoleSpecialty != "Estilista" {
			t.Errorf("RoleSpecialty = %v; want Estilista", entry.RoleSpecialty)
		}
		if len(entry.Specialties) != 1 || entry.Specialties[0] != "s-lavado" {
			t.Errorf("Specialties = %v; want [s-lavado]", entry.Specialties)
		}
	})

	t.Run("empty specialties yields an empty list, never the full catalog", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindAllActiveFn: func(ctx context.Context) ([]*entity.Professional, error) {
			return []*entity.Professional{
				{ID: "p-1", Name: "Romina", Status: "active"},
				{ID: "p-2", Name: "Santiago", Status: "active", Specialties: ptr("[]")},
				{ID: "p-3", Name: "Joaquin", Status: "active", Specialties: ptr(`{not json}`)},
			}, nil
		}}
		services := &mockServicesRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Service, error) {
			return &entity.Service{ID: id, Name: "Lavado de Cabello", Active: true}, nil
		}}

		result, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), ServiceID: "s-lavado"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Results == nil {
			t.Fatal("Results = nil; want an empty (non-nil) slice so the wire renders []")
		}
		if len(result.Results) != 0 {
			t.Fatalf("got %d results, want 0 (fail-closed, never the full catalog): %+v", len(result.Results), result.Results)
		}
	})

	t.Run("unknown service_id returns a semantic NOT_FOUND", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{}
		services := &mockServicesRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Service, error) {
			return nil, domain.ErrNotFound
		}}

		_, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), ServiceID: "s-missing"})
		assertSemanticError(t, err, domain.ErrCodeNotFound, "el servicio indicado no existe")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("sentinel lost: %v", err)
		}
	})

	t.Run("textual match is case-insensitive over name and role_specialty", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindAllActiveFn: func(ctx context.Context) ([]*entity.Professional, error) {
			return catalogProfessionals(), nil
		}}
		services := &mockServicesRepo{}

		byName, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), QueryText: "ROMI"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(byName.Results) != 1 || byName.Results[0].ID != "p-romina" {
			t.Fatalf("by name got %+v; want only p-romina", byName.Results)
		}

		byRole, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), QueryText: "barbero"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(byRole.Results) != 1 || byRole.Results[0].ID != "p-santiago" {
			t.Fatalf("by role_specialty got %+v; want only p-santiago", byRole.Results)
		}
	})

	t.Run("no filters returns the whole active catalog", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindAllActiveFn: func(ctx context.Context) ([]*entity.Professional, error) {
			return catalogProfessionals(), nil
		}}
		services := &mockServicesRepo{}

		result, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 2 {
			t.Fatalf("got %d results, want 2 (full active catalog)", len(result.Results))
		}
	})

	t.Run("inactive professionals are not listed", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindAllActiveFn: func(ctx context.Context) ([]*entity.Professional, error) {
			return []*entity.Professional{
				{ID: "p-active", Name: "Romina", Status: "active"},
				{ID: "p-gone", Name: "Fantasma", Status: "inactive"},
			}, nil
		}}
		services := &mockServicesRepo{}

		result, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 || result.Results[0].ID != "p-active" {
			t.Fatalf("got %+v; want only p-active (inactive rows never reach the wire)", result.Results)
		}
	})

	t.Run("query_text and service_id combine", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindAllActiveFn: func(ctx context.Context) ([]*entity.Professional, error) {
			return append(catalogProfessionals(),
				&entity.Professional{ID: "p-romina-2", Name: "Romina II", Status: "active", Specialties: ptr(`["s-corte"]`)},
			), nil
		}}
		services := &mockServicesRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Service, error) {
			return &entity.Service{ID: id, Name: "Corte de Cabello", Active: true}, nil
		}}

		result, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), QueryText: "romina", ServiceID: "s-corte"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 || result.Results[0].ID != "p-romina-2" {
			t.Fatalf("got %+v; want only p-romina-2 (both filters must hold)", result.Results)
		}
	})

	t.Run("nil catalog rows are skipped instead of panicking", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindAllActiveFn: func(ctx context.Context) ([]*entity.Professional, error) {
			return []*entity.Professional{nil, {ID: "p-1", Name: "Romina", Status: "active"}}, nil
		}}
		services := &mockServicesRepo{}

		result, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 || result.Results[0].ID != "p-1" {
			t.Fatalf("got %+v; want only p-1", result.Results)
		}
	})

	t.Run("over-long service_id is rejected before any lookup", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{}
		services := &mockServicesRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Service, error) {
			t.Error("FindByID must not run for an over-long service_id")
			return nil, domain.ErrNotFound
		}}

		_, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), ServiceID: strings.Repeat("a", 65)})
		assertSemanticError(t, err, domain.ErrCodeInvalidInput, "el identificador del servicio no es válido.")
	})

	t.Run("FTS operators are rejected with the shared semantic validator", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{}
		services := &mockServicesRepo{}

		_, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), QueryText: "corte* OR algo"})
		assertSemanticError(t, err, domain.ErrCodeInvalidInput, "")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("sentinel lost: %v", err)
		}
	})

	t.Run("over-long query is rejected", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{}
		services := &mockServicesRepo{}

		_, err := newSearchProfessionalsUC(professionals, services).Execute(context.Background(),
			dto.SearchProfessionalsAdvancedInput{Caller: clientCaller("c1"), QueryText: strings.Repeat("a", 201)})
		assertSemanticError(t, err, domain.ErrCodeInvalidInput, "")
	})

	t.Run("unauthenticated rejected", func(t *testing.T) {
		uc := NewSearchProfessionalsAdvancedUseCase(&mockProfessionalsRepo{}, &mockServicesRepo{}, repository.QueryValidator{})
		_, err := uc.Execute(context.Background(), dto.SearchProfessionalsAdvancedInput{QueryText: "romina"})
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("got %v, want ErrUnauthenticated", err)
		}
	})
}
