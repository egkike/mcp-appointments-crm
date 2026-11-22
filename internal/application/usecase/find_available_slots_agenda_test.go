package usecase

import (
	"context"
	"fmt"
	"strings"
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

// ── Direct tests of the four agenda helpers (S2, issue #247) ──────────────────
//
// These helpers are only reachable through Execute today, and Execute answers
// "how many slots came out", not "which window was resolved" nor "why the day
// was dropped". The tests below pin exactly the boundaries Execute cannot
// distinguish: an exception window vs. an is_closed flag, malformed and
// inverted HH:MM on EACH branch (the exception branch fails loud, the weekly
// branch fails closed), empty and duplicate exception dates, and the seven
// business_hours day keys.

// TestIndexExceptionsByDate pins the date index the per-day lookup reads: it
// skips rows that cannot be keyed and it is the LAST row that wins a duplicate
// date (the range read returns rows in storage order, so this is the documented
// tie-break, not an accident of map iteration).
func TestIndexExceptionsByDate(t *testing.T) {
	t.Run("empty input yields an empty index", func(t *testing.T) {
		index := indexExceptionsByDate(nil)
		if index == nil {
			t.Fatal("index = nil; want an empty map (callers look up by key, they do not nil-check)")
		}
		if len(index) != 0 {
			t.Errorf("len(index) = %d; want 0", len(index))
		}
	})

	t.Run("rows without a usable date are skipped", func(t *testing.T) {
		closed := &entity.BusinessHoursException{ID: 1, ExceptionDate: "2031-03-03", IsClosed: true}
		index := indexExceptionsByDate([]*entity.BusinessHoursException{
			nil,
			{ID: 2, ExceptionDate: ""},
			closed,
		})
		if len(index) != 1 {
			t.Fatalf("len(index) = %d; want 1 (a nil row and an empty date are not keys)", len(index))
		}
		if index["2031-03-03"] != closed {
			t.Errorf("index[2031-03-03] = %+v; want the dated row", index["2031-03-03"])
		}
	})

	t.Run("a duplicate date keeps the last row", func(t *testing.T) {
		first := &entity.BusinessHoursException{ID: 10, ExceptionDate: "2031-03-04", IsClosed: true}
		last := &entity.BusinessHoursException{ID: 11, ExceptionDate: "2031-03-04", OpenTime: ptr("10:00"), CloseTime: ptr("12:00")}
		index := indexExceptionsByDate([]*entity.BusinessHoursException{first, last})
		if len(index) != 1 {
			t.Fatalf("len(index) = %d; want 1 (one key per calendar date)", len(index))
		}
		if index["2031-03-04"] != last {
			t.Errorf("index[2031-03-04] = %+v; want the last row (%+v)", index["2031-03-04"], last)
		}
	})
}

// TestClockMinutes pins the HH:MM conversion and the format gate it delegates to
// entity.ValidHHMM. The malformed cases are the ones a lenient parser would
// accept and silently mis-read (unpadded hour, out-of-range hour or minute, a
// stray sign, space or missing colon).
func TestClockMinutes(t *testing.T) {
	valid := []struct {
		value string
		want  int
	}{
		{"00:00", 0},
		{"09:30", 570},
		{"12:00", 720},
		{"18:00", 1080},
		{"23:59", 1439},
	}
	for _, tt := range valid {
		t.Run(tt.value, func(t *testing.T) {
			got, ok := clockMinutes(tt.value)
			if !ok {
				t.Fatalf("clockMinutes(%q) ok = false; want true", tt.value)
			}
			if got != tt.want {
				t.Errorf("clockMinutes(%q) = %d; want %d", tt.value, got, tt.want)
			}
		})
	}

	invalid := []string{"", "9:00", "0900", "24:00", "23:60", "09:0", "09:00 ", " 09:00", "-1:00", "18:00pm", "aa:bb"}
	for _, value := range invalid {
		t.Run("rejects "+value, func(t *testing.T) {
			got, ok := clockMinutes(value)
			if ok {
				t.Errorf("clockMinutes(%q) ok = true (value %d); want false", value, got)
			}
			if got != 0 {
				t.Errorf("clockMinutes(%q) = %d; want 0 on rejection", value, got)
			}
		})
	}
}

// TestParseSlotWindow pins the pair-level rule: both bounds must parse AND the
// close must be strictly after the open. Equal bounds are rejected on purpose —
// a zero-length window is not a day of hours — and the malformed side is
// reported on either bound, not only on the open one.
func TestParseSlotWindow(t *testing.T) {
	valid := []struct {
		open, close         string
		wantOpen, wantClose int
	}{
		{"09:00", "18:00", 540, 1080},
		{"00:00", "23:59", 0, 1439},
		{"23:00", "23:59", 1380, 1439},
	}
	for _, tt := range valid {
		t.Run(tt.open+"-"+tt.close, func(t *testing.T) {
			openMin, closeMin, ok := parseSlotWindow(tt.open, tt.close)
			if !ok {
				t.Fatalf("parseSlotWindow(%q, %q) ok = false; want true", tt.open, tt.close)
			}
			if openMin != tt.wantOpen || closeMin != tt.wantClose {
				t.Errorf("parseSlotWindow(%q, %q) = %d, %d; want %d, %d",
					tt.open, tt.close, openMin, closeMin, tt.wantOpen, tt.wantClose)
			}
		})
	}

	invalid := []struct {
		name, open, close string
	}{
		{"malformed open", "9:00", "18:00"},
		{"malformed close", "09:00", "6pm"},
		{"inverted bounds", "18:00", "09:00"},
		{"equal bounds", "09:00", "09:00"},
		{"empty open", "", "18:00"},
		{"empty close", "09:00", ""},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			openMin, closeMin, ok := parseSlotWindow(tt.open, tt.close)
			if ok {
				t.Errorf("parseSlotWindow(%q, %q) ok = true (%d, %d); want false", tt.open, tt.close, openMin, closeMin)
			}
			if openMin != 0 || closeMin != 0 {
				t.Errorf("parseSlotWindow(%q, %q) = %d, %d; want 0, 0 on rejection", tt.open, tt.close, openMin, closeMin)
			}
		})
	}
}

