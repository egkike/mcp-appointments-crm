package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ─── Test helpers ────────────────────────────────────────────────────────────

// bookingRowColumns is the column order used in sqlmock.NewRows for booking scans.
var bookingRowColumns = []string{
	"id", "client_id", "professional_id", "service_id",
	"start_datetime", "end_datetime", "status", "notes", "payment_method",
	"created_at", "updated_at",
}

// newBookingRows creates a sqlmock.Rows with one booking row.
func newBookingRows(b *entity.Booking) *sqlmock.Rows {
	return sqlmock.NewRows(bookingRowColumns).AddRow(
		b.ID, b.ClientID, b.ProfessionalID, b.ServiceID,
		FormatStorage(b.StartDatetime), FormatStorage(b.EndDatetime),
		string(b.Status), b.Notes, b.PaymentMethod,
		FormatStorage(b.CreatedAt), FormatStorage(b.UpdatedAt),
	)
}

// sampleBooking returns a test booking with known values.
func sampleBooking() *entity.Booking {
	notes := "Test booking"
	return &entity.Booking{
		ID:             "b-1",
		ClientID:       "c-1",
		ProfessionalID: "p-1",
		ServiceID:      "svc-1",
		StartDatetime:  time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		EndDatetime:    time.Date(2026, 7, 13, 13, 30, 0, 0, time.UTC),
		Status:         entity.BookingStatusPending,
		Notes:          &notes,
		PaymentMethod:  nil,
		CreatedAt:      time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	}
}

// ─── Create ──────────────────────────────────────────────────────────────────

