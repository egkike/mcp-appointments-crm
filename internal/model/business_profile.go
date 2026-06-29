package model

// BusinessProfile is the singleton configuration row for the business.
// There is exactly one row with ID="singleton" (enforced by CHECK constraint).
type BusinessProfile struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Industry               *string  `json:"industry"`
	Country                *string  `json:"country"`
	Address                *string  `json:"address"`
	Latitude               *float64 `json:"latitude"`
	Longitude              *float64 `json:"longitude"`
	CoverPhotoURL          *string  `json:"cover_photo_url"`
	PublicPhone            *string  `json:"public_phone"`
	MessengerPlatform      *string  `json:"messenger_platform"`
	MessengerID            *string  `json:"messenger_id"`
	ContactEmail           *string  `json:"contact_email"`
	WebsiteURL             *string  `json:"website_url"`
	GeneralDescription     *string  `json:"general_description"`
	CurrencyCode           string   `json:"currency_code"`
	CurrencySymbol         string   `json:"currency_symbol"`
	AcceptedPaymentMethods *string  `json:"accepted_payment_methods"`
	Timezone               string   `json:"timezone"`
	SlotIntervalMinutes    int      `json:"slot_interval_minutes"`
	BusinessHours          string   `json:"business_hours"`
	CreatedAt              string   `json:"created_at"`
	UpdatedAt              string   `json:"updated_at"`
}
