package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
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

	t.Run("staff gets the active booking catalog, not the FTS view", func(t *testing.T) {
		uc := NewSearchServicesAdvancedUseCase(&mockServicesRepo{
			FindActiveFn: func(context.Context) ([]*entity.Service, error) {
				return []*entity.Service{{ID: "s1", Name: "Corte", Active: true}}, nil
			},
			// SearchFTSFn deliberately unset: the mock panics if the maintenance
			// path is taken, which is the assertion that matters here.
		})
		result, err := uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{
			Caller:    auth.Caller{ID: "staff-1", Role: auth.RoleStaff, ProfessionalID: ptr("p1")},
			QueryText: "corte",
		})
		if err != nil {
			t.Fatalf("staff must receive the booking catalog (issue #255): %v", err)
		}
		if len(result.Results) != 1 || result.Results[0].ID != "s1" {
			t.Errorf("got %+v; want the active service", result.Results)
		}
	})

	t.Run("client gets the same booking catalog", func(t *testing.T) {
		uc := NewSearchServicesAdvancedUseCase(&mockServicesRepo{
			FindActiveFn: func(context.Context) ([]*entity.Service, error) {
				return []*entity.Service{{ID: "s1", Name: "Corte", Active: true}}, nil
			},
		})
		result, err := uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{
			Caller:    auth.Caller{ID: "client-1", Role: auth.RoleClient, ClientID: ptr("c1")},
			QueryText: "corte",
		})
		if err != nil {
			t.Fatalf("client must receive the booking catalog: %v", err)
		}
		if len(result.Results) != 1 {
			t.Errorf("got %d results; want 1", len(result.Results))
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

// TestSearchServicesAdvancedBadFTSInputIsSemantic wires the real repository
// adapter, so a disallowed FTS character is rejected by the adapter validation
// and must surface at the use-case boundary as *domain.SemanticError with
// ErrCodeInvalidInput and a clean Spanish message (F2). Test-only import of the
// persistence adapter, same shape as internal/mcp/server_integration_test.go.
func TestSearchServicesAdvancedBadFTSInputIsSemantic(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	uc := NewSearchServicesAdvancedUseCase(repository.NewServicesRepo(db))
	_, err = uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{
		Caller:    auth.Caller{ID: "owner-1", Role: auth.RoleOwner},
		QueryText: "corte*",
	})

	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "la consulta de búsqueda contiene caracteres no permitidos.")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("sentinel lost: %v", err)
	}
}

// TestSearchServicesAdvancedBookingCatalogEmptyTerm pins the rule the integration
// tier only observed indirectly: for a non-privileged caller an empty (or
// whitespace-only) term means THE WHOLE ACTIVE CATALOG, not an error and not an
// empty result. The Booking catalog is what a client reads before it knows any
// service name, so emptying it on a blank term would break the flow silently.
func TestSearchServicesAdvancedBookingCatalogEmptyTerm(t *testing.T) {
	for _, term := range []string{"", "   "} {
		uc := NewSearchServicesAdvancedUseCase(&mockServicesRepo{
			FindActiveFn: func(context.Context) ([]*entity.Service, error) {
				return []*entity.Service{
					{ID: "s1", Name: "Corte", Active: true},
					{ID: "s2", Name: "Color", Active: true},
				}, nil
			},
			// SearchFTSFn unset: the mock panics if the maintenance path is taken.
		})

		result, err := uc.Execute(context.Background(), dto.SearchServicesAdvancedInput{
			Caller:    auth.Caller{ID: "client-1", Role: auth.RoleClient, ClientID: ptr("c1")},
			QueryText: term,
		})
		if err != nil {
			t.Fatalf("term %q: a client with an empty term must get the whole catalog: %v", term, err)
		}
		if len(result.Results) != 2 {
			t.Errorf("term %q: got %d services; want the whole active catalog (2)", term, len(result.Results))
		}
	}
}