// TestBusinessWindow pins which window the iteration bounds resolve to, per
// branch, and — crucially — the ASYMMETRIC failure posture Execute hides:
// corrupt hours on an OPEN exception are a loud internal error, while corrupt
// hours in the weekly business_hours profile close the day silently.
func TestBusinessWindow(t *testing.T) {
	loc := slotsTestLocation(t)
	// 2031-03-03 is a Monday, so business_hours key "1" is the key in play.
	day := slotsTestDay(loc, 0)

	weekly := func(hours string) *entity.BusinessProfile {
		return &entity.BusinessProfile{ID: "singleton", BusinessHours: hours}
	}
	openException := func(open, close string) *entity.BusinessHoursException {
		return &entity.BusinessHoursException{
			ID:            1,
			ExceptionDate: day.Format("2006-01-02"),
			OpenTime:      ptr(open),
			CloseTime:     ptr(close),
		}
	}

	tests := []struct {
		name      string
		profile   *entity.BusinessProfile
		exception *entity.BusinessHoursException
		wantOpen  int
		wantClose int
		wantDay   bool
		wantCode  domain.ErrCode // empty: no error expected
	}{
		{
			name:    "open exception replaces the weekly hours",
			profile: weekly(slotsAllWeekHours), exception: openException("10:00", "14:00"),
			wantOpen: 600, wantClose: 840, wantDay: true,
		},
		{
			name:    "closed exception is a closed day",
			profile: weekly(slotsAllWeekHours),
			exception: &entity.BusinessHoursException{
				ExceptionDate: day.Format("2006-01-02"), IsClosed: true,
			},
			wantDay: false,
		},
		{
			name:    "closed exception ignores a window it also carries",
			profile: weekly(slotsAllWeekHours),
			exception: &entity.BusinessHoursException{
				ExceptionDate: day.Format("2006-01-02"), IsClosed: true,
				OpenTime: ptr("10:00"), CloseTime: ptr("14:00"),
			},
			wantDay: false,
		},
		{
			name:    "open exception without hours falls through to the weekly hours",
			profile: weekly(slotsAllWeekHours),
			exception: &entity.BusinessHoursException{
				ExceptionDate: day.Format("2006-01-02"), IsClosed: false,
			},
			wantOpen: 540, wantClose: 1080, wantDay: true,
		},
		{
			name:    "malformed hours on the exception branch fail loud",
			profile: weekly(slotsAllWeekHours), exception: openException("09:00", "6pm"),
			wantCode: domain.ErrCodeInternal,
		},
		{
			name:    "malformed open on the exception branch fails loud",
			profile: weekly(slotsAllWeekHours), exception: openException("25:00", "18:00"),
			wantCode: domain.ErrCodeInternal,
		},
		{
			name:    "inverted hours on the exception branch fail loud",
			profile: weekly(slotsAllWeekHours), exception: openException("14:00", "10:00"),
			wantCode: domain.ErrCodeInternal,
		},
		{
			name:    "equal bounds on the exception branch fail loud",
			profile: weekly(slotsAllWeekHours), exception: openException("10:00", "10:00"),
			wantCode: domain.ErrCodeInternal,
		},
		{
			name:     "no exception resolves the weekly hours",
			profile:  weekly(slotsAllWeekHours),
			wantOpen: 540, wantClose: 1080, wantDay: true,
		},
		{
			name:    "a day absent from business_hours is closed",
			profile: weekly(`{"2":{"open":"09:00","close":"18:00"}}`),
			wantDay: false,
		},
		{
			name:    "empty business_hours is closed",
			profile: weekly(""),
			wantDay: false,
		},
		{
			name:     "unreadable business_hours fails loud",
			profile:  weekly("{"),
			wantCode: domain.ErrCodeInternal,
		},
		{
			name:     "null business_hours fails loud",
			profile:  weekly("null"),
			wantCode: domain.ErrCodeInternal,
		},
		{
			name:    "malformed weekly hours fail closed, not loud",
			profile: weekly(`{"1":{"open":"9:00","close":"18:00"}}`),
			wantDay: false,
		},
		{
			name:    "inverted weekly hours fail closed, not loud",
			profile: weekly(`{"1":{"open":"18:00","close":"09:00"}}`),
			wantDay: false,
		},
		{
			name:    "empty weekly hours are a closed day",
			profile: weekly(`{"1":{"open":"","close":""}}`),
			wantDay: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openMin, closeMin, open, err := businessWindow(tt.profile, tt.exception, day)
			if tt.wantCode != "" {
				wantSemantic(t, err, tt.wantCode)
				if open || openMin != 0 || closeMin != 0 {
					t.Errorf("window = %d..%d open=%v; want no window alongside the error", openMin, closeMin, open)
				}
				return
			}
			if err != nil {
				t.Fatalf("businessWindow: %v; want no error", err)
			}
			if open != tt.wantDay {
				t.Errorf("open = %v; want %v", open, tt.wantDay)
			}
			if openMin != tt.wantOpen || closeMin != tt.wantClose {
				t.Errorf("window = %d..%d; want %d..%d", openMin, closeMin, tt.wantOpen, tt.wantClose)
			}
		})
	}
}

