package entity

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// businessHoursHHMMRegex matches a zero-padded 24-hour HH:MM time (00:00..23:59).
var businessHoursHHMMRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// businessProfileCurrencyCodeRegex matches the mandatory ISO 4217 alpha-3
// shape (v_currency in the install.sh wizard): exactly three uppercase letters.
var businessProfileCurrencyCodeRegex = regexp.MustCompile(`^[A-Z]{3}$`)

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

// semanticInvalid builds the canonical client-shaped invalid-input error:
// every BusinessProfile validation failure is a *domain.SemanticError so its
// detailed Spanish message reaches the LLM verbatim (the update use case
// passes SemanticError through and only collapses bare sentinel errors). This
// keeps the "which failures are client-shaped" classification inside the
// domain instead of leaking it to the callers.
func semanticInvalid(message string) *domain.SemanticError {
	return &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: message,
		Cause:   domain.ErrInvalidInput,
	}
}

// businessHoursDay represents the schedule for a single day.
//
// Deliberate wire-tag exception (F-4 decision D-F): entities are wire-free,
// but this unexported type parses the persisted business_hours TEXT column —
// these are persistence parse tags, not a wire payload contract.
type businessHoursDay struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// ProfileDayKey translates a time.Weekday into the day key used by
// BusinessProfile.BusinessHours and by the profile day lookups (GetOpenClose).
// It is the single translation point between the two day
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

// GetOpenClose returns the open and close times (HH:MM) for the given day of
// week (1=Monday, 7=Sunday). Contract:
//
//   - open day:      open/close set, ok=true, err=nil
//   - closed day:    ok=false, err=nil (day key absent, or BusinessHours empty)
//   - corrupt value: ok=false, err != nil (BusinessHours is not readable)
//
// Callers that book or report availability MUST treat err != nil as a
// fail-closed internal error; only err == nil && !ok means "legitimately
// closed". The day must come from ProfileDayKey (or an equivalent "1".."7"
// literal), never from a raw int(time.Weekday()).
func (bp *BusinessProfile) GetOpenClose(dayOfWeek int) (open, close string, ok bool, err error) {
	hours, err := bp.parseBusinessHours()
	if err != nil {
		return "", "", false, err
	}
	day, exists := hours[dayOfWeek]
	if !exists {
		return "", "", false, nil
	}
	return day.Open, day.Close, true, nil
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
	var raw map[string]*businessHoursDay
	if err := json.Unmarshal([]byte(bp.BusinessHours), &raw); err != nil {
		// Wrap so the server-side log carries which column broke (this is a
		// read-path, non-client-shaped error: the sentinel wrapping convention
		// stays).
		return nil, fmt.Errorf("la configuración de business_hours contiene JSON inválido: %w", err)
	}
	// Fail secure against corrupt stored data (GGA round 8): JSON "null"
	// unmarshals successfully into a nil map, which indistinguishably looks
	// like "no configured days" — stale or half-migrated schedules must not
	// read worldwide as a closed business. Only an explicit {} means none.
	// Fail secure against corrupt stored data (GGA round 13): a null DAY
	// ("{"1":null}") decodes into a zero struct without error and would come
	// back as an open day with empty times. Same treatment as the top-level
	// null below. Corrupt HH:MM values get the write-path rules here too.
	if raw == nil {
		return nil, fmt.Errorf("la configuración de business_hours no puede ser null: %w", domain.ErrInvalidInput)
	}
	result := make(map[int]businessHoursDay, len(raw))
	// Deterministic first-error: same rationale as validateBusinessHoursJSON.
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		day, err := parseBusinessHoursDayKey(k)
		if err != nil {
			return nil, err
		}
		dayPtr := raw[k]
		// Per-day fail-closed (GGA round 13): a null day decodes into a zero
		// struct — it would masquerade as an open day with empty times and end
		// up read as "closed business". HH:MM conformance stays with the compare
		// stage (hhmmToMinutes in the booking validator), which converts malformed
		// stored values into a fail-closed internal error; the write path checks
		// the shapes anyway.
		if dayPtr == nil {
			return nil, fmt.Errorf("el horario del día %s no puede ser null: %w", k, domain.ErrInvalidInput)
		}
		result[day] = *dayPtr
	}
	return result, nil
}

