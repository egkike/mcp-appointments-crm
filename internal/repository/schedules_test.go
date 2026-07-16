package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestSchedulesRepo_GetByProfessionalAndDay(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "professional_id", "day_of_week", "start_time", "end_time",
		}).AddRow(1, "pro-1", 1, "09:00", "18:00")
		mock.ExpectQuery(`SELECT .+ FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 1).
			WillReturnRows(rows)

		schedule, err := repo.GetByProfessionalAndDay(adminCtx(), "pro-1", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if schedule.StartTime != "09:00" {
			t.Errorf("got StartTime=%q, want %q", schedule.StartTime, "09:00")
		}
		if schedule.EndTime != "18:00" {
			t.Errorf("got EndTime=%q, want %q", schedule.EndTime, "18:00")
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 3).
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByProfessionalAndDay(adminCtx(), "pro-1", 3)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("invalid day_of_week returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.GetByProfessionalAndDay(adminCtx(), "pro-1", 7)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for day_of_week=7, got %v", err)
		}
	})

	t.Run("negative day_of_week returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.GetByProfessionalAndDay(adminCtx(), "pro-1", -1)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for day_of_week=-1, got %v", err)
		}
	})

	t.Run("staff caller is restricted to own professional_id", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		// Even though "pro-999" is passed, staff ctx has professionalID="pro-1"
		// so the query should use "pro-1"
		rows := sqlmock.NewRows([]string{
			"id", "professional_id", "day_of_week", "start_time", "end_time",
		}).AddRow(1, "pro-1", 1, "09:00", "18:00")
		mock.ExpectQuery(`SELECT .+ FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 1).
			WillReturnRows(rows)

		schedule, err := repo.GetByProfessionalAndDay(staffCtx("pro-1"), "pro-999", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if schedule.ProfessionalID != "pro-1" {
			t.Errorf("got ProfessionalID=%q, want %q", schedule.ProfessionalID, "pro-1")
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.GetByProfessionalAndDay(context.Background(), "pro-1", 1)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("staff with nil ProfessionalID returns ErrCodeUnauthenticated (fail-secure)", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		// Staff caller with nil ProfessionalID — must fail-secure, not fall through
		ctx := auth.WithCaller(context.Background(), auth.Caller{
			ID:             "staff-nil",
			Role:           auth.RoleStaff,
			ProfessionalID: nil,
		})
		_, err := repo.GetByProfessionalAndDay(ctx, "pro-1", 1)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("admin without ProfessionalID succeeds", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "professional_id", "day_of_week", "start_time", "end_time",
		}).AddRow(1, "pro-1", 1, "09:00", "18:00")
		mock.ExpectQuery(`SELECT .+ FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 1).
			WillReturnRows(rows)

		// Admin has no ProfessionalID — should still succeed
		schedule, err := repo.GetByProfessionalAndDay(adminCtx(), "pro-1", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if schedule.ProfessionalID != "pro-1" {
			t.Errorf("got ProfessionalID=%q, want %q", schedule.ProfessionalID, "pro-1")
		}
	})
}

func TestSchedulesRepo_Upsert(t *testing.T) {
	t.Run("insert new schedule", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectExec(`INSERT INTO schedules`).
			WithArgs("pro-1", 1, "09:00", "18:00").
			WillReturnResult(sqlmock.NewResult(1, 1))

		schedule := &model.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		err := repo.Upsert(adminCtx(), schedule)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("update existing schedule", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		// INSERT fails with UNIQUE violation
		mock.ExpectExec(`INSERT INTO schedules`).
			WithArgs("pro-1", 1, "10:00", "19:00").
			WillReturnError(errors.New("UNIQUE constraint failed: schedules.professional_id, schedules.day_of_week"))

		// Then UPDATE succeeds
		mock.ExpectExec(`UPDATE schedules SET`).
			WithArgs("10:00", "19:00", "pro-1", 1).
			WillReturnResult(sqlmock.NewResult(0, 1))

		schedule := &model.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "10:00",
			EndTime:        "19:00",
		}
		err := repo.Upsert(adminCtx(), schedule)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid day_of_week returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &model.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      7,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		err := repo.Upsert(adminCtx(), schedule)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("invalid time format returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &model.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "9:00 AM",
			EndTime:        "18:00",
		}
		err := repo.Upsert(adminCtx(), schedule)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for malformed time, got %v", err)
		}
	})

	t.Run("start_time not before end_time returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &model.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "18:00",
			EndTime:        "09:00",
		}
		err := repo.Upsert(adminCtx(), schedule)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput when start >= end, got %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &model.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		err := repo.Upsert(context.Background(), schedule)
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
		repo := NewSchedulesRepo(db)

		schedule := &model.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		err := repo.Upsert(clientCtx("c-1"), schedule)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})
}

func TestSchedulesRepo_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectExec(`DELETE FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 1).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(adminCtx(), "pro-1", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectExec(`DELETE FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 3).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(adminCtx(), "pro-1", 3)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("invalid day_of_week returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		err := repo.Delete(adminCtx(), "pro-1", 7)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		err := repo.Delete(context.Background(), "pro-1", 1)
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
		repo := NewSchedulesRepo(db)

		err := repo.Delete(staffCtx("pro-1"), "pro-1", 1)
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
