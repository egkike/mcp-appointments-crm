package entity

import (
	"testing"
	"time"
)

// TestScheduleDayWeekdayTable pins the complete 0..6 table of the schedules/day
// encoding: Sunday=0 .. Saturday=6, matching time.Weekday and
// schedules.day_of_week. This is the encoding the persistence port receives.
func TestScheduleDayWeekdayTable(t *testing.T) {
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		if got, want := ScheduleDay(wd), ScheduleWeekday(wd); got != want {
			t.Errorf("ScheduleDay(%v) = %d; want %d", wd, got, want)
		}
	}
}

// TestProfileDayKeyWeekdayTable pins the complete 0..6 table of the
// business_hours encoding: Monday=1 .. Sunday=7. Sunday is the asymmetric case
// (7, never 0); every other day is its Go index.
func TestProfileDayKeyWeekdayTable(t *testing.T) {
	want := [7]DayKey{7, 1, 2, 3, 4, 5, 6} // indexed by time.Weekday (Sunday=0)
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		if got := ProfileDayKey(wd); got != want[wd] {
			t.Errorf("ProfileDayKey(%v) = %d; want %d", wd, got, want[wd])
		}
	}
}

// TestDayKeySundayAndMondayBoundary pins the two days where the conventions are
// easiest to mix up: Sunday is 7 in business_hours but 0 in schedules, while
// Monday is 1 in both.
func TestDayKeySundayAndMondayBoundary(t *testing.T) {
	tests := []struct {
		name       string
		weekday    time.Weekday
		wantDayKey DayKey
		wantSched  ScheduleWeekday
	}{
		{"sunday is 7 in business_hours and 0 in schedules", time.Sunday, 7, 0},
		{"monday is 1 in both encodings", time.Monday, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProfileDayKey(tt.weekday); got != tt.wantDayKey {
				t.Errorf("ProfileDayKey(%v) = %d; want %d", tt.weekday, got, tt.wantDayKey)
			}
			if got := ScheduleDay(tt.weekday); got != tt.wantSched {
				t.Errorf("ScheduleDay(%v) = %d; want %d", tt.weekday, got, tt.wantSched)
			}
		})
	}
}

// TestGetOpenCloseDayKeyBoundary pins the fail-closed frontier of the typed day
// key entry point: an out-of-range or unconfigured day is "legitimately
// closed" (ok=false, err=nil), while a corrupt business_hours payload fails
// closed with an error for every key, so it can never read as another day's
// hours.
func TestGetOpenCloseDayKeyBoundary(t *testing.T) {
	bp := &BusinessProfile{BusinessHours: `{"1":{"open":"09:00","close":"18:00"}}`}

	// DayKey(0) and DayKey(8) are out of the documented 1..7 range; DayKey(3)
	// is in range but absent from the payload. All three are closed days, not
	// errors, and carry no times.
	for _, day := range []DayKey{0, 3, 8} {
		open, close, ok, err := bp.GetOpenClose(day)
		if err != nil {
			t.Errorf("GetOpenClose(DayKey(%d)) error = %v; want nil (legitimately closed)", day, err)
		}
		if ok {
			t.Errorf("GetOpenClose(DayKey(%d)) ok = true; want false", day)
		}
		if open != "" || close != "" {
			t.Errorf("GetOpenClose(DayKey(%d)) = %q-%q; want empty times", day, open, close)
		}
	}

	// The configured Monday must still resolve through ProfileDayKey.
	open, close, ok, err := bp.GetOpenClose(ProfileDayKey(time.Monday))
	if err != nil || !ok {
		t.Fatalf("GetOpenClose(ProfileDayKey(Monday)) = ok=%v err=%v; want ok=true err=nil", ok, err)
	}
	if open != "09:00" || close != "18:00" {
		t.Errorf("GetOpenClose(ProfileDayKey(Monday)) = %s-%s; want 09:00-18:00", open, close)
	}

	// A corrupt, in-range day entry fails closed for any valid key.
	corrupt := &BusinessProfile{BusinessHours: `{"1":null}`}
	for _, day := range []DayKey{1, 7} {
		_, _, ok, err := corrupt.GetOpenClose(day)
		if ok {
			t.Errorf("GetOpenClose(DayKey(%d)) on corrupt payload ok = true; want false", day)
		}
		if err == nil {
			t.Errorf("GetOpenClose(DayKey(%d)) on corrupt payload error = nil; want a fail-closed error", day)
		}
	}
}