// MaxBusinessNameLen bounds the business profile name: the single source
// of truth (Validate) that admin.RenderSkillTemplate reuses when re-rendering
// the receptionist skill, so the write paths and the regeneration share one
// limit.
const MaxBusinessNameLen = 120

// businessNameUnsafeChars are the characters that could alter the SKILL.md
// the Hermes agent loads: markdown-active metacharacters (emphasis, heading,
// code, links, html) plus brace/dollar delimiters that could re-enter the
// template placeholder machinery. `~|# findings`: names legitimately hold
// none of them.
const businessNameUnsafeChars = "*_`#~<>|$\\[[]{}"

// BusinessNameHasUnsafeChars reports whether name carries a character the
// skill renderer and this entity both reject (single source of truth for the
// render-safe charset; GGA round 8: blocking line breaks alone still allowed
// inline markdown/instruction injection).
func BusinessNameHasUnsafeChars(name string) bool {
	return strings.ContainsAny(name, businessNameUnsafeChars)
}

// Validate checks business-rule invariants for a business profile.
// The mandatory identity scalars (name, currency code, currency symbol, slot
// interval) must always be present: the repo lazy-init creates an empty
// placeholder singleton, so only the write paths can gate them — fail closed,
// every write path (MCP, TUI, seeder) is covered. Their rules mirror the
// install.sh wizard validators (v_nonempty / v_currency / v_symbol /
// v_positive_int).
// Optional fields (MessengerPlatform, AcceptedPaymentMethods, BusinessHours, Timezone)
// are only validated when non-empty.
func (bp *BusinessProfile) Validate() error {
	// Mandatory identity scalars first (deterministic first-error order): a
	// profile without its identity columns can never be written.
	if strings.TrimSpace(bp.Name) == "" {
		return semanticInvalid("el nombre del negocio no puede estar vacío.")
	}
	// The name is re-rendered into the receptionist skill (setup/templates), so
	// it is bounded and must stay single-line (GGA review finding: a stored
	// multi-line name would inject markdown into the SKILL.md the Hermes agent
	// loads; 120 runes mirrors admin.RenderSkillTemplate's boundary).
	if utf8.RuneCountInString(strings.TrimSpace(bp.Name)) > MaxBusinessNameLen {
		return semanticInvalid("el nombre del negocio no puede superar 120 caracteres.")
	}
	if strings.ContainsAny(bp.Name, "\r\n\t") {
		return semanticInvalid("el nombre del negocio no puede contener saltos de línea ni tabulaciones.")
	}
	if BusinessNameHasUnsafeChars(bp.Name) {
		return semanticInvalid("el nombre del negocio no puede contener caracteres especiales de marcado.")
	}
	if !businessProfileCurrencyCodeRegex.MatchString(bp.CurrencyCode) {
		return semanticInvalid("el código de moneda debe tener exactamente 3 letras mayúsculas (ISO 4217).")
	}
	if strings.TrimSpace(bp.CurrencySymbol) == "" {
		return semanticInvalid("el símbolo de la moneda no puede estar vacío.")
	}
	if bp.SlotIntervalMinutes <= 0 {
		return semanticInvalid("el intervalo de turnos debe ser un entero positivo (en minutos).")
	}

	// latitude/longitude must be nil or inside their WGS-84 bounds. The transport
	// `geo:` parser rejects out-of-range decimals, but a raw numeric
	// {"latitude": 200} bypasses it in favor of the derived maps_url; the domain
	// is the authoritative gate so every write path (MCP, TUI, seeder) is covered.
	if bp.Latitude != nil && (math.IsNaN(*bp.Latitude) || math.IsInf(*bp.Latitude, 0) || *bp.Latitude < -90 || *bp.Latitude > 90) {
		return semanticInvalid("la latitud debe estar entre -90 y 90")
	}
	if bp.Longitude != nil && (math.IsNaN(*bp.Longitude) || math.IsInf(*bp.Longitude, 0) || *bp.Longitude < -180 || *bp.Longitude > 180) {
		return semanticInvalid("la longitud debe estar entre -180 y 180")
	}

	// messenger_platform must be nil, "whatsapp", or "telegram".
	if bp.MessengerPlatform != nil {
		v := *bp.MessengerPlatform
		if v != "whatsapp" && v != "telegram" {
			return semanticInvalid(fmt.Sprintf("la plataforma de mensajería debe ser \"whatsapp\" o \"telegram\", se recibió: %q.", v))
		}
	}

	// accepted_payment_methods must be nil or a valid JSON array of non-empty strings.
	if bp.AcceptedPaymentMethods != nil {
		if err := bp.validatePaymentMethodsJSON(*bp.AcceptedPaymentMethods); err != nil {
			return err // already client-shaped (*domain.SemanticError)
		}
	}

	// business_hours must be empty or valid JSON object.
	if err := bp.validateBusinessHoursJSON(); err != nil {
		return err // already client-shaped (*domain.SemanticError)
	}

	// timezone must be empty or valid IANA zone.
	if err := bp.validateTimezone(); err != nil {
		return err // already client-shaped (*domain.SemanticError)
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
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return semanticInvalid("el campo business_hours debe ser un objeto JSON.")
	}
	var hours map[string]businessHoursDay
	if err := json.Unmarshal([]byte(s), &hours); err != nil {
		// Reached only when the payload starts with '{' but is broken JSON: the
		// leading-'{' check above already rejected valid non-object JSON, so this
		// is exactly the shape the former json.Valid pre-check caught — same
		// message, one parse instead of two.
		return semanticInvalid("el campo business_hours debe ser JSON válido.")
	}
	// Deterministic first-error: Go map iteration is randomized, so multiple
	// invalid days would otherwise make the reported day vary run to run.
	keys := make([]string, 0, len(hours))
	for key := range hours {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		// Write-only strictness (GGA r2 #3): the READ path normalizes legacy
		// zero-padded keys ("01", "007") for databases written before the strict
		// validator, but a write gate has no legacy DB to be compatible with —
		// day keys must be canonical ("1".."7"), or "1" and "01" would both
		// validate and silently resolve to the same day.
		day, keyErr := strconv.Atoi(key)
		if keyErr != nil || strconv.Itoa(day) != key || day < 1 || day > 7 {
			return semanticInvalid(fmt.Sprintf("la clave de día %q es inválida: debe ser un número exacto del 1 al 7.", key))
		}
		if !ValidHHMM(hours[key].Open) {
			return semanticInvalid(fmt.Sprintf("el horario del día %s: la hora de apertura debe tener formato HH:MM (24h).", key))
		}
		if !ValidHHMM(hours[key].Close) {
			return semanticInvalid(fmt.Sprintf("el horario del día %s: la hora de cierre debe tener formato HH:MM (24h).", key))
		}
		if hours[key].Open >= hours[key].Close {
			return semanticInvalid(fmt.Sprintf("el horario del día %s: la hora de apertura debe ser anterior al cierre.", key))
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
		return semanticInvalid(fmt.Sprintf("la zona horaria %q no es válida (debe ser un nombre IANA, ej. America/Argentina/Buenos_Aires).", bp.Timezone))
	}
	return nil
}

// validatePaymentMethodsJSON checks that s is a valid JSON array of non-empty strings.
// Rejects JSON "null", primitives, and objects.
func (bp *BusinessProfile) validatePaymentMethodsJSON(s string) error {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return semanticInvalid("los métodos de pago deben ser un array JSON válido.")
	}
	var methods []string
	if err := json.Unmarshal([]byte(s), &methods); err != nil {
		return semanticInvalid("los métodos de pago deben ser un array JSON válido.")
	}
	for i, m := range methods {
		if m == "" {
			return semanticInvalid(fmt.Sprintf("el método de pago en la posición %d está vacío.", i))
		}
	}
	return nil
}
