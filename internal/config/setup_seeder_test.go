package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/db"
)

// All seeder tests use a file-based temporary database. NEVER use ":memory:"
// because db.NewDatabase.verifyPragmas requires WAL mode, and SQLite returns
// "memory" for in-memory databases (ADR-SD-10).

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	database, err := db.NewDatabase(ctx, filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database.Conn
}

func TestSeed_HappyPath(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	if _, err := isFreshDB(ctx, conn); err != nil {
		t.Fatalf("isFreshDB failed: %v", err)
	}
	data := mustLoadTestdata(t)

	if err := seed(ctx, conn, data); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	// Singleton row exists, is exactly one, and carries the wizard name.
	var name, businessHours string
	var count int
	err := conn.QueryRowContext(ctx, "SELECT COUNT(*), name, business_hours FROM business_profile WHERE id = 'singleton' GROUP BY id").Scan(&count, &name, &businessHours)
	if err != nil {
		t.Fatalf("query business_profile: %v", err)
	}
	if count != 1 {
		t.Errorf("business_profile rows = %d, want 1", count)
	}
	if name != data.Business.Name {
		t.Errorf("business name = %q, want %q", name, data.Business.Name)
	}
	mappedHours, err := mapBusinessHours(data.Business.BusinessHours)
	if err != nil {
		t.Fatalf("mapBusinessHours: %v", err)
	}
	if businessHours != mappedHours {
		t.Errorf("business_hours = %q, want %q", businessHours, mappedHours)
	}

	// All 18 wizard fields round-tripped.
	assertBusinessProfileFields(t, ctx, conn, data.Business)

	// Professionals and schedules counts.
	var profCount, schedCount int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM professionals").Scan(&profCount); err != nil {
		t.Fatalf("count professionals: %v", err)
	}
	if profCount != len(data.Staff) {
		t.Errorf("professionals count = %d, want %d", profCount, len(data.Staff))
	}
	wantSchedules := 0
	for _, m := range data.Staff {
		wantSchedules += len(m.Schedule)
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schedules").Scan(&schedCount); err != nil {
		t.Fatalf("count schedules: %v", err)
	}
	if schedCount != wantSchedules {
		t.Errorf("schedules count = %d, want %d", schedCount, wantSchedules)
	}

	// Services count.
	var svcCount int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM services").Scan(&svcCount); err != nil {
		t.Fatalf("count services: %v", err)
	}
	if svcCount != len(data.Services) {
		t.Errorf("services count = %d, want %d", svcCount, len(data.Services))
	}

	// FTS trigger fired: seeded service is searchable.
	var ftsCount int
	if err := conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM services_fts WHERE services_fts MATCH ?", data.Services[0].Name).Scan(&ftsCount); err != nil {
		t.Fatalf("query services_fts: %v", err)
	}
	if ftsCount != 1 {
		t.Errorf("services_fts MATCH count = %d, want 1", ftsCount)
	}
}

func TestSeed_ZeroPriceService(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	if _, err := isFreshDB(ctx, conn); err != nil {
		t.Fatalf("isFreshDB failed: %v", err)
	}
	data := mustLoadTestdata(t)
	data.Services = append(data.Services, SetupService{
		Name:            "Consulta gratuita",
		Description:     strPtr("Primera evaluación sin cargo"),
		DurationMinutes: 30,
		Price:           0,
		IsActive:        1,
	})

	if err := seed(ctx, conn, data); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	var price float64
	if err := conn.QueryRowContext(ctx,
		"SELECT price FROM services WHERE name = ?", "Consulta gratuita").Scan(&price); err != nil {
		t.Fatalf("query zero-price service: %v", err)
	}
	if price != 0 {
		t.Errorf("price = %v, want 0", price)
	}
}

func TestSeed_RollbackOnDuplicateDay(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	if _, err := isFreshDB(ctx, conn); err != nil {
		t.Fatalf("isFreshDB failed: %v", err)
	}
	data := mustLoadTestdata(t)

	// Force a mid-transaction UNIQUE violation on schedules.
	data.Staff[0].Schedule = append(data.Staff[0].Schedule, SetupScheduleEntry{
		DayOfWeek: 1,
		StartTime: "10:00",
		EndTime:   "11:00",
	})

	err := seed(ctx, conn, data)
	if err == nil {
		t.Fatal("expected error for duplicate schedule day, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ya existe un horario para ese día") {
		t.Errorf("error %q does not contain expected Spanish constraint message", msg)
	}

	var name string
	if err := conn.QueryRowContext(ctx,
		"SELECT name FROM business_profile WHERE id = 'singleton'").Scan(&name); err != nil {
		t.Fatalf("query business_profile after rollback: %v", err)
	}
	if name != "" {
		t.Errorf("business_profile.name = %q, want empty after rollback", name)
	}

	var profCount, svcCount int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM professionals").Scan(&profCount); err != nil {
		t.Fatalf("count professionals after rollback: %v", err)
	}
	if profCount != 0 {
		t.Errorf("professionals count after rollback = %d, want 0", profCount)
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM services").Scan(&svcCount); err != nil {
		t.Fatalf("count services after rollback: %v", err)
	}
	if svcCount != 0 {
		t.Errorf("services count after rollback = %d, want 0", svcCount)
	}
}

func TestIsFreshDB(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)

	fresh, err := isFreshDB(ctx, conn)
	if err != nil {
		t.Fatalf("isFreshDB failed: %v", err)
	}
	if !fresh {
		t.Error("expected fresh=true on new DB")
	}

	// Seed once.
	data := mustLoadTestdata(t)
	if err := seed(ctx, conn, data); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	fresh, err = isFreshDB(ctx, conn)
	if err != nil {
		t.Fatalf("isFreshDB after seed failed: %v", err)
	}
	if fresh {
		t.Error("expected fresh=false after seed")
	}

	// Delete all professionals: guard must still report not-fresh.
	if _, err := conn.ExecContext(ctx, "DELETE FROM professionals"); err != nil {
		t.Fatalf("delete professionals: %v", err)
	}
	fresh, err = isFreshDB(ctx, conn)
	if err != nil {
		t.Fatalf("isFreshDB after deleting professionals failed: %v", err)
	}
	if fresh {
		t.Error("expected fresh=false after deleting professionals (guard uses business_profile.name only)")
	}
}

