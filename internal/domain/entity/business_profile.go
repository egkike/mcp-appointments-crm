package entity

import "encoding/json"

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
