package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// TestFindAvailableSlotsCachesTheAgendaPerWeekday pins the day-read budget: a
// 14-day range touches each weekday twice but must read each (professional, day)
// pair once.
func TestFindAvailableSlotsCachesTheAgendaPerWeekday(t *testing.T) {
	loc := slotsTestLocation(t)
	first := slotsTestDay(loc, 0)
	last := slotsTestDay(loc, 13)

	f := newSlotsFixture(t)
	result, err := f.execute(t, first, last, strPtr("p1"), intPtr(50))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if f.scheduleReads != 7 {
		t.Errorf("schedule reads = %d; want 7 (one per weekday, cached across the 14 days)", f.scheduleReads)
	}
	if result.TotalCandidates != 14*8 {
		t.Errorf("total_candidates = %d; want %d", result.TotalCandidates, 14*8)
	}
	if f.rangeReads != 1 {
		t.Errorf("bookings range reads = %d; want 1", f.rangeReads)
	}
}

// TestFindAvailableSlotsCorruptOpenExceptionFailsLoud pins the S11 posture for a
// corrupt business-hours exception: an OPEN exception whose hours
// parseSlotWindow rejects is an internal problem (ErrCodeInternal), never a day
// that silently produces no slots. A closed exception stays a closed day.
func TestFindAvailableSlotsCorruptOpenExceptionFailsLoud(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	tests := []struct {
		name      string
		openTime  string
		closeTime string
	}{
		{name: "inverted hours", openTime: "10:00", closeTime: "09:00"},
		{name: "malformed open", openTime: "25:00", closeTime: "18:00"},
		{name: "malformed close", openTime: "10:00", closeTime: "6pm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSlotsFixture(t)
			f.exceptions.ListFn = func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
				return []*entity.BusinessHoursException{{
					ID:            9,
					ExceptionDate: day.Format("2006-01-02"),
					IsClosed:      false,
					OpenTime:      ptr(tt.openTime),
					CloseTime:     ptr(tt.closeTime),
				}}, nil
			}

			result, err := f.execute(t, day, day, strPtr("p1"), nil)
			if result != nil {
				t.Errorf("result = %+v; want nil (a corrupt window returns no slot list)", result)
			}
			wantSemantic(t, err, domain.ErrCodeInternal)
		})
	}
}

// TestFindAvailableSlotsClosedExceptionIsNotCorruption guards the other side of
// the previous test: a declared closure is data, so it keeps producing a closed
// day (no slots, no error), even with no hours at all.
func TestFindAvailableSlotsClosedExceptionIsNotCorruption(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	f.exceptions.ListFn = func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
		return []*entity.BusinessHoursException{{
			ID:            10,
			ExceptionDate: day.Format("2006-01-02"),
			IsClosed:      true,
		}}, nil
	}

	result, err := f.execute(t, day, day, strPtr("p1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v (a closed exception is not corruption)", err)
	}
	if len(result.Slots) != 0 || result.TotalCandidates != 0 || result.Truncated {
		t.Errorf("result = %+v; want the empty result, not an error", result)
	}
}

// TestFindAvailableSlotsDiscardsAStaffScopedSchedule is the fail-closed identity
// check: SchedulesRepo.FindByProfessionalAndDay answers a staff caller with the
// CALLER's own row, so a row whose ProfessionalID does not match the requested
// professional must be discarded instead of published as that professional's
// availability (cross-professional leak + silently wrong slots).
func TestFindAvailableSlotsDiscardsAStaffScopedSchedule(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	f.schedules.FindByProfessionalAndDayFn = func(_ context.Context, professionalID string, day entity.ScheduleWeekday) (*entity.Schedule, error) {
		f.scheduleReads++
		// The adapter overrides the requested id with the caller's own for staff.
		return slotsShift("someone-else", day, 9, 17), nil
	}

	result, err := f.execute(t, day, day, strPtr("p1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Slots) != 0 || result.TotalCandidates != 0 {
		t.Errorf("result = %+v; want no slots (a foreign shift must never be attributed to p1)", result)
	}
	if result.Slots == nil {
		t.Error("Slots is nil; want an empty slice")
	}
}
