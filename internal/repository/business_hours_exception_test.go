package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestBusinessHoursExceptionRepo_Create(t *testing.T) {
	t.Run("happy path is_closed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		reason := "Navidad"
		mock.ExpectExec(`INSERT INTO business_hours_exception`).
			WithArgs("2026-12-25", true, nil, nil, &reason).
			WillReturnResult(sqlmock.NewResult(1, 1))

		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
			Reason:        &reason,
		}
		err := repo.Create(context.Background(), ex)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("happy path open with valid times", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "10:00"
		close := "14:00"
		mock.ExpectExec(`INSERT INTO business_hours_exception`).
			WithArgs("2026-12-24", false, &open, &close, nil).
			WillReturnResult(sqlmock.NewResult(1, 1))

		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(context.Background(), ex)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed date returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-25T00:00:00",
			IsClosed:      true,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for datetime, got %v", err)
		}
	})

	t.Run("date with slashes returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		ex := &model.BusinessHoursException{
			ExceptionDate: "25/12/2026",
			IsClosed:      true,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for slashes, got %v", err)
		}
	})

	t.Run("is_closed false without open_time returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		close := "14:00"
		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      nil,
			CloseTime:     &close,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for nil open_time, got %v", err)
		}
	})

	t.Run("is_closed false without close_time returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "10:00"
		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     nil,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for nil close_time, got %v", err)
		}
	})

	t.Run("open_time after close_time returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "18:00"
		close := "09:00"
		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for open>close, got %v", err)
		}
	})

	t.Run("is_closed true with times set returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "10:00"
		close := "14:00"
		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for is_closed with times, got %v", err)
		}
	})

	t.Run("invalid HH:MM format returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "9:00"
		close := "14:00"
		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for single-digit hour, got %v", err)
		}
	})

	t.Run("time with seconds returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "10:00:00"
		close := "14:00"
		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for time with seconds, got %v", err)
		}
	})

	t.Run("UNIQUE violation returns ErrConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`INSERT INTO business_hours_exception`).
			WithArgs("2026-12-25", true, nil, nil, nil).
			WillReturnError(errors.New("UNIQUE constraint failed: business_hours_exception.exception_date"))

		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
		}
		err := repo.Create(context.Background(), ex)
		if !errors.Is(err, ErrConflict) {
			t.Errorf("expected ErrConflict, got %v", err)
		}
	})

	t.Run("non-UNIQUE DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`INSERT INTO business_hours_exception`).
			WithArgs("2026-12-25", true, nil, nil, nil).
			WillReturnError(errors.New("disk full"))

		ex := &model.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
		}
		err := repo.Create(context.Background(), ex)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, ErrConflict) {
			t.Error("non-UNIQUE error should not return ErrConflict")
		}
	})
}

func TestBusinessHoursExceptionRepo_GetByDate(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "exception_date", "is_closed", "open_time", "close_time", "reason", "created_at",
		}).AddRow(1, "2026-12-25", true, nil, nil, strPtr("Navidad"),
			"2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date = \?`).
			WithArgs("2026-12-25").
			WillReturnRows(rows)

		ex, err := repo.GetByDate(context.Background(), "2026-12-25")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ex.ExceptionDate != "2026-12-25" {
			t.Errorf("got ExceptionDate=%q, want %q", ex.ExceptionDate, "2026-12-25")
		}
		if !ex.IsClosed {
			t.Error("expected IsClosed=true")
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date = \?`).
			WithArgs("2026-01-01").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByDate(context.Background(), "2026-01-01")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date = \?`).
			WithArgs("2026-12-25").
			WillReturnError(errors.New("connection lost"))

		_, err := repo.GetByDate(context.Background(), "2026-12-25")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBusinessHoursExceptionRepo_List(t *testing.T) {
	t.Run("returns all ordered by date", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "exception_date", "is_closed", "open_time", "close_time", "reason", "created_at",
		}).
			AddRow(1, "2026-12-24", false, strPtr("10:00"), strPtr("14:00"), strPtr("Nochebuena"),
				"2026-01-01T00:00:00.000Z").
			AddRow(2, "2026-12-25", true, nil, nil, strPtr("Navidad"),
				"2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception ORDER BY exception_date`).
			WillReturnRows(rows)

		exceptions, err := repo.List(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(exceptions) != 2 {
			t.Fatalf("got %d exceptions, want 2", len(exceptions))
		}
		if exceptions[0].ExceptionDate != "2026-12-24" {
			t.Errorf("got first=%q, want %q", exceptions[0].ExceptionDate, "2026-12-24")
		}
	})

	t.Run("empty result returns nil slice", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "exception_date", "is_closed", "open_time", "close_time", "reason", "created_at",
		})
		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception ORDER BY exception_date`).
			WillReturnRows(rows)

		exceptions, err := repo.List(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(exceptions) != 0 {
			t.Errorf("got %d exceptions, want 0", len(exceptions))
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception ORDER BY exception_date`).
			WillReturnError(errors.New("connection lost"))

		_, err := repo.List(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBusinessHoursExceptionRepo_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`DELETE FROM business_hours_exception WHERE id = \?`).
			WithArgs("1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(context.Background(), "1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`DELETE FROM business_hours_exception WHERE id = \?`).
			WithArgs("999").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(context.Background(), "999")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`DELETE FROM business_hours_exception WHERE id = \?`).
			WithArgs("1").
			WillReturnError(errors.New("disk full"))

		err := repo.Delete(context.Background(), "1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
