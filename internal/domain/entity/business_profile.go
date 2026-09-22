package entity

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// businessHoursHHMMRegex matches a zero-padded 24-hour HH:MM time (00:00..23:59).
var businessHoursHHMMRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// ValidHHMM reports whether s is a strict zero-padded 24-hour "HH:MM" time
// (00:00..23:59). It is the single format gate shared by every consumer of
// stored times: unpadded input ("9:00"), a leading sign or space, a missing
// colon, and trailing characters are all rejected. Consumers in upper layers
// call this instead of re-declaring the pattern, so the format cannot drift.
func ValidHHMM(s string) bool {
	return businessHoursHHMMRegex.MatchString(s)
}

// BusinessProfile is the singleton configuration row for the business.
// There is exactly one row with ID="singleton" (enforced by CHECK constraint).
type BusinessProfile struct {
	ID                     string
	Name                   string
	Industry               *string
	Country                *string
	Address                *string
	Latitude               *float64
	Longitude              *float64
	CoverPhotoURL          *string
	PublicPhone            *string
	MessengerPlatform      *string
	MessengerID            *string
	ContactEmail           *string
	WebsiteURL             *string
	GeneralDescription     *string
	CurrencyCode           string
	CurrencySymbol         string
	AcceptedPaymentMethods *string
	Timezone               string
	SlotIntervalMinutes    int
	BusinessHours          string // JSON: {"1":{"open":"09:00","close":"18:00"},...}
	CreatedAt              string
	UpdatedAt              string
}

// businessHoursDay represents the schedule for a single day.
type businessHoursDay struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// ProfileDayKey translates a time.Weekday into the day key used by
// BusinessProfile.BusinessHours and by the profile day lookups (IsOpenOn,
// GetOpenClose). It is the single translation point between the two day
// encodings used in this project, which must never be mixed up:
//
//   - time.Weekday (Go) and schedules.day_of_week: 0..6, Sunday=0, Monday=1,
//     ..., Saturday=6.
//   - business_hours JSON keys: "1".."7", Monday=1, ..., Saturday=6, Sunday=7.
func ProfileDayKey(w time.Weekday) int {
	if w == time.Sunday {
		return 7
	}
	return int(w)
}

// IsOpenOn reports whether the business is open on the given day of week
// (1=Monday, 7=Sunday). Returns false if BusinessHours is empty or invalid JSON.
// The day must come from ProfileDayKey (or an equivalent "1".."7" literal),
// never from a raw int(time.Weekday()).
func (bp *BusinessProfile) IsOpenOn(dayOfWeek int) bool {
	hours, err := bp.parseBusinessHours()
	if err != nil {
		return false
	}
	_, exists := hours[dayOfWeek]
	return exists
}

// GetOpenClose returns the open and close times (HH:MM) for the given day of week
// (1=Monday, 7=Sunday). Returns ok=false if the business is closed that day or
// BusinessHours is invalid. The day must come from ProfileDayKey (or an
// equivalent "1".."7" literal), never from a raw int(time.Weekday()).
func (bp *BusinessProfile) GetOpenClose(dayOfWeek int) (open, close string, ok bool) {
	hours, err := bp.parseBusinessHours()
	if err != nil {
		return "", "", false
	}
	day, exists := hours[dayOfWeek]
	if !exists {
		return "", "", false
	}
	return day.Open, day.Close, true
}

// parseBusinessHoursDayKey converts a business_hours JSON key to its integer day
// number (1=Monday, 7=Sunday). Digit-only keys of any length are accepted: legacy
// zero-padded keys ("01", "007") normalize for read compatibility with databases
// written before the strict validator (no migration), while the canonical emitted
// form remains "1".."7". Sign-prefixed keys ("+1"/"-1"), whitespace, and values
// outside 1..7 stay rejected; it is the single rule shared by the read/write paths.
func parseBusinessHoursDayKey(key string) (int, error) {
	day, err := strconv.Atoi(key)
	if err != nil || day < 1 || day > 7 || strings.ContainsFunc(key, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0, fmt.Errorf("clave de día %q inválida (1..7): %w", key, domain.ErrInvalidInput)
	}
	return day, nil
}

// parseBusinessHours parses the BusinessHours JSON string. Every key must be a
// day number in 1..7; any other key is rejected with domain.ErrInvalidInput
// instead of being silently mapped to a garbage day number.
func (bp *BusinessProfile) parseBusinessHours() (map[int]businessHoursDay, error) {
	if bp.BusinessHours == "" {
		return nil, nil
	}
	var raw map[string]businessHoursDay
	if err := json.Unmarshal([]byte(bp.BusinessHours), &raw); err != nil {
		return nil, err
	}
	result := make(map[int]businessHoursDay, len(raw))
	for k, v := range raw {
		day, err := parseBusinessHoursDayKey(k)
		if err != nil {
			return nil, err
		}
		result[day] = v
	}
	return result, nil
}

