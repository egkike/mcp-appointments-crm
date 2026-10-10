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

// TestSearchClientsAdvancedBadFTSInputIsSemantic wires the real repository
// adapter, so a disallowed FTS character is rejected by the adapter validation
// and must surface at the use-case boundary as *domain.SemanticError with
// ErrCodeInvalidInput and a clean Spanish message (F2). Before the fix the
// adapter returned plain fmt.Errorf wraps, which collapsed to -32603 at the
// transport. sqlmock only supplies the *sql.DB handle: validation short-circuits
// before any statement is prepared, so an unexpected query fails the test.
// Test-only import of the persistence adapter mirrors
// internal/mcp/server_integration_test.go; production edges stay inward.
func TestSearchClientsAdvancedBadFTSInputIsSemantic(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	uc := NewSearchClientsAdvancedUseCase(repository.NewClientsRepo(db))
	_, err = uc.Execute(context.Background(), dto.SearchClientsAdvancedInput{
		Caller:    auth.Caller{ID: "admin-1", Role: auth.RoleAdmin},
		QueryText: "juan*",
	})

	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "la consulta de búsqueda contiene caracteres no permitidos.")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("sentinel lost: %v", err)
	}
}
