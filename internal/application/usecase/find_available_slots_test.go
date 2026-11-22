package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestFindAvailableSlotsNoFreeSlotIsNotAnError(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	t.Run("crossing no day with agenda", func(t *testing.T) {
		f := newSlotsFixture(t)
		f.schedules.FindByProfessionalAndDayFn = func(context.Context, string, entity.ScheduleWeekday) (*entity.Schedule, error) {
			return nil, fmt.Errorf("sin agenda: %w", domain.ErrNotFound)
		}
		result, err := f.execute(t, day, day, strPtr("p1"), nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.Slots == nil {
			t.Error("Slots is nil; want a non-nil empty slice ([] on the wire)")
		}
		if len(result.Slots) != 0 || result.TotalCandidates != 0 || result.Truncated {
			t.Errorf("result = %+v; want {slots: [], total_candidates: 0, truncated: false}", result)
		}
		if f.rangeReads != 1 {
			t.Errorf("bookings range reads = %d; want 1 (the range is read even when empty)", f.rangeReads)
		}
	})

	t.Run("range entirely outside the business hours", func(t *testing.T) {
		f := newSlotsFixture(t)
		// Business hours 02:00-03:00 with a 09:00-17:00 shift leaves no overlap,
		// so the combined window is empty and no candidate is generated.
		f.profile.GetFn = func(context.Context) (*entity.BusinessProfile, error) {
			return &entity.BusinessProfile{
				ID:       "singleton",
				Name:     "Negocio",
				Timezone: loc.String(),
				BusinessHours: `{"1":{"open":"02:00","close":"03:00"},"2":{"open":"02:00","close":"03:00"},` +
					`"3":{"open":"02:00","close":"03:00"},"4":{"open":"02:00","close":"03:00"},` +
					`"5":{"open":"02:00","close":"03:00"},"6":{"open":"02:00","close":"03:00"},` +
					`"7":{"open":"02:00","close":"03:00"}}`,
			}, nil
		}
		result, err := f.execute(t, day, day, strPtr("p1"), nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(result.Slots) != 0 || result.TotalCandidates != 0 || result.Truncated {
			t.Errorf("result = %+v; want the empty result, not an error", result)
		}
	})
}

func TestFindAvailableSlotsClosedExceptionRemovesTheDate(t *testing.T) {
	loc := slotsTestLocation(t)
	closedDay := slotsTestDay(loc, 0)
	openDay := slotsTestDay(loc, 1)

	f := newSlotsFixture(t)
	f.exceptions.ListFn = func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
		return []*entity.BusinessHoursException{{
			ID:            7,
			ExceptionDate: closedDay.Format("2006-01-02"),
			IsClosed:      true,
			Reason:        ptr("Feriado"),
		}}, nil
	}

	result, err := f.execute(t, closedDay, openDay, strPtr("p1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The closed date contributes nothing; the open date keeps its full grid.
	wantSlots(t, result.Slots, slotsAt(openDay, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16))
	if result.TotalCandidates != 8 {
		t.Errorf("total_candidates = %d; want 8 (only the open day)", result.TotalCandidates)
	}
	if result.Truncated {
		t.Error("truncated = true; want false")
	}
}

func TestFindAvailableSlotsOpenExceptionReplacesWeeklyHours(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	f.exceptions.ListFn = func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
		return []*entity.BusinessHoursException{{
			ID:            8,
			ExceptionDate: day.Format("2006-01-02"),
			IsClosed:      false,
			OpenTime:      ptr("10:00"),
			CloseTime:     ptr("12:00"),
		}}, nil
	}

	result, err := f.execute(t, day, day, strPtr("p1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The exception window (10:00-12:00) replaces the weekly 09:00-18:00 hours.
	wantSlots(t, result.Slots, slotsAt(day, "p1", "Ana", 10, 11))
	if result.TotalCandidates != 2 {
		t.Errorf("total_candidates = %d; want 2", result.TotalCandidates)
	}
}

func TestFindAvailableSlotsOverlapFiltersSilently(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	f.bookings.ListBookingsForRangeFn = func(_ context.Context, _, _ time.Time) ([]*entity.Booking, error) {
		f.rangeReads++
		return []*entity.Booking{
			{
				ID:             "b-busy",
				ClientID:       "c-secret",
				ProfessionalID: "p1",
				ServiceID:      "s1",
				StartDatetime:  time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, loc).UTC(),
				EndDatetime:    time.Date(day.Year(), day.Month(), day.Day(), 11, 0, 0, 0, loc).UTC(),
				Status:         entity.BookingStatusConfirmed,
			},
			{
				// Cancelled bookings never block a slot: same exclusion as
				// FindOverlapping's status != 'cancelled'.
				ID:             "b-cancelled",
				ClientID:       "c-secret",
				ProfessionalID: "p1",
				ServiceID:      "s1",
				StartDatetime:  time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc).UTC(),
				EndDatetime:    time.Date(day.Year(), day.Month(), day.Day(), 13, 0, 0, 0, loc).UTC(),
				Status:         entity.BookingStatusCancelled,
			},
		}, nil
	}

	result, err := f.execute(t, day, day, strPtr("p1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v (an overlap must be filtered, never reported)", err)
	}

	want := slotsAt(day, "p1", "Ana", 9, 11, 12, 13, 14, 15, 16)
	wantSlots(t, result.Slots, want)
	if result.TotalCandidates != 7 {
		t.Errorf("total_candidates = %d; want 7 (10:00 filtered by the overlap)", result.TotalCandidates)
	}
	if result.Truncated {
		t.Error("truncated = true; want false")
	}
	if f.rangeReads != 1 {
		t.Errorf("bookings range reads = %d; want 1", f.rangeReads)
	}
	if f.overlapCalls != 0 {
		t.Errorf("repository overlap calls = %d; want 0", f.overlapCalls)
	}
}

func TestFindAvailableSlotsOverlapOnlyAffectsItsProfessional(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	f.bookings.ListBookingsForRangeFn = func(_ context.Context, _, _ time.Time) ([]*entity.Booking, error) {
		f.rangeReads++
		return []*entity.Booking{{
			ID:             "b-ana-9",
			ClientID:       "c-secret",
			ProfessionalID: "p1",
			ServiceID:      "s1",
			StartDatetime:  time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, loc).UTC(),
			EndDatetime:    time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, loc).UTC(),
			Status:         entity.BookingStatusPending,
		}}, nil
	}

	result, err := f.execute(t, day, day, strPtr("p1"), intPtr(50))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// p1 lost 09:00 only; the rest of the grid is intact.
	wantSlots(t, result.Slots, slotsAt(day, "p1", "Ana", 10, 11, 12, 13, 14, 15, 16))
	if result.TotalCandidates != 7 {
		t.Errorf("total_candidates = %d; want 7", result.TotalCandidates)
	}
}