func assertBusinessProfileFields(t *testing.T, ctx context.Context, conn *sql.DB, want SetupBusiness) {
	t.Helper()

	var (
		name, currencyCode, currencySymbol, timezone string
		industry, country, address                   sql.NullString
		latitude, longitude                          sql.NullFloat64
		coverPhotoURL, publicPhone                   sql.NullString
		messengerPlatform, messengerID               sql.NullString
		contactEmail, websiteURL, generalDescription sql.NullString
		acceptedPaymentMethods                       sql.NullString
		slotIntervalMinutes                          int
		businessHours                                string
	)
	err := conn.QueryRowContext(ctx, `
		SELECT
			name, industry, country, address, latitude, longitude,
			cover_photo_url, public_phone, messenger_platform, messenger_id,
			contact_email, website_url, general_description, accepted_payment_methods,
			currency_code, currency_symbol, timezone, slot_interval_minutes,
			business_hours
		FROM business_profile WHERE id = 'singleton'`).Scan(
		&name, &industry, &country, &address, &latitude, &longitude,
		&coverPhotoURL, &publicPhone, &messengerPlatform, &messengerID,
		&contactEmail, &websiteURL, &generalDescription, &acceptedPaymentMethods,
		&currencyCode, &currencySymbol, &timezone, &slotIntervalMinutes,
		&businessHours,
	)
	if err != nil {
		t.Fatalf("scan business_profile: %v", err)
	}

	assertNullStringEqual(t, "industry", industry, want.Industry)
	assertNullStringEqual(t, "country", country, want.Country)
	assertNullStringEqual(t, "address", address, want.Address)
	assertNullFloat64Equal(t, "latitude", latitude, want.Latitude)
	assertNullFloat64Equal(t, "longitude", longitude, want.Longitude)
	assertNullStringEqual(t, "cover_photo_url", coverPhotoURL, want.CoverPhotoURL)
	assertNullStringEqual(t, "public_phone", publicPhone, want.PublicPhone)
	assertNullStringEqual(t, "messenger_platform", messengerPlatform, want.MessengerPlatform)
	assertNullStringEqual(t, "messenger_id", messengerID, want.MessengerID)
	assertNullStringEqual(t, "contact_email", contactEmail, want.ContactEmail)
	assertNullStringEqual(t, "website_url", websiteURL, want.WebsiteURL)
	assertNullStringEqual(t, "general_description", generalDescription, want.GeneralDescription)

	if name != want.Name {
		t.Errorf("name = %q, want %q", name, want.Name)
	}
	if currencyCode != want.CurrencyCode {
		t.Errorf("currency_code = %q, want %q", currencyCode, want.CurrencyCode)
	}
	if currencySymbol != want.CurrencySymbol {
		t.Errorf("currency_symbol = %q, want %q", currencySymbol, want.CurrencySymbol)
	}
	if timezone != want.Timezone {
		t.Errorf("timezone = %q, want %q", timezone, want.Timezone)
	}
	if slotIntervalMinutes != want.SlotIntervalMinutes {
		t.Errorf("slot_interval_minutes = %d, want %d", slotIntervalMinutes, want.SlotIntervalMinutes)
	}

	if acceptedPaymentMethods.Valid {
		var gotMethods []string
		if err := json.Unmarshal([]byte(acceptedPaymentMethods.String), &gotMethods); err != nil {
			t.Fatalf("unmarshal accepted_payment_methods: %v", err)
		}
		if len(gotMethods) != len(want.AcceptedPaymentMethods) {
			t.Errorf("accepted_payment_methods length = %d, want %d", len(gotMethods), len(want.AcceptedPaymentMethods))
		}
		for i, m := range want.AcceptedPaymentMethods {
			if gotMethods[i] != m {
				t.Errorf("accepted_payment_methods[%d] = %q, want %q", i, gotMethods[i], m)
			}
		}
	} else if want.AcceptedPaymentMethods != nil {
		t.Errorf("accepted_payment_methods = NULL, want non-nil")
	}

	mappedHours, err := mapBusinessHours(want.BusinessHours)
	if err != nil {
		t.Fatalf("mapBusinessHours: %v", err)
	}
	if businessHours != mappedHours {
		t.Errorf("business_hours = %q, want %q", businessHours, mappedHours)
	}
}

func assertNullStringEqual(t *testing.T, field string, got sql.NullString, want *string) {
	t.Helper()
	switch {
	case !got.Valid && want == nil:
		return
	case !got.Valid && want != nil:
		t.Errorf("%s = NULL, want %q", field, *want)
	case got.Valid && want == nil:
		t.Errorf("%s = %q, want NULL", field, got.String)
	case got.Valid && want != nil && got.String != *want:
		t.Errorf("%s = %q, want %q", field, got.String, *want)
	}
}

func assertNullFloat64Equal(t *testing.T, field string, got sql.NullFloat64, want *float64) {
	t.Helper()
	switch {
	case !got.Valid && want == nil:
		return
	case !got.Valid && want != nil:
		t.Errorf("%s = NULL, want %v", field, *want)
	case got.Valid && want == nil:
		t.Errorf("%s = %v, want NULL", field, got.Float64)
	case got.Valid && want != nil && got.Float64 != *want:
		t.Errorf("%s = %v, want %v", field, got.Float64, *want)
	}
}

func mustLoadTestdata(t *testing.T) *SetupData {
	t.Helper()
	data, err := LoadSetup("testdata")
	if err != nil {
		t.Fatalf("LoadSetup testdata: %v", err)
	}
	return data
}

func strPtr(s string) *string {
	return &s
}
