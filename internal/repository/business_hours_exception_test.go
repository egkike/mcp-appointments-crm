package repository

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestBusinessHoursExceptionRepo_Create(t *testing.T) {
	t.Run("happy path is_closed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		reason := "Navidad"
		mock.ExpectExec(`INSERT INTO business_hours_exception`).
			WithArgs("2026-12-25", true, nil, nil, &reason).
			WillReturnResult(sqlmock.NewResult(1, 1))

		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
			Reason:        &reason,
		}
		err := repo.Create(adminCtx(), ex)
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

		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed date returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-25T00:00:00",
			IsClosed:      true,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for datetime, got %v", err)
		}
	})

	t.Run("date with slashes returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		ex := &entity.BusinessHoursException{
			ExceptionDate: "25/12/2026",
			IsClosed:      true,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for slashes, got %v", err)
		}
	})

	t.Run("is_closed false without open_time returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		close := "14:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      nil,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for nil open_time, got %v", err)
		}
	})

	t.Run("is_closed false without close_time returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "10:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     nil,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for nil close_time, got %v", err)
		}
	})

	t.Run("open_time after close_time returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "18:00"
		close := "09:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for open>close, got %v", err)
		}
	})

	t.Run("is_closed true with times set returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "10:00"
		close := "14:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for is_closed with times, got %v", err)
		}
	})

	t.Run("invalid HH:MM format returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "9:00"
		close := "14:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for single-digit hour, got %v", err)
		}
	})

	t.Run("time with seconds returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "10:00:00"
		close := "14:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for time with seconds, got %v", err)
		}
	})

	t.Run("hour 25:00 returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "25:00"
		close := "26:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for hour=25, got %v", err)
		}
	})

	t.Run("minute 12:70 returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		open := "12:70"
		close := "14:00"
		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-24",
			IsClosed:      false,
			OpenTime:      &open,
			CloseTime:     &close,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for minute=70, got %v", err)
		}
	})

	t.Run("invalid calendar date 2026-13-45 returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-13-45",
			IsClosed:      true,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for invalid calendar date, got %v", err)
		}
	})

	t.Run("invalid calendar date 2026-02-30 returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-02-30",
			IsClosed:      true,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for Feb 30, got %v", err)
		}
	})

	t.Run("UNIQUE violation returns domain.ErrConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`INSERT INTO business_hours_exception`).
			WithArgs("2026-12-25", true, nil, nil, nil).
			WillReturnError(errors.New("UNIQUE constraint failed: business_hours_exception.exception_date"))

		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
		}
		err := repo.Create(adminCtx(), ex)
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("expected domain.ErrConflict, got %v", err)
		}
	})

	t.Run("non-UNIQUE DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`INSERT INTO business_hours_exception`).
			WithArgs("2026-12-25", true, nil, nil, nil).
			WillReturnError(errors.New("disk full"))

		ex := &entity.BusinessHoursException{
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
		}
		err := repo.Create(adminCtx(), ex)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, domain.ErrConflict) {
			t.Error("non-UNIQUE error should not return domain.ErrConflict")
		}
	})
}

func TestBusinessHoursExceptionRepo_Get(t *testing.T) {
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

		date := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
		ex, err := repo.Get(adminCtx(), date)
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

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date = \?`).
			WithArgs("2026-01-01").
			WillReturnError(sql.ErrNoRows)

		date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		_, err := repo.Get(adminCtx(), date)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date = \?`).
			WithArgs("2026-12-25").
			WillReturnError(errors.New("connection lost"))

		date := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
		_, err := repo.Get(adminCtx(), date)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBusinessHoursExceptionRepo_List(t *testing.T) {
	t.Run("returns exceptions in date range ordered by date", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "exception_date", "is_closed", "open_time", "close_time", "reason", "created_at",
		}).
			AddRow(1, "2026-12-24", false, strPtr("10:00"), strPtr("14:00"), strPtr("Nochebuena"),
				"2026-01-01T00:00:00.000Z").
			AddRow(2, "2026-12-25", true, nil, nil, strPtr("Navidad"),
				"2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date >= \? AND exception_date <= \? ORDER BY exception_date`).
			WithArgs("2026-12-01", "2026-12-31").
			WillReturnRows(rows)

		from := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		exceptions, err := repo.List(adminCtx(), from, to)
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
		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date >= \? AND exception_date <= \? ORDER BY exception_date`).
			WithArgs("2026-01-01", "2026-01-31").
			WillReturnRows(rows)

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
		exceptions, err := repo.List(adminCtx(), from, to)
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

		mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date >= \? AND exception_date <= \? ORDER BY exception_date`).
			WithArgs("2026-01-01", "2026-12-31").
			WillReturnError(errors.New("connection lost"))

		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		_, err := repo.List(adminCtx(), from, to)
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
			WithArgs(1).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(adminCtx(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`DELETE FROM business_hours_exception WHERE id = \?`).
			WithArgs(999).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(adminCtx(), 999)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBusinessHoursExceptionRepo(db)

		mock.ExpectExec(`DELETE FROM business_hours_exception WHERE id = \?`).
			WithArgs(1).
			WillReturnError(errors.New("disk full"))

		err := repo.Delete(adminCtx(), 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
