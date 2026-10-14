package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestBusinessProfileRepo_Get(t *testing.T) {
	t.Run("first call inserts singleton and returns row", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
			WithArgs("singleton", "").
			WillReturnResult(sqlmock.NewResult(1, 1))

		rows := sqlmock.NewRows([]string{
			"id", "name", "industry", "country", "address",
			"latitude", "longitude", "cover_photo_url", "public_phone",
			"messenger_platform", "messenger_id", "contact_email",
			"website_url", "general_description", "currency_code",
			"currency_symbol", "accepted_payment_methods", "timezone",
			"slot_interval_minutes", "business_hours", "created_at", "updated_at",
		}).AddRow(
			"singleton", "My Business", nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, "ARS",
			"$", nil, "UTC",
			30, "{}", "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z",
		)
		mock.ExpectQuery(`SELECT .+ FROM business_profile WHERE id = \?`).
			WithArgs("singleton").
			WillReturnRows(rows)

		profile, err := repo.Get(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile.ID != "singleton" {
			t.Errorf("got ID=%q, want %q", profile.ID, "singleton")
		}
		if profile.Name != "My Business" {
			t.Errorf("got Name=%q, want %q", profile.Name, "My Business")
		}
		if profile.CurrencyCode != "ARS" {
			t.Errorf("got CurrencyCode=%q, want %q", profile.CurrencyCode, "ARS")
		}
		if profile.Timezone != "UTC" {
			t.Errorf("got Timezone=%q, want %q", profile.Timezone, "UTC")
		}
		if profile.SlotIntervalMinutes != 30 {
			t.Errorf("got SlotIntervalMinutes=%d, want %d", profile.SlotIntervalMinutes, 30)
		}
	})

	t.Run("second call is idempotent", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
			WithArgs("singleton", "").
			WillReturnResult(sqlmock.NewResult(1, 0)) // row already exists
		rows := sqlmock.NewRows([]string{
			"id", "name", "industry", "country", "address",
			"latitude", "longitude", "cover_photo_url", "public_phone",
			"messenger_platform", "messenger_id", "contact_email",
			"website_url", "general_description", "currency_code",
			"currency_symbol", "accepted_payment_methods", "timezone",
			"slot_interval_minutes", "business_hours", "created_at", "updated_at",
		}).AddRow(
			"singleton", "Existing", nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, "ARS",
			"$", nil, "UTC",
			30, "{}", "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z",
		)
		mock.ExpectQuery(`SELECT .+ FROM business_profile WHERE id = \?`).
			WithArgs("singleton").
			WillReturnRows(rows)

		profile, err := repo.Get(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile.Name != "Existing" {
			t.Errorf("got Name=%q, want %q", profile.Name, "Existing")
		}
	})

	t.Run("non-UNIQUE INSERT error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
			WithArgs("singleton", "").
			WillReturnError(errors.New("disk I/O error"))

		_, err := repo.Get(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "disk I/O error") {
			t.Errorf("expected error to contain 'disk I/O error', got: %v", err)
		}
	})

	t.Run("UNIQUE INSERT error is swallowed and SELECT succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
			WithArgs("singleton", "").
			WillReturnError(errors.New("UNIQUE constraint failed: business_profile.id"))

		rows := sqlmock.NewRows([]string{
			"id", "name", "industry", "country", "address",
			"latitude", "longitude", "cover_photo_url", "public_phone",
			"messenger_platform", "messenger_id", "contact_email",
			"website_url", "general_description", "currency_code",
			"currency_symbol", "accepted_payment_methods", "timezone",
			"slot_interval_minutes", "business_hours", "created_at", "updated_at",
		}).AddRow(
			"singleton", "Existing", nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, "ARS",
			"$", nil, "UTC",
			30, "{}", "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z",
		)
		mock.ExpectQuery(`SELECT .+ FROM business_profile WHERE id = \?`).
			WithArgs("singleton").
			WillReturnRows(rows)

		profile, err := repo.Get(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile.Name != "Existing" {
			t.Errorf("got Name=%q, want %q", profile.Name, "Existing")
		}
	})

	t.Run("SELECT error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
			WithArgs("singleton", "").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectQuery(`SELECT .+ FROM business_profile WHERE id = \?`).
			WithArgs("singleton").
			WillReturnError(errors.New("connection lost"))

		_, err := repo.Get(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// profileReturningTestColumns mirrors businessProfileReturningColumns in scan
// order, for the sqlmock UPDATE ... RETURNING result sets below.
var profileReturningTestColumns = []string{
	"id", "name", "industry", "country", "address",
	"latitude", "longitude", "cover_photo_url", "public_phone",
	"messenger_platform", "messenger_id", "contact_email",
	"website_url", "general_description", "currency_code",
	"currency_symbol", "accepted_payment_methods", "timezone",
	"slot_interval_minutes", "business_hours", "created_at", "updated_at",
}

// storedProfileRow is the row UPDATE ... RETURNING yields on success, carrying
// a distinct server-side updated_at so a stale pre-update value is visible.
func storedProfileRow() *sqlmock.Rows {
	return sqlmock.NewRows(profileReturningTestColumns).AddRow(
		"singleton", "Stored", nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, "", "", nil, "", 0, "",
		"2026-01-01T00:00:00.000Z", "2026-01-02T00:00:00.000Z",
	)
}

// validProfile returns a profile whose mandatory identity scalars pass the
// entity.Validate() identity gate (name non-empty, ISO-4217 alpha-3 currency
// code, symbol, slot interval > 0 — the install.sh wizard baseline). Each
// Update subtest overrides the field its own rule exercises, so a failed
// assertion never comes from the identity gate.
func validProfile(name string) *entity.BusinessProfile {
	return &entity.BusinessProfile{
		ID: "singleton", Name: name,
		CurrencyCode: "ARS", CurrencySymbol: "$", SlotIntervalMinutes: 30,
	}
}

func TestBusinessProfileRepo_Update(t *testing.T) {
	t.Run("success returns the stored row", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WithArgs(
				"Updated", nil, nil, nil, nil, // name, industry, country, address, latitude
				nil, nil, nil, // longitude, cover_photo_url, public_phone
				nil, nil, nil, // messenger_platform, messenger_id, contact_email
				nil, nil, // website_url, general_description
				"ARS", "$", nil, // currency_code, currency_symbol, accepted_payment_methods
				"", 30, "", // timezone, slot_interval_minutes, business_hours
				"singleton", // WHERE id = ?
			).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Updated")
		stored, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stored == nil || stored.Name != "Stored" || stored.UpdatedAt != "2026-01-02T00:00:00.000Z" {
			t.Fatalf("stored = %+v; want the UPDATE ... RETURNING row with the server-side updated_at", stored)
		}
	})

	t.Run("vanished row returns domain.ErrRowGone", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WithArgs(
				"Ghost", nil, nil, nil, nil,
				nil, nil, nil,
				nil, nil, nil,
				nil, nil,
				"ARS", "$", nil,
				"", 30, "",
				"singleton",
			).
			WillReturnRows(sqlmock.NewRows(profileReturningTestColumns))

		profile := validProfile("Ghost")
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrRowGone) {
			t.Errorf("expected domain.ErrRowGone, got %v", err)
		}
		if errors.Is(err, domain.ErrNotFound) {
			t.Error("zero matched rows must be the concurrent-delete race (ErrRowGone), not ErrNotFound")
		}
	})

	t.Run("valid messenger_platform whatsapp succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		platform := "whatsapp"
		profile := validProfile("Test")
		profile.MessengerPlatform = &platform
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid messenger_platform telegram succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		platform := "telegram"
		profile := validProfile("Test")
		profile.MessengerPlatform = &platform
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid messenger_platform returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		platform := "facebook"
		profile := validProfile("Test")
		profile.MessengerPlatform = &platform
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for invalid platform, got %v", err)
		}
	})

	t.Run("valid accepted_payment_methods JSON array succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		methods := `["cash","credit_card"]`
		profile := validProfile("Test")
		profile.AcceptedPaymentMethods = &methods
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty JSON array accepted_payment_methods succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		methods := `[]`
		profile := validProfile("Test")
		profile.AcceptedPaymentMethods = &methods
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid JSON accepted_payment_methods returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		methods := `not-json`
		profile := validProfile("Test")
		profile.AcceptedPaymentMethods = &methods
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for invalid JSON, got %v", err)
		}
	})

	t.Run("accepted_payment_methods with empty string returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		methods := `["cash",""]`
		profile := validProfile("Test")
		profile.AcceptedPaymentMethods = &methods
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for empty string in array, got %v", err)
		}
	})

	t.Run("nil messenger_platform and nil accepted_payment_methods succeed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Test")
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("DB error on UPDATE propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnError(errors.New("disk full"))

		profile := validProfile("Test")
		_, err := repo.Update(adminCtx(), profile)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("valid business_hours JSON object succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Test")
		profile.BusinessHours = `{"1":{"open":"09:00","close":"18:00"}}`
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty business_hours is allowed (optional field)", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Test")
		profile.BusinessHours = ""
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid business_hours JSON returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := validProfile("Test")
		profile.BusinessHours = `{invalid`
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for malformed JSON, got %v", err)
		}
	})

	t.Run("business_hours as JSON array returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := validProfile("Test")
		profile.BusinessHours = `["not","an","object"]`
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for JSON array, got %v", err)
		}
	})

	t.Run("business_hours as JSON string returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := validProfile("Test")
		profile.BusinessHours = `"just a string"`
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for JSON string, got %v", err)
		}
	})

	t.Run("valid IANA timezone succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Test")
		profile.Timezone = "America/Argentina/Buenos_Aires"
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty timezone is allowed (optional field)", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Test")
		profile.Timezone = ""
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid timezone returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := validProfile("Test")
		profile.Timezone = "Not/A/Real/Zone"
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for invalid timezone, got %v", err)
		}
	})

	t.Run("accepted_payment_methods JSON null returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		methods := `null`
		profile := validProfile("Test")
		profile.AcceptedPaymentMethods = &methods
		_, err := repo.Update(adminCtx(), profile)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for JSON null, got %v", err)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := validProfile("Test")
		_, err := repo.Update(context.Background(), profile)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("client role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := validProfile("Test")
		_, err := repo.Update(clientCtx("c-1"), profile)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("staff role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := validProfile("Test")
		_, err := repo.Update(staffCtx("pro-1"), profile)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("admin role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Test")
		_, err := repo.Update(adminCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectQuery(`UPDATE business_profile SET`).
			WillReturnRows(storedProfileRow())

		profile := validProfile("Test")
		_, err := repo.Update(ownerCtx(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// Verify that auth.Caller is used (prevent unused import)
var _ = auth.RoleAdmin