func TestFindAvailableSlotsLimitAndTruncated(t *testing.T) {
	loc := slotsTestLocation(t)
	first := slotsTestDay(loc, 0)
	second := slotsTestDay(loc, 1)

	t.Run("default limit truncates a longer range", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, first, second, strPtr("p1"), nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		// 2 days x 8 slots = 16 candidates, default limit 10.
		if result.TotalCandidates != 16 {
			t.Errorf("total_candidates = %d; want 16 (counted BEFORE the limit)", result.TotalCandidates)
		}
		if !result.Truncated {
			t.Error("truncated = false; want true")
		}
		want := append(
			slotsAt(first, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16),
			slotsAt(second, "p1", "Ana", 9, 10)...,
		)
		wantSlots(t, result.Slots, want)
	})

	t.Run("explicit limit 3", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, first, second, strPtr("p1"), intPtr(3))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.TotalCandidates != 16 || !result.Truncated {
			t.Errorf("total_candidates=%d truncated=%v; want 16/true", result.TotalCandidates, result.Truncated)
		}
		wantSlots(t, result.Slots, slotsAt(first, "p1", "Ana", 9, 10, 11))
	})

	t.Run("limit equal to the candidate count is not truncated", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, first, first, strPtr("p1"), intPtr(8))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.TotalCandidates != 8 || result.Truncated {
			t.Errorf("total_candidates=%d truncated=%v; want 8/false", result.TotalCandidates, result.Truncated)
		}
		wantSlots(t, result.Slots, slotsAt(first, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16))
	})

	t.Run("maximum limit", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, first, second, strPtr("p1"), intPtr(50))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.TotalCandidates != 16 || result.Truncated || len(result.Slots) != 16 {
			t.Errorf("total_candidates=%d truncated=%v slots=%d; want 16/false/16",
				result.TotalCandidates, result.Truncated, len(result.Slots))
		}
	})
}

