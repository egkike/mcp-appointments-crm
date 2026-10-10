package repository

import (
	"errors"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// assertSemanticInvalidInput proves an FTS validation error is a
// *domain.SemanticError carrying domain.ErrCodeInvalidInput, while the
// domain.ErrInvalidInput sentinel stays reachable through the chain (F2).
func assertSemanticInvalidInput(t *testing.T, err error) {
	t.Helper()
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Errorf("got %v (%T); want *domain.SemanticError", err, err)
		return
	}
	if sem.Code != domain.ErrCodeInvalidInput {
		t.Errorf("Code = %q; want %q", sem.Code, domain.ErrCodeInvalidInput)
	}
}

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
					return
				}
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Errorf("validateExceptionDate(%q) = %v; want domain.ErrInvalidInput", tt.date, err)
				}
				// Same contract as the FTS assertions (F1): a *domain.SemanticError
				// with ErrCodeInvalidInput so the transport maps it to -32002.
				var sem *domain.SemanticError
				if !errors.As(err, &sem) {
					t.Errorf("validateExceptionDate(%q) = %v (%T); want *domain.SemanticError", tt.date, err, err)
				} else if sem.Code != domain.ErrCodeInvalidInput {
					t.Errorf("validateExceptionDate(%q) Code = %q; want %q", tt.date, sem.Code, domain.ErrCodeInvalidInput)
				}
			} else if err != nil {
				t.Errorf("validateExceptionDate(%q) unexpected error: %v", tt.date, err)
			}
		})
	}
}

// TestValidateExceptionDateSemanticMessages pins the user-facing Spanish
// messages for date failures and proves the raw date input is NOT reflected in
// Message: once these errors map to -32002, Message reaches the upstream LLM,
// so only Cause may carry the raw %q echo.
func TestValidateExceptionDateSemanticMessages(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		wantMsg string
	}{
		{"invalid format", "25/12/2026", "la fecha debe tener formato YYYY-MM-DD."},
		{"invalid calendar", "2026-02-30", "la fecha no es una fecha válida (formato YYYY-MM-DD)."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExceptionDate(tt.date)
			var sem *domain.SemanticError
			if !errors.As(err, &sem) {
				t.Fatalf("validateExceptionDate(%q) = %v (%T); want *domain.SemanticError", tt.date, err, err)
			}
			if sem.Message != tt.wantMsg {
				t.Errorf("Message = %q; want %q", sem.Message, tt.wantMsg)
			}
			if strings.Contains(sem.Message, tt.date) {
				t.Errorf("Message %q echoes raw date %q", sem.Message, tt.date)
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("sentinel lost: %v", err)
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
		{"valid alphanumeric term", "b2b", false},
		{"valid accented acronym", "ÁÉÍ", false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"asterisk", "juan*", true},
		{"plus sign", "juan+", true},
		{"quotes", `"juan"`, true},
		{"hyphen prefix", "-penicilina", true},
		{"hyphen at start", "-geo", true},
		{"hyphen at end", "geo-", true},
		{"hyphen inside multi-term", "a- b", true},
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
					return
				}
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Errorf("validateFTSQuery(%q) = %v; want domain.ErrInvalidInput", tt.query, err)
				}
				// F2: the error must also be a *domain.SemanticError so the
				// transport maps it to -32002 instead of -32603.
				var sem *domain.SemanticError
				if !errors.As(err, &sem) {
					t.Errorf("validateFTSQuery(%q) = %v (%T); want *domain.SemanticError", tt.query, err, err)
				} else if sem.Code != domain.ErrCodeInvalidInput {
					t.Errorf("validateFTSQuery(%q) Code = %q; want %q", tt.query, sem.Code, domain.ErrCodeInvalidInput)
				}
			} else if err != nil {
				t.Errorf("validateFTSQuery(%q) unexpected error: %v", tt.query, err)
			}
		})
	}
}

// TestValidateFTSQuerySemanticMessages pins the four user-facing Spanish
// messages (F2): SemanticError.Message is what reaches the LLM, so it must be
// the clean message with no wrapped sentinel/error tail.
func TestValidateFTSQuerySemanticMessages(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantMsg string
	}{
		{"empty", "   ", "la consulta de búsqueda está vacía."},
		{"too long", strings.Repeat("a", MaxFTSQueryLen+1), "la consulta de búsqueda excede la longitud máxima (200 caracteres)."},
		{"forbidden characters", "juan*", "la consulta de búsqueda contiene caracteres no permitidos."},
		{"forbidden operator", "juan AND maria", "la consulta de búsqueda contiene operadores no permitidos."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFTSQuery(tt.query)
			var sem *domain.SemanticError
			if !errors.As(err, &sem) {
				t.Fatalf("validateFTSQuery(%q) = %v (%T); want *domain.SemanticError", tt.query, err, err)
			}
			if sem.Message != tt.wantMsg {
				t.Errorf("Message = %q; want %q", sem.Message, tt.wantMsg)
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("sentinel lost: %v", err)
			}
		})
	}
}
