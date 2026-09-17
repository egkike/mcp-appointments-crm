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

// sundayInTZ returns a fixed future Sunday (2027-01-04 + 6 days = 2027-01-10)
// at hh:mm in loc, mirroring futureDateInTZ's Monday anchor.
func sundayInTZ(hhmm string, loc *time.Location) time.Time {
	return futureDateInTZ(hhmm, loc).AddDate(0, 0, 6)
}

// saturdayInTZ returns a fixed future Saturday (2027-01-04 + 5 days
// = 2027-01-09) at hh:mm in loc.
func saturdayInTZ(hhmm string, loc *time.Location) time.Time {
	return futureDateInTZ(hhmm, loc).AddDate(0, 0, 5)
}

// requireWeekday fails the test when the fixture date does not land on want,
// so a drifted anchor can never silently turn a day-key regression into a pass.
func requireWeekday(t *testing.T, want time.Weekday, start time.Time) {
	t.Helper()
	if got := start.Weekday(); got != want {
		t.Fatalf("fixture day = %v; want %v (anchor date drifted)", got, want)
	}
}

// TestValidateBookingTimeSlotDayKey pins the translation between the two
// day encodings crossing ValidateBookingTimeSlot:
//
//   - time.Weekday (and schedules.day_of_week): 0..6, Sunday=0;
//   - business_hours JSON keys: "1".."7", Monday=1, Sunday=7.
//
// Before the fix the validator fed int(Weekday()) straight into
// BusinessProfile.GetOpenClose, so Sunday (0) looked up a non-existent key "0"
// and the business was reported closed even when its profile was open.
func TestValidateBookingTimeSlotDayKey(t *testing.T) {
	loc := buenosAiresLoc(t)
	code := func(c domain.ErrCode) *domain.ErrCode { return &c }

	// "1".."6": Monday..Saturday open, Sunday absent (the weekend-closed profile).
	weekdaysOnlyHours := `{"1":{"open":"09:00","close":"18:00"},"2":{"open":"09:00","close":"18:00"},` +
		`"3":{"open":"09:00","close":"18:00"},"4":{"open":"09:00","close":"18:00"},` +
		`"5":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`
	// "7": Sunday only.
	sundayOnlyHours := `{"7":{"open":"09:00","close":"18:00"}}`

	tests := []struct {
		name          string
		start         time.Time
		weekday       time.Weekday
		businessHours string
		schedule      *entity.Schedule
		wantCode      *domain.ErrCode
		wantMessage   string
	}{
		{
			name:          "sunday_open_when_profile_has_key_7",
			start:         sundayInTZ("10:00", loc),
			weekday:       time.Sunday,
			businessHours: sundayOnlyHours,
			schedule:      &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 0, StartTime: "09:00", EndTime: "17:00"},
			wantCode:      nil,
		},
		{
			name:          "sunday_closed_when_profile_has_no_key_7",
			start:         sundayInTZ("10:00", loc),
			weekday:       time.Sunday,
			businessHours: weekdaysOnlyHours,
			schedule:      &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 0, StartTime: "09:00", EndTime: "17:00"},
			wantCode:      code(domain.ErrCodeBusinessClosed),
			wantMessage:   "domingo",
		},
		{
			name:          "monday_uses_profile_key_1",
			start:         futureDateInTZ("10:00", loc),
			weekday:       time.Monday,
			businessHours: weekdaysOnlyHours,
			schedule:      &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00"},
			wantCode:      nil,
		},
		{
			name:          "monday_closed_when_profile_has_sunday_hours_only",
			start:         futureDateInTZ("10:00", loc),
			weekday:       time.Monday,
			businessHours: sundayOnlyHours,
			schedule:      &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00"},
			wantCode:      code(domain.ErrCodeBusinessClosed),
			wantMessage:   "lunes",
		},
		{
			name:          "saturday_uses_profile_key_6",
			start:         saturdayInTZ("11:00", loc),
			weekday:       time.Saturday,
			businessHours: weekdaysOnlyHours,
			schedule:      &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 6, StartTime: "10:00", EndTime: "14:00"},
			wantCode:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireWeekday(t, tt.weekday, tt.start)

			slot, deps := defaultTimeSlot(t)
			slot.Start = tt.start
			slot.BusinessProfile.BusinessHours = tt.businessHours
			slot.Schedule = tt.schedule

			err := ValidateBookingTimeSlot(context.Background(), slot, deps)

			if tt.wantCode == nil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected code %q, got nil error", *tt.wantCode)
			}
			if err.Code != *tt.wantCode {
				t.Errorf("code = %q; want %q", err.Code, *tt.wantCode)
			}
			if tt.wantMessage != "" && !strings.Contains(err.Message, tt.wantMessage) {
				t.Errorf("message = %q; want contains %q", err.Message, tt.wantMessage)
			}
		})
	}
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

