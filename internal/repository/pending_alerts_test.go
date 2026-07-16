package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestPendingAlertsRepo_Create(t *testing.T) {
	t.Run("happy path with confirmation_requested", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		bookingID := "b-001"
		mock.ExpectExec(`INSERT INTO pending_alerts`).
			WithArgs("confirmation_requested", "Confirmar reserva", "2026-07-13T13:00:00.000Z", &bookingID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		alert := &model.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Confirmar reserva",
			ScheduledDatetime: "2026-07-13T13:00:00.000Z",
			RelatedBookingID:  &bookingID,
		}
		err := repo.Create(adminCtx(), alert)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if alert.ID == 0 {
			t.Error("expected ID to be auto-assigned, got 0")
		}
		if alert.Status != "pending" {
			t.Errorf("expected Status='pending', got %q", alert.Status)
		}
	})

	t.Run("unknown type returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &model.PendingAlert{
			Type:              "unknown_kind",
			Message:           "Test",
			ScheduledDatetime: "2026-07-13T13:00:00.000Z",
		}
		err := repo.Create(adminCtx(), alert)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for unknown type, got %v", err)
		}
	})

	t.Run("reminder_24h not supported in Fase 1", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &model.PendingAlert{
			Type:              "reminder_24h",
			Message:           "Test",
			ScheduledDatetime: "2026-07-13T13:00:00.000Z",
		}
		err := repo.Create(adminCtx(), alert)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for reminder_24h in Fase 1, got %v", err)
		}
	})

	t.Run("empty message returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &model.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "",
			ScheduledDatetime: "2026-07-13T13:00:00.000Z",
		}
		err := repo.Create(adminCtx(), alert)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for empty message, got %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &model.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Test",
			ScheduledDatetime: "2026-07-13T13:00:00.000Z",
		}
		err := repo.Create(context.Background(), alert)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("staff role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &model.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Test",
			ScheduledDatetime: "2026-07-13T13:00:00.000Z",
		}
		err := repo.Create(staffCtx("pro-1"), alert)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})
}

func TestPendingAlertsRepo_ListPending(t *testing.T) {
	t.Run("returns due alerts in ascending order", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "type", "message", "scheduled_datetime", "status", "related_booking_id", "created_at",
		}).
			AddRow(1, "confirmation_requested", "Alert 1", "2026-07-13T10:00:00.000Z", "pending", nil, "2026-07-13T09:00:00.000Z").
			AddRow(2, "confirmation_requested", "Alert 2", "2026-07-13T11:00:00.000Z", "pending", nil, "2026-07-13T09:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM pending_alerts WHERE status = .pending. AND scheduled_datetime <= \? ORDER BY scheduled_datetime ASC LIMIT \?`).
			WithArgs("2026-07-13T12:00:00.000Z", 10).
			WillReturnRows(rows)

		alerts, err := repo.ListPending(adminCtx(), 10, "2026-07-13T12:00:00.000Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(alerts) != 2 {
			t.Fatalf("got %d alerts, want 2", len(alerts))
		}
		if alerts[0].ID != 1 {
			t.Errorf("got first ID=%d, want 1", alerts[0].ID)
		}
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "type", "message", "scheduled_datetime", "status", "related_booking_id", "created_at",
		})
		mock.ExpectQuery(`SELECT .+ FROM pending_alerts WHERE status = .pending. AND scheduled_datetime <= \? ORDER BY scheduled_datetime ASC LIMIT \?`).
			WithArgs("2026-07-13T12:00:00.000Z", 10).
			WillReturnRows(rows)

		alerts, err := repo.ListPending(adminCtx(), 10, "2026-07-13T12:00:00.000Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(alerts) != 0 {
			t.Errorf("got %d alerts, want 0", len(alerts))
		}
	})

	t.Run("limit caps result size", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "type", "message", "scheduled_datetime", "status", "related_booking_id", "created_at",
		}).
			AddRow(1, "confirmation_requested", "Alert 1", "2026-07-13T10:00:00.000Z", "pending", nil, "2026-07-13T09:00:00.000Z").
			AddRow(2, "confirmation_requested", "Alert 2", "2026-07-13T11:00:00.000Z", "pending", nil, "2026-07-13T09:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM pending_alerts WHERE status = .pending. AND scheduled_datetime <= \? ORDER BY scheduled_datetime ASC LIMIT \?`).
			WithArgs("2026-07-13T12:00:00.000Z", 2).
			WillReturnRows(rows)

		alerts, err := repo.ListPending(adminCtx(), 2, "2026-07-13T12:00:00.000Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(alerts) != 2 {
			t.Errorf("got %d alerts, want 2 (limit)", len(alerts))
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		_, err := repo.ListPending(context.Background(), 10, "2026-07-13T12:00:00.000Z")
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("limit zero returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		_, err := repo.ListPending(adminCtx(), 0, "2026-07-13T12:00:00.000Z")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for limit=0, got %v", err)
		}
	})

	t.Run("limit negative returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		_, err := repo.ListPending(adminCtx(), -1, "2026-07-13T12:00:00.000Z")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for limit=-1, got %v", err)
		}
	})
}

func TestPendingAlertsRepo_MarkAsSent(t *testing.T) {
	t.Run("pending alert marked as sent", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = .sent. WHERE id = \? AND status = .pending.`).
			WithArgs(42).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.MarkAsSent(adminCtx(), 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("already-sent alert is no-op", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = .sent. WHERE id = \? AND status = .pending.`).
			WithArgs(42).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.MarkAsSent(adminCtx(), 42)
		if err != nil {
			t.Fatalf("expected no error for already-sent alert, got %v", err)
		}
	})

	t.Run("cancelled alert is no-op", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		// UPDATE with status='pending' returns 0 rows (alert is cancelled)
		mock.ExpectExec(`UPDATE pending_alerts SET status = .sent. WHERE id = \? AND status = .pending.`).
			WithArgs(42).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.MarkAsSent(adminCtx(), 42)
		if err != nil {
			t.Fatalf("expected no error for cancelled alert, got %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.MarkAsSent(context.Background(), 42)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})
}

func TestPendingAlertsRepo_Cancel(t *testing.T) {
	t.Run("pending alert cancelled", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = .cancelled. WHERE id = \? AND status = .pending.`).
			WithArgs(42).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Cancel(adminCtx(), 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("already-cancelled alert is no-op", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = .cancelled. WHERE id = \? AND status = .pending.`).
			WithArgs(42).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Cancel(adminCtx(), 42)
		if err != nil {
			t.Fatalf("expected no error for already-cancelled alert, got %v", err)
		}
	})

	t.Run("sent alert is no-op", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		mock.ExpectExec(`UPDATE pending_alerts SET status = .cancelled. WHERE id = \? AND status = .pending.`).
			WithArgs(42).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Cancel(adminCtx(), 42)
		if err != nil {
			t.Fatalf("expected no error for sent alert, got %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.Cancel(context.Background(), 42)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("client role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.Cancel(clientCtx("c-1"), 42)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})
}

// Verify that auth.Caller is used (prevent unused import)
var _ = auth.RoleAdmin
