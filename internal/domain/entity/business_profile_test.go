package entity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestProfileDayKey(t *testing.T) {
	tests := []struct {
		name    string
		weekday time.Weekday
		want    int
	}{
		{"sunday maps to 7", time.Sunday, 7},
		{"monday maps to 1", time.Monday, 1},
		{"tuesday maps to 2", time.Tuesday, 2},
		{"wednesday maps to 3", time.Wednesday, 3},
		{"thursday maps to 4", time.Thursday, 4},
		{"friday maps to 5", time.Friday, 5},
		{"saturday maps to 6", time.Saturday, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProfileDayKey(tt.weekday); got != tt.want {
				t.Errorf("ProfileDayKey(%v) = %d; want %d", tt.weekday, got, tt.want)
			}
		})
	}
}

// TestProfileDayKeyMatchesBusinessHoursEncoding pins the translation against
// the real business_hours JSON encoding ("1"=Monday .. "7"=Sunday), so the two
// encodings can never silently drift apart.
func TestProfileDayKeyMatchesBusinessHoursEncoding(t *testing.T) {
	bp := &BusinessProfile{BusinessHours: `{"1":{"open":"09:00","close":"18:00"},` +
		`"2":{"open":"09:00","close":"18:00"},"3":{"open":"09:00","close":"18:00"},` +
		`"4":{"open":"09:00","close":"18:00"},"5":{"open":"09:00","close":"18:00"},` +
		`"6":{"open":"10:00","close":"14:00"},"7":{"open":"10:00","close":"14:00"}}`}

	weekdays := []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday}
	for _, wd := range weekdays {
		key := ProfileDayKey(wd)
		if key < 1 || key > 7 {
			t.Errorf("ProfileDayKey(%v) = %d; want a key inside 1..7", wd, key)
			continue
		}
		if !bp.IsOpenOn(key) {
			t.Errorf("IsOpenOn(ProfileDayKey(%v)=%d) = false; want true", wd, key)
		}
	}
}

func TestBusinessProfile_IsOpenOn(t *testing.T) {
	tests := []struct {
		name          string
		businessHours string
		dayOfWeek     int
		want          bool
	}{
		{
			name:          "open on monday (day 1)",
			businessHours: `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`,
			dayOfWeek:     1,
			want:          true,
		},
		{
			name:          "closed on wednesday (day 3)",
			businessHours: `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`,
			dayOfWeek:     3,
			want:          false,
		},
		{
			name:          "open on saturday (day 6)",
			businessHours: `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`,
			dayOfWeek:     6,
			want:          true,
		},
		{
			name:          "empty business hours",
			businessHours: `{}`,
			dayOfWeek:     1,
			want:          false,
		},
		{
			name:          "invalid JSON returns closed",
			businessHours: `{invalid`,
			dayOfWeek:     1,
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &BusinessProfile{BusinessHours: tt.businessHours}
			if got := bp.IsOpenOn(tt.dayOfWeek); got != tt.want {
				t.Errorf("IsOpenOn(%d) = %v, want %v", tt.dayOfWeek, got, tt.want)
			}
		})
	}
}

func TestBusinessProfile_GetOpenClose(t *testing.T) {
	bh := `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`
	bp := &BusinessProfile{BusinessHours: bh}

	t.Run("existing day returns hours", func(t *testing.T) {
		open, close, ok := bp.GetOpenClose(1)
		if !ok {
			t.Fatal("GetOpenClose(1) returned ok=false, want true")
		}
		if open != "09:00" {
			t.Errorf("open = %q, want %q", open, "09:00")
		}
		if close != "18:00" {
			t.Errorf("close = %q, want %q", close, "18:00")
		}
	})

	t.Run("saturday hours", func(t *testing.T) {
		open, close, ok := bp.GetOpenClose(6)
		if !ok {
			t.Fatal("GetOpenClose(6) returned ok=false, want true")
		}
		if open != "10:00" {
			t.Errorf("open = %q, want %q", open, "10:00")
		}
		if close != "14:00" {
			t.Errorf("close = %q, want %q", close, "14:00")
		}
	})

	t.Run("non-existing day returns false", func(t *testing.T) {
		_, _, ok := bp.GetOpenClose(3)
		if ok {
			t.Error("GetOpenClose(3) returned ok=true, want false")
		}
	})

	t.Run("invalid JSON returns false", func(t *testing.T) {
		bpBad := &BusinessProfile{BusinessHours: `{broken`}
		_, _, ok := bpBad.GetOpenClose(1)
		if ok {
			t.Error("GetOpenClose with invalid JSON returned ok=true, want false")
		}
	})
}

