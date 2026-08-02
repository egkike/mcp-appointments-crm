package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// futureDate is 2027-01-04, a Monday (dayOfWeek=1 in Go convention).
const futureDate = "2027-01-04"

func futureDateInTZ(hhmm string, loc *time.Location) time.Time {
	parts := strings.Split(hhmm, ":")
	h := int(parts[0][0]-'0')*10 + int(parts[0][1]-'0')
	m := int(parts[1][0]-'0')*10 + int(parts[1][1]-'0')
	return time.Date(2027, 1, 4, h, m, 0, 0, loc)
}

func strPtr(s string) *string { return &s }

// defaultDeps returns deps where every step passes for a Monday 10:00 slot.
func defaultDeps(t *testing.T) (AvailabilityDeps, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return AvailabilityDeps{
		Services: &mockServicesRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Service, error) {
			return &entity.Service{ID: "svc-1", Name: "Corte", DurationMinutes: 60, Active: true}, nil
		}},
		Professionals: &mockProfessionalsRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Professional, error) {
			return &entity.Professional{ID: "pro-1", Name: "Juan", Status: "active"}, nil
		}},
		BusinessProfile: &mockBusinessProfileRepo{OnGet: func(_ context.Context) (*entity.BusinessProfile, error) {
			return &entity.BusinessProfile{
				Timezone:      "America/Argentina/Buenos_Aires",
				BusinessHours: `{"1":{"open":"09:00","close":"18:00"}}`,
			}, nil
		}},
		BusinessHoursExceptions: &mockBusinessHoursExceptionRepo{OnGet: func(_ context.Context, _ time.Time) (*entity.BusinessHoursException, error) {
			return nil, domain.ErrNotFound
		}},
		Schedules: &mockSchedulesRepo{OnFindByProfessionalAndDay: func(_ context.Context, _ string, _ int) (*entity.Schedule, error) {
			return &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00"}, nil
		}},
		Bookings: &mockBookingsRepo{OnFindOverlapping: func(_ context.Context, _ string, _, _ time.Time) ([]*entity.Booking, error) {
			return nil, nil
		}},
	}, loc
}

func paramsAt(hhmm string, loc *time.Location) *CheckAvailabilityParams {
	return &CheckAvailabilityParams{
		ServiceID:      "svc-1",
		ProfessionalID: "pro-1",
		StartDatetime:  futureDateInTZ(hhmm, loc).Format(time.RFC3339),
	}
}

func assertSemanticError(t *testing.T, err error, wantCode domain.ErrCode, wantContains string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
	}
	if sem.Code != wantCode {
		t.Errorf("code = %q; want %q", sem.Code, wantCode)
	}
	if wantContains != "" && !strings.Contains(sem.Message, wantContains) {
		t.Errorf("message = %q; want contains %q", sem.Message, wantContains)
	}
}

