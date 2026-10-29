package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestSchedulesRepo_FindByProfessionalAndDay(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "professional_id", "day_of_week", "start_time", "end_time",
		}).AddRow(1, "pro-1", 1, "09:00", "18:00")
		mock.ExpectQuery(`SELECT .+ FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 1).
			WillReturnRows(rows)

		schedule, err := repo.FindByProfessionalAndDay(adminCtx(), "pro-1", 1)
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

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 3).
			WillReturnError(sql.ErrNoRows)

		_, err := repo.FindByProfessionalAndDay(adminCtx(), "pro-1", 3)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("invalid day_of_week returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.FindByProfessionalAndDay(adminCtx(), "pro-1", 7)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for day_of_week=7, got %v", err)
		}
	})

	t.Run("negative day_of_week returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.FindByProfessionalAndDay(adminCtx(), "pro-1", -1)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for day_of_week=-1, got %v", err)
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

		schedule, err := repo.FindByProfessionalAndDay(staffCtx("pro-1"), "pro-999", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if schedule.ProfessionalID != "pro-1" {
			t.Errorf("got ProfessionalID=%q, want %q", schedule.ProfessionalID, "pro-1")
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.FindByProfessionalAndDay(context.Background(), "pro-1", 1)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("staff with nil ProfessionalID returns domain.ErrCodeUnauthenticated (fail-secure)", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		// Staff caller with nil ProfessionalID — must fail-secure, not fall through
		ctx := auth.WithCaller(context.Background(), auth.Caller{
			ID:             "staff-nil",
			Role:           auth.RoleStaff,
			ProfessionalID: nil,
		})
		_, err := repo.FindByProfessionalAndDay(ctx, "pro-1", 1)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
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
		schedule, err := repo.FindByProfessionalAndDay(adminCtx(), "pro-1", 1)
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

		// INSERT ... RETURNING: the write and the read-back are one statement, so
		// the row scanned is the one this call wrote.
		rows := sqlmock.NewRows([]string{
			"id", "professional_id", "day_of_week", "start_time", "end_time",
		}).AddRow(1, "pro-1", 1, "09:00", "18:00")
		mock.ExpectQuery(`INSERT INTO schedules`).
			WithArgs("pro-1", 1, "09:00", "18:00").
			WillReturnRows(rows)

		schedule := &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		stored, err := repo.Upsert(adminCtx(), schedule)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Pins the port contract: the adapter returns the persisted row (times as
		// passed in), not a bare error the caller would have to read back itself.
		if stored == nil || stored.ID != 1 || stored.ProfessionalID != "pro-1" ||
			stored.DayOfWeek != 1 || stored.StartTime != "09:00" || stored.EndTime != "18:00" {
			t.Errorf("stored = %+v; want the row returned by INSERT ... RETURNING", stored)
		}
	})

	t.Run("update existing schedule", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		// INSERT fails with UNIQUE violation
		mock.ExpectQuery(`INSERT INTO schedules`).
			WithArgs("pro-1", 1, "10:00", "19:00").
			WillReturnError(errors.New("UNIQUE constraint failed: schedules.professional_id, schedules.day_of_week"))

		// Then UPDATE succeeds; its RETURNING clause returns the updated row.
		rows := sqlmock.NewRows([]string{
			"id", "professional_id", "day_of_week", "start_time", "end_time",
		}).AddRow(1, "pro-1", 1, "10:00", "19:00")
		mock.ExpectQuery(`UPDATE schedules SET`).
			WithArgs("10:00", "19:00", "pro-1", 1).
			WillReturnRows(rows)

		schedule := &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "10:00",
			EndTime:        "19:00",
		}
		stored, err := repo.Upsert(adminCtx(), schedule)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stored == nil || stored.ID != 1 || stored.StartTime != "10:00" || stored.EndTime != "19:00" {
			t.Errorf("stored = %+v; want the row returned by UPDATE ... RETURNING", stored)
		}
	})

	t.Run("update on a row that vanished returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectQuery(`INSERT INTO schedules`).
			WithArgs("pro-1", 1, "10:00", "19:00").
			WillReturnError(errors.New("UNIQUE constraint failed: schedules.professional_id, schedules.day_of_week"))
		// UPDATE ... RETURNING matches no row -> sql.ErrNoRows (concurrent delete).
		mock.ExpectQuery(`UPDATE schedules SET`).
			WithArgs("10:00", "19:00", "pro-1", 1).
			WillReturnError(sql.ErrNoRows)

		stored, err := repo.Upsert(adminCtx(), &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "10:00",
			EndTime:        "19:00",
		})
		if stored != nil {
			t.Errorf("stored = %+v; want nil on a vanished row", stored)
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("foreign key violation returns domain.ErrConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		// schedules.professional_id references professionals(id) (schema.go), so a
		// slot for a professional that does not exist fails on INSERT. sqlmock
		// cannot build a *sqlite.Error, so this pins the message fallback of the
		// classifier.
		mock.ExpectQuery(`INSERT INTO schedules`).
			WithArgs("ghost", 1, "09:00", "13:00").
			WillReturnError(errors.New("FOREIGN KEY constraint failed"))

		_, err := repo.Upsert(adminCtx(), &entity.Schedule{
			ProfessionalID: "ghost",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "13:00",
		})
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("expected domain.ErrConflict, got %v", err)
		}
	})

	t.Run("non-constraint DB error propagates unclassified", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectQuery(`INSERT INTO schedules`).
			WithArgs("pro-1", 1, "09:00", "18:00").
			WillReturnError(errors.New("connection lost"))

		_, err := repo.Upsert(adminCtx(), &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "18:00",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, domain.ErrConflict) {
			t.Errorf("an unclassified failure must not become a conflict: %v", err)
		}
	})

	t.Run("invalid day_of_week returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      7,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		_, err := repo.Upsert(adminCtx(), schedule)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput, got %v", err)
		}
	})

	t.Run("invalid time format returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "9:00 AM",
			EndTime:        "18:00",
		}
		_, err := repo.Upsert(adminCtx(), schedule)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for malformed time, got %v", err)
		}
	})

	t.Run("start_time not before end_time returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "18:00",
			EndTime:        "09:00",
		}
		_, err := repo.Upsert(adminCtx(), schedule)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput when start >= end, got %v", err)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		schedule := &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		_, err := repo.Upsert(context.Background(), schedule)
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
		repo := NewSchedulesRepo(db)

		schedule := &entity.Schedule{
			ProfessionalID: "pro-1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "18:00",
		}
		_, err := repo.Upsert(clientCtx("c-1"), schedule)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
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

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectExec(`DELETE FROM schedules WHERE professional_id = \? AND day_of_week = \?`).
			WithArgs("pro-1", 3).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(adminCtx(), "pro-1", 3)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("invalid day_of_week returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		err := repo.Delete(adminCtx(), "pro-1", 7)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput, got %v", err)
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		err := repo.Delete(context.Background(), "pro-1", 1)
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
		repo := NewSchedulesRepo(db)

		err := repo.Delete(staffCtx("pro-1"), "pro-1", 1)
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

// ── S7b (feat-owner-chat): FindByProfessional ──

// TestSchedulesRepo_FindByProfessional pins the weekly read contract: rows come
// back in calendar order and the two "no data" cases stay distinguishable —
// an unknown professional is NOT_FOUND, a professional with no slot loaded is an
// empty list.
func TestSchedulesRepo_FindByProfessional(t *testing.T) {
	scheduleColumns := []string{"id", "professional_id", "day_of_week", "start_time", "end_time"}

	t.Run("returns the week ordered by day_of_week", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		rows := sqlmock.NewRows(scheduleColumns).
			AddRow(3, "pro-1", 1, "09:00", "13:00").
			AddRow(7, "pro-1", 3, "14:00", "18:00").
			AddRow(9, "pro-1", 6, "09:00", "12:00")
		// The ORDER BY lives in the SQL: the adapter must not leave the ordering
		// to the caller (design §8.3).
		mock.ExpectQuery(`SELECT id, professional_id, day_of_week, start_time, end_time\s+FROM schedules WHERE professional_id = \? ORDER BY day_of_week ASC`).
			WithArgs("pro-1").
			WillReturnRows(rows)

		schedules, err := repo.FindByProfessional(ownerCtx(), "pro-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(schedules) != 3 {
			t.Fatalf("got %d rows, want 3: %+v", len(schedules), schedules)
		}
		wantDays := []int{1, 3, 6}
		for i, want := range wantDays {
			if schedules[i].DayOfWeek != want {
				t.Errorf("row %d DayOfWeek = %d, want %d", i, schedules[i].DayOfWeek, want)
			}
		}
		if schedules[0].StartTime != "09:00" || schedules[0].EndTime != "13:00" {
			t.Errorf("row 0 = %+v; want 09:00-13:00", schedules[0])
		}
		if schedules[0].ProfessionalID != "pro-1" || schedules[0].ID != 3 {
			t.Errorf("row 0 identity = %+v; want id=3 pro-1", schedules[0])
		}
	})

	t.Run("professional without rows returns an empty list, not NOT_FOUND", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectQuery(`SELECT id, professional_id, day_of_week, start_time, end_time\s+FROM schedules WHERE professional_id = \? ORDER BY day_of_week ASC`).
			WithArgs("pro-2").
			WillReturnRows(sqlmock.NewRows(scheduleColumns))
		// The existence probe is what separates "no agenda" from "no such id".
		mock.ExpectQuery(`SELECT 1 FROM professionals WHERE id = \?`).
			WithArgs("pro-2").
			WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

		schedules, err := repo.FindByProfessional(ownerCtx(), "pro-2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if schedules == nil {
			t.Fatal("schedules = nil; want an empty (non-nil) slice")
		}
		if len(schedules) != 0 {
			t.Fatalf("got %d rows, want 0", len(schedules))
		}
	})

	t.Run("unknown professional_id returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectQuery(`SELECT id, professional_id, day_of_week, start_time, end_time\s+FROM schedules WHERE professional_id = \? ORDER BY day_of_week ASC`).
			WithArgs("pro-missing").
			WillReturnRows(sqlmock.NewRows(scheduleColumns))
		mock.ExpectQuery(`SELECT 1 FROM professionals WHERE id = \?`).
			WithArgs("pro-missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.FindByProfessional(ownerCtx(), "pro-missing")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("row scan error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewSchedulesRepo(db)

		mock.ExpectQuery(`SELECT id, professional_id, day_of_week, start_time, end_time\s+FROM schedules WHERE professional_id = \? ORDER BY day_of_week ASC`).
			WithArgs("pro-1").
			WillReturnError(errors.New("disk I/O error"))

		_, err := repo.FindByProfessional(ownerCtx(), "pro-1")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if errors.Is(err, domain.ErrNotFound) {
			t.Errorf("infrastructure failure collapsed into NOT_FOUND: %v", err)
		}
	})

	// ── TRIANGULATE ──

	t.Run("admin caller is rejected with ErrCodeForbidden", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.FindByProfessional(adminCtx(), "pro-1")
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewSchedulesRepo(db)

		_, err := repo.FindByProfessional(context.Background(), "pro-1")
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
	})
}

// TestSchedulesRepo_UpsertNilSlot pins the fail-closed guard added after the
// GGA round on the S7b slice: a nil slot is an invalid input, never a panic.
func TestSchedulesRepo_UpsertNilSlot(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewSchedulesRepo(db)

	_, err := repo.Upsert(adminCtx(), nil)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected domain.ErrInvalidInput for a nil slot, got %v", err)
	}
}

// TestSchedulesRepo_FindByProfessionalOrdersByDayOfWeekOnRealSQLite pins the
// ORDER BY day_of_week contract against a real SQLite database (B15 A1):
// the sqlmock suite only proves the query LETTER, so a dropped or inverted
// ordering clause would stay green there as long as the SQL text still matched.
// This case seeds four slots whose insertion order runs backwards across the
// calendar (Wednesday, Friday, Sunday, Monday) and requires the stored agenda
// through the public port to come back Sunday-first (0..6).
func TestSchedulesRepo_FindByProfessionalOrdersByDayOfWeekOnRealSQLite(t *testing.T) {
	ctx := context.Background()
	realDB, err := db.NewDatabase(ctx, filepath.Join(t.TempDir(), "appointments.db"))
	if err != nil {
		t.Fatalf("open real sqlite: %v", err)
	}
	t.Cleanup(func() { _ = realDB.Close() })
	db := realDB.Conn

	probe := &entity.Professional{Name: "SQLITE Order Probe"}
	if err := NewProfessionalsRepo(db).Save(adminCtx(), probe); err != nil {
		t.Fatalf("seed professional: %v", err)
	}

	seeded := []*entity.Schedule{
		{ProfessionalID: probe.ID, DayOfWeek: 3, StartTime: "09:00", EndTime: "17:00"}, // Wednesday
		{ProfessionalID: probe.ID, DayOfWeek: 5, StartTime: "09:00", EndTime: "13:00"}, // Friday
		{ProfessionalID: probe.ID, DayOfWeek: 0, StartTime: "10:00", EndTime: "18:00"}, // Sunday
		{ProfessionalID: probe.ID, DayOfWeek: 1, StartTime: "08:00", EndTime: "12:00"}, // Monday
	}
	repo := NewSchedulesRepo(db)
	for i, slot := range seeded {
		if _, err := repo.Upsert(adminCtx(), slot); err != nil {
			t.Fatalf("seed slot %d (day %d): %v", i, slot.DayOfWeek, err)
		}
	}

	agenda, err := repo.FindByProfessional(ownerCtx(), probe.ID)
	if err != nil {
		t.Fatalf("FindByProfessional: %v", err)
	}
	wantOrder := []int{0, 1, 3, 5}
	if len(agenda) != len(wantOrder) {
		t.Fatalf("agenda = %d rows, want %d", len(agenda), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got := agenda[i].DayOfWeek; got != want {
			t.Errorf("agenda[%d] = day %d, want day %d (calendar order 0..6)", i, got, want)
		}
	}
}
