package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.BusinessProfileRepo = (*BusinessProfileRepo)(nil)

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

// Get returns the singleton business profile, creating a
// placeholder row on first call (lazy-init). Idempotent and safe under
// concurrent access: INSERT OR IGNORE ensures at most one row exists.
func (r *BusinessProfileRepo) Get(ctx context.Context) (*entity.BusinessProfile, error) {
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

	p := &entity.BusinessProfile{}
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

// businessProfileReturningColumns is the read-back projection of an UPDATE, in
// the same scan order as Get. RETURNING makes the write and the read a single
// atomic statement, so the returned row carries the server-side updated_at and
// cannot observe another writer.
const businessProfileReturningColumns = ` RETURNING id, name, industry, country, address, latitude, longitude,
		cover_photo_url, public_phone, messenger_platform, messenger_id,
		contact_email, website_url, general_description,
		currency_code, currency_symbol, accepted_payment_methods,
		timezone, slot_interval_minutes, business_hours,
		created_at, updated_at`

// scanStoredBusinessProfile scans the row produced by
// businessProfileReturningColumns.
func scanStoredBusinessProfile(row *sql.Row) (*entity.BusinessProfile, error) {
	p := &entity.BusinessProfile{}
	if err := row.Scan(
		&p.ID, &p.Name, &p.Industry, &p.Country, &p.Address,
		&p.Latitude, &p.Longitude, &p.CoverPhotoURL, &p.PublicPhone,
		&p.MessengerPlatform, &p.MessengerID, &p.ContactEmail,
		&p.WebsiteURL, &p.GeneralDescription,
		&p.CurrencyCode, &p.CurrencySymbol, &p.AcceptedPaymentMethods,
		&p.Timezone, &p.SlotIntervalMinutes, &p.BusinessHours,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return p, nil
}

// Update replaces the singleton row and returns it as written by this call
// (single atomic UPDATE ... RETURNING), so the response carries the server-set
// updated_at instead of a stale pre-update value. Execute always Gets first,
// which lazily inserts the singleton row, so a zero-row match can only be the
// singleton row vanishing between that read and this write: that race returns
// domain.ErrRowGone (not ErrNotFound), following the concurrent-delete
// precedent. Requires admin or owner role.
func (r *BusinessProfileRepo) Update(ctx context.Context, p *entity.BusinessProfile) (*entity.BusinessProfile, error) {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return nil, fmt.Errorf("actualizar perfil del negocio: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("actualizar perfil de negocio: %w", err)
	}
	stored, err := scanStoredBusinessProfile(r.db.QueryRowContext(ctx,
		`UPDATE business_profile SET
			name=?, industry=?, country=?, address=?, latitude=?,
			longitude=?, cover_photo_url=?, public_phone=?,
			messenger_platform=?, messenger_id=?, contact_email=?,
			website_url=?, general_description=?, currency_code=?,
			currency_symbol=?, accepted_payment_methods=?, timezone=?,
			slot_interval_minutes=?, business_hours=?,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`+businessProfileReturningColumns,
		p.Name, p.Industry, p.Country, p.Address, p.Latitude,
		p.Longitude, p.CoverPhotoURL, p.PublicPhone,
		p.MessengerPlatform, p.MessengerID, p.ContactEmail,
		p.WebsiteURL, p.GeneralDescription, p.CurrencyCode,
		p.CurrencySymbol, p.AcceptedPaymentMethods, p.Timezone,
		p.SlotIntervalMinutes, p.BusinessHours,
		singletonID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("actualizar perfil del negocio: la fila desapareció durante la operación: %w", domain.ErrRowGone)
		}
		return nil, fmt.Errorf("actualizar perfil del negocio: %w", err)
	}
	return stored, nil
}
