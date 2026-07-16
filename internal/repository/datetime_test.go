package repository

import (
	"testing"
	"time"
)

func TestParseBusinessTimezone(t *testing.T) {
	t.Run("valid IANA timezone", func(t *testing.T) {
		loc, err := ParseBusinessTimezone("America/Argentina/Buenos_Aires")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc == nil {
			t.Fatal("expected non-nil *time.Location")
		}
		if loc.String() != "America/Argentina/Buenos_Aires" {
			t.Errorf("got location %q; want %q", loc.String(), "America/Argentina/Buenos_Aires")
		}
	})

	t.Run("UTC is valid", func(t *testing.T) {
		loc, err := ParseBusinessTimezone("UTC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc.String() != "UTC" {
			t.Errorf("got location %q; want %q", loc.String(), "UTC")
		}
	})

	t.Run("invalid IANA returns error", func(t *testing.T) {
		_, err := ParseBusinessTimezone("Not/A_Timezone")
		if err == nil {
			t.Fatal("expected error for invalid timezone, got nil")
		}
	})

	t.Run("empty string returns error", func(t *testing.T) {
		_, err := ParseBusinessTimezone("")
		if err == nil {
			t.Fatal("expected error for empty timezone, got nil")
		}
	})
}

func TestParseStartDatetime(t *testing.T) {
	t.Run("RFC3339 with explicit offset", func(t *testing.T) {
		loc, _ := time.LoadLocation("America/Argentina/Buenos_Aires")
		dt, err := ParseStartDatetime("2026-06-25T23:00:00-03:00", loc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 2026-06-25T23:00:00-03:00 = 2026-06-26T02:00:00Z
		want := time.Date(2026, 6, 26, 2, 0, 0, 0, time.UTC)
		if !dt.UTC().Equal(want) {
			t.Errorf("got UTC %v; want %v", dt.UTC(), want)
		}
	})

	t.Run("RFC3339 with Z suffix", func(t *testing.T) {
		loc, _ := time.LoadLocation("UTC")
		dt, err := ParseStartDatetime("2026-07-13T13:00:00Z", loc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dt.Hour() != 13 || dt.Day() != 13 {
			t.Errorf("got %v; want 2026-07-13T13:00:00Z", dt)
		}
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		loc, _ := time.LoadLocation("UTC")
		_, err := ParseStartDatetime("not-a-date", loc)
		if err == nil {
			t.Fatal("expected error for invalid datetime, got nil")
		}
	})

	t.Run("date only returns error", func(t *testing.T) {
		loc, _ := time.LoadLocation("UTC")
		_, err := ParseStartDatetime("2026-07-13", loc)
		if err == nil {
			t.Fatal("expected error for date-only input, got nil")
		}
	})
}

func TestFormatStorage(t *testing.T) {
	t.Run("UTC time formatted with milliseconds", func(t *testing.T) {
		ts := time.Date(2026, 7, 13, 13, 30, 0, 0, time.UTC)
		got := FormatStorage(ts)
		want := "2026-07-13T13:30:00.000Z"
		if got != want {
			t.Errorf("FormatStorage() = %q; want %q", got, want)
		}
	})

	t.Run("non-UTC time is converted to UTC", func(t *testing.T) {
		loc, _ := time.LoadLocation("America/Argentina/Buenos_Aires")
		ts := time.Date(2026, 6, 25, 23, 0, 0, 0, loc) // -03:00 → 02:00 UTC next day
		got := FormatStorage(ts)
		want := "2026-06-26T02:00:00.000Z"
		if got != want {
			t.Errorf("FormatStorage() = %q; want %q", got, want)
		}
	})

	t.Run("millisecond precision preserved", func(t *testing.T) {
		ts := time.Date(2026, 1, 1, 0, 0, 0, 123000000, time.UTC) // 123ms
		got := FormatStorage(ts)
		want := "2026-01-01T00:00:00.123Z"
		if got != want {
			t.Errorf("FormatStorage() = %q; want %q", got, want)
		}
	})
}

func TestDatetimeRoundTrip(t *testing.T) {
	t.Run("parse then format then parse yields same instant", func(t *testing.T) {
		loc, _ := time.LoadLocation("America/Argentina/Buenos_Aires")
		input := "2026-06-25T23:00:00-03:00"

		dt1, err := ParseStartDatetime(input, loc)
		if err != nil {
			t.Fatalf("first parse: %v", err)
		}

		formatted := FormatStorage(dt1)

		dt2, err := ParseStartDatetime(formatted, time.UTC)
		if err != nil {
			t.Fatalf("second parse: %v", err)
		}

		if !dt1.UTC().Equal(dt2.UTC()) {
			t.Errorf("round-trip failed: %v != %v", dt1.UTC(), dt2.UTC())
		}
	})

	t.Run("cross-timezone same instant", func(t *testing.T) {
		locBA, _ := time.LoadLocation("America/Argentina/Buenos_Aires")
		locUTC, _ := time.LoadLocation("UTC")

		dt1, err := ParseStartDatetime("2026-06-25T23:00:00-03:00", locBA)
		if err != nil {
			t.Fatalf("parse BA: %v", err)
		}
		dt2, err := ParseStartDatetime("2026-06-26T02:00:00Z", locUTC)
		if err != nil {
			t.Fatalf("parse UTC: %v", err)
		}

		if !dt1.Equal(dt2) {
			t.Errorf("expected same instant, got %v and %v", dt1, dt2)
		}
	})
}