// TestBusinessWindowResolvesEveryDayKey pins the seven business_hours keys
// (1=Monday .. 7=Sunday, entity.ProfileDayKey) with a distinct window per key,
// so a wrong translation — the classic off-by-one where Sunday reads as key 0 —
// cannot pass by landing on another day's identical hours.
func TestBusinessWindowResolvesEveryDayKey(t *testing.T) {
	loc := slotsTestLocation(t)
	monday := slotsTestDay(loc, 0)

	hours := make([]string, 0, 7)
	for key := 1; key <= 7; key++ {
		hours = append(hours, fmt.Sprintf(`"%d":{"open":"%02d:00","close":"%02d:30"}`, key, key, key+1))
	}
	profile := &entity.BusinessProfile{ID: "singleton", BusinessHours: "{" + strings.Join(hours, ",") + "}"}

	for offset := 0; offset < 7; offset++ {
		day := monday.AddDate(0, 0, offset)
		key := offset + 1 // Monday = key 1 ... Sunday = key 7
		t.Run(day.Weekday().String(), func(t *testing.T) {
			openMin, closeMin, open, err := businessWindow(profile, nil, day)
			if err != nil {
				t.Fatalf("businessWindow: %v; want no error", err)
			}
			if !open {
				t.Fatalf("open = false; want the window of business_hours key %d", key)
			}
			wantOpen, wantClose := key*60, (key+1)*60+30
			if openMin != wantOpen || closeMin != wantClose {
				t.Errorf("window = %d..%d; want %d..%d (business_hours key %d)", openMin, closeMin, wantOpen, wantClose, key)
			}
		})
	}
}
