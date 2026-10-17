package entity

import (
	"errors"
	"math"
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
		if _, _, ok, err := bp.GetOpenClose(key); err != nil || !ok {
			t.Errorf("GetOpenClose(ProfileDayKey(%v)=%d) = ok=%v err=%v; want ok=true err=nil", wd, key, ok, err)
		}
	}
}

func TestBusinessProfile_GetOpenClose(t *testing.T) {
	bh := `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`
	bp := &BusinessProfile{BusinessHours: bh}

	t.Run("existing day returns hours", func(t *testing.T) {
		open, close, ok, err := bp.GetOpenClose(1)
		if err != nil {
			t.Fatalf("GetOpenClose(1) error = %v, want nil", err)
		}
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
		open, close, ok, err := bp.GetOpenClose(6)
		if err != nil {
			t.Fatalf("GetOpenClose(6) error = %v, want nil", err)
		}
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

	t.Run("non-existing day returns false without error", func(t *testing.T) {
		_, _, ok, err := bp.GetOpenClose(3)
		if ok {
			t.Error("GetOpenClose(3) returned ok=true, want false")
		}
		if err != nil {
			t.Errorf("GetOpenClose(3) error = %v, want nil (legitimately closed)", err)
		}
	})

	t.Run("invalid JSON returns false with the parse error", func(t *testing.T) {
		bpBad := &BusinessProfile{BusinessHours: `{broken`}
		_, _, ok, err := bpBad.GetOpenClose(1)
		if ok {
			t.Error("GetOpenClose with invalid JSON returned ok=true, want false")
		}
		if err == nil {
			t.Error("GetOpenClose with invalid JSON returned nil error, want the parse error")
		}
	})

	t.Run("legacy zero-padded day keys normalize", func(t *testing.T) {
		legacy := &BusinessProfile{BusinessHours: `{"01":{"open":"09:00","close":"18:00"},"007":{"open":"10:00","close":"14:00"}}`}
		if _, _, ok, err := legacy.GetOpenClose(1); !ok || err != nil {
			t.Errorf(`GetOpenClose(1) = ok=%v err=%v; want legacy key "01" normalized to day 1`, ok, err)
		}
		if _, _, ok, err := legacy.GetOpenClose(7); !ok || err != nil {
			t.Errorf(`GetOpenClose(7) = ok=%v err=%v; want legacy key "007" normalized to day 7`, ok, err)
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
		{
			name:          "signed key",
			businessHours: `{"+1":{"open":"09:00","close":"18:00"}}`,
			wantKey:       `"+1"`,
		},
		{
			name:          "key zero",
			businessHours: `{"0":{"open":"09:00","close":"18:00"}}`,
			wantKey:       `"0"`,
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
			// Write-only leniency guard: the READ path normalizes legacy
			// zero-padded keys ("01","007"), but a WRITE gate has no legacy DB
			// to be compatible with — non-canonical keys must not accumulate.
			name:          "zero-padded day key rejected on write",
			businessHours: `{"01":{"open":"09:00","close":"18:00"}}`,
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

// coordPtr returns a *float64 for the coordinate-validation table. Nil-coordinate
// cases pass nil explicitly.
func coordPtr(v float64) *float64 { return &v }

// nanFloat/infFloat build the non-finite coordinate values the NaN/Inf
// defense-in-depth guards must reject.
func nanFloat() *float64         { v := math.NaN(); return &v }
func infFloat(sign int) *float64 { v := math.Inf(sign); return &v }

// TestBusinessProfile_ValidateCoordinates pins the WGS-84 range invariant of
// the singleton profile: the domain is the authoritative gate, so an
// out-of-range numeric coordinate must be rejected even when it bypasses the
// `geo:` URI parser (e.g. a raw {"latitude": 200} tool call). Exact boundaries
// are inclusive; nil means "not set" and is always allowed.
func TestBusinessProfile_ValidateCoordinates(t *testing.T) {
	// withValidScalars supplies the mandatory identity scalars: this table
	// exercises ONLY the coordinate rule, and its fixtures must carry the
	// valid baseline that the mandatory-scalars table already pins.
	withValidScalars := func(lat, long *float64) *BusinessProfile {
		return &BusinessProfile{
			Name: "Barbería Don José", CurrencyCode: "ARS", CurrencySymbol: "$",
			SlotIntervalMinutes: 30, Latitude: lat, Longitude: long,
		}
	}
	tests := []struct {
		name    string
		lat     *float64
		long    *float64
		wantErr bool
		wantSub string
	}{
		{name: "nil coordinates are allowed"},
		{name: "in-range coordinates are allowed", lat: coordPtr(-34.6037), long: coordPtr(-58.3816)},
		{name: "latitude 90.000001 is rejected", lat: coordPtr(90.000001), wantErr: true, wantSub: "latitud"},
		{name: "latitude -90.000001 is rejected", lat: coordPtr(-90.000001), wantErr: true, wantSub: "latitud"},
		{name: "longitude 180.000001 is rejected", long: coordPtr(180.000001), wantErr: true, wantSub: "longitud"},
		{name: "longitude -180.000001 is rejected", long: coordPtr(-180.000001), wantErr: true, wantSub: "longitud"},
		{name: "latitude 200 is rejected", lat: coordPtr(200), wantErr: true, wantSub: "latitud"},
		{name: "exact latitude -90 is allowed", lat: coordPtr(-90)},
		{name: "exact latitude 90 is allowed", lat: coordPtr(90)},
		{name: "exact longitude -180 is allowed", long: coordPtr(-180)},
		{name: "exact longitude 180 is allowed", long: coordPtr(180)},
		// NaN/Inf bypass both range comparisons (defense-in-depth: today no
		// parse path can produce them, but entity.Validate is declared the
		// authoritative invariant for raw numerics).
		{name: "latitude NaN is rejected", lat: nanFloat(), wantErr: true, wantSub: "latitud"},
		{name: "longitude +Inf is rejected", long: infFloat(+1), wantErr: true, wantSub: "longitud"},
		{name: "longitude -Inf is rejected", long: infFloat(-1), wantErr: true, wantSub: "longitud"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := withValidScalars(tt.lat, tt.long)
			err := bp.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				return
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("Validate() error = %v, want wrapping domain.ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("Validate() error = %q, want it to mention %q", err, tt.wantSub)
			}
		})
	}
}

func TestBusinessProfile_ValidateMandatoryScalars(t *testing.T) {
	// validScalars returns a profile whose mandatory identity scalars (name,
	// currency code, currency symbol, slot interval) are all valid, mirroring
	// the install.sh wizard semantics (v_nonempty / v_currency / v_symbol /
	// v_positive_int); individual cases break one at a time.
	validScalars := func() BusinessProfile {
		return BusinessProfile{Name: "Barbería Don José", CurrencyCode: "ARS", CurrencySymbol: "$", SlotIntervalMinutes: 30}
	}
	tests := []struct {
		name    string
		mutate  func(*BusinessProfile)
		wantErr bool
		wantSub string
	}{
		{name: "valid identity scalars pass", mutate: func(*BusinessProfile) {}},
		{
			name:    "empty name is rejected (v_nonempty)",
			mutate:  func(bp *BusinessProfile) { bp.Name = "" },
			wantErr: true, wantSub: "nombre",
		},
		{
			name:    "whitespace-only name is rejected",
			mutate:  func(bp *BusinessProfile) { bp.Name = "   " },
			wantErr: true, wantSub: "nombre",
		},
		{
			name:    "empty currency code is rejected (v_currency)",
			mutate:  func(bp *BusinessProfile) { bp.CurrencyCode = "" },
			wantErr: true, wantSub: "moneda",
		},
		{
			name:    "lowercase currency code is rejected",
			mutate:  func(bp *BusinessProfile) { bp.CurrencyCode = "ars" },
			wantErr: true, wantSub: "moneda",
		},
		{
			name:    "two-letter currency code is rejected",
			mutate:  func(bp *BusinessProfile) { bp.CurrencyCode = "AR" },
			wantErr: true, wantSub: "moneda",
		},
		{
			name:    "four-letter currency code is rejected",
			mutate:  func(bp *BusinessProfile) { bp.CurrencyCode = "ARSS" },
			wantErr: true, wantSub: "moneda",
		},
		{
			name:    "empty currency symbol is rejected (v_symbol)",
			mutate:  func(bp *BusinessProfile) { bp.CurrencySymbol = "" },
			wantErr: true, wantSub: "símbolo",
		},
		{
			name:    "zero slot interval is rejected (v_positive_int)",
			mutate:  func(bp *BusinessProfile) { bp.SlotIntervalMinutes = 0 },
			wantErr: true, wantSub: "intervalo",
		},
		{
			name:    "negative slot interval is rejected",
			mutate:  func(bp *BusinessProfile) { bp.SlotIntervalMinutes = -15 },
			wantErr: true, wantSub: "intervalo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := validScalars()
			tt.mutate(&bp)
			err := bp.Validate()
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("Validate() error = %v; want wrapping domain.ErrInvalidInput", err)
				}
				var sem *domain.SemanticError
				if !errors.As(err, &sem) {
					t.Fatalf("Validate() error = %v; want *domain.SemanticError", err)
				}
				if sem.Code != domain.ErrCodeInvalidInput {
					t.Errorf("SemanticError.Code = %v; want ErrCodeInvalidInput", sem.Code)
				}
				if !strings.Contains(err.Error(), tt.wantSub) {
					t.Errorf("Validate() error = %q; want it to mention %q", err, tt.wantSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v; want nil", err)
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
	// Mandatory identity scalars baseline: every fixture carries them so each
	// case exercises its own rule (the mandatory gate itself has its own
	// dedicated table above).
	validName, validCode, validSymbol, validSlot := "Barbería Don José", "ARS", "$", 30

	tests := []struct {
		name    string
		bp      *BusinessProfile
		wantErr bool
	}{
		{
			name: "valid profile with all optional fields",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
				MessengerPlatform:      &whatsapp,
				AcceptedPaymentMethods: &validPayments,
				BusinessHours:          validHours,
				Timezone:               validTZ,
			},
		},
		{
			name: "valid profile with nil optional fields",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
			},
		},
		{
			name: "valid profile with telegram platform",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
				MessengerPlatform: &telegram,
			},
		},
		{
			name: "invalid messenger platform",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
				MessengerPlatform: &invalidPlatform,
			},
			wantErr: true,
		},
		{
			name: "invalid payment methods (not an array)",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
				AcceptedPaymentMethods: &invalidPayments,
			},
			wantErr: true,
		},
		{
			name: "invalid payment methods (empty string in array)",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
				AcceptedPaymentMethods: &emptyStrPayment,
			},
			wantErr: true,
		},
		{
			name: "invalid business hours (array instead of object)",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
				BusinessHours: invalidHours,
			},
			wantErr: true,
		},
		{
			name: "invalid timezone",
			bp: &BusinessProfile{
				Name: validName, CurrencyCode: validCode, CurrencySymbol: validSymbol, SlotIntervalMinutes: validSlot,
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

func TestBusinessProfile_validateBusinessHoursJSON_deterministicFirstError(t *testing.T) {
	// Two invalid days in randomized map order: key "9" fails the day-key rule
	// ("clave de día"), key "2" fails the HH:MM rule. The sorted-first key ("2")
	// must be reported every run — a key-order regression would surface the
	// "clave de día" error instead.
	bp := &BusinessProfile{BusinessHours: `{"9":{"open":"09:00","close":"18:00"},"2":{"open":"25:00","close":"18:00"}}`}
	err := bp.validateBusinessHoursJSON()
	if err == nil {
		t.Fatal("validateBusinessHoursJSON() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "día 2") || !strings.Contains(err.Error(), "HH:MM") {
		t.Errorf("validateBusinessHoursJSON() error = %q, want the sorted-first day (\"2\") HH:MM error", err)
	}
	if strings.Contains(err.Error(), "clave de día") {
		t.Errorf("validateBusinessHoursJSON() error = %q, want the day-2 HH:MM error, not the key-9 rule", err)
	}
}

func TestBusinessProfile_parseBusinessHours_deterministicFirstError(t *testing.T) {
	// Two invalid day keys ("9" and "8"): the sorted-first key ("8") must be
	// reported every run.
	bp := &BusinessProfile{BusinessHours: `{"9":{"open":"09:00","close":"18:00"},"8":{"open":"09:00","close":"18:00"}}`}
	_, err := bp.parseBusinessHours()
	if err == nil {
		t.Fatal("parseBusinessHours() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `"8"`) {
		t.Errorf("parseBusinessHours() error = %q, want the sorted-first key (\"8\") error", err)
	}
}

func TestParseBusinessHoursRejectsNullMap(t *testing.T) {
	// GGA round 8: JSON "null" unmarshals successfully into a nil map, which
	// would read as "no configured days". Corrupt/half-migrated stored data
	// must fail instead of looking like a closed schedule.
	bp := BusinessProfile{BusinessHours: "null"}
	days, err := bp.parseBusinessHours()
	if err == nil {
		t.Fatal("parseBusinessHours(\"null\") error = nil, want a corruption failure")
	}
	if days != nil {
		t.Errorf("days = %v, want nil on failure", days)
	}
	if !strings.Contains(err.Error(), "business_hours no puede ser null") {
		t.Errorf("err = %v, want it to name the null corruption", err)
	}
}

func TestBusinessProfileValidateRejectsUnsafeNameChars(t *testing.T) {
	// GGA round 8: the name re-renders into the SKILL.md Hermes loads, so
	// markdown-active characters are rejected at the domain gate.
	bp := &BusinessProfile{
		Name:                "Acme ** Herrajes",
		CurrencyCode:        "ARS",
		CurrencySymbol:      "$",
		SlotIntervalMinutes: 15,
	}
	err := bp.Validate()
	if err == nil {
		t.Fatal("Validate(markdown name) error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "caracteres especiales de marcado") {
		t.Errorf("err = %v, want it to name the unsafe-character rule", err)
	}
}

func TestParseBusinessHoursRejectsNullDayAndBadTimes(t *testing.T) {
	// GGA round 13: per-day fail-closed on the read path — a null day decodes
	// into a zero struct; corrupt times must not read as availability.
	cases := []struct {
		name   string
		json   string
		substr string
	}{
		{"null day", `{"1":null}`, "no puede ser null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bp := BusinessProfile{BusinessHours: tc.json}
			days, err := bp.parseBusinessHours()
			if err == nil {
				t.Fatalf("parseBusinessHours(%s) error = nil, want rejection", tc.json)
			}
			if days != nil {
				t.Errorf("days = %v, want nil on failure", days)
			}
			if !strings.Contains(err.Error(), tc.substr) {
				t.Errorf("err = %v, want it to contain %q", err, tc.substr)
			}
		})
	}
}
