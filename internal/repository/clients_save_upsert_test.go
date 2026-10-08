package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// strPtr returns a pointer to s (entity.Client uses *string for optional fields).
func saveTestStrPtr(s string) *string { return &s }

// TestClientsRepo_Save_UpsertPreservesRow pins the gate finding: Save must
// UPSERT in place (ON CONFLICT(id) DO UPDATE). INSERT OR REPLACE deletes the
// conflicting row, which with foreign_keys=ON cascades bookings.client_id
// ON DELETE CASCADE and silently wipes every booking of an existing client.
func TestClientsRepo_Save_UpsertPreservesRow(t *testing.T) {
	ctx := context.Background()
	ownerCtx := auth.WithCaller(ctx, auth.Caller{ID: "owner-1", Role: auth.RoleOwner})

	// seedFixture inserts the FK chain service → professional → booking so the
	// CASCADE danger is exercised for real against the production pragmas.
	seedFixture := func(t *testing.T, conn *sql.DB) {
		t.Helper()
		stmts := []string{
			`INSERT INTO clients (id, name, phone, created_at) VALUES ('client-1','Viejo','+5491111111111','2026-01-01T10:00:00.000Z')`,
			`INSERT INTO clients (id, name, phone) VALUES ('client-2','Otro','+5492222222222')`,
			`INSERT INTO services (id, name, duration_minutes, price) VALUES ('svc-1','Corte',30,100)`,
			`INSERT INTO professionals (id, name) VALUES ('pro-1','Ana')`,
			`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime)
			 VALUES ('b-1','client-1','pro-1','svc-1','2026-02-01T10:00:00Z','2026-02-01T11:00:00Z')`,
		}
		for _, q := range stmts {
			if _, err := conn.ExecContext(ctx, q); err != nil {
				t.Fatalf("seed %q: %v", q, err)
			}
		}
	}

	t.Run("save on existing client preserves bookings and created_at", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		repo := NewClientsRepo(conn)
		seedFixture(t, conn)

		var bookingsBefore int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookings WHERE client_id = 'client-1'`).Scan(&bookingsBefore); err != nil {
			t.Fatalf("count bookings: %v", err)
		}
		if bookingsBefore != 1 {
			t.Fatalf("booking seed count = %d, want 1", bookingsBefore)
		}

		err := repo.Save(ownerCtx, &entity.Client{ID: "client-1", Name: "Nuevo", Phone: "+5491111111111", Email: saveTestStrPtr("nuevo@mail.test")})
		if err != nil {
			t.Fatalf("save existing client: %v", err)
		}

		var bookingsAfter int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookings WHERE client_id = 'client-1'`).Scan(&bookingsAfter); err != nil {
			t.Fatalf("count bookings after save: %v", err)
		}
		if bookingsAfter != 1 {
			t.Errorf("bookings after save = %d, want 1 (CASCADE must never fire on Save)", bookingsAfter)
		}
		var name, email, createdAtAfter string
		if err := conn.QueryRowContext(ctx, `SELECT name, email, created_at FROM clients WHERE id = 'client-1'`).Scan(&name, &email, &createdAtAfter); err != nil {
			t.Fatalf("read saved row: %v", err)
		}
		if name != "Nuevo" || email != "nuevo@mail.test" {
			t.Errorf("saved (name, email) = (%q, %q), want (Nuevo, nuevo@mail.test)", name, email)
		}
		if createdAtAfter != "2026-01-01T10:00:00.000Z" {
			t.Errorf("created_at = %q, want preserved 2026-01-01T10:00:00.000Z (REPLACE would reset it)", createdAtAfter)
		}
	})

	t.Run("save with a phone used by another client conflicts", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		repo := NewClientsRepo(conn)
		seedFixture(t, conn)

		err := repo.Save(ownerCtx, &entity.Client{ID: "client-1", Name: "Nuevo", Phone: "+5492222222222"})
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("save with foreign phone: err = %v, want ErrConflict", err)
		}
	})

	t.Run("save new client inserts", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		repo := NewClientsRepo(conn)

		err := repo.Save(ownerCtx, &entity.Client{ID: "client-9", Name: "Nuevo", Phone: "+5493999999999"})
		if err != nil {
			t.Fatalf("save new client: %v", err)
		}
		if n := countClientRowsByID(t, conn, "client-9"); n != 1 {
			t.Errorf("row count = %d, want 1", n)
		}
	})
}