func TestFindAvailableSlotsRangeAndLimitBounds(t *testing.T) {
	loc := slotsTestLocation(t)
	first := slotsTestDay(loc, 0)

	tests := []struct {
		name  string
		from  time.Time
		to    time.Time
		limit *int
	}{
		{name: "range of 15 days", from: first, to: slotsTestDay(loc, 14), limit: nil},
		{name: "inverted range", from: slotsTestDay(loc, 1), to: first, limit: nil},
		{name: "limit zero", from: first, to: first, limit: intPtr(0)},
		{name: "limit negative", from: first, to: first, limit: intPtr(-1)},
		{name: "limit above the cap", from: first, to: first, limit: intPtr(51)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSlotsFixture(t)
			result, err := f.execute(t, tt.from, tt.to, strPtr("p1"), tt.limit)
			if result != nil {
				t.Errorf("result = %+v; want nil (a rejected input returns no slot list)", result)
			}
			wantSemantic(t, err, domain.ErrCodeInvalidInput)
			wantNoReads(t, f)
		})
	}

	t.Run("malformed date bound", func(t *testing.T) {
		f := newSlotsFixture(t)
		_, err := f.uc.Execute(context.Background(), dto.FindAvailableSlotsInput{
			Caller:    ownerCaller(),
			ServiceID: "s1",
			From:      "2031-3-3",
			To:        first.Format("2006-01-02"),
		})
		wantSemantic(t, err, domain.ErrCodeInvalidInput)
		wantNoReads(t, f)
	})

	t.Run("14 inclusive days is accepted", func(t *testing.T) {
		f := newSlotsFixture(t)
		last := slotsTestDay(loc, 13) // 14 days inclusive
		result, err := f.execute(t, first, last, strPtr("p1"), intPtr(50))
		if err != nil {
			t.Fatalf("Execute: %v (14 inclusive days must be accepted)", err)
		}
		if result.TotalCandidates != 14*8 {
			t.Errorf("total_candidates = %d; want %d", result.TotalCandidates, 14*8)
		}
		// 112 candidates exceed even the maximum limit, so the cap is what binds.
		if !result.Truncated {
			t.Error("truncated = false; want true (112 candidates over the maximum limit 50)")
		}
		if len(result.Slots) != 50 {
			t.Errorf("slots = %d; want 50 (the maximum limit)", len(result.Slots))
		}
	})
}

