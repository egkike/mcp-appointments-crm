package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/db"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// newAggregateTestDB creates a real SQLite database for AggregateByClient
// integration tests. The returned cleanup function closes the database.
func newAggregateTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.NewDatabase(context.Background(), filepath.Join(dir, "aggregate.db"))
	if err != nil {
		t.Fatalf("create aggregate test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database.Conn, func() { _ = database.Close() }
}

// execSeed executes raw INSERT statements against the test database.
func execSeed(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
}

// insertBooking inserts a booking row directly, bypassing overlap checks.
func insertBooking(t *testing.T, db *sql.DB, id, clientID, professionalID, serviceID, start, end, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, clientID, professionalID, serviceID, start, end, status,
	)
	if err != nil {
		t.Fatalf("insert booking %s: %v", id, err)
	}
}

func TestBookingsRepo_AggregateByClient(t *testing.T) {
	ctx := adminCtx()

	t.Run("counts only non-cancelled bookings", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		execSeed(t, db,
			`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Pro', 'active')`,
			`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Svc', 30, 1.0, 1)`,
			`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Ana', '+1')`,
		)
		base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		insertBooking(t, db, "b1", "c1", "p1", "s1", FormatStorage(base.Add(time.Hour)), FormatStorage(base.Add(2*time.Hour)), "confirmed")
		insertBooking(t, db, "b2", "c1", "p1", "s1", FormatStorage(base.Add(3*time.Hour)), FormatStorage(base.Add(4*time.Hour)), "confirmed")
		insertBooking(t, db, "b3", "c1", "p1", "s1", FormatStorage(base.Add(5*time.Hour)), FormatStorage(base.Add(6*time.Hour)), "cancelled")

		repo := NewBookingsRepo(db)
		got, err := repo.AggregateByClient(ctx, base, base.Add(24*time.Hour), 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d rows, want 1", len(got))
		}
		if got[0].BookingCount != 2 {
			t.Errorf("booking_count = %d, want 2", got[0].BookingCount)
		}
	})

	t.Run("window is inclusive start exclusive end", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		execSeed(t, db,
			`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Pro', 'active')`,
			`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Svc', 30, 1.0, 1)`,
			`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Ana', '+1')`,
		)
		start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
		end := time.Date(2026, 7, 1, 14, 0, 0, 0, time.UTC)
		insertBooking(t, db, "b1", "c1", "p1", "s1", FormatStorage(start), FormatStorage(start.Add(time.Hour)), "confirmed")
		insertBooking(t, db, "b2", "c1", "p1", "s1", FormatStorage(end), FormatStorage(end.Add(time.Hour)), "confirmed")

		repo := NewBookingsRepo(db)
		got, err := repo.AggregateByClient(ctx, start, end, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d rows, want 1", len(got))
		}
		if got[0].BookingCount != 1 {
			t.Errorf("booking_count = %d, want 1", got[0].BookingCount)
		}
	})

	t.Run("cancelled-only clients are excluded", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		execSeed(t, db,
			`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Pro', 'active')`,
			`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Svc', 30, 1.0, 1)`,
			`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Ana', '+1')`,
		)
		base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		insertBooking(t, db, "b1", "c1", "p1", "s1", FormatStorage(base.Add(time.Hour)), FormatStorage(base.Add(2*time.Hour)), "cancelled")

		repo := NewBookingsRepo(db)
		got, err := repo.AggregateByClient(ctx, base, base.Add(24*time.Hour), 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d rows, want 0", len(got))
		}
	})

	t.Run("orders by count desc then name asc", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		execSeed(t, db,
			`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Pro', 'active')`,
			`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Svc', 30, 1.0, 1)`,
			`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Beto', '+1')`,
			`INSERT INTO clients (id, name, phone) VALUES ('c2', 'Ana', '+2')`,
		)
		base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		// Both clients have 2 bookings; Ana should win the tie-break.
		insertBooking(t, db, "b1", "c1", "p1", "s1", FormatStorage(base.Add(time.Hour)), FormatStorage(base.Add(2*time.Hour)), "confirmed")
		insertBooking(t, db, "b2", "c1", "p1", "s1", FormatStorage(base.Add(3*time.Hour)), FormatStorage(base.Add(4*time.Hour)), "confirmed")
		insertBooking(t, db, "b3", "c2", "p1", "s1", FormatStorage(base.Add(5*time.Hour)), FormatStorage(base.Add(6*time.Hour)), "confirmed")
		insertBooking(t, db, "b4", "c2", "p1", "s1", FormatStorage(base.Add(7*time.Hour)), FormatStorage(base.Add(8*time.Hour)), "confirmed")

		repo := NewBookingsRepo(db)
		got, err := repo.AggregateByClient(ctx, base, base.Add(24*time.Hour), 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d rows, want 2", len(got))
		}
		if got[0].Name != "Ana" || got[1].Name != "Beto" {
			t.Errorf("order = %q, %q; want Ana, Beto", got[0].Name, got[1].Name)
		}
		if got[0].BookingCount != 2 || got[1].BookingCount != 2 {
			t.Errorf("counts = %d, %d; want 2, 2", got[0].BookingCount, got[1].BookingCount)
		}
	})

	t.Run("limit caps result", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		execSeed(t, db,
			`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Pro', 'active')`,
			`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Svc', 30, 1.0, 1)`,
			`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Ana', '+1')`,
			`INSERT INTO clients (id, name, phone) VALUES ('c2', 'Beto', '+2')`,
			`INSERT INTO clients (id, name, phone) VALUES ('c3', 'Caro', '+3')`,
		)
		base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		insertBooking(t, db, "b1", "c1", "p1", "s1", FormatStorage(base.Add(time.Hour)), FormatStorage(base.Add(2*time.Hour)), "confirmed")
		insertBooking(t, db, "b2", "c2", "p1", "s1", FormatStorage(base.Add(3*time.Hour)), FormatStorage(base.Add(4*time.Hour)), "confirmed")
		insertBooking(t, db, "b3", "c3", "p1", "s1", FormatStorage(base.Add(5*time.Hour)), FormatStorage(base.Add(6*time.Hour)), "confirmed")

		repo := NewBookingsRepo(db)
		got, err := repo.AggregateByClient(ctx, base, base.Add(24*time.Hour), 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d rows, want 2", len(got))
		}
	})

	t.Run("empty result is non-nil slice", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		execSeed(t, db,
			`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Pro', 'active')`,
			`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Svc', 30, 1.0, 1)`,
			`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Ana', '+1')`,
		)
		base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

		repo := NewBookingsRepo(db)
		got, err := repo.AggregateByClient(ctx, base, base.Add(time.Hour), 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("got %v, want empty non-nil slice", got)
		}
	})

	t.Run("result carries client id, name and phone", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		execSeed(t, db,
			`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Pro', 'active')`,
			`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Svc', 30, 1.0, 1)`,
			`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Ana', '+5491100000001')`,
		)
		base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		insertBooking(t, db, "b1", "c1", "p1", "s1", FormatStorage(base.Add(time.Hour)), FormatStorage(base.Add(2*time.Hour)), "confirmed")

		repo := NewBookingsRepo(db)
		got, err := repo.AggregateByClient(ctx, base, base.Add(24*time.Hour), 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d rows, want 1", len(got))
		}
		if got[0].ClientID != "c1" || got[0].Name != "Ana" || got[0].Phone != "+5491100000001" {
			t.Errorf("got %+v, want c1/Ana/+5491100000001", got[0])
		}
	})

	t.Run("no caller returns unauthenticated", func(t *testing.T) {
		db, cleanup := newAggregateTestDB(t)
		defer cleanup()

		repo := NewBookingsRepo(db)
		_, err := repo.AggregateByClient(context.Background(), time.Now(), time.Now(), 10)
		assertSemanticCode(t, err, "UNAUTHENTICATED")
	})
}

// Compile-time guard: the concrete repo satisfies the widened domain interface.
var _ interface {
	AggregateByClient(context.Context, time.Time, time.Time, int) ([]domainrepo.ClientBookingCount, error)
} = (*BookingsRepo)(nil)