// TestValidateBookingTimeSlotMissingEntities pins the fail-closed contract for
// missing resolved entities: a nil BusinessProfile, Professional, or Service
// (or a non-positive Service duration) MUST return an INTERNAL SemanticError
// with the opaque message instead of panicking, and MUST short-circuit before
// the overlap query (defense in depth / zero trust).
func TestValidateBookingTimeSlotMissingEntities(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SlotInput)
	}{
		{
			name:   "nil_business_profile",
			mutate: func(s *SlotInput) { s.BusinessProfile = nil },
		},
		{
			name:   "nil_professional",
			mutate: func(s *SlotInput) { s.Professional = nil },
		},
		{
			name:   "nil_service",
			mutate: func(s *SlotInput) { s.Service = nil },
		},
		{
			name: "zero_service_duration",
			mutate: func(s *SlotInput) {
				s.Service = &entity.Service{ID: "svc-1", Name: "Corte", DurationMinutes: 0}
			},
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
				return nil, nil
			}
			tt.mutate(&slot)

			err := ValidateBookingTimeSlot(context.Background(), slot, deps)

			if err == nil {
				t.Fatal("expected an internal error, got nil")
			}
			if err.Code != domain.ErrCodeInternal {
				t.Errorf("code = %q; want %q", err.Code, domain.ErrCodeInternal)
			}
			if err.Message != "Error interno al validar el horario." {
				t.Errorf("message = %q; want the opaque internal message", err.Message)
			}
			if overlapCalls != 0 {
				t.Errorf("FindOverlapping called %d times; want 0 (fail closed before the overlap step)", overlapCalls)
			}
		})
	}
}

// TestValidateBookingTimeSlotMalformedHHMM pins the integer-minute comparison
// contract: a malformed HH:MM bound coming from stored hours or a schedule MUST
// fail closed with the internal error instead of being compared as text.
func TestValidateBookingTimeSlotMalformedHHMM(t *testing.T) {
	defaultHours := `{"1":{"open":"09:00","close":"18:00"}}`
	defaultSchedule := func() *entity.Schedule {
		return &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "17:00"}
	}

	tests := []struct {
		name          string
		businessHours string
		schedule      *entity.Schedule
	}{
		{
			name:          "malformed_business_open",
			businessHours: `{"1":{"open":"9am","close":"18:00"}}`,
			schedule:      defaultSchedule(),
		},
		{
			name:          "malformed_business_close",
			businessHours: `{"1":{"open":"09:00","close":"25:00"}}`,
			schedule:      defaultSchedule(),
		},
		{
			name:          "malformed_professional_start",
			businessHours: defaultHours,
			schedule:      &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "9am", EndTime: "17:00"},
		},
		{
			name:          "malformed_professional_end",
			businessHours: defaultHours,
			schedule:      &entity.Schedule{ProfessionalID: "pro-1", DayOfWeek: 1, StartTime: "09:00", EndTime: "5pm"},
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
				return nil, nil
			}
			slot.BusinessProfile.BusinessHours = tt.businessHours
			slot.Schedule = tt.schedule

			err := ValidateBookingTimeSlot(context.Background(), slot, deps)

			if err == nil {
				t.Fatal("expected an internal error, got nil")
			}
			if err.Code != domain.ErrCodeInternal {
				t.Errorf("code = %q; want %q", err.Code, domain.ErrCodeInternal)
			}
			if err.Message != "Error interno al validar el horario." {
				t.Errorf("message = %q; want the opaque internal message", err.Message)
			}
			if overlapCalls != 0 {
				t.Errorf("FindOverlapping called %d times; want 0 (fail closed before the overlap step)", overlapCalls)
			}
		})
	}
}