// Validate checks business-rule invariants for a business profile.
// Optional fields (MessengerPlatform, AcceptedPaymentMethods, BusinessHours, Timezone)
// are only validated when non-empty.
func (bp *BusinessProfile) Validate() error {
	// messenger_platform must be nil, "whatsapp", or "telegram".
	if bp.MessengerPlatform != nil {
		v := *bp.MessengerPlatform
		if v != "whatsapp" && v != "telegram" {
			return fmt.Errorf("la plataforma de mensajería debe ser \"whatsapp\" o \"telegram\", se recibió: %q: %w",
				v, domain.ErrInvalidInput)
		}
	}

	// accepted_payment_methods must be nil or a valid JSON array of non-empty strings.
	if bp.AcceptedPaymentMethods != nil {
		if err := bp.validatePaymentMethodsJSON(*bp.AcceptedPaymentMethods); err != nil {
			return fmt.Errorf("actualizar perfil del negocio: %w", err)
		}
	}

	// business_hours must be empty or valid JSON object.
	if err := bp.validateBusinessHoursJSON(); err != nil {
		return fmt.Errorf("actualizar perfil del negocio: %w", err)
	}

	// timezone must be empty or valid IANA zone.
	if err := bp.validateTimezone(); err != nil {
		return fmt.Errorf("actualizar perfil del negocio: %w", err)
	}

	return nil
}

// validateBusinessHoursJSON checks that BusinessHours is a valid JSON object
// (not null, array, or primitive) whose shape is usable by the scheduling
// code: every key is a day number in 1..7, every open/close is a zero-padded
// 24-hour HH:MM time, and open is strictly before close. Empty string is
// allowed (the field is optional). It performs a single unmarshal, so it is
// cheap enough to run on the profile read and write paths.
func (bp *BusinessProfile) validateBusinessHoursJSON() error {
	s := bp.BusinessHours
	if s == "" {
		return nil
	}
	if !json.Valid([]byte(s)) {
		return fmt.Errorf("el campo business_hours debe ser JSON válido: %w", domain.ErrInvalidInput)
	}
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("el campo business_hours debe ser un objeto JSON: %w", domain.ErrInvalidInput)
	}
	var hours map[string]businessHoursDay
	if err := json.Unmarshal([]byte(s), &hours); err != nil {
		return fmt.Errorf("el campo business_hours debe ser un objeto JSON: %w", domain.ErrInvalidInput)
	}
	for key, day := range hours {
		if _, err := parseBusinessHoursDayKey(key); err != nil {
			return err
		}
		if !ValidHHMM(day.Open) {
			return fmt.Errorf("el horario del día %s: la hora de apertura debe tener formato HH:MM (24h): %w",
				key, domain.ErrInvalidInput)
		}
		if !ValidHHMM(day.Close) {
			return fmt.Errorf("el horario del día %s: la hora de cierre debe tener formato HH:MM (24h): %w",
				key, domain.ErrInvalidInput)
		}
		if day.Open >= day.Close {
			return fmt.Errorf("el horario del día %s: la hora de apertura debe ser anterior al cierre: %w",
				key, domain.ErrInvalidInput)
		}
	}
	return nil
}

// validateTimezone checks that Timezone is a valid IANA timezone name.
// Empty string is allowed (defaults to UTC at DB level).
func (bp *BusinessProfile) validateTimezone() error {
	if bp.Timezone == "" {
		return nil
	}
	if _, err := time.LoadLocation(bp.Timezone); err != nil {
		return fmt.Errorf("la zona horaria %q no es válida: %w", bp.Timezone, domain.ErrInvalidInput)
	}
	return nil
}

// validatePaymentMethodsJSON checks that s is a valid JSON array of non-empty strings.
// Rejects JSON "null", primitives, and objects.
func (bp *BusinessProfile) validatePaymentMethodsJSON(s string) error {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return fmt.Errorf("los métodos de pago deben ser un array JSON válido: %w", domain.ErrInvalidInput)
	}
	var methods []string
	if err := json.Unmarshal([]byte(s), &methods); err != nil {
		return fmt.Errorf("los métodos de pago deben ser un array JSON válido: %w", domain.ErrInvalidInput)
	}
	for i, m := range methods {
		if m == "" {
			return fmt.Errorf("el método de pago en la posición %d está vacío: %w", i, domain.ErrInvalidInput)
		}
	}
	return nil
}