func TestFindAvailableSlotsInactiveProfessional(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	t.Run("omitted professional_id silently omits the inactive one", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, day, day, nil, nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		wantSlots(t, result.Slots, slotsAt(day, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16))
		if result.TotalCandidates != 8 {
			t.Errorf("total_candidates = %d; want 8 (p3 is inactive and excluded silently)", result.TotalCandidates)
		}
	})

	t.Run("named inactive professional is rejected", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, day, day, strPtr("p3"), nil)
		if result != nil {
			t.Errorf("result = %+v; want nil", result)
		}
		wantSemantic(t, err, domain.ErrCodeProfessionalNotActive)
		wantNoReads(t, f)
	})

	t.Run("named unknown professional is rejected", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, day, day, strPtr("p-unknown"), nil)
		if result != nil {
			t.Errorf("result = %+v; want nil", result)
		}
		wantSemantic(t, err, domain.ErrCodeNotFound)
		wantNoReads(t, f)
	})

	t.Run("empty professional_id behaves as omitted", func(t *testing.T) {
		f := newSlotsFixture(t)
		result, err := f.execute(t, day, day, strPtr("   "), nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.TotalCandidates != 8 {
			t.Errorf("total_candidates = %d; want 8 (whitespace is 'absent', not a professional id)", result.TotalCandidates)
		}
	})

	t.Run("professional without the specialty is not a candidate", func(t *testing.T) {
		f := newSlotsFixture(t)
		other := &entity.Professional{ID: "p9", Name: "Sin especialidad", Status: "active"}
		f.professionals.FindAllActiveFn = func(context.Context) ([]*entity.Professional, error) {
			return []*entity.Professional{other}, nil
		}
		result, err := f.execute(t, day, day, nil, nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(result.Slots) != 0 || result.TotalCandidates != 0 {
			t.Errorf("result = %+v; want no slots (fail-closed on an undeclared specialty)", result)
		}
	})
}

func TestFindAvailableSlotsServiceResolution(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	t.Run("unknown service", func(t *testing.T) {
		f := newSlotsFixture(t)
		_, err := f.uc.Execute(context.Background(), dto.FindAvailableSlotsInput{
			Caller:    ownerCaller(),
			ServiceID: "s-missing",
			From:      day.Format("2006-01-02"),
			To:        day.Format("2006-01-02"),
		})
		wantSemantic(t, err, domain.ErrCodeNotFound)
		wantNoReads(t, f)
	})

	t.Run("inactive service", func(t *testing.T) {
		f := newSlotsFixture(t)
		f.services.FindByIDFn = func(context.Context, string) (*entity.Service, error) {
			return &entity.Service{ID: "s1", Name: "Corte", DurationMinutes: 60, Active: false}, nil
		}
		_, err := f.execute(t, day, day, strPtr("p1"), nil)
		wantSemantic(t, err, domain.ErrCodeServiceNotActive)
		wantNoReads(t, f)
	})

	t.Run("non-positive duration fails closed", func(t *testing.T) {
		f := newSlotsFixture(t)
		f.services.FindByIDFn = func(context.Context, string) (*entity.Service, error) {
			return &entity.Service{ID: "s1", Name: "Corte", DurationMinutes: 0, Active: true}, nil
		}
		_, err := f.execute(t, day, day, strPtr("p1"), nil)
		wantSemantic(t, err, domain.ErrCodeInternal)
		wantNoReads(t, f)
	})
}

func TestFindAvailableSlotsRequiresAuthentication(t *testing.T) {
	f := newSlotsFixture(t)
	_, err := f.uc.Execute(context.Background(), dto.FindAvailableSlotsInput{ServiceID: "s1"})
	wantSemantic(t, err, domain.ErrCodeUnauthenticated)
	wantNoReads(t, f)
}

// TestFindAvailableSlotsReadsTheRangeOnce is the S9 proof: several days and
// several professionals still produce ONE range read and ZERO repository overlap
// reads, because the chain's overlap step is answered from the in-memory reader.
func TestFindAvailableSlotsReadsTheRangeOnce(t *testing.T) {
	loc := slotsTestLocation(t)
	first := slotsTestDay(loc, 0)
	second := slotsTestDay(loc, 1)

	f := newSlotsFixture(t)
	f.professionals.FindAllActiveFn = func(context.Context) ([]*entity.Professional, error) {
		return []*entity.Professional{
			slotsProfessional("p1", "Ana", "active"),
			slotsProfessional("p2", "Beto", "active"),
		}, nil
	}

	result, err := f.execute(t, first, second, nil, intPtr(50))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if f.rangeReads != 1 {
		t.Errorf("bookings range reads = %d; want exactly 1 (S9: one read for the whole range)", f.rangeReads)
	}
	if f.overlapCalls != 0 {
		t.Errorf("repository overlap calls = %d; want 0 (no per-slot, no per-professional query)", f.overlapCalls)
	}
	// One day read per (professional, weekday) pair: 2 professionals x 2 distinct
	// weekdays, cached so the range length never multiplies it.
	if f.scheduleReads != 4 {
		t.Errorf("schedule reads = %d; want 4 (2 professionals x 2 weekdays, cached)", f.scheduleReads)
	}

	// The range window handed to the repository covers every inclusive day.
	wantFrom := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, loc).UTC()
	wantTo := time.Date(second.Year(), second.Month(), second.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1).UTC()
	if !f.rangeReadFrom.Equal(wantFrom) || !f.rangeReadTo.Equal(wantTo) {
		t.Errorf("range read = [%s, %s); want [%s, %s)", f.rangeReadFrom, f.rangeReadTo, wantFrom, wantTo)
	}

	// 2 days x 8 slots x 2 professionals = 32 candidates, ordered by start then name.
	if result.TotalCandidates != 32 {
		t.Fatalf("total_candidates = %d; want 32", result.TotalCandidates)
	}
	if len(result.Slots) != 32 || result.Truncated {
		t.Errorf("slots = %d truncated = %v; want 32/false", len(result.Slots), result.Truncated)
	}
	want := append(
		slotsAt(first, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16),
		slotsAt(second, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16)...,
	)
	// p2 shares the same grid; the sort breaks each start tie by professional name.
	wantAll := make([]dto.AvailableSlot, 0, 32)
	for index := 0; index < len(want); index += 1 {
		wantAll = append(wantAll, want[index])
		wantAll = append(wantAll, dto.AvailableSlot{
			StartDatetime:    want[index].StartDatetime,
			ProfessionalID:   "p2",
			ProfessionalName: "Beto",
		})
	}
	// "Ana" < "Beto", so each start keeps Ana before Beto: the interleaving above
	// is exactly the required order.
	wantSlots(t, result.Slots, wantAll)
}

