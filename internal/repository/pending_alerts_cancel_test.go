package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestPendingAlertsRepo_InsertForBooking(t *testing.T) {
	bookingID := "b-123"
	scheduled := time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC)

	t.Run("inserts confirmation alert with caller authentication", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs(bookingID).
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))
		mock.ExpectExec(`INSERT INTO pending_alerts \(type, message, scheduled_datetime, related_booking_id\) VALUES \(\?, \?, \?, \?\)`).
			WithArgs("confirmation_requested", "Confirmar reserva", FormatStorage(scheduled), &bookingID).
			WillReturnResult(sqlmock.NewResult(42, 1))

		id := "p1"
		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff, ProfessionalID: &id})
		err := repo.InsertForBooking(ctx, &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Confirmar reserva",
			ScheduledDatetime: scheduled,
			RelatedBookingID:  &bookingID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("insert for cancelled booking is no-op", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs(bookingID).
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("cancelled"))

		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff})
		alert := &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Confirmar reserva",
			ScheduledDatetime: scheduled,
			RelatedBookingID:  &bookingID,
		}
		err := repo.InsertForBooking(ctx, alert)
		if err != nil {
			t.Fatalf("unexpected error for cancelled booking no-op: %v", err)
		}
		if alert.ID != 0 {
			t.Errorf("expected ID to remain 0 for no-op insert, got %d", alert.ID)
		}
	})

	t.Run("no caller returns unauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.InsertForBooking(context.Background(), &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Confirmar reserva",
			ScheduledDatetime: scheduled,
			RelatedBookingID:  &bookingID,
		})
		assertSemanticCode(t, err, string(domain.ErrCodeUnauthenticated))
	})

	t.Run("empty message returns invalid input", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff})
		err := repo.InsertForBooking(ctx, &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "   ",
			ScheduledDatetime: scheduled,
			RelatedBookingID:  &bookingID,
		})
		assertSemanticCode(t, err, string(domain.ErrCodeInvalidInput))
	})
}

func TestPendingAlertsRepo_CancelByBookingID(t *testing.T) {
	t.Run("cancels pending alert by booking id", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = 'cancelled' WHERE related_booking_id = \? AND status = 'pending'`).
			WithArgs("b-123").
			WillReturnResult(sqlmock.NewResult(0, 1))

		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff})
		err := repo.CancelByBookingID(ctx, "b-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("sent alert remains untouched and returns nil", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = 'cancelled' WHERE related_booking_id = \? AND status = 'pending'`).
			WithArgs("b-123").
			WillReturnResult(sqlmock.NewResult(0, 0))

		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff})
		err := repo.CancelByBookingID(ctx, "b-123")
		if err != nil {
			t.Fatalf("expected no error for sent alert, got: %v", err)
		}
	})

	t.Run("cancelled alert remains untouched and returns nil", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = 'cancelled' WHERE related_booking_id = \? AND status = 'pending'`).
			WithArgs("b-456").
			WillReturnResult(sqlmock.NewResult(0, 0))

		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff})
		err := repo.CancelByBookingID(ctx, "b-456")
		if err != nil {
			t.Fatalf("expected no error for cancelled alert, got: %v", err)
		}
	})

	t.Run("no match returns nil", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = 'cancelled' WHERE related_booking_id = \? AND status = 'pending'`).
			WithArgs("b-missing").
			WillReturnResult(sqlmock.NewResult(0, 0))

		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff})
		err := repo.CancelByBookingID(ctx, "b-missing")
		if err != nil {
			t.Fatalf("expected nil for no-match, got: %v", err)
		}
	})

	t.Run("idempotent second call returns nil", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = 'cancelled' WHERE related_booking_id = \? AND status = 'pending'`).
			WithArgs("b-789").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE pending_alerts SET status = 'cancelled' WHERE related_booking_id = \? AND status = 'pending'`).
			WithArgs("b-789").
			WillReturnResult(sqlmock.NewResult(0, 0))

		ctx := auth.WithCaller(context.Background(), auth.Caller{ID: "staff-1", Role: auth.RoleStaff})
		if err := repo.CancelByBookingID(ctx, "b-789"); err != nil {
			t.Fatalf("first call failed: %v", err)
		}
		if err := repo.CancelByBookingID(ctx, "b-789"); err != nil {
			t.Fatalf("second call failed: %v", err)
		}
	})

	t.Run("no caller returns unauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.CancelByBookingID(context.Background(), "b-123")
		assertSemanticCode(t, err, string(domain.ErrCodeUnauthenticated))
	})
}

// assertSemanticCode asserts that err is a *domain.SemanticError with the expected code.
func assertSemanticCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var sErr *domain.SemanticError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
	}
	if string(sErr.Code) != want {
		t.Errorf("got Code=%q, want %q", sErr.Code, want)
	}
}
