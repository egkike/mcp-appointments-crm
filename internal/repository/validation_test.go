package repository

import (
	"errors"
	"testing"
)

func TestValidateExceptionDate(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		wantErr bool
	}{
		{"valid date", "2026-12-25", false},
		{"valid leap year", "2024-02-29", false},
		{"invalid format: datetime", "2026-12-25T00:00:00", true},
		{"invalid format: slashes", "25/12/2026", true},
		{"invalid format: empty", "", true},
		{"invalid calendar: month 13", "2026-13-45", true},
		{"invalid calendar: feb 30", "2026-02-30", true},
		{"invalid calendar: non-leap feb 29", "2025-02-29", true},
		{"invalid format: partial", "2026-12", true},
		{"invalid format: letters", "abcd-ef-gh", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExceptionDate(tt.date)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateExceptionDate(%q) expected error, got nil", tt.date)
				} else if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("validateExceptionDate(%q) = %v; want ErrInvalidInput", tt.date, err)
				}
			} else if err != nil {
				t.Errorf("validateExceptionDate(%q) unexpected error: %v", tt.date, err)
			}
		})
	}
}

func TestDatePattern(t *testing.T) {
	tests := []struct {
		input string
		match bool
	}{
		{"2026-12-25", true},
		{"2026-1-25", false},
		{"2026-12-25T00:00:00", false},
		{"", false},
		{"abcd-ef-gh", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := datePattern.MatchString(tt.input)
			if got != tt.match {
				t.Errorf("datePattern.MatchString(%q) = %v; want %v", tt.input, got, tt.match)
			}
		})
	}
}

func TestTimeHHMMRe(t *testing.T) {
	tests := []struct {
		input string
		match bool
	}{
		{"00:00", true},
		{"23:59", true},
		{"09:30", true},
		{"24:00", false},
		{"9:00", false},
		{"12:70", false},
		{"12:0", false},
		{"12:00:00", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := timeHHMMRe.MatchString(tt.input)
			if got != tt.match {
				t.Errorf("timeHHMMRe.MatchString(%q) = %v; want %v", tt.input, got, tt.match)
			}
		})
	}
}

func TestValidateFTSQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"valid simple", "juan", false},
		{"valid accented", "María", false},
		{"valid with space", "juan perez", false},
		{"valid with hyphen", "geo-local", false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"asterisk", "juan*", true},
		{"plus sign", "juan+", true},
		{"quotes", `"juan"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFTSQuery(tt.query)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateFTSQuery(%q) expected error, got nil", tt.query)
				} else if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("validateFTSQuery(%q) = %v; want ErrInvalidInput", tt.query, err)
				}
			} else if err != nil {
				t.Errorf("validateFTSQuery(%q) unexpected error: %v", tt.query, err)
			}
		})
	}
}

func TestValidateBusinessHoursJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty allowed", "", false},
		{"valid object", `{"mon":{"open":"09:00","close":"18:00"}}`, false},
		{"empty object", `{}`, false},
		{"malformed JSON", `{invalid`, true},
		{"JSON array", `["not","object"]`, true},
		{"JSON string", `"just a string"`, true},
		{"JSON number", `42`, true},
		{"JSON null", `null`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBusinessHoursJSON(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateBusinessHoursJSON(%q) expected error, got nil", tt.input)
				} else if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("validateBusinessHoursJSON(%q) = %v; want ErrInvalidInput", tt.input, err)
				}
			} else if err != nil {
				t.Errorf("validateBusinessHoursJSON(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestValidateTimezone(t *testing.T) {
	tests := []struct {
		name    string
		tz      string
		wantErr bool
	}{
		{"empty allowed", "", false},
		{"UTC", "UTC", false},
		{"IANA zone", "America/Argentina/Buenos_Aires", false},
		{"US zone", "US/Eastern", false},
		{"invalid zone", "Not/A/Real/Zone", true},
		{"garbage", "foobar", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimezone(tt.tz)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateTimezone(%q) expected error, got nil", tt.tz)
				} else if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("validateTimezone(%q) = %v; want ErrInvalidInput", tt.tz, err)
				}
			} else if err != nil {
				t.Errorf("validateTimezone(%q) unexpected error: %v", tt.tz, err)
			}
		})
	}
}

func TestValidateAcceptedPaymentMethodsJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid array", `["cash","credit_card"]`, false},
		{"empty array", `[]`, false},
		{"JSON null", `null`, true},
		{"malformed JSON", `not-json`, true},
		{"empty string", ``, true},
		{"object instead of array", `{"a":"b"}`, true},
		{"empty string in array", `["cash",""]`, true},
		{"string instead of array", `"cash"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAcceptedPaymentMethodsJSON(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateAcceptedPaymentMethodsJSON(%q) expected error, got nil", tt.input)
				} else if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("validateAcceptedPaymentMethodsJSON(%q) = %v; want ErrInvalidInput", tt.input, err)
				}
			} else if err != nil {
				t.Errorf("validateAcceptedPaymentMethodsJSON(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}
