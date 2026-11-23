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

// seedFullProjectionBooking inserts a booking with EVERY column populated, so a
// narrow projection shows up as absent data rather than as absent rows.
func seedFullProjectionBooking(t *testing.T, db *sql.DB, id, clientID, professionalID, start, end, status, notes, paymentMethod string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes, payment_method)
		 VALUES (?, ?, ?, 's1', ?, ?, ?, ?, ?)`,
		id, clientID, professionalID, start, end, status, notes, paymentMethod,
	); err != nil {
		t.Fatalf("insert full booking %s: %v", id, err)
	}
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

// bookingSpans renders rows as "professional_id@<start>" in query order. The
// reader's projection has no id column (L5, issue #247), so its assertions
// identify rows by the columns it does load instead of by primary key.
//
// Consequence to keep in mind when extending these tests (R2-span-identity,
// issue #249): two rows sharing a professional and a start are INDISTINGUISHABLE
// here. Cases give their spans distinct values for that reason; a case that must
// tell two such rows apart has to assert on another loaded column instead of
// assuming the primary key is available.
//
// It delegates the formatting to bookingSpan so the format string lives in ONE
// place (R2-dup-span-helpers, review-f94fd31c48e77ce8): a future change to the
// span shape must not be able to update one caller and miss the other.
func bookingSpans(bookings []*entity.Booking) []string {
	spans := make([]string, 0, len(bookings))
	for _, booking := range bookings {
		spans = append(spans, bookingSpan(booking.ProfessionalID, booking.StartDatetime))
	}
	return spans
}

// wantBookingSpans compares two span slices exactly (order included).
func wantBookingSpans(t *testing.T, got []*entity.Booking, want []string) {
	t.Helper()
	gotSpans := bookingSpans(got)
	if len(gotSpans) != len(want) {
		t.Fatalf("got %v; want %v", gotSpans, want)
	}
	for index := range want {
		if gotSpans[index] != want[index] {
			t.Fatalf("got %v; want %v", gotSpans, want)
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
		wantBookingSpans(t, got, []string{
			bookingSpan("p1", base.Add(time.Hour)),
			bookingSpan("p1", base.Add(3*time.Hour)),
		})
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
		wantBookingSpans(t, got, []string{bookingSpan("p2", base.Add(time.Hour))})
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
		wantBookingSpans(t, got, []string{
			bookingSpan("p1", base.Add(time.Hour)),
			bookingSpan("p1", base.Add(3*time.Hour)),
			bookingSpan("p2", base.Add(time.Hour)),
		})
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

// bookingSpan renders one row the way bookingSpans does, so the expected values
// read in the same shape as the got values.
func bookingSpan(professionalID string, start time.Time) string {
	return professionalID + "@" + start.UTC().Format(time.RFC3339)
}

// TestAvailabilityBookingsReaderProjectionIsNarrow is L5 (issue #247): the
// reader must never load the columns its consumers do not read. The seed row has
// every column populated, so the empty fields below can only mean "not selected"
// — a row that carried them would prove the projection is still the full one.
//
// Both reads are checked, and each one carries a NEGATIVE CONTROL: the four
// fields the consumers actually read must come back POPULATED, so a scanner that
// silently returned an empty struct cannot pass this test.
func TestAvailabilityBookingsReaderProjectionIsNarrow(t *testing.T) {
	db := newAvailabilityTestDB(t)
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	start := base.Add(time.Hour)
	end := base.Add(2 * time.Hour)
	windowEnd := base.Add(9 * time.Hour)
	seedFullProjectionBooking(t, db, "b-full", "c1", "p1",
		FormatStorage(start), FormatStorage(end), "confirmed", "nota privada", "cash")

	reader := NewAvailabilityBookingsReader(db)

	// wantNarrow asserts the whole projection: the four columns the reader
	// selects are populated, and the columns it does not select stay at their
	// zero value.
	wantNarrow := func(t *testing.T, got []*entity.Booking) {
		t.Helper()
		if len(got) != 1 {
			t.Fatalf("got %d rows; want 1", len(got))
		}
		b := got[0]

		// Negative control: a broken scan cannot pass by returning an empty struct.
		if b.ProfessionalID != "p1" {
			t.Errorf("ProfessionalID = %q; want %q (the projection must load it)", b.ProfessionalID, "p1")
		}
		if !b.StartDatetime.Equal(start) {
			t.Errorf("StartDatetime = %s; want %s (the projection must load it)", b.StartDatetime, start)
		}
		if !b.EndDatetime.Equal(end) {
			t.Errorf("EndDatetime = %s; want %s (the projection must load it)", b.EndDatetime, end)
		}
		if b.Status != entity.BookingStatusConfirmed {
			t.Errorf("Status = %q; want %q (the projection must load it)", b.Status, entity.BookingStatusConfirmed)
		}

		// The columns the narrow projection omits (S3, S6): never loaded, so the
		// zero value is the correct answer and nothing to leak.
		if b.ClientID != "" {
			t.Errorf("ClientID = %q; want the zero value (the availability reader must not load client_id)", b.ClientID)
		}
		if b.Notes != nil {
			t.Errorf("Notes = %q; want nil (the availability reader must not load notes)", *b.Notes)
		}
		if b.PaymentMethod != nil {
			t.Errorf("PaymentMethod = %q; want nil (the availability reader must not load payment_method)", *b.PaymentMethod)
		}
		if b.ID != "" {
			t.Errorf("ID = %q; want the zero value (id is not part of the availability projection)", b.ID)
		}
		if b.ServiceID != "" {
			t.Errorf("ServiceID = %q; want the zero value (service_id is not part of the availability projection)", b.ServiceID)
		}
		if !b.CreatedAt.IsZero() || !b.UpdatedAt.IsZero() {
			t.Errorf("CreatedAt/UpdatedAt = %s/%s; want the zero value (not part of the availability projection)", b.CreatedAt, b.UpdatedAt)
		}
	}

	t.Run("FindOverlapping", func(t *testing.T) {
		got, err := reader.FindOverlapping(context.Background(), "p1", base, windowEnd)
		if err != nil {
			t.Fatalf("FindOverlapping: %v", err)
		}
		wantNarrow(t, got)
	})

	t.Run("ListBookingsForRange", func(t *testing.T) {
		got, err := reader.ListBookingsForRange(context.Background(), base, windowEnd)
		if err != nil {
			t.Fatalf("ListBookingsForRange: %v", err)
		}
		wantNarrow(t, got)
	})
}

// TestBookingsRepoDataPathsKeepTheFullProjection is the trap guard for S3a: the
// narrow projection belongs to the availability reader ONLY. A data path that
// switched to it would still return the right ROWS, so nothing but an assertion
// on the columns can catch it — hence the explicit field-by-field check on every
// method that shares the parametrised builders.
func TestBookingsRepoDataPathsKeepTheFullProjection(t *testing.T) {
	db := newAvailabilityTestDB(t)
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	start := base.Add(time.Hour)
	end := base.Add(2 * time.Hour)
	windowEnd := base.Add(9 * time.Hour)
	seedFullProjectionBooking(t, db, "b-full", "c1", "p1",
		FormatStorage(start), FormatStorage(end), "confirmed", "nota privada", "cash")

	repo := NewBookingsRepo(db)

	reads := []struct {
		name string
		run  func(ctx context.Context) ([]*entity.Booking, error)
	}{
		{"FindOverlapping", func(ctx context.Context) ([]*entity.Booking, error) {
			return repo.FindOverlapping(ctx, "p1", base, windowEnd)
		}},
		{"FindByStaffAndRange", func(ctx context.Context) ([]*entity.Booking, error) {
			return repo.FindByStaffAndRange(ctx, "p1", base, windowEnd)
		}},
		{"ListBookingsForRange", func(ctx context.Context) ([]*entity.Booking, error) {
			return repo.ListBookingsForRange(ctx, base, windowEnd)
		}},
		{"SearchByNotes", func(ctx context.Context) ([]*entity.Booking, error) {
			return repo.SearchByNotes(ctx, "privada")
		}},
	}

	for _, read := range reads {
		t.Run(read.name+" still selects every column", func(t *testing.T) {
			got, err := read.run(adminCtx())
			if err != nil {
				t.Fatalf("%s: %v", read.name, err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d rows; want 1", len(got))
			}
			b := got[0]
			if b.ID != "b-full" {
				t.Errorf("ID = %q; want %q", b.ID, "b-full")
			}
			if b.ClientID != "c1" {
				t.Errorf("ClientID = %q; want %q (a data path must keep loading client_id)", b.ClientID, "c1")
			}
			if b.ServiceID != "s1" {
				t.Errorf("ServiceID = %q; want %q (a data path must keep loading service_id)", b.ServiceID, "s1")
			}
			if b.Notes == nil || *b.Notes != "nota privada" {
				t.Errorf("Notes = %v; want %q (a data path must keep loading notes)", b.Notes, "nota privada")
			}
			if b.PaymentMethod == nil || *b.PaymentMethod != "cash" {
				t.Errorf("PaymentMethod = %v; want %q (a data path must keep loading payment_method)", b.PaymentMethod, "cash")
			}
			if b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() {
				t.Errorf("CreatedAt/UpdatedAt = %s/%s; want both loaded", b.CreatedAt, b.UpdatedAt)
			}
			if b.ProfessionalID != "p1" || b.Status != entity.BookingStatusConfirmed {
				t.Errorf("ProfessionalID/Status = %q/%q; want %q/%q", b.ProfessionalID, b.Status, "p1", entity.BookingStatusConfirmed)
			}
			if !b.StartDatetime.Equal(start) || !b.EndDatetime.Equal(end) {
				t.Errorf("Start/End = %s/%s; want %s/%s", b.StartDatetime, b.EndDatetime, start, end)
			}
		})
	}
}
