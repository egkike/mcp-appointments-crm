package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// singletonID is the fixed primary-key value for the unique business_profile row.
const singletonID = "singleton"

// BusinessProfileRepo provides access to the singleton business_profile row.
// GetBusinessProfile uses lazy-init (INSERT OR IGNORE + SELECT) so that a
// fresh install never returns an empty result.
type BusinessProfileRepo struct {
	db *sql.DB
}

// NewBusinessProfileRepo creates a new BusinessProfileRepo.
func NewBusinessProfileRepo(db *sql.DB) *BusinessProfileRepo {
	return &BusinessProfileRepo{db: db}
}

// validateBusinessProfile checks business-rule invariants for a business
// profile before it reaches the database.
// Optional fields (BusinessHours, Timezone) are only validated when non-empty.
func validateBusinessProfile(p *model.BusinessProfile) error {
	// messenger_platform must be nil, "whatsapp", or "telegram".
	if p.MessengerPlatform != nil {
		v := *p.MessengerPlatform
		if v != "whatsapp" && v != "telegram" {
			return fmt.Errorf("actualizar perfil del negocio: la plataforma de mensajería debe ser \"whatsapp\" o \"telegram\", se recibió: %q: %w",
				v, ErrInvalidInput)
		}
	}

	// accepted_payment_methods must be nil or a valid JSON array of non-empty strings.
	// JSON "null" is explicitly rejected (must be an array or omitted).
	if p.AcceptedPaymentMethods != nil {
		if err := validateAcceptedPaymentMethodsJSON(*p.AcceptedPaymentMethods); err != nil {
			return fmt.Errorf("actualizar perfil del negocio: %w", err)
		}
	}

	// business_hours must be empty or valid JSON object (optional field).
	if err := validateBusinessHoursJSON(p.BusinessHours); err != nil {
		return fmt.Errorf("actualizar perfil del negocio: %w", err)
	}

	// timezone must be empty or valid IANA zone (optional field).
	if err := validateTimezone(p.Timezone); err != nil {
		return fmt.Errorf("actualizar perfil del negocio: %w", err)
	}

	return nil
}

// GetBusinessProfile returns the singleton business profile, creating a
// placeholder row on first call (lazy-init). Idempotent and safe under
// concurrent access: INSERT OR IGNORE ensures at most one row exists.
func (r *BusinessProfileRepo) GetBusinessProfile(ctx context.Context) (*model.BusinessProfile, error) {
	// Lazy-init: ensure the singleton row exists.
	// INSERT OR IGNORE silently no-ops when the row already exists (UNIQUE
	// conflict on id='singleton'). Any other error is surfaced.
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO business_profile (id, name) VALUES (?, ?)`,
		singletonID, "")
	if err != nil && !isUniqueViolation(err) {
		return nil, fmt.Errorf("obtener perfil del negocio: %w", err)
	}

	const query = `SELECT id, name, industry, country, address, latitude, longitude,
		cover_photo_url, public_phone, messenger_platform, messenger_id,
		contact_email, website_url, general_description,
		currency_code, currency_symbol, accepted_payment_methods,
		timezone, slot_interval_minutes, business_hours,
		created_at, updated_at
		FROM business_profile WHERE id = ?`

	p := &model.BusinessProfile{}
	err = r.db.QueryRowContext(ctx, query, singletonID).Scan(
		&p.ID, &p.Name, &p.Industry, &p.Country, &p.Address,
		&p.Latitude, &p.Longitude, &p.CoverPhotoURL, &p.PublicPhone,
		&p.MessengerPlatform, &p.MessengerID, &p.ContactEmail,
		&p.WebsiteURL, &p.GeneralDescription,
		&p.CurrencyCode, &p.CurrencySymbol, &p.AcceptedPaymentMethods,
		&p.Timezone, &p.SlotIntervalMinutes, &p.BusinessHours,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("obtener perfil del negocio: %w", err)
	}
	return p, nil
}

// UpdateBusinessProfile updates the singleton row. Returns ErrNotFound if
// no row matches (should not happen in practice due to lazy-init).
func (r *BusinessProfileRepo) UpdateBusinessProfile(ctx context.Context, p *model.BusinessProfile) error {
	if err := validateBusinessProfile(p); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE business_profile SET
			name=?, industry=?, country=?, address=?, latitude=?,
			longitude=?, cover_photo_url=?, public_phone=?,
			messenger_platform=?, messenger_id=?, contact_email=?,
			website_url=?, general_description=?, currency_code=?,
			currency_symbol=?, accepted_payment_methods=?, timezone=?,
			slot_interval_minutes=?, business_hours=?,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		p.Name, p.Industry, p.Country, p.Address, p.Latitude,
		p.Longitude, p.CoverPhotoURL, p.PublicPhone,
		p.MessengerPlatform, p.MessengerID, p.ContactEmail,
		p.WebsiteURL, p.GeneralDescription, p.CurrencyCode,
		p.CurrencySymbol, p.AcceptedPaymentMethods, p.Timezone,
		p.SlotIntervalMinutes, p.BusinessHours,
		singletonID,
	)
	if err != nil {
		return fmt.Errorf("actualizar perfil del negocio: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar perfil del negocio: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("actualizar perfil del negocio: %w", ErrNotFound)
	}
	return nil
}
