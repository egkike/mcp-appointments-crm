package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// buenosAiresLoc returns the business timezone used across the validation tests.
func buenosAiresLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

// defaultTimeSlot returns a valid Monday 10:00 slot plus deps where every step
// of the 5-step chain passes. Tests mutate the slot/deps to force a failure.
func defaultTimeSlot(t *testing.T) (SlotInput, BookingTimeValidatorDeps) {
	t.Helper()
	loc := buenosAiresLoc(t)
	start := futureDateInTZ("10:00", loc)
	slot := SlotInput{
		ProfessionalID: "pro-1",
		Service:        &entity.Service{ID: "svc-1", Name: "Corte", DurationMinutes: 60},
		Professional:   &entity.Professional{ID: "pro-1", Name: "Juan"},
		BusinessProfile: &entity.BusinessProfile{
			Timezone:      "America/Argentina/Buenos_Aires",
			BusinessHours: `{"1":{"open":"09:00","close":"18:00"}}`,
		},
		Schedule: &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00"},
		Start:    start,
	}
	deps := BookingTimeValidatorDeps{
		Bookings: &mockBookingsRepo{OnFindOverlapping: func(context.Context, string, time.Time, time.Time) ([]*entity.Booking, error) {
			return nil, nil
		}},
	}
	return slot, deps
}

func TestValidateBookingTimeSlot(t *testing.T) {
	code := func(c domain.ErrCode) *domain.ErrCode { return &c }
	scheduleProStartsAt := func(hhmm string) *entity.Schedule {
		return &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: hhmm, EndTime: "17:00"}
	}

	tests := []struct {
		name          string
		mutate        func(*SlotInput)
		overlapResult []*entity.Booking
		overlapErr    error
		wantCode      *domain.ErrCode
		wantNoOverlap bool
	}{
		{
			name: "past_time",
			mutate: func(s *SlotInput) {
				s.Start = time.Now().Add(-2 * time.Hour)
			},
			wantCode:      code(domain.ErrCodeSlotInPast),
			wantNoOverlap: true,
		},
		{
			name: "business_closed_exception",
			mutate: func(s *SlotInput) {
				s.Exception = &entity.BusinessHoursException{IsClosed: true, Reason: strPtr("feriado")}
			},
			wantCode: code(domain.ErrCodeBusinessClosed),
		},
		{
			name: "business_closed_json_fallback",
			mutate: func(s *SlotInput) {
				s.BusinessProfile.BusinessHours = `{}`
			},
			wantCode: code(domain.ErrCodeBusinessClosed),
		},
		{
			name: "professional_not_working",
			mutate: func(s *SlotInput) {
				s.Schedule = nil
			},
			wantCode: code(domain.ErrCodeProfessionalNotWorking),
		},
		{
			name: "slot_ends_after_close",
			mutate: func(s *SlotInput) {
				s.Start = futureDateInTZ("17:00", buenosAiresLoc(t))
				s.Service = &entity.Service{ID: "svc-1", Name: "Tinte", DurationMinutes: 120}
			},
			wantCode: code(domain.ErrCodeSlotOutOfHours),
		},
		{
			name: "slot_starts_before_business_open",
			mutate: func(s *SlotInput) {
				s.Start = futureDateInTZ("08:00", buenosAiresLoc(t))
			},
			wantCode: code(domain.ErrCodeSlotOutOfHours),
		},
		{
			name: "slot_starts_before_professional_start",
			mutate: func(s *SlotInput) {
				s.Schedule = scheduleProStartsAt("10:00")
				s.Start = futureDateInTZ("09:30", buenosAiresLoc(t))
			},
			wantCode: code(domain.ErrCodeSlotOutOfHours),
		},
		{
			name: "overlap_detected",
			overlapResult: []*entity.Booking{
				{ProfessionalID: "pro-1", StartDatetime: futureDateInTZ("10:00", buenosAiresLoc(t)), EndDatetime: futureDateInTZ("11:00", buenosAiresLoc(t))},
			},
			wantCode: code(domain.ErrCodeBookingOverlap),
		},
		{
			name:       "find_overlapping_error",
			overlapErr: errors.New("db timeout"),
			wantCode:   code(domain.ErrCodeInternal),
		},
		{
			name:     "all_pass",
			wantCode: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot, deps := defaultTimeSlot(t)
			bk, ok := deps.Bookings.(*mockBookingsRepo)
			if !ok {
				t.Fatal("deps.Bookings is not *mockBookingsRepo")
			}
			var overlapCalls int
			bk.OnFindOverlapping = func(context.Context, string, time.Time, time.Time) ([]*entity.Booking, error) {
				overlapCalls++
				return tt.overlapResult, tt.overlapErr
			}
			if tt.mutate != nil {
				tt.mutate(&slot)
			}

			err := ValidateBookingTimeSlot(context.Background(), slot, deps)

			if tt.wantCode == nil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected code %q, got nil error", *tt.wantCode)
				}
				if err.Code != *tt.wantCode {
					t.Errorf("code = %q; want %q", err.Code, *tt.wantCode)
				}
			}

			if tt.wantNoOverlap && overlapCalls != 0 {
				t.Errorf("FindOverlapping called %d times; want 0 (short-circuit)", overlapCalls)
			}
			if tt.name == "all_pass" && overlapCalls == 0 {
				t.Errorf("all_pass expected the overlap query to run; got %d calls", overlapCalls)
			}
		})
	}
}
