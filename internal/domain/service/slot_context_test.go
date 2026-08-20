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

func intPtr(i int) *int { return &i }

// defaultSlotMocks returns the four repos wired so every resolution step
// succeeds for a Monday slot: an active professional, the Buenos Aires
// business profile, a concrete exception and a Monday schedule. The closures
// record the weekday and exception date the resolver asks for, so tests can
// assert the LOCAL values reach the repos.
func defaultSlotMocks(capturedDay *int, capturedExcDate *time.Time) (
	pros *mockProfessionalsRepo,
	profile *mockBusinessProfileRepo,
	excs *mockBusinessHoursExceptionRepo,
	scheds *mockSchedulesRepo,
) {
	pros = &mockProfessionalsRepo{OnFindByID: func(_ context.Context, _ string) (*entity.Professional, error) {
		return &entity.Professional{ID: "pro-1", Name: "Juan", Status: "active"}, nil
	}}
	profile = &mockBusinessProfileRepo{OnGet: func(_ context.Context) (*entity.BusinessProfile, error) {
		return &entity.BusinessProfile{Timezone: "America/Argentina/Buenos_Aires"}, nil
	}}
	excs = &mockBusinessHoursExceptionRepo{OnGet: func(_ context.Context, d time.Time) (*entity.BusinessHoursException, error) {
		*capturedExcDate = d
		return &entity.BusinessHoursException{ID: 1, ExceptionDate: "2026-08-03", IsClosed: false}, nil
	}}
	scheds = &mockSchedulesRepo{OnFindByProfessionalAndDay: func(_ context.Context, _ string, day int) (*entity.Schedule, error) {
		*capturedDay = day
		return &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00"}, nil
	}}
	return pros, profile, excs, scheds
}

