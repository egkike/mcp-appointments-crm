package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestGetBusinessProfileUseCase(t *testing.T) {
	t.Run("happy path returns the singleton profile", func(t *testing.T) {
		want := businessProfileUTC()
		repo := &mockBusinessProfileRepo{
			GetFn: func(ctx context.Context) (*entity.BusinessProfile, error) {
				if ctx == nil {
					t.Error("Get called with nil context")
				}
				return want, nil
			},
		}
		uc := NewGetBusinessProfileUseCase(repo)

		got, err := uc.Execute(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Execute() = %p; want the repo result %p", got, want)
		}
	})

	t.Run("missing profile maps to semantic not-found error", func(t *testing.T) {
		repo := &mockBusinessProfileRepo{
			GetFn: func(context.Context) (*entity.BusinessProfile, error) {
				return nil, domain.ErrNotFound
			},
		}
		uc := NewGetBusinessProfileUseCase(repo)

		_, err := uc.Execute(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("error type = %T; want *domain.SemanticError", err)
		}
		if sem.Code != domain.ErrCodeNotFound {
			t.Errorf("SemanticError.Code = %q; want %q", sem.Code, domain.ErrCodeNotFound)
		}
		if sem.Message != "perfil del negocio no encontrado" {
			t.Errorf("SemanticError.Message = %q; want %q", sem.Message, "perfil del negocio no encontrado")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("error must wrap domain.ErrNotFound (errors.Is)")
		}
	})

	t.Run("repo failure is wrapped, not masked", func(t *testing.T) {
		inner := errors.New("disk on fire")
		repo := &mockBusinessProfileRepo{
			GetFn: func(context.Context) (*entity.BusinessProfile, error) {
				return nil, inner
			},
		}
		uc := NewGetBusinessProfileUseCase(repo)

		_, err := uc.Execute(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, inner) {
			t.Errorf("error must wrap the repo error (errors.Is)")
		}
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			t.Errorf("plain repo failures must not be masked as SemanticError: %v", err)
		}
	})
}
