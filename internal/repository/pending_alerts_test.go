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

func TestPendingAlertsRepo_Save(t *testing.T) {
	t.Run("happy path with confirmation_requested", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		bookingID := "b-001"
		scheduled := time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC)
		mock.ExpectExec(`INSERT INTO pending_alerts`).
			WithArgs("confirmation_requested", "Confirmar reserva", FormatStorage(scheduled), &bookingID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		alert := &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Confirmar reserva",
			ScheduledDatetime: scheduled,
			RelatedBookingID:  &bookingID,
		}
		err := repo.Save(adminCtx(), alert)
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

	t.Run("unknown type returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &entity.PendingAlert{
			Type:              "unknown_kind",
			Message:           "Test",
			ScheduledDatetime: time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		}
		err := repo.Save(adminCtx(), alert)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for unknown type, got %v", err)
		}
	})

	t.Run("reminder_24h not supported in Fase 1", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &entity.PendingAlert{
			Type:              "reminder_24h",
			Message:           "Test",
			ScheduledDatetime: time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		}
		err := repo.Save(adminCtx(), alert)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for reminder_24h in Fase 1, got %v", err)
		}
	})

	t.Run("empty message returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "",
			ScheduledDatetime: time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		}
		err := repo.Save(adminCtx(), alert)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for empty message, got %v", err)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Test",
			ScheduledDatetime: time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		}
		err := repo.Save(context.Background(), alert)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("staff role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		alert := &entity.PendingAlert{
			Type:              "confirmation_requested",
			Message:           "Test",
			ScheduledDatetime: time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		}
		err := repo.Save(staffCtx("pro-1"), alert)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})
}

func TestPendingAlertsRepo_FindPending(t *testing.T) {
	nowStr := "2026-07-13T12:00:00.000Z"
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	t.Run("returns due alerts in ascending order", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "type", "message", "scheduled_datetime", "status", "related_booking_id", "created_at",
		}).
			AddRow(1, "confirmation_requested", "Alert 1", "2026-07-13T10:00:00.000Z", "pending", nil, "2026-07-13T09:00:00.000Z").
			AddRow(2, "confirmation_requested", "Alert 2", "2026-07-13T11:00:00.000Z", "pending", nil, "2026-07-13T09:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM pending_alerts WHERE status = .pending. AND scheduled_datetime <= \? ORDER BY scheduled_datetime ASC`).
			WithArgs(nowStr).
			WillReturnRows(rows)

		alerts, err := repo.FindPending(adminCtx(), now)
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
		mock.ExpectQuery(`SELECT .+ FROM pending_alerts WHERE status = .pending. AND scheduled_datetime <= \? ORDER BY scheduled_datetime ASC`).
			WithArgs(nowStr).
			WillReturnRows(rows)

		alerts, err := repo.FindPending(adminCtx(), now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(alerts) != 0 {
			t.Errorf("got %d alerts, want 0", len(alerts))
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		_, err := repo.FindPending(context.Background(), now)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
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

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.MarkAsSent(context.Background(), 42)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
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

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.Cancel(context.Background(), 42)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("client role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewPendingAlertsRepo(db)

		err := repo.Cancel(clientCtx("c-1"), 42)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})
}

// Verify that auth.Caller is used (prevent unused import)
var _ = auth.RoleAdmin
