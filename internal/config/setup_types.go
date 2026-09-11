package config

// BusinessHoursEntry is one open day: {"open":"09:00","close":"18:00"}.
// A nil *BusinessHoursEntry in the map means the day is closed (JSON null).
type BusinessHoursEntry struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// SetupBusiness pins setup_business.json: 18 wizard fields + nested business_hours
// keyed by day name (monday..sunday), each entry {open,close} or null.
type SetupBusiness struct {
	Name                   string                         `json:"name"`
	Industry               *string                        `json:"industry"`
	Country                *string                        `json:"country"`
	Address                *string                        `json:"address"`
	Latitude               *float64                       `json:"latitude"`
	Longitude              *float64                       `json:"longitude"`
	CoverPhotoURL          *string                        `json:"cover_photo_url"`
	PublicPhone            *string                        `json:"public_phone"`
	MessengerPlatform      *string                        `json:"messenger_platform"`
	MessengerID            *string                        `json:"messenger_id"`
	ContactEmail           *string                        `json:"contact_email"`
	WebsiteURL             *string                        `json:"website_url"`
	GeneralDescription     *string                        `json:"general_description"`
	AcceptedPaymentMethods []string                       `json:"accepted_payment_methods"` // nil ⇔ JSON null/absent
	CurrencyCode           string                         `json:"currency_code"`
	CurrencySymbol         string                         `json:"currency_symbol"`
	Timezone               string                         `json:"timezone"`
	SlotIntervalMinutes    int                            `json:"slot_interval_minutes"`
	BusinessHours          map[string]*BusinessHoursEntry `json:"business_hours"` // day-name keys
}

// SetupScheduleEntry pins one staff schedule row. day_of_week 0–6, 0=Sunday.
// Passed through 1:1 — NO transformation (D7; differs from business_hours's 1–7).
type SetupScheduleEntry struct {
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// SetupStaffMember pins one setup_staff.json array element.
type SetupStaffMember struct {
	Name          string               `json:"name"`
	RoleSpecialty *string              `json:"role_specialty"`
	Status        string               `json:"status"` // wizard emits "active"
	Email         *string              `json:"email"`
	Phone         *string              `json:"phone"`
	Specialties   []string             `json:"specialties"` // wizard always emits []
	Schedule      []SetupScheduleEntry `json:"schedule"`
}

// SetupService pins one setup_services.json array element.
// IsActive is int because the wizard emits JSON 1/0 (ADR-SD-3).
type SetupService struct {
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	DurationMinutes int     `json:"duration_minutes"`
	Price           float64 `json:"price"`
	IsActive        int     `json:"is_active"` // 0/1, validated
}

// SetupData bundles the three decoded files.
type SetupData struct {
	Business SetupBusiness
	Staff    []SetupStaffMember
	Services []SetupService
}