func TestFindAvailableSlotsPastSlotsAreFiltered(t *testing.T) {
	loc := slotsTestLocation(t)
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	// Skip weekends-only fixtures: the profile opens every day and the agenda
	// covers every weekday, so "today" always yields a grid.
	f := newSlotsFixture(t)
	result, err := f.execute(t, today, today, strPtr("p1"), intPtr(50))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, slot := range result.Slots {
		if slot.StartDatetime.Before(time.Now()) {
			t.Errorf("slot %s is in the past; the chain's step 1 must have filtered it", slot.StartDatetime)
		}
	}
	if result.TotalCandidates != len(result.Slots) {
		t.Errorf("total_candidates = %d; want %d (no truncation with limit 50)", result.TotalCandidates, len(result.Slots))
	}
}

// TestFindAvailableSlotsFullyPastRangeReadsNothing is the S11 efficiency
// invariant: a window entirely before today answers with the OLD empty shape and
// performs NO range read at all — neither exceptions nor bookings. It must never
// become a semantic error: the observable contract of a past range is unchanged.
func TestFindAvailableSlotsFullyPastRangeReadsNothing(t *testing.T) {
	loc := slotsTestLocation(t)

	f := newSlotsFixture(t)
	today := slotsPinToday(t, f, loc)
	exceptionReads := 0
	var excFrom, excTo time.Time
	f.exceptions.ListFn = func(_ context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
		exceptionReads++
		excFrom, excTo = from, to
		return nil, nil
	}

	// Five days ending yesterday: every day of the window is in the past.
	result, err := f.execute(t, today.AddDate(0, 0, -5), today.AddDate(0, 0, -1), strPtr("p1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v (a past range is the empty result, never an error)", err)
	}
	if result == nil {
		t.Fatal("result is nil; want the empty result shape")
	}
	if result.Slots == nil {
		t.Error("Slots is nil; want a non-nil empty slice ([] on the wire)")
	}
	if len(result.Slots) != 0 || result.TotalCandidates != 0 || result.Truncated {
		t.Errorf("result = %+v; want {slots: [], total_candidates: 0, truncated: false}", result)
	}
	if exceptionReads != 0 {
		t.Errorf("exception range reads = %d; want 0 (a fully past window reads no exceptions)", exceptionReads)
	}
	if !excFrom.IsZero() || !excTo.IsZero() {
		t.Errorf("exception window = [%s, %s); want no window at all", excFrom, excTo)
	}
	if f.rangeReads != 0 {
		t.Errorf("bookings range reads = %d; want 0 (a fully past window reads no bookings)", f.rangeReads)
	}
	if f.scheduleReads != 0 {
		t.Errorf("schedule reads = %d; want 0 (no day is walked)", f.scheduleReads)
	}
	if f.overlapCalls != 0 {
		t.Errorf("repository overlap calls = %d; want 0", f.overlapCalls)
	}
}

