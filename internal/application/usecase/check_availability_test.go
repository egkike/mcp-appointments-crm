package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// availabilityMocks holds all mocks needed for CheckAvailabilityUseCase tests.
type availabilityMocks struct {
	services      *mockServicesRepo
	professionals *mockProfessionalsRepo
	profile       *mockBusinessProfileRepo
	exceptions    *mockBusinessHoursExceptionRepo
	schedules     *mockSchedulesRepo
	bookings      *mockBookingsRepo
}

// mondayFuture is 2026-08-03 10:00 UTC, a Monday within business hours.
var mondayFuture = time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

// newAvailabilityMocks returns mocks pre-configured for the happy path
// (Monday 10:00 UTC, all entities active, no exceptions, no overlaps).
func newAvailabilityMocks() *availabilityMocks {
	m := &availabilityMocks{
		services: &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return activeService(), nil
			},
		},
		professionals: &mockProfessionalsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Professional, error) {
				return &entity.Professional{ID: "p1", Name: "Juan", Status: "active"}, nil
			},
		},
		profile: &mockBusinessProfileRepo{
			GetFn: func(_ context.Context) (*entity.BusinessProfile, error) {
				return &entity.BusinessProfile{
					Timezone:      "UTC",
					BusinessHours: `{"1":{"open":"09:00","close":"18:00"}}`,
				}, nil
			},
		},
		exceptions: &mockBusinessHoursExceptionRepo{
			GetFn: func(_ context.Context, _ time.Time) (*entity.BusinessHoursException, error) {
				return nil, domain.ErrNotFound
			},
		},
		schedules: &mockSchedulesRepo{
			FindByProfessionalAndDayFn: func(_ context.Context, _ string, _ int) (*entity.Schedule, error) {
				return &entity.Schedule{ProfessionalID: "p1", DayOfWeek: 1, StartTime: "09:00", EndTime: "18:00"}, nil
			},
		},
		bookings: &mockBookingsRepo{
			FindOverlappingFn: func(_ context.Context, _ string, _, _ time.Time) ([]*entity.Booking, error) {
				return nil, nil
			},
		},
	}
	return m
}

func (m *availabilityMocks) deps() service.AvailabilityDeps {
	return service.AvailabilityDeps{
		Services:                m.services,
		Professionals:           m.professionals,
		BusinessProfile:         m.profile,
		BusinessHoursExceptions: m.exceptions,
		Schedules:               m.schedules,
		Bookings:                m.bookings,
	}
}

func (m *availabilityMocks) newUseCase() *CheckAvailabilityUseCase {
	return NewCheckAvailabilityUseCase(service.NewAvailabilityService(), m.deps())
}

func TestCheckAvailabilityUseCase(t *testing.T) {
	t.Run("happy path returns available true", func(t *testing.T) {
		m := newAvailabilityMocks()
		uc := m.newUseCase()

		result, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Available {
			t.Error("expected Available=true")
		}
	})

	t.Run("empty caller does not trigger auth error", func(t *testing.T) {
		m := newAvailabilityMocks()
		uc := m.newUseCase()

		result, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			Caller:         emptyCaller(),
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err != nil {
			t.Fatalf("expected no error for empty caller (non-auth use case); got %v", err)
		}
		if !result.Available {
			t.Error("expected Available=true")
		}
	})

	t.Run("service not found", func(t *testing.T) {
		m := newAvailabilityMocks()
		m.services.FindByIDFn = func(_ context.Context, _ string) (*entity.Service, error) {
			return nil, domain.ErrNotFound
		}
		uc := m.newUseCase()

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			ServiceID:      "s-nonexistent",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
		if !strings.Contains(err.Error(), "consultar servicio") {
			t.Errorf("expected error to mention service lookup; got %q", err.Error())
		}
	})

	t.Run("professional not found", func(t *testing.T) {
		m := newAvailabilityMocks()
		m.professionals.FindByIDFn = func(_ context.Context, _ string) (*entity.Professional, error) {
			return nil, domain.ErrNotFound
		}
		uc := m.newUseCase()

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			ServiceID:      "s1",
			ProfessionalID: "p-nonexistent",
			StartDatetime:  mondayFuture,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
		if !strings.Contains(err.Error(), "consultar profesional") {
			t.Errorf("expected error to mention professional lookup; got %q", err.Error())
		}
	})

	t.Run("business closed on exception date", func(t *testing.T) {
		m := newAvailabilityMocks()
		m.exceptions.GetFn = func(_ context.Context, _ time.Time) (*entity.BusinessHoursException, error) {
			return &entity.BusinessHoursException{
				IsClosed: true,
				Reason:   ptr("feriado"),
			}, nil
		}
		uc := m.newUseCase()

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeBusinessClosed {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeBusinessClosed)
		}
		if !strings.Contains(sem.Message, "cerrado") {
			t.Errorf("expected Spanish closed message; got %q", sem.Message)
		}
	})

	t.Run("slot before business open hours", func(t *testing.T) {
		m := newAvailabilityMocks()
		uc := m.newUseCase()

		// 07:00 UTC is before 09:00 business open
		earlyTime := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  earlyTime,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeSlotOutOfHours {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeSlotOutOfHours)
		}
	})

	t.Run("slot in the past", func(t *testing.T) {
		m := newAvailabilityMocks()
		uc := m.newUseCase()

		// 2025-01-06 is a Monday, 10:00 UTC — within hours but in the past
		pastTime := time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC)
		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  pastTime,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeSlotInPast {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeSlotInPast)
		}
		if !strings.Contains(sem.Message, "pasado") {
			t.Errorf("expected Spanish past message; got %q", sem.Message)
		}
	})

	t.Run("overlap with existing booking", func(t *testing.T) {
		m := newAvailabilityMocks()
		m.bookings.FindOverlappingFn = func(_ context.Context, _ string, _, _ time.Time) ([]*entity.Booking, error) {
			return []*entity.Booking{
				{
					ProfessionalID: "p1",
					StartDatetime:  mondayFuture,
					EndDatetime:    mondayFuture.Add(60 * time.Minute),
				},
			}, nil
		}
		uc := m.newUseCase()

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityParams{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeBookingOverlap {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeBookingOverlap)
		}
		if !strings.Contains(sem.Message, "ya tiene una reserva") {
			t.Errorf("expected Spanish overlap message; got %q", sem.Message)
		}
	})
}