func TestBookingsRepo_Create(t *testing.T) {
	t.Run("successful insert with no overlap", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		b := sampleBooking()
		err := repo.Create(adminCtx(), b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("overlap returns error wrapping domain.ErrConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		b := sampleBooking()
		err := repo.Create(adminCtx(), b)
		if err == nil {
			t.Fatal("expected error for overlap, got nil")
		}
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("expected errors.Is(err, domain.ErrConflict), got %v", err)
		}
	})

	t.Run("client creating own booking passes", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		b := sampleBooking()
		err := repo.Create(clientCtx("c-1"), b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("staff can create on behalf of client", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		b := sampleBooking()
		err := repo.Create(staffCtx("p-1"), b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("succeeds without caller (auth deferred to use case layer)", func(t *testing.T) {
		// Create does NOT call auth.RequireCaller — auth is enforced in the use case layer
		// (per design Decisión 11). The repo just does the INSERT. This test verifies
		// that Create does NOT require a caller in context.
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		b := sampleBooking()
		err := repo.Create(context.Background(), b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// ─── FindByID ────────────────────────────────────────────────────────────────

func TestBookingsRepo_FindByID(t *testing.T) {
	t.Run("found returns entity with time.Time fields", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(newBookingRows(b))

		got, err := repo.FindByID(adminCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "b-1" {
			t.Errorf("got ID=%q, want %q", got.ID, "b-1")
		}
		if got.Status != entity.BookingStatusPending {
			t.Errorf("got Status=%q, want %q", got.Status, entity.BookingStatusPending)
		}
		if !got.StartDatetime.Equal(b.StartDatetime) {
			t.Errorf("got StartDatetime=%v, want %v", got.StartDatetime, b.StartDatetime)
		}
		if !got.EndDatetime.Equal(b.EndDatetime) {
			t.Errorf("got EndDatetime=%v, want %v", got.EndDatetime, b.EndDatetime)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \?`).
			WithArgs("b-bogus").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.FindByID(adminCtx(), "b-bogus")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("client can see own booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnRows(newBookingRows(b))

		got, err := repo.FindByID(clientCtx("c-1"), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ClientID != "c-1" {
			t.Errorf("got ClientID=%q, want %q", got.ClientID, "c-1")
		}
	})

	t.Run("client cannot see another clients booking returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.FindByID(clientCtx("c-1"), "b-1")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound for cross-tenant, got %v", err)
		}
	})

	t.Run("staff can see own professional booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \? AND professional_id = \?`).
			WithArgs("b-1", "p-1").
			WillReturnRows(newBookingRows(b))

		got, err := repo.FindByID(staffCtx("p-1"), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ProfessionalID != "p-1" {
			t.Errorf("got ProfessionalID=%q, want %q", got.ProfessionalID, "p-1")
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		_, err := repo.FindByID(context.Background(), "b-1")
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})
}

// ─── Update ──────────────────────────────────────────────────────────────────

func TestBookingsRepo_Update(t *testing.T) {
	t.Run("successful update", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`UPDATE bookings SET .+ WHERE id=\?`).
			WithArgs("c-1", "p-1", "svc-1",
				"2026-07-13T13:00:00.000Z", "2026-07-13T13:30:00.000Z",
				"confirmed", sqlmock.AnyArg(), sqlmock.AnyArg(),
				"b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		b := sampleBooking()
		b.Status = entity.BookingStatusConfirmed
		err := repo.Update(adminCtx(), b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`UPDATE bookings SET .+ WHERE id=\?`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		b := sampleBooking()
		err := repo.Update(adminCtx(), b)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("client cannot update another clients booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Auth filter adds AND client_id = ? → no row matches → 0 affected
		mock.ExpectExec(`UPDATE bookings SET .+ WHERE id=\? AND client_id = \?`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		b := sampleBooking()
		err := repo.Update(clientCtx("c-999"), b)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound for cross-tenant, got %v", err)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		err := repo.Update(context.Background(), b)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})
}

// ─── Cancel ──────────────────────────────────────────────────────────────────

func TestBookingsRepo_Cancel(t *testing.T) {
	t.Run("pending to cancelled", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Cancel(adminCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("confirmed to cancelled", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("confirmed"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Cancel(adminCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cancelled to cancelled returns error", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("cancelled"))

		err := repo.Cancel(adminCtx(), "b-1")
		if err == nil {
			t.Fatal("expected error for cancelled→cancelled transition, got nil")
		}
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T", err)
		}
		if sErr.Code != domain.ErrCodeInvalidInput {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeInvalidInput)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-bogus").
			WillReturnError(sql.ErrNoRows)

		err := repo.Cancel(adminCtx(), "b-bogus")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("client can cancel own booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1", "c-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Cancel(clientCtx("c-1"), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("client cannot cancel another clients booking returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnError(sql.ErrNoRows)

		err := repo.Cancel(clientCtx("c-1"), "b-1")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound for cross-tenant client, got %v", err)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		err := repo.Cancel(context.Background(), "b-1")
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})
}

// ─── Reschedule ──────────────────────────────────────────────────────────────

func TestBookingsRepo_Reschedule(t *testing.T) {
	newStart := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)
	newEnd := time.Date(2026, 7, 13, 14, 30, 0, 0, time.UTC)

	t.Run("successful reschedule with no overlap", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "professional_id"}).
				AddRow("pending", "p-1"))

		mock.ExpectExec(`UPDATE bookings SET start_datetime`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Reschedule(adminCtx(), "b-1", newStart, newEnd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("atomic overlap returns error wrapping domain.ErrConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "professional_id"}).
				AddRow("pending", "p-1"))

		mock.ExpectExec(`UPDATE bookings SET start_datetime`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Re-check status: still pending → overlap
		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		err := repo.Reschedule(adminCtx(), "b-1", newStart, newEnd)
		if err == nil {
			t.Fatal("expected error for overlap, got nil")
		}
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("expected errors.Is(err, domain.ErrConflict), got %v", err)
		}
	})

	t.Run("cancelled booking cannot be rescheduled", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "professional_id"}).
				AddRow("cancelled", "p-1"))

		err := repo.Reschedule(adminCtx(), "b-1", newStart, newEnd)
		if err == nil {
			t.Fatal("expected error for rescheduling cancelled booking, got nil")
		}
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T", err)
		}
		if sErr.Code != domain.ErrCodeInvalidInput {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeInvalidInput)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-bogus").
			WillReturnError(sql.ErrNoRows)

		err := repo.Reschedule(adminCtx(), "b-bogus", newStart, newEnd)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("client can reschedule own booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status, professional_id FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "professional_id"}).
				AddRow("pending", "p-1"))

		mock.ExpectExec(`UPDATE bookings SET start_datetime`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z", "c-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Reschedule(clientCtx("c-1"), "b-1", newStart, newEnd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("concurrent cancellation returns domain.ErrCodeInvalidInput", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "professional_id"}).
				AddRow("pending", "p-1"))

		mock.ExpectExec(`UPDATE bookings SET start_datetime`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("cancelled"))

		err := repo.Reschedule(adminCtx(), "b-1", newStart, newEnd)
		if err == nil {
			t.Fatal("expected error for concurrent cancellation, got nil")
		}
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeInvalidInput {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeInvalidInput)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		err := repo.Reschedule(context.Background(), "b-1", newStart, newEnd)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})
}

// ─── FindOverlapping ─────────────────────────────────────────────────────────

func TestBookingsRepo_FindOverlapping(t *testing.T) {
	start := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)

	t.Run("returns overlapping bookings", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE professional_id = \? AND status != .cancelled.`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start)).
			WillReturnRows(newBookingRows(b))

		got, err := repo.FindOverlapping(adminCtx(), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d bookings, want 1", len(got))
		}
		if got[0].ID != "b-1" {
			t.Errorf("got ID=%q, want %q", got[0].ID, "b-1")
		}
	})

	t.Run("returns empty slice when no overlap", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE professional_id = \? AND status != .cancelled.`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start)).
			WillReturnRows(sqlmock.NewRows(bookingRowColumns))

		got, err := repo.FindOverlapping(adminCtx(), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("got %d bookings, want 0", len(got))
		}
	})

	t.Run("returns multiple overlapping bookings", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b1 := sampleBooking()
		b2 := sampleBooking()
		b2.ID = "b-2"
		b2.StartDatetime = time.Date(2026, 7, 13, 10, 30, 0, 0, time.UTC)
		b2.EndDatetime = time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)

		rows := sqlmock.NewRows(bookingRowColumns)
		for _, b := range []*entity.Booking{b1, b2} {
			rows.AddRow(b.ID, b.ClientID, b.ProfessionalID, b.ServiceID,
				FormatStorage(b.StartDatetime), FormatStorage(b.EndDatetime),
				string(b.Status), b.Notes, b.PaymentMethod,
				FormatStorage(b.CreatedAt), FormatStorage(b.UpdatedAt))
		}
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE professional_id = \? AND status != .cancelled.`).
			WillReturnRows(rows)

		got, err := repo.FindOverlapping(adminCtx(), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d bookings, want 2", len(got))
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		_, err := repo.FindOverlapping(context.Background(), "p-1", start, end)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("client caller adds AND client_id = ? before ORDER BY", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ AND client_id = \? ORDER BY`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start), "c-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.FindOverlapping(clientCtx("c-1"), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})

	t.Run("staff caller adds AND professional_id = ? before ORDER BY", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ AND professional_id = \? ORDER BY`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start), "p-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.FindOverlapping(staffCtx("p-1"), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})
}

// ─── FindByStaffAndRange ─────────────────────────────────────────────────────

func TestBookingsRepo_FindByStaffAndRange(t *testing.T) {
	start := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 13, 18, 0, 0, 0, time.UTC)

	t.Run("returns bookings in range ordered by start", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE professional_id = \? AND status != .cancelled.`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start)).
			WillReturnRows(newBookingRows(b))

		got, err := repo.FindByStaffAndRange(adminCtx(), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d bookings, want 1", len(got))
		}
		if got[0].ID != "b-1" {
			t.Errorf("got ID=%q, want %q", got[0].ID, "b-1")
		}
	})

	t.Run("returns empty slice when no bookings in range", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE professional_id = \? AND status != .cancelled.`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start)).
			WillReturnRows(sqlmock.NewRows(bookingRowColumns))

		got, err := repo.FindByStaffAndRange(adminCtx(), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("got %d bookings, want 0", len(got))
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		_, err := repo.FindByStaffAndRange(context.Background(), "p-1", start, end)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("client caller adds AND client_id = ? before ORDER BY", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ AND client_id = \? ORDER BY`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start), "c-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.FindByStaffAndRange(clientCtx("c-1"), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})

	t.Run("staff caller adds AND professional_id = ? before ORDER BY", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ AND professional_id = \? ORDER BY`).
			WithArgs("p-1", FormatStorage(end), FormatStorage(start), "p-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.FindByStaffAndRange(staffCtx("p-1"), "p-1", start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})
}

// ─── ListBookingsForRange ────────────────────────────────────────────────────

func TestBookingsRepo_ListBookingsForRange(t *testing.T) {
	start := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 13, 18, 0, 0, 0, time.UTC)

	t.Run("returns all bookings in range across staff", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE status != .cancelled.`).
			WithArgs(FormatStorage(end), FormatStorage(start)).
			WillReturnRows(newBookingRows(b))

		got, err := repo.ListBookingsForRange(adminCtx(), start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d bookings, want 1", len(got))
		}
	})

	t.Run("returns empty slice when no bookings in range", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE status != .cancelled.`).
			WithArgs(FormatStorage(end), FormatStorage(start)).
			WillReturnRows(sqlmock.NewRows(bookingRowColumns))

		got, err := repo.ListBookingsForRange(adminCtx(), start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("got %d bookings, want 0", len(got))
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		_, err := repo.ListBookingsForRange(context.Background(), start, end)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("client caller adds AND client_id = ? before ORDER BY", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ AND client_id = \? ORDER BY`).
			WithArgs(FormatStorage(end), FormatStorage(start), "c-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.ListBookingsForRange(clientCtx("c-1"), start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})

	t.Run("staff caller adds AND professional_id = ? before ORDER BY", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ AND professional_id = \? ORDER BY`).
			WithArgs(FormatStorage(end), FormatStorage(start), "p-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.ListBookingsForRange(staffCtx("p-1"), start, end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})
}

// ─── SearchByNotes ───────────────────────────────────────────────────────────

func TestBookingsRepo_SearchByNotes(t *testing.T) {
	t.Run("returns bookings matching notes substring", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE notes LIKE`).
			WithArgs("Test").
			WillReturnRows(newBookingRows(b))

		got, err := repo.SearchByNotes(adminCtx(), "Test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d bookings, want 1", len(got))
		}
		if got[0].ID != "b-1" {
			t.Errorf("got ID=%q, want %q", got[0].ID, "b-1")
		}
	})

	t.Run("returns empty slice when no match", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE notes LIKE`).
			WithArgs("nonexistent").
			WillReturnRows(sqlmock.NewRows(bookingRowColumns))

		got, err := repo.SearchByNotes(adminCtx(), "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("got %d bookings, want 0", len(got))
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		_, err := repo.SearchByNotes(context.Background(), "anything")
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("client caller adds AND client_id = ? before ORDER BY and LIMIT", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ client_id = \? ORDER BY`).
			WithArgs("Test", "c-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.SearchByNotes(clientCtx("c-1"), "Test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})

	t.Run("staff caller adds AND professional_id = ? before ORDER BY and LIMIT", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE .+ professional_id = \? ORDER BY`).
			WithArgs("Test", "p-1").
			WillReturnRows(newBookingRows(sampleBooking()))

		got, err := repo.SearchByNotes(staffCtx("p-1"), "Test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d bookings, want 1", len(got))
		}
	})
}

// ─── UpdateStatus ────────────────────────────────────────────────────────────

func TestBookingsRepo_UpdateStatus(t *testing.T) {
	t.Run("successful status update", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`UPDATE bookings SET status=\?.+WHERE id=\?`).
			WithArgs("confirmed", "b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateStatus(adminCtx(), "b-1", entity.BookingStatusConfirmed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`UPDATE bookings SET status=\?.+WHERE id=\?`).
			WithArgs("confirmed", "b-bogus").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.UpdateStatus(adminCtx(), "b-bogus", entity.BookingStatusConfirmed)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("client cannot update status of another clients booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`UPDATE bookings SET status=\?.+WHERE id=\? AND client_id = \?`).
			WithArgs("confirmed", "b-1", "c-999").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.UpdateStatus(clientCtx("c-999"), "b-1", entity.BookingStatusConfirmed)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound for cross-tenant, got %v", err)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		err := repo.UpdateStatus(context.Background(), "b-1", entity.BookingStatusConfirmed)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})
}

// ─── Owner role tests ────────────────────────────────────────────────────────

func TestBookingsRepo_OwnerRole(t *testing.T) {
	t.Run("owner can FindByID without filter", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		b := sampleBooking()
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(newBookingRows(b))

		booking, err := repo.FindByID(ownerCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if booking.ID != "b-1" {
			t.Errorf("got ID=%q, want %q", booking.ID, "b-1")
		}
	})

	t.Run("owner can Cancel", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Cancel(ownerCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner can Reschedule", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		newStart := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)
		newEnd := time.Date(2026, 7, 13, 14, 30, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "professional_id"}).
				AddRow("pending", "p-1"))

		mock.ExpectExec(`UPDATE bookings SET start_datetime`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Reschedule(ownerCtx(), "b-1", newStart, newEnd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner can Create for any client", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		b := sampleBooking()
		b.ClientID = "c-any"
		err := repo.Create(ownerCtx(), b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
