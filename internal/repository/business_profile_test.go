package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestBusinessProfileRepo_GetBusinessProfile(t *testing.T) {
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

		profile, err := repo.GetBusinessProfile(context.Background())
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

		profile, err := repo.GetBusinessProfile(context.Background())
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

		_, err := repo.GetBusinessProfile(context.Background())
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

		profile, err := repo.GetBusinessProfile(context.Background())
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

		_, err := repo.GetBusinessProfile(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBusinessProfileRepo_UpdateBusinessProfile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WithArgs(
				"Updated", nil, nil, nil, nil, // name, industry, country, address, latitude
				nil, nil, nil, // longitude, cover_photo_url, public_phone
				nil, nil, nil, // messenger_platform, messenger_id, contact_email
				nil, nil, // website_url, general_description
				"", "", nil, // currency_code, currency_symbol, accepted_payment_methods
				"", 0, "", // timezone, slot_interval_minutes, business_hours
				"singleton", // WHERE id = ?
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		profile := &model.BusinessProfile{ID: "singleton", Name: "Updated"}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WithArgs(
				"Ghost", nil, nil, nil, nil,
				nil, nil, nil,
				nil, nil, nil,
				nil, nil,
				"", "", nil,
				"", 0, "",
				"singleton",
			).
			WillReturnResult(sqlmock.NewResult(0, 0))

		profile := &model.BusinessProfile{ID: "singleton", Name: "Ghost"}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("valid messenger_platform whatsapp succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		platform := "whatsapp"
		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", MessengerPlatform: &platform}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid messenger_platform telegram succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		platform := "telegram"
		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", MessengerPlatform: &platform}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid messenger_platform returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		platform := "facebook"
		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", MessengerPlatform: &platform}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for invalid platform, got %v", err)
		}
	})

	t.Run("valid accepted_payment_methods JSON array succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		methods := `["cash","credit_card"]`
		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", AcceptedPaymentMethods: &methods}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty JSON array accepted_payment_methods succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		methods := `[]`
		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", AcceptedPaymentMethods: &methods}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid JSON accepted_payment_methods returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		methods := `not-json`
		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", AcceptedPaymentMethods: &methods}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for invalid JSON, got %v", err)
		}
	})

	t.Run("accepted_payment_methods with empty string returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		methods := `["cash",""]`
		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", AcceptedPaymentMethods: &methods}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for empty string in array, got %v", err)
		}
	})

	t.Run("nil messenger_platform and nil accepted_payment_methods succeed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		profile := &model.BusinessProfile{ID: "singleton", Name: "Test"}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("DB error on UPDATE propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnError(errors.New("disk full"))

		profile := &model.BusinessProfile{ID: "singleton", Name: "Test"}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("valid business_hours JSON object succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		profile := &model.BusinessProfile{
			ID:            "singleton",
			Name:          "Test",
			BusinessHours: `{"mon":{"open":"09:00","close":"18:00"}}`,
		}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty business_hours is allowed (optional field)", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", BusinessHours: ""}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid business_hours JSON returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := &model.BusinessProfile{
			ID:            "singleton",
			Name:          "Test",
			BusinessHours: `{invalid`,
		}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for malformed JSON, got %v", err)
		}
	})

	t.Run("business_hours as JSON array returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := &model.BusinessProfile{
			ID:            "singleton",
			Name:          "Test",
			BusinessHours: `["not","an","object"]`,
		}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for JSON array, got %v", err)
		}
	})

	t.Run("business_hours as JSON string returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := &model.BusinessProfile{
			ID:            "singleton",
			Name:          "Test",
			BusinessHours: `"just a string"`,
		}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for JSON string, got %v", err)
		}
	})

	t.Run("valid IANA timezone succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		profile := &model.BusinessProfile{
			ID:       "singleton",
			Name:     "Test",
			Timezone: "America/Argentina/Buenos_Aires",
		}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty timezone is allowed (optional field)", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`UPDATE business_profile SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		profile := &model.BusinessProfile{ID: "singleton", Name: "Test", Timezone: ""}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid timezone returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		profile := &model.BusinessProfile{
			ID:       "singleton",
			Name:     "Test",
			Timezone: "Not/A/Real/Zone",
		}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for invalid timezone, got %v", err)
		}
	})

	t.Run("accepted_payment_methods JSON null returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		methods := `null`
		profile := &model.BusinessProfile{
			ID:                     "singleton",
			Name:                   "Test",
			AcceptedPaymentMethods: &methods,
		}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for JSON null, got %v", err)
		}
	})
}
