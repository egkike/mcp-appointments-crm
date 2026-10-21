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

// ── S7b (feat-owner-chat): get_professional_schedules ──

// mockProfessionalSchedulesRepo is the S7b read double. It embeds the shared
// schedules mock from mocks_test.go so the other suites keep their fn tables,
// and adds the one read this tool needs.
type mockProfessionalSchedulesRepo struct {
	*mockSchedulesRepo
	FindByProfessionalFn func(ctx context.Context, professionalID string) ([]*entity.Schedule, error)
}

func (m *mockProfessionalSchedulesRepo) FindByProfessional(ctx context.Context, professionalID string) ([]*entity.Schedule, error) {
	if m.FindByProfessionalFn == nil {
		panic("mockProfessionalSchedulesRepo.FindByProfessionalFn not set")
	}
	return m.FindByProfessionalFn(ctx, professionalID)
}

// FindByProfessional completes mockSchedulesRepo's implementation of the
// domainrepo.SchedulesRepo port, which S7b grew with the weekly read. It lives
// here (and not next to the rest of the mock) because this slice's edit surface
// stops at the S7b test file; no other use case consumes the read, so an
// unexpected call means a test is wired against the wrong double: fail loudly
// instead of answering with a fabricated agenda.
func (m *mockSchedulesRepo) FindByProfessional(context.Context, string) ([]*entity.Schedule, error) {
	panic("mockSchedulesRepo.FindByProfessional: use mockProfessionalSchedulesRepo (get_professional_schedules_test.go)")
}

// agendaOwnerProfessional is the fixture used by most subtests: an active
// professional whose weekly agenda is the two rows below.
func agendaOwnerProfessional() *entity.Professional {
	return &entity.Professional{ID: "p-romina", Name: "Romina", Status: "active"}
}

func agendaWeek() []*entity.Schedule {
	return []*entity.Schedule{
		{ID: 1, ProfessionalID: "p-romina", DayOfWeek: 1, StartTime: "09:00", EndTime: "13:00"},
		{ID: 2, ProfessionalID: "p-romina", DayOfWeek: 3, StartTime: "14:00", EndTime: "18:00"},
	}
}

