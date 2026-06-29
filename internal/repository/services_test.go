package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestServicesRepo_Create(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`INSERT INTO services`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &model.Service{
			ID:              "svc-1",
			Name:            "Corte",
			DurationMinutes: 30,
			Price:           500.0,
			IsActive:        true,
		}
		err := repo.Create(context.Background(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("zero duration returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &model.Service{Name: "Bad", DurationMinutes: 0}
		err := repo.Create(context.Background(), svc)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("negative duration returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &model.Service{Name: "Bad", DurationMinutes: -5}
		err := repo.Create(context.Background(), svc)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})
}

func TestServicesRepo_Get(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "description", "duration_minutes", "price",
			"is_active", "created_at", "updated_at",
		}).AddRow("svc-1", "Corte", strPtr("Corte clásico"), 30, 500.0,
			true, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM services WHERE id = \?`).
			WillReturnRows(rows)

		svc, err := repo.Get(context.Background(), "svc-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if svc.Name != "Corte" {
			t.Errorf("got Name=%q, want %q", svc.Name, "Corte")
		}
		if svc.DurationMinutes != 30 {
			t.Errorf("got DurationMinutes=%d, want %d", svc.DurationMinutes, 30)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM services WHERE id = \?`).
			WillReturnError(sql.ErrNoRows)

		_, err := repo.Get(context.Background(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestServicesRepo_ListActive(t *testing.T) {
	t.Run("returns only active services", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "description", "duration_minutes", "price",
			"is_active", "created_at", "updated_at",
		}).
			AddRow("svc-1", "Corte", nil, 30, 500.0, true, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z").
			AddRow("svc-2", "Color", nil, 60, 1500.0, true, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM services WHERE is_active = 1 ORDER BY name`).
			WillReturnRows(rows)

		services, err := repo.ListActive(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(services) != 2 {
			t.Fatalf("got %d services, want 2", len(services))
		}
		if services[0].Name != "Corte" {
			t.Errorf("got first=%q, want %q", services[0].Name, "Corte")
		}
	})
}

func TestServicesRepo_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`UPDATE services SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &model.Service{ID: "svc-1", Name: "Updated", DurationMinutes: 45, Price: 600.0, IsActive: true}
		err := repo.Update(context.Background(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`UPDATE services SET`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		svc := &model.Service{ID: "missing", Name: "Ghost", DurationMinutes: 30, Price: 0, IsActive: true}
		err := repo.Update(context.Background(), svc)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestServicesRepo_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`DELETE FROM services WHERE id = \?`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(context.Background(), "svc-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`DELETE FROM services WHERE id = \?`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(context.Background(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestServicesRepo_SearchFTS(t *testing.T) {
	t.Run("valid query returns ranked results", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "description", "duration_minutes", "price",
			"is_active", "created_at", "updated_at",
		}).
			AddRow("svc-1", "Corte", strPtr("Corte clásico"), 30, 500.0,
				true, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z").
			AddRow("svc-2", "Corte premium", strPtr("Corte + lavado"), 45, 800.0,
				true, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT s\.id[\s\S]+FROM services s[\s\S]+JOIN services_fts`).
			WillReturnRows(rows)

		results, err := repo.SearchFTS(context.Background(), "Corte")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("got %d results, want 2", len(results))
		}
		// Verify ordering is preserved from FTS rank
		if results[0].Name != "Corte" {
			t.Errorf("got first=%q, want %q", results[0].Name, "Corte")
		}
	})

	t.Run("empty query returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		_, err := repo.SearchFTS(context.Background(), "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for empty query, got %v", err)
		}
	})

	t.Run("query with forbidden chars returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		_, err := repo.SearchFTS(context.Background(), "corte* OR algo")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for query with *, got %v", err)
		}
	})
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }
