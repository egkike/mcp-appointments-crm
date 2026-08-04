package service

import (
	"context"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// newValidateInput returns a valid Monday 10:00 booking input where the 5-step
// chain passes. Tests mutate it to exercise each validation branch through the
// BookingValidator.Validate entry point (which delegates to the helper).
func newValidateInput(t *testing.T) ValidateBookingInput {
	t.Helper()
	loc := buenosAiresLoc(t)
	return ValidateBookingInput{
		Service:      &entity.Service{ID: "svc-1", Name: "Corte", DurationMinutes: 60, Active: true},
		Professional: &entity.Professional{ID: "pro-1", Name: "Juan", Status: "active"},
		BusinessProfile: &entity.BusinessProfile{
			Timezone:      "America/Argentina/Buenos_Aires",
			BusinessHours: `{"1":{"open":"09:00","close":"18:00"}}`,
		},
		ProfessionalSchedule: &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00"},
		NewStart:             futureDateInTZ("10:00", loc),
		Bookings: &mockBookingsRepo{OnFindOverlapping: func(context.Context, string, time.Time, time.Time) ([]*entity.Booking, error) {
			return nil, nil
		}},
	}
}

func TestBookingValidator_Validate(t *testing.T) {
	code := func(c domain.ErrCode) *domain.ErrCode { return &c }

	tests := []struct {
		name          string
		mutate        func(*ValidateBookingInput)
		overlapResult []*entity.Booking
		wantCode      *domain.ErrCode
		wantNoOverlap bool
	}{
		{
			name:     "all_pass",
			wantCode: nil,
		},
		{
			name: "past_slot",
			mutate: func(in *ValidateBookingInput) {
				in.NewStart = time.Now().Add(-2 * time.Hour)
			},
			wantCode:      code(domain.ErrCodeSlotInPast),
			wantNoOverlap: true,
		},
		{
			name: "business_closed_exception",
			mutate: func(in *ValidateBookingInput) {
				in.Exception = &entity.BusinessHoursException{IsClosed: true, Reason: strPtr("feriado")}
			},
			wantCode: code(domain.ErrCodeBusinessClosed),
		},
		{
			name: "business_closed_json_fallback",
			mutate: func(in *ValidateBookingInput) {
				in.BusinessProfile.BusinessHours = `{}`
			},
			wantCode: code(domain.ErrCodeBusinessClosed),
		},
		{
			name: "professional_not_working",
			mutate: func(in *ValidateBookingInput) {
				in.ProfessionalSchedule = nil
			},
			wantCode: code(domain.ErrCodeProfessionalNotWorking),
		},
		{
			name: "slot_ends_after_close",
			mutate: func(in *ValidateBookingInput) {
				in.NewStart = futureDateInTZ("17:00", buenosAiresLoc(t))
				in.Service = &entity.Service{ID: "svc-1", Name: "Tinte", DurationMinutes: 120, Active: true}
			},
			wantCode: code(domain.ErrCodeSlotOutOfHours),
		},
		{
			name: "slot_starts_before_business_open",
			mutate: func(in *ValidateBookingInput) {
				in.NewStart = futureDateInTZ("08:00", buenosAiresLoc(t))
			},
			wantCode: code(domain.ErrCodeSlotOutOfHours),
		},
		{
			name: "slot_starts_before_professional_start",
			mutate: func(in *ValidateBookingInput) {
				in.ProfessionalSchedule = &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "10:00", EndTime: "17:00"}
				in.NewStart = futureDateInTZ("09:30", buenosAiresLoc(t))
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
			name: "inactive_service_not_validated",
			mutate: func(in *ValidateBookingInput) {
				in.Service = &entity.Service{ID: "svc-1", Name: "Corte", DurationMinutes: 60, Active: false}
			},
			wantCode: nil,
		},
		{
			name: "inactive_professional_not_validated",
			mutate: func(in *ValidateBookingInput) {
				in.Professional = &entity.Professional{ID: "pro-1", Name: "Juan", Status: "inactive"}
			},
			wantCode: nil,
		},
		{
			name: "first_error_short_circuits",
			mutate: func(in *ValidateBookingInput) {
				in.NewStart = time.Now().Add(-2 * time.Hour)
			},
			overlapResult: []*entity.Booking{
				{ProfessionalID: "pro-1", StartDatetime: time.Now(), EndDatetime: time.Now().Add(30 * time.Minute)},
			},
			wantCode:      code(domain.ErrCodeSlotInPast),
			wantNoOverlap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := newValidateInput(t)
			bk, ok := in.Bookings.(*mockBookingsRepo)
			if !ok {
				t.Fatal("in.Bookings is not *mockBookingsRepo")
			}
			var overlapCalls int
			bk.OnFindOverlapping = func(context.Context, string, time.Time, time.Time) ([]*entity.Booking, error) {
				overlapCalls++
				return tt.overlapResult, nil
			}
			if tt.mutate != nil {
				tt.mutate(&in)
			}

			v := NewBookingValidator()
			err := v.Validate(context.Background(), in)

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
		})
	}
}
