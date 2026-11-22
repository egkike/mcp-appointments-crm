package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ── AvailabilityBookingsReader vs the caller-scoped BookingsRepo (issue #243) ──
//
// Behaviour tests over a REAL SQLite database, not sqlmock: the whole point is
// WHICH ROWS come back, and a mock cannot answer that. Both adapters run against
// the same seeded rows, so the only variable is the adapter itself.

// newAvailabilityTestDB opens a real database seeded with the rows the bookings
// foreign keys require: two professionals, one service and two clients.
func newAvailabilityTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, cleanup := newAggregateTestDB(t)
	t.Cleanup(cleanup)
	execSeed(t, db,
		`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Ana', 'active')`,
		`INSERT INTO professionals (id, name, status) VALUES ('p2', 'Beto', 'active')`,
		`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Corte', 60, 100.0, 1)`,
		`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Cliente Uno', '+5491100000001')`,
		`INSERT INTO clients (id, name, phone) VALUES ('c2', 'Cliente Dos', '+5491100000002')`,
	)
	return db
}

// seedAvailabilityScopeBookings inserts the four rows every assertion below
// reasons about: one booking of c1 with p1, one of c2 with p1, one of c2 with p2
// and one cancelled booking that must never block anything.
func seedAvailabilityScopeBookings(t *testing.T, db *sql.DB, base time.Time) {
	t.Helper()
	insertBooking(t, db, "b-c1-p1", "c1", "p1", "s1",
		FormatStorage(base.Add(time.Hour)), FormatStorage(base.Add(2*time.Hour)), "confirmed")
	insertBooking(t, db, "b-c2-p1", "c2", "p1", "s1",
		FormatStorage(base.Add(3*time.Hour)), FormatStorage(base.Add(4*time.Hour)), "confirmed")
	insertBooking(t, db, "b-c2-p2", "c2", "p2", "s1",
		FormatStorage(base.Add(time.Hour)), FormatStorage(base.Add(2*time.Hour)), "confirmed")
	insertBooking(t, db, "b-cancelled-p1", "c1", "p1", "s1",
		FormatStorage(base.Add(5*time.Hour)), FormatStorage(base.Add(6*time.Hour)), "cancelled")
}

// bookingIDs renders the returned bookings as IDs, in query order, so every
// assertion names the COMPLETE observable result instead of a count.
func bookingIDs(bookings []*entity.Booking) []string {
	ids := make([]string, 0, len(bookings))
	for _, booking := range bookings {
		ids = append(ids, booking.ID)
	}
	return ids
}

// wantBookingIDs compares two ID slices exactly (order included).
func wantBookingIDs(t *testing.T, got []*entity.Booking, want []string) {
	t.Helper()
	gotIDs := bookingIDs(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("got %v; want %v", gotIDs, want)
	}
	for index := range want {
		if gotIDs[index] != want[index] {
			t.Fatalf("got %v; want %v", gotIDs, want)
		}
	}
}

func TestAvailabilityBookingsReader_OverlapIsRoleNeutral(t *testing.T) {
	db := newAvailabilityTestDB(t)
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	seedAvailabilityScopeBookings(t, db, base)
	windowEnd := base.Add(9 * time.Hour)

	reader := NewAvailabilityBookingsReader(db)
	filtered := NewBookingsRepo(db)

	t.Run("reader returns every client's booking of the professional and needs no caller", func(t *testing.T) {
		// context.Background() carries NO caller on purpose (S6): availability is
		// a question about the calendar, so a missing caller must not be an error
		// here (trap 1 — RequireCaller would turn this into -32000).
		got, err := reader.FindOverlapping(context.Background(), "p1", base, windowEnd)
		if err != nil {
			t.Fatalf("FindOverlapping: %v (the reader must not require a caller)", err)
		}
		// b-c2-p1 belongs to another client: visible anyway, and the cancelled row
		// stays excluded exactly like the filtered query excludes it.
		wantBookingIDs(t, got, []string{"b-c1-p1", "b-c2-p1"})
	})

	t.Run("filtered repo still hides the other client's booking", func(t *testing.T) {
		got, err := filtered.FindOverlapping(clientCtx("c1"), "p1", base, windowEnd)
		if err != nil {
			t.Fatalf("FindOverlapping: %v", err)
		}
		wantBookingIDs(t, got, []string{"b-c1-p1"})
	})

	t.Run("reader sees another professional's agenda", func(t *testing.T) {
		got, err := reader.FindOverlapping(context.Background(), "p2", base, windowEnd)
		if err != nil {
			t.Fatalf("FindOverlapping: %v", err)
		}
		wantBookingIDs(t, got, []string{"b-c2-p2"})
	})

	t.Run("filtered repo returns nothing when staff asks about another professional", func(t *testing.T) {
		// This is the L2 shape of the defect: staffCtx("p1") asking about p2 makes
		// the query `professional_id = 'p2' AND professional_id = 'p1'`, which can
		// never match — the agenda looked empty, so every slot looked free.
		got, err := filtered.FindOverlapping(staffCtx("p1"), "p2", base, windowEnd)
		if err != nil {
			t.Fatalf("FindOverlapping: %v", err)
		}
		if got == nil {
			t.Fatal("got nil; want a non-nil empty slice")
		}
		wantBookingIDs(t, got, nil)
	})

	t.Run("both adapters share the half-open window bound", func(t *testing.T) {
		// [start, start+1h) must not match b-c1-p1, which starts exactly at the
		// window end. adminCtx() adds no clause, so the filtered repo is expected
		// to agree with the reader row for row.
		read, err := reader.FindOverlapping(context.Background(), "p1", base, base.Add(time.Hour))
		if err != nil {
			t.Fatalf("reader FindOverlapping: %v", err)
		}
		filt, err := filtered.FindOverlapping(adminCtx(), "p1", base, base.Add(time.Hour))
		if err != nil {
			t.Fatalf("filtered FindOverlapping: %v", err)
		}
		wantBookingIDs(t, read, nil)
		wantBookingIDs(t, filt, nil)
	})
}

func TestAvailabilityBookingsReader_RangeIsRoleNeutral(t *testing.T) {
	db := newAvailabilityTestDB(t)
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	seedAvailabilityScopeBookings(t, db, base)
	windowEnd := base.Add(9 * time.Hour)

	reader := NewAvailabilityBookingsReader(db)
	filtered := NewBookingsRepo(db)

	t.Run("reader returns every professional's bookings without a caller", func(t *testing.T) {
		got, err := reader.ListBookingsForRange(context.Background(), base, windowEnd)
		if err != nil {
			t.Fatalf("ListBookingsForRange: %v (the reader must not require a caller)", err)
		}
		// Ordered by professional_id then start_datetime, as the shared builder
		// declares; the cancelled row is excluded.
		wantBookingIDs(t, got, []string{"b-c1-p1", "b-c2-p1", "b-c2-p2"})
	})

	t.Run("filtered repo still scopes a client caller to its own bookings", func(t *testing.T) {
		got, err := filtered.ListBookingsForRange(clientCtx("c1"), base, windowEnd)
		if err != nil {
			t.Fatalf("ListBookingsForRange: %v", err)
		}
		wantBookingIDs(t, got, []string{"b-c1-p1"})
	})

	t.Run("filtered repo still scopes a staff caller to its own professional", func(t *testing.T) {
		got, err := filtered.ListBookingsForRange(staffCtx("p1"), base, windowEnd)
		if err != nil {
			t.Fatalf("ListBookingsForRange: %v", err)
		}
		wantBookingIDs(t, got, []string{"b-c1-p1", "b-c2-p1"})
	})
}
