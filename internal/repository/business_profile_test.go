package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestBusinessProfileRepo_GetBusinessProfile(t *testing.T) {
	t.Run("first call inserts singleton and returns row", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
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
		mock.ExpectQuery(`SELECT .+ FROM business_profile WHERE id = .singleton.`).
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

		// First call
		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
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
		mock.ExpectQuery(`SELECT .+ FROM business_profile WHERE id = .singleton.`).
			WillReturnRows(rows)

		profile, err := repo.GetBusinessProfile(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile.Name != "Existing" {
			t.Errorf("got Name=%q, want %q", profile.Name, "Existing")
		}
	})

	t.Run("DB error on INSERT propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessProfileRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO business_profile`).
			WillReturnError(errors.New("disk I/O error"))

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
			WillReturnResult(sqlmock.NewResult(0, 0))

		profile := &model.BusinessProfile{ID: "singleton", Name: "Ghost"}
		err := repo.UpdateBusinessProfile(context.Background(), profile)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}