func TestCheckAvailability(t *testing.T) {
	svc := NewAvailabilityService()
	t.Run("happy_path", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		result, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Available {
			t.Error("expected Available=true")
		}
	})
	t.Run("exception_is_closed", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.BusinessHoursExceptions = &mockBusinessHoursExceptionRepo{OnGet: func(_ context.Context, _ time.Time) (*entity.BusinessHoursException, error) {
			return &entity.BusinessHoursException{IsClosed: true, Reason: strPtr("feriado")}, nil
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeBusinessClosed, futureDate+" (feriado)")
	})
	t.Run("json_fallback_no_hours", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.BusinessProfile = &mockBusinessProfileRepo{OnGet: func(_ context.Context) (*entity.BusinessProfile, error) {
			return &entity.BusinessProfile{Timezone: "America/Argentina/Buenos_Aires", BusinessHours: `{}`}, nil
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeBusinessClosed, "lunes")
	})
	t.Run("exception_overrides_hours", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.BusinessHoursExceptions = &mockBusinessHoursExceptionRepo{OnGet: func(_ context.Context, _ time.Time) (*entity.BusinessHoursException, error) {
			return &entity.BusinessHoursException{IsClosed: false, OpenTime: strPtr("10:00"), CloseTime: strPtr("14:00")}, nil
		}}
		result, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Available {
			t.Error("expected Available=true with exception override")
		}
	})
	t.Run("professional_not_working", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.Schedules = &mockSchedulesRepo{OnFindByProfessionalAndDay: func(_ context.Context, _ string, _ int) (*entity.Schedule, error) {
			return nil, domain.ErrNotFound
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeProfessionalNotWorking, "Juan no trabaja los lunes")
	})
	t.Run("slot_ends_after_effective_close", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.Services = &mockServicesRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Service, error) {
			return &entity.Service{ID: "svc-1", Name: "Tinte", DurationMinutes: 120, Active: true}, nil
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("17:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeSlotOutOfHours, "dura 120 minutos pero solo quedan 0 antes del cierre a las 17:00")
	})
	t.Run("slot_starts_before_business_open", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		_, err := svc.CheckAvailability(context.Background(), paramsAt("08:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeSlotOutOfHours, "comienza a las 09:00")
	})
	t.Run("slot_starts_before_professional_start", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.Schedules = &mockSchedulesRepo{OnFindByProfessionalAndDay: func(_ context.Context, _ string, _ int) (*entity.Schedule, error) {
			return &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "10:00", EndTime: "17:00"}, nil
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("09:30", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeSlotOutOfHours, "Juan empieza a las 10:00")
	})
	t.Run("overlap_detected", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		exStart := futureDateInTZ("10:00", loc)
		deps.Bookings = &mockBookingsRepo{OnFindOverlapping: func(_ context.Context, _ string, _, _ time.Time) ([]*entity.Booking, error) {
			return []*entity.Booking{{ProfessionalID: "pro-1", StartDatetime: exStart, EndDatetime: exStart.Add(60 * time.Minute)}}, nil
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeBookingOverlap, "Juan ya tiene una reserva")
	})
	t.Run("past_slot", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		past := time.Date(2020, 6, 15, 10, 0, 0, 0, loc) // Monday
		params := &CheckAvailabilityParams{ServiceID: "svc-1", ProfessionalID: "pro-1", StartDatetime: past.Format(time.RFC3339)}
		_, err := svc.CheckAvailability(context.Background(), params, deps)
		assertSemanticError(t, err, domain.ErrCodeSlotInPast, "pasado")
	})
	t.Run("service_not_found", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.Services = &mockServicesRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Service, error) {
			return nil, domain.ErrNotFound
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
	})
	t.Run("professional_not_found", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.Professionals = &mockProfessionalsRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Professional, error) {
			return nil, domain.ErrNotFound
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
	})
	t.Run("service_inactive", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.Services = &mockServicesRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Service, error) {
			return &entity.Service{ID: "svc-1", Name: "Corte", DurationMinutes: 60, Active: false}, nil
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeServiceNotActive, "Servicio Corte no está activo")
	})
	t.Run("professional_inactive", func(t *testing.T) {
		deps, loc := defaultDeps(t)
		deps.Professionals = &mockProfessionalsRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Professional, error) {
			return &entity.Professional{ID: "pro-1", Name: "Juan", Status: "inactive"}, nil
		}}
		_, err := svc.CheckAvailability(context.Background(), paramsAt("10:00", loc), deps)
		assertSemanticError(t, err, domain.ErrCodeProfessionalNotActive, "Profesional Juan no está activo")
	})
}

func TestHHMMToMinutes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"midnight", "00:00", 0, false},
		{"morning", "09:30", 570, false},
		{"afternoon", "18:00", 1080, false},
		{"invalid", "abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hhmmToMinutes(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Errorf("expected errors.Is(err, ErrInvalidInput); got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("hhmmToMinutes(%q) = %d; want %d", tt.input, got, tt.want)
			}
		})
	}
}
