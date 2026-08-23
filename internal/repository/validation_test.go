package repository

import (
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
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
				} else if !errors.Is(err, domain.ErrInvalidInput) {
					t.Errorf("validateExceptionDate(%q) = %v; want domain.ErrInvalidInput", tt.date, err)
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
		{"hyphen prefix", "-penicilina", true},
		{"plus prefix", "+alergia", true},
		{"whitespace then hyphen operator", "alergia -penicilina", true},
		{"whole word AND", "juan AND maria", true},
		{"whole word OR", "juan OR maria", true},
		{"whole word NOT", "juan NOT maria", true},
		{"lowercase operator", "juan and maria", true},
		{"trailing operator", "juan AND", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFTSQuery(tt.query)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateFTSQuery(%q) expected error, got nil", tt.query)
				} else if !errors.Is(err, domain.ErrInvalidInput) {
					t.Errorf("validateFTSQuery(%q) = %v; want domain.ErrInvalidInput", tt.query, err)
				}
			} else if err != nil {
				t.Errorf("validateFTSQuery(%q) unexpected error: %v", tt.query, err)
			}
		})
	}
}
