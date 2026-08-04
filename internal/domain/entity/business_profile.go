package entity

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

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

// IsOpenOn reports whether the business is open on the given day of week
// (1=Monday, 7=Sunday). Returns false if BusinessHours is empty or invalid JSON.
func (bp *BusinessProfile) IsOpenOn(dayOfWeek int) bool {
	hours, err := bp.parseBusinessHours()
	if err != nil {
		return false
	}
	_, exists := hours[dayOfWeek]
	return exists
}

// GetOpenClose returns the open and close times (HH:MM) for the given day of week.
// Returns ok=false if the business is closed that day or BusinessHours is invalid.
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

// parseBusinessHours parses the BusinessHours JSON string.
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
		var day int
		for _, c := range k {
			day = day*10 + int(c-'0')
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
// (not null, array, or primitive). Empty string is allowed.
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
