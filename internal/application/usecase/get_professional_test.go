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

// ── feat-staff-self-read S1/S2/S4 (issue #276): get_professional ──

// contactProfessional is the fixture of the record read: a professional that
// carries both contact fields and a specialty list, so every assertion below can
// tell "field present" from "field omitted" instead of comparing zeros.
func contactProfessional() *entity.Professional {
	role := "Estilista"
	email := "romina@example.com"
	phone := "+5491100000009"
	specialties := `["s-lavado","s-corte"]`
	return &entity.Professional{
		ID:            "p-romina",
		Name:          "Romina",
		RoleSpecialty: &role,
		Status:        "active",
		Email:         &email,
		Phone:         &phone,
		Specialties:   &specialties,
	}
}

func TestGetProfessionalUseCase(t *testing.T) {
	t.Run("owner reads any professional with contact data", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			if id != "p-romina" {
				t.Errorf("FindByID(%q); want p-romina", id)
			}
			return contactProfessional(), nil
		}}

		result, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: ownerCaller(), ProfessionalID: " p-romina "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("result = nil; want the professional record")
		}
		got := result.Professional
		if got.ID != "p-romina" || got.Name != "Romina" || got.Status != "active" {
			t.Errorf("Professional = %+v; want p-romina/Romina/active", got)
		}
		if got.RoleSpecialty == nil || *got.RoleSpecialty != "Estilista" {
			t.Errorf("RoleSpecialty = %v; want Estilista", got.RoleSpecialty)
		}
		if len(got.Specialties) != 2 || got.Specialties[0] != "s-lavado" || got.Specialties[1] != "s-corte" {
			t.Errorf("Specialties = %v; want [s-lavado s-corte]", got.Specialties)
		}
		if result.Email == nil || *result.Email != "romina@example.com" {
			t.Errorf("Email = %v; the owner must receive the contact data (S4)", result.Email)
		}
		if result.Phone == nil || *result.Phone != "+5491100000009" {
			t.Errorf("Phone = %v; the owner must receive the contact data (S4)", result.Phone)
		}
	})

	t.Run("staff omitting the id reads its own record with contact data", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			if id != "p-romina" {
				t.Errorf("FindByID(%q); want the caller's own p-romina", id)
			}
			return contactProfessional(), nil
		}}

		result, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: staffCaller("staff1", "p-romina")})
		if err != nil {
			t.Fatalf("staff must read its own record (S1): %v", err)
		}
		if result.Professional.ID != "p-romina" {
			t.Errorf("Professional.ID = %q; want the caller's own p-romina", result.Professional.ID)
		}
		if result.Email == nil || result.Phone == nil {
			t.Errorf("contact = %v/%v; the professional itself must see its own email and phone (S4)",
				result.Email, result.Phone)
		}
	})

	t.Run("staff naming its own id is the same read", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return contactProfessional(), nil
		}}

		result, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: staffCaller("staff1", "p-romina"), ProfessionalID: "p-romina"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Email == nil || result.Phone == nil {
			t.Errorf("contact = %v/%v; own record keeps the contact fields (S4)", result.Email, result.Phone)
		}
	})

	t.Run("admin reads the record but never the contact fields", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return contactProfessional(), nil
		}}

		result, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: adminCaller(), ProfessionalID: "p-romina"})
		if err != nil {
			t.Fatalf("admin must read the record (S1): %v", err)
		}
		if result.Professional.Name != "Romina" {
			t.Errorf("Professional = %+v; the identity must cross for admin", result.Professional)
		}
		// S4 / owner decision L2: oversight does not need the roster's phones.
		if result.Email != nil || result.Phone != nil {
			t.Errorf("contact = %v/%v; admin MUST NOT receive email/phone (S4)", result.Email, result.Phone)
		}
	})

	// ── scope (S2) ──

	t.Run("staff naming another professional is forbidden, not unauthenticated", func(t *testing.T) {
		// The repository stays fn-less on purpose: the scope must be decided before
		// any read (a read would panic in the mock).
		professionals := &mockProfessionalsRepo{}

		_, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: staffCaller("staff1", "p-romina"), ProfessionalID: "p-otro"})
		assertSemanticError(t, err, domain.ErrCodeForbidden, auth.MsgForbiddenTool)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("sentinel lost: %v", err)
		}
	})

	t.Run("a caller without a professional that omits the id gets an input error", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{}

		_, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: ownerCaller()})
		assertSemanticError(t, err, domain.ErrCodeInvalidInput, auth.MsgProfessionalScopeRequired)
	})

	t.Run("client is rejected by the role gate", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{}

		_, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: clientCaller("c1"), ProfessionalID: "p-romina"})
		assertSemanticError(t, err, domain.ErrCodeForbidden, `Rol "client" no tiene permiso para esta operación`)
	})

	// ── repository answers ──

	t.Run("unknown professional id returns a semantic NOT_FOUND", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return nil, domain.ErrNotFound
		}}

		_, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: ownerCaller(), ProfessionalID: "p-missing"})
		assertSemanticError(t, err, domain.ErrCodeNotFound, "el profesional indicado no existe")
	})

	t.Run("the read re-injects the caller instead of trusting the transport", func(t *testing.T) {
		// Only input.Caller is populated (no caller on ctx): if the use case did not
		// re-inject it, auth.RequireRole would answer UNAUTHENTICATED.
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			if _, err := auth.RequireCaller(ctx); err != nil {
				t.Errorf("caller missing from ctx: %v", err)
			}
			return contactProfessional(), nil
		}}

		if _, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: ownerCaller(), ProfessionalID: "p-romina"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// ── TRIANGULATE ──

	t.Run("inactive professional is returned with its status", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return &entity.Professional{ID: "p-old", Name: "Joaquin", Status: "inactive"}, nil
		}}

		result, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
			dto.GetProfessionalInput{Caller: ownerCaller(), ProfessionalID: "p-old"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Professional.Status != "inactive" {
			t.Errorf("Status = %q; want inactive (the record is never hidden from someone allowed to read it)",
				result.Professional.Status)
		}
	})

	t.Run("absent or unreadable specialties render an empty list", func(t *testing.T) {
		for name, stored := range map[string]*string{
			"nil":     nil,
			"empty":   ptr(""),
			"null":    ptr("null"),
			"invalid": ptr("not json"),
		} {
			t.Run(name, func(t *testing.T) {
				professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
					return &entity.Professional{ID: "p-romina", Name: "Romina", Status: "active", Specialties: stored}, nil
				}}

				result, err := NewGetProfessionalUseCase(professionals).Execute(context.Background(),
					dto.GetProfessionalInput{Caller: ownerCaller(), ProfessionalID: "p-romina"})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Professional.Specialties == nil {
					t.Fatal("Specialties = nil; want an empty (non-nil) slice so the wire renders []")
				}
				if len(result.Professional.Specialties) != 0 {
					t.Errorf("Specialties = %v; want empty", result.Professional.Specialties)
				}
			})
		}
	})
}