func TestBusinessProfile_parseBusinessHoursInvalidKey(t *testing.T) {
	tests := []struct {
		name          string
		businessHours string
		wantKey       string
	}{
		{
			name:          "alphabetic key",
			businessHours: `{"a":{"open":"09:00","close":"18:00"}}`,
			wantKey:       `"a"`,
		},
		{
			name:          "day-name key",
			businessHours: `{"mon":{"open":"09:00","close":"18:00"}}`,
			wantKey:       `"mon"`,
		},
		{
			name:          "key out of range",
			businessHours: `{"8":{"open":"09:00","close":"18:00"}}`,
			wantKey:       `"8"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &BusinessProfile{BusinessHours: tt.businessHours}
			hours, err := bp.parseBusinessHours()
			if err == nil {
				t.Fatalf("parseBusinessHours() error = nil, want error mentioning %s", tt.wantKey)
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("parseBusinessHours() error = %v, want wrapping domain.ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("parseBusinessHours() error = %q, want it to mention key %s", err, tt.wantKey)
			}
			if hours != nil {
				t.Errorf("parseBusinessHours() hours = %v, want nil", hours)
			}
		})
	}
}

func TestBusinessProfile_validateBusinessHoursJSON(t *testing.T) {
	tests := []struct {
		name          string
		businessHours string
		wantSub       string
	}{
		{
			name:          "empty string is allowed",
			businessHours: "",
		},
		{
			name:          "valid single day",
			businessHours: `{"1":{"open":"09:00","close":"18:00"}}`,
		},
		{
			name:          "valid full range day",
			businessHours: `{"1":{"open":"00:00","close":"23:59"},"7":{"open":"10:00","close":"14:00"}}`,
		},
		{
			name:          "non-object JSON array",
			businessHours: `[1,2,3]`,
			wantSub:       "objeto JSON",
		},
		{
			name:          "non-object JSON string",
			businessHours: `"just a string"`,
			wantSub:       "objeto JSON",
		},
		{
			name:          "malformed JSON",
			businessHours: `{invalid`,
			wantSub:       "JSON válido",
		},
		{
			name:          "unpadded opening hour",
			businessHours: `{"1":{"open":"9:00","close":"18:00"}}`,
			wantSub:       "HH:MM",
		},
		{
			name:          "non-numeric hour",
			businessHours: `{"1":{"open":"xx:00","close":"18:00"}}`,
			wantSub:       "HH:MM",
		},
		{
			name:          "out-of-range hour",
			businessHours: `{"1":{"open":"25:00","close":"18:00"}}`,
			wantSub:       "HH:MM",
		},
		{
			name:          "open equals close",
			businessHours: `{"1":{"open":"09:00","close":"09:00"}}`,
			wantSub:       "anterior al cierre",
		},
		{
			name:          "open after close",
			businessHours: `{"1":{"open":"18:00","close":"09:00"}}`,
			wantSub:       "anterior al cierre",
		},
		{
			name:          "day key zero",
			businessHours: `{"0":{"open":"09:00","close":"18:00"}}`,
			wantSub:       "clave de día",
		},
		{
			name:          "day key eight",
			businessHours: `{"8":{"open":"09:00","close":"18:00"}}`,
			wantSub:       "clave de día",
		},
		{
			name:          "day key ninety-nine",
			businessHours: `{"99":{"open":"09:00","close":"18:00"}}`,
			wantSub:       "clave de día",
		},
		{
			name:          "non-numeric day key",
			businessHours: `{"mon":{"open":"09:00","close":"18:00"}}`,
			wantSub:       "clave de día",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &BusinessProfile{BusinessHours: tt.businessHours}
			err := bp.validateBusinessHoursJSON()
			if tt.wantSub == "" {
				if err != nil {
					t.Fatalf("validateBusinessHoursJSON() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateBusinessHoursJSON() error = nil, want error containing %q", tt.wantSub)
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("validateBusinessHoursJSON() error = %v, want wrapping domain.ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("validateBusinessHoursJSON() error = %q, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestBusinessProfile_Validate(t *testing.T) {
	whatsapp := "whatsapp"
	telegram := "telegram"
	invalidPlatform := "signal"
	validPayments := `["cash","card"]`
	invalidPayments := `"not-an-array"`
	emptyStrPayment := `["cash",""]`
	validHours := `{"1":{"open":"09:00","close":"18:00"}}`
	invalidHours := `[1,2,3]`
	validTZ := "America/Argentina/Buenos_Aires"
	invalidTZ := "Mars/Unknown"

	tests := []struct {
		name    string
		bp      *BusinessProfile
		wantErr bool
	}{
		{
			name: "valid profile with all optional fields",
			bp: &BusinessProfile{
				MessengerPlatform:      &whatsapp,
				AcceptedPaymentMethods: &validPayments,
				BusinessHours:          validHours,
				Timezone:               validTZ,
			},
		},
		{
			name: "valid profile with nil optional fields",
			bp:   &BusinessProfile{},
		},
		{
			name: "valid profile with telegram platform",
			bp: &BusinessProfile{
				MessengerPlatform: &telegram,
			},
		},
		{
			name: "invalid messenger platform",
			bp: &BusinessProfile{
				MessengerPlatform: &invalidPlatform,
			},
			wantErr: true,
		},
		{
			name: "invalid payment methods (not an array)",
			bp: &BusinessProfile{
				AcceptedPaymentMethods: &invalidPayments,
			},
			wantErr: true,
		},
		{
			name: "invalid payment methods (empty string in array)",
			bp: &BusinessProfile{
				AcceptedPaymentMethods: &emptyStrPayment,
			},
			wantErr: true,
		},
		{
			name: "invalid business hours (array instead of object)",
			bp: &BusinessProfile{
				BusinessHours: invalidHours,
			},
			wantErr: true,
		},
		{
			name: "invalid timezone",
			bp: &BusinessProfile{
				Timezone: invalidTZ,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bp.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
