package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/model"
)

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

// GetBusinessProfile returns the singleton business profile, creating a
// placeholder row on first call (lazy-init). Idempotent and safe under
// concurrent access: INSERT OR IGNORE ensures at most one row exists.
func (r *BusinessProfileRepo) GetBusinessProfile(ctx context.Context) (*model.BusinessProfile, error) {
	// Lazy-init: ensure the singleton row exists.
	_, _ = r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO business_profile (id, name) VALUES ('singleton', '')`)

	const query = `SELECT id, name, industry, country, address, latitude, longitude,
		cover_photo_url, public_phone, messenger_platform, messenger_id,
		contact_email, website_url, general_description,
		currency_code, currency_symbol, accepted_payment_methods,
		timezone, slot_interval_minutes, business_hours,
		created_at, updated_at
		FROM business_profile WHERE id = 'singleton'`

	p := &model.BusinessProfile{}
	err := r.db.QueryRowContext(ctx, query).Scan(
		&p.ID, &p.Name, &p.Industry, &p.Country, &p.Address,
		&p.Latitude, &p.Longitude, &p.CoverPhotoURL, &p.PublicPhone,
		&p.MessengerPlatform, &p.MessengerID, &p.ContactEmail,
		&p.WebsiteURL, &p.GeneralDescription,
		&p.CurrencyCode, &p.CurrencySymbol, &p.AcceptedPaymentMethods,
		&p.Timezone, &p.SlotIntervalMinutes, &p.BusinessHours,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get business profile: %w", err)
	}
	return p, nil
}

// UpdateBusinessProfile updates the singleton row. Returns ErrNotFound if
// no row matches (should not happen in practice due to lazy-init).
func (r *BusinessProfileRepo) UpdateBusinessProfile(ctx context.Context, p *model.BusinessProfile) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE business_profile SET
			name=?, industry=?, country=?, address=?, latitude=?,
			longitude=?, cover_photo_url=?, public_phone=?,
			messenger_platform=?, messenger_id=?, contact_email=?,
			website_url=?, general_description=?, currency_code=?,
			currency_symbol=?, accepted_payment_methods=?, timezone=?,
			slot_interval_minutes=?, business_hours=?,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = 'singleton'`,
		p.Name, p.Industry, p.Country, p.Address, p.Latitude,
		p.Longitude, p.CoverPhotoURL, p.PublicPhone,
		p.MessengerPlatform, p.MessengerID, p.ContactEmail,
		p.WebsiteURL, p.GeneralDescription, p.CurrencyCode,
		p.CurrencySymbol, p.AcceptedPaymentMethods, p.Timezone,
		p.SlotIntervalMinutes, p.BusinessHours,
	)
	if err != nil {
		return fmt.Errorf("update business profile: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update business profile rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("update business profile: %w", ErrNotFound)
	}
	return nil
}

// isUniqueViolation checks whether err is a SQLite UNIQUE constraint error.
// Uses string matching for driver portability (works with both mattn/go-sqlite3
// and modernc.org/sqlite).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