// TestFindAvailableSlotsClampsTheReadWindowToToday pins the clamp itself: a range
// that starts in the past and ends ahead is narrowed to today, the day count is
// recomputed from the CLAMPED window, and both range reads receive that same
// window (the loop and the exceptions read must agree). The returned slots are
// the ones the unclamped scan would have produced: past days never had any.
func TestFindAvailableSlotsClampsTheReadWindowToToday(t *testing.T) {
	loc := slotsTestLocation(t)

	f := newSlotsFixture(t)
	today := slotsPinToday(t, f, loc)
	tomorrow := today.AddDate(0, 0, 1)

	exceptionReads := 0
	var excFrom, excTo time.Time
	f.exceptions.ListFn = func(_ context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
		exceptionReads++
		excFrom, excTo = from, to
		return nil, nil
	}

	result, err := f.execute(t, today.AddDate(0, 0, -3), tomorrow, strPtr("p1"), intPtr(50))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wantFrom := today.UTC()
	wantTo := tomorrow.AddDate(0, 0, 1).UTC()
	if f.rangeReads != 1 {
		t.Errorf("bookings range reads = %d; want 1", f.rangeReads)
	}
	if !f.rangeReadFrom.Equal(wantFrom) || !f.rangeReadTo.Equal(wantTo) {
		t.Errorf("bookings range read = [%s, %s); want [%s, %s) (clamped to today)",
			f.rangeReadFrom, f.rangeReadTo, wantFrom, wantTo)
	}
	if exceptionReads != 1 {
		t.Errorf("exception range reads = %d; want 1", exceptionReads)
	}
	if !excFrom.Equal(today) || !excTo.Equal(tomorrow) {
		t.Errorf("exception range read = [%s, %s]; want [%s, %s] (same clamped window as the loop)",
			excFrom, excTo, today, tomorrow)
	}

	// Tomorrow always contributes its full grid; today contributes only slots that
	// are still ahead, so the total is asserted against its invariant, not a clock.
	now := time.Now()
	for _, slot := range result.Slots {
		if slot.StartDatetime.Before(now) {
			t.Errorf("slot %s is in the past; past days and past hours must not be returned", slot.StartDatetime)
		}
		if slot.StartDatetime.Before(today) {
			t.Errorf("slot %s predates today; the clamp must have dropped it", slot.StartDatetime)
		}
	}
	lastDay := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, loc)
	var tomorrowSlots []dto.AvailableSlot
	for _, slot := range result.Slots {
		if !slot.StartDatetime.Before(lastDay) {
			tomorrowSlots = append(tomorrowSlots, slot)
		}
	}
	wantSlots(t, tomorrowSlots, slotsAt(tomorrow, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16))
	if result.TotalCandidates != len(result.Slots) {
		t.Errorf("total_candidates = %d; want %d (limit 50 does not truncate)", result.TotalCandidates, len(result.Slots))
	}
	if result.Truncated {
		t.Error("truncated = true; want false")
	}
}