func TestGetProfessionalSchedulesUseCase(t *testing.T) {
	t.Run("owner reads the agenda with the professional identity", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			if id != "p-romina" {
				t.Errorf("FindByID(%q); want p-romina", id)
			}
			return agendaOwnerProfessional(), nil
		}}
		schedules := &mockProfessionalSchedulesRepo{
			mockSchedulesRepo: &mockSchedulesRepo{},
			FindByProfessionalFn: func(ctx context.Context, professionalID string) ([]*entity.Schedule, error) {
				if professionalID != "p-romina" {
					t.Errorf("FindByProfessional(%q); want p-romina", professionalID)
				}
				return agendaWeek(), nil
			},
		}

		result, err := NewGetProfessionalSchedulesUseCase(professionals, schedules).Execute(context.Background(),
			dto.GetProfessionalSchedulesInput{Caller: ownerCaller(), ProfessionalID: "p-romina"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("result = nil; want the professional identity plus the week")
		}
		if result.Professional.ID != "p-romina" || result.Professional.Name != "Romina" || result.Professional.Status != "active" {
			t.Errorf("Professional = %+v; want p-romina/Romina/active", result.Professional)
		}
		if len(result.Schedules) != 2 {
			t.Fatalf("Schedules = %+v; want the two stored rows", result.Schedules)
		}
		if result.Schedules[0].DayOfWeek != 1 || result.Schedules[1].DayOfWeek != 3 {
			t.Errorf("Schedules days = %d,%d; want 1,3 in day order",
				result.Schedules[0].DayOfWeek, result.Schedules[1].DayOfWeek)
		}
	})

	t.Run("admin staff and client are rejected with a semantic forbidden", func(t *testing.T) {
		cases := map[string]struct {
			caller auth.Caller
			role   string
		}{
			"admin":  {adminCaller(), "admin"},
			"staff":  {staffCaller("staff1", "p-romina"), "staff"},
			"client": {clientCaller("c1"), "client"},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				// Both repos stay fn-less on purpose: a non-owner caller must be
				// rejected before any read (a read would panic in the mocks).
				professionals := &mockProfessionalsRepo{}
				schedules := &mockProfessionalSchedulesRepo{mockSchedulesRepo: &mockSchedulesRepo{}}

				_, err := NewGetProfessionalSchedulesUseCase(professionals, schedules).Execute(context.Background(),
					dto.GetProfessionalSchedulesInput{Caller: tc.caller, ProfessionalID: "p-romina"})
				assertSemanticError(t, err, domain.ErrCodeForbidden, `Rol "`+tc.role+`" no tiene permiso para esta operación`)
				if !errors.Is(err, domain.ErrForbidden) {
					t.Errorf("sentinel lost: %v", err)
				}
			})
		}
	})

	t.Run("unknown professional id returns a semantic NOT_FOUND", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return nil, domain.ErrNotFound
		}}
		schedules := &mockProfessionalSchedulesRepo{mockSchedulesRepo: &mockSchedulesRepo{}}

		_, err := NewGetProfessionalSchedulesUseCase(professionals, schedules).Execute(context.Background(),
			dto.GetProfessionalSchedulesInput{Caller: ownerCaller(), ProfessionalID: "p-missing"})
		assertSemanticError(t, err, domain.ErrCodeNotFound, "el profesional indicado no existe")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("sentinel lost: %v", err)
		}
	})

	t.Run("existing professional without rows returns an empty list", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return agendaOwnerProfessional(), nil
		}}
		schedules := &mockProfessionalSchedulesRepo{
			mockSchedulesRepo: &mockSchedulesRepo{},
			FindByProfessionalFn: func(ctx context.Context, professionalID string) ([]*entity.Schedule, error) {
				return nil, nil
			},
		}

		result, err := NewGetProfessionalSchedulesUseCase(professionals, schedules).Execute(context.Background(),
			dto.GetProfessionalSchedulesInput{Caller: ownerCaller(), ProfessionalID: "p-romina"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Schedules == nil {
			t.Fatal("Schedules = nil; want an empty (non-nil) slice so the wire renders []")
		}
		if len(result.Schedules) != 0 {
			t.Fatalf("Schedules = %+v; want empty (the professional exists, the agenda does not)", result.Schedules)
		}
		if result.Professional.Name != "Romina" {
			t.Errorf("Professional = %+v; the identity must survive an empty agenda", result.Professional)
		}
	})

	// ── TRIANGULATE ──

	t.Run("inactive professional is returned with its status, never hidden", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return &entity.Professional{ID: "p-old", Name: "Joaquin", Status: "inactive"}, nil
		}}
		schedules := &mockProfessionalSchedulesRepo{
			mockSchedulesRepo: &mockSchedulesRepo{},
			FindByProfessionalFn: func(ctx context.Context, professionalID string) ([]*entity.Schedule, error) {
				return agendaWeek(), nil
			},
		}

		result, err := NewGetProfessionalSchedulesUseCase(professionals, schedules).Execute(context.Background(),
			dto.GetProfessionalSchedulesInput{Caller: ownerCaller(), ProfessionalID: "p-old"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Professional.Status != "inactive" {
			t.Errorf("Status = %q; want inactive (the delete gate must see whom it confirms)", result.Professional.Status)
		}
		if len(result.Schedules) != 2 {
			t.Errorf("Schedules = %+v; want the stored rows of the inactive professional", result.Schedules)
		}
	})

	t.Run("the read re-injects the caller instead of trusting the transport", func(t *testing.T) {
		// Only input.Caller is populated (no caller on ctx): if the use case did
		// not re-inject it, auth.RequireRole would answer UNAUTHENTICATED instead
		// of resolving the owner.
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			if _, err := auth.RequireCaller(ctx); err != nil {
				t.Errorf("caller missing from ctx: %v", err)
			}
			return agendaOwnerProfessional(), nil
		}}
		schedules := &mockProfessionalSchedulesRepo{
			mockSchedulesRepo: &mockSchedulesRepo{},
			FindByProfessionalFn: func(ctx context.Context, professionalID string) ([]*entity.Schedule, error) {
				return agendaWeek(), nil
			},
		}

		if _, err := NewGetProfessionalSchedulesUseCase(professionals, schedules).Execute(context.Background(),
			dto.GetProfessionalSchedulesInput{Caller: ownerCaller(), ProfessionalID: "p-romina"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("the repository sentinel from the read is mapped to NOT_FOUND", func(t *testing.T) {
		professionals := &mockProfessionalsRepo{FindByIDFn: func(ctx context.Context, id string) (*entity.Professional, error) {
			return agendaOwnerProfessional(), nil
		}}
		schedules := &mockProfessionalSchedulesRepo{
			mockSchedulesRepo: &mockSchedulesRepo{},
			FindByProfessionalFn: func(ctx context.Context, professionalID string) ([]*entity.Schedule, error) {
				return nil, domain.ErrNotFound
			},
		}

		_, err := NewGetProfessionalSchedulesUseCase(professionals, schedules).Execute(context.Background(),
			dto.GetProfessionalSchedulesInput{Caller: ownerCaller(), ProfessionalID: "p-romina"})
		assertSemanticError(t, err, domain.ErrCodeNotFound, "el profesional indicado no existe")
	})
}