func TestResolveSlotContext(t *testing.T) {
	loc := buenosAiresLoc(t)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// 2026-08-03 10:00 UTC is a Monday that lands at 07:00 in Buenos Aires
	// (UTC-3, no DST): the slot's LOCAL wall-clock window.
	mondayUTC := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	mondayLocal := time.Date(2026, 8, 3, 7, 0, 0, 0, loc)
	mondayDate := time.Date(2026, 8, 3, 0, 0, 0, 0, loc)

	// 2026-08-03 23:30 UTC lands at 2026-08-04 08:30 in Tokyo (UTC+9): the
	// schedule must be looked up for the LOCAL weekday (Tuesday=2) and the
	// exception for the LOCAL date (2026-08-04), not the UTC values.
	tokyoStart := time.Date(2026, 8, 3, 23, 30, 0, 0, time.UTC)
	tokyoLocal := time.Date(2026, 8, 4, 8, 30, 0, 0, tokyo)
	tokyoDate := time.Date(2026, 8, 4, 0, 0, 0, 0, tokyo)

	// 2026-08-04 01:00 UTC is still 2026-08-03 22:00 in Buenos Aires: the
	// exception date must be the LOCAL date (the 3rd), not the UTC one.
	lateStart := time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC)
	lateLocal := time.Date(2026, 8, 3, 22, 0, 0, 0, loc)

	tests := []struct {
		name                string
		start               time.Time
		mutate              func(pros *mockProfessionalsRepo, profile *mockBusinessProfileRepo, excs *mockBusinessHoursExceptionRepo, scheds *mockSchedulesRepo)
		wantLocal           *time.Time
		wantDay             *int
		wantExcDate         *time.Time
		wantExceptionNil    bool
		wantScheduleNil     bool
		wantSemCode         domain.ErrCode // "" = not a semantic error
		wantErrHas          string         // substring of the wrapped error message
		wantErrInvalidInput bool
	}{
		{
			name:        "happy_path_resolves_all_entities",
			start:       mondayUTC,
			wantLocal:   &mondayLocal,
			wantDay:     intPtr(1),
			wantExcDate: &mondayDate,
		},
		{
			name:  "schedule_and_exception_keyed_on_local_weekday_and_date_across_utc_boundary",
			start: tokyoStart,
			mutate: func(_ *mockProfessionalsRepo, profile *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				profile.OnGet = func(context.Context) (*entity.BusinessProfile, error) {
					return &entity.BusinessProfile{Timezone: "Asia/Tokyo"}, nil
				}
			},
			wantLocal:   &tokyoLocal,
			wantDay:     intPtr(2),
			wantExcDate: &tokyoDate,
		},
		{
			name:        "exception_date_uses_local_date_when_utc_date_differs",
			start:       lateStart,
			wantLocal:   &lateLocal,
			wantDay:     intPtr(1),
			wantExcDate: &mondayDate,
		},
		{
			name:  "professional_not_found_returns_semantic_not_found",
			start: mondayUTC,
			mutate: func(pros *mockProfessionalsRepo, _ *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				pros.OnFindByID = func(context.Context, string) (*entity.Professional, error) {
					return nil, domain.ErrNotFound
				}
			},
			wantSemCode: domain.ErrCodeNotFound,
			wantErrHas:  "no encontrado",
		},
		{
			name:  "professional_lookup_failure_wraps_operation_context",
			start: mondayUTC,
			mutate: func(pros *mockProfessionalsRepo, _ *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				pros.OnFindByID = func(context.Context, string) (*entity.Professional, error) {
					return nil, errors.New("boom")
				}
			},
			wantErrHas: "crear reserva: consultar profesional",
		},
		{
			name:  "inactive_professional_returns_semantic_not_active",
			start: mondayUTC,
			mutate: func(pros *mockProfessionalsRepo, _ *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				pros.OnFindByID = func(_ context.Context, _ string) (*entity.Professional, error) {
					return &entity.Professional{ID: "pro-1", Name: "Juan", Status: "inactive"}, nil
				}
			},
			wantSemCode: domain.ErrCodeProfessionalNotActive,
			wantErrHas:  "no está activo",
		},
		{
			name:  "profile_lookup_failure_wraps_operation_context",
			start: mondayUTC,
			mutate: func(_ *mockProfessionalsRepo, profile *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				profile.OnGet = func(context.Context) (*entity.BusinessProfile, error) {
					return nil, errors.New("boom")
				}
			},
			wantErrHas: "crear reserva: consultar perfil de negocio",
		},
		{
			name:  "invalid_timezone_wraps_err_invalid_input",
			start: mondayUTC,
			mutate: func(_ *mockProfessionalsRepo, profile *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				profile.OnGet = func(context.Context) (*entity.BusinessProfile, error) {
					return &entity.BusinessProfile{Timezone: "Mars/Olympus_Mons"}, nil
				}
			},
			wantErrHas:          "zona horaria",
			wantErrInvalidInput: true,
		},
		{
			name:  "exception_lookup_failure_wraps_operation_context",
			start: mondayUTC,
			mutate: func(_ *mockProfessionalsRepo, _ *mockBusinessProfileRepo, excs *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				excs.OnGet = func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
					return nil, errors.New("boom")
				}
			},
			wantErrHas: "crear reserva: consultar excepción",
		},
		{
			name:  "missing_exception_is_tolerated",
			start: mondayUTC,
			mutate: func(_ *mockProfessionalsRepo, _ *mockBusinessProfileRepo, excs *mockBusinessHoursExceptionRepo, _ *mockSchedulesRepo) {
				excs.OnGet = func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
					return nil, domain.ErrNotFound
				}
			},
			wantExceptionNil: true,
		},
		{
			name:  "schedule_lookup_failure_wraps_operation_context",
			start: mondayUTC,
			mutate: func(_ *mockProfessionalsRepo, _ *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, scheds *mockSchedulesRepo) {
				scheds.OnFindByProfessionalAndDay = func(context.Context, string, int) (*entity.Schedule, error) {
					return nil, errors.New("boom")
				}
			},
			wantErrHas: "crear reserva: consultar horario",
		},
		{
			name:  "missing_schedule_is_tolerated",
			start: mondayUTC,
			mutate: func(_ *mockProfessionalsRepo, _ *mockBusinessProfileRepo, _ *mockBusinessHoursExceptionRepo, scheds *mockSchedulesRepo) {
				scheds.OnFindByProfessionalAndDay = func(context.Context, string, int) (*entity.Schedule, error) {
					return nil, domain.ErrNotFound
				}
			},
			wantScheduleNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedDay int
			var capturedExcDate time.Time
			pros, profile, excs, scheds := defaultSlotMocks(&capturedDay, &capturedExcDate)
			if tt.mutate != nil {
				tt.mutate(pros, profile, excs, scheds)
			}

			got, err := ResolveSlotContext(context.Background(), "crear reserva", pros, profile, excs, scheds, "pro-1", tt.start)

			if tt.wantSemCode != "" {
				assertSemanticError(t, err, tt.wantSemCode, tt.wantErrHas)
				return
			}
			if tt.wantErrHas != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrHas) {
					t.Errorf("error = %q; want contains %q", err.Error(), tt.wantErrHas)
				}
				if tt.wantErrInvalidInput && !errors.Is(err, domain.ErrInvalidInput) {
					t.Errorf("expected errors.Is(err, ErrInvalidInput); got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Professional == nil || got.Professional.ID != "pro-1" {
				t.Errorf("Professional = %+v; want pro-1", got.Professional)
			}
			if got.Profile == nil {
				t.Error("Profile = nil; want the business profile")
			}
			if tt.wantLocal != nil && !got.LocalStart.Equal(*tt.wantLocal) {
				t.Errorf("LocalStart = %v; want %v", got.LocalStart, *tt.wantLocal)
			}
			if tt.wantDay != nil && capturedDay != *tt.wantDay {
				t.Errorf("schedule lookup day = %d; want %d (LOCAL weekday)", capturedDay, *tt.wantDay)
			}
			if tt.wantExcDate != nil && !capturedExcDate.Equal(*tt.wantExcDate) {
				t.Errorf("exception lookup date = %v; want %v (LOCAL date)", capturedExcDate, *tt.wantExcDate)
			}
			if tt.wantExceptionNil {
				if got.Exception != nil {
					t.Errorf("Exception = %+v; want nil (no exception that day)", got.Exception)
				}
			} else if got.Exception == nil {
				t.Error("Exception = nil; want the resolved exception")
			}
			if tt.wantScheduleNil {
				if got.Schedule != nil {
					t.Errorf("Schedule = %+v; want nil (no schedule that day)", got.Schedule)
				}
			} else if got.Schedule == nil {
				t.Error("Schedule = nil; want the resolved schedule")
			}
		})
	}
}
