package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestServicesRepo_Save(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		desc := "Corte clásico"
		mock.ExpectExec(`INSERT INTO services`).
			WithArgs("svc-1", "Corte", &desc, 30, 500.0, true).
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &entity.Service{
			ID:              "svc-1",
			Name:            "Corte",
			Description:     &desc,
			DurationMinutes: 30,
			Price:           500.0,
			Active:          true,
		}
		err := repo.Save(adminCtx(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("zero duration returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "Bad", DurationMinutes: 0}
		err := repo.Save(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput, got %v", err)
		}
	})

	t.Run("negative duration returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "Bad", DurationMinutes: -5}
		err := repo.Save(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput, got %v", err)
		}
	})

	t.Run("empty name returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "", DurationMinutes: 30}
		err := repo.Save(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for empty name, got %v", err)
		}
	})

	t.Run("zero price returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "Test", DurationMinutes: 30, Price: 0}
		err := repo.Save(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for zero price, got %v", err)
		}
	})

	t.Run("negative price returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "Test", DurationMinutes: 30, Price: -100}
		err := repo.Save(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for negative price, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`INSERT INTO services`).
			WithArgs("svc-1", "Corte", nil, 30, 500.0, true).
			WillReturnError(errors.New("disk full"))

		svc := &entity.Service{ID: "svc-1", Name: "Corte", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Save(adminCtx(), svc)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "Test", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Save(context.Background(), svc)
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
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "Test", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Save(clientCtx("c-1"), svc)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("staff role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{Name: "Test", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Save(staffCtx("pro-1"), svc)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("admin role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`INSERT INTO services`).
			WithArgs(sqlmock.AnyArg(), "Test", nil, 30, 500.0, true).
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &entity.Service{ID: "svc-admin", Name: "Test", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Save(adminCtx(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`INSERT INTO services`).
			WithArgs(sqlmock.AnyArg(), "Test", nil, 30, 500.0, true).
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &entity.Service{ID: "svc-owner", Name: "Test", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Save(ownerCtx(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestServicesRepo_FindByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "description", "duration_minutes", "price",
			"is_active", "created_at", "updated_at",
		}).AddRow("svc-1", "Corte", strPtr("Corte clásico"), 30, 500.0,
			true, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(rows)

		svc, err := repo.FindByID(context.Background(), "svc-1")
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

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM services WHERE id = \?`).
			WithArgs("missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.FindByID(context.Background(), "missing")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnError(errors.New("connection lost"))

		_, err := repo.FindByID(context.Background(), "svc-1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestServicesRepo_FindActive(t *testing.T) {
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

		services, err := repo.FindActive(context.Background())
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

	t.Run("empty result returns nil slice", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "description", "duration_minutes", "price",
			"is_active", "created_at", "updated_at",
		})
		mock.ExpectQuery(`SELECT .+ FROM services WHERE is_active = 1 ORDER BY name`).
			WillReturnRows(rows)

		services, err := repo.FindActive(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(services) != 0 {
			t.Errorf("got %d services, want 0", len(services))
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM services WHERE is_active = 1 ORDER BY name`).
			WillReturnError(errors.New("connection lost"))

		_, err := repo.FindActive(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestServicesRepo_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`UPDATE services SET`).
			WithArgs("Updated", nil, 45, 600.0, true, "svc-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &entity.Service{ID: "svc-1", Name: "Updated", DurationMinutes: 45, Price: 600.0, Active: true}
		err := repo.Update(adminCtx(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`UPDATE services SET`).
			WithArgs("Ghost", nil, 30, 500.0, true, "missing").
			WillReturnResult(sqlmock.NewResult(0, 0))

		svc := &entity.Service{ID: "missing", Name: "Ghost", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Update(adminCtx(), svc)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("empty name returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{ID: "svc-1", Name: "", DurationMinutes: 30, Price: 500.0}
		err := repo.Update(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput, got %v", err)
		}
	})

	t.Run("zero duration returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 0, Price: 500.0}
		err := repo.Update(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput, got %v", err)
		}
	})

	t.Run("zero price returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 30, Price: 0}
		err := repo.Update(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for zero price, got %v", err)
		}
	})

	t.Run("negative price returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 30, Price: -1}
		err := repo.Update(adminCtx(), svc)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`UPDATE services SET`).
			WithArgs("Updated", nil, 30, 500.0, true, "svc-1").
			WillReturnError(errors.New("disk full"))

		svc := &entity.Service{ID: "svc-1", Name: "Updated", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Update(adminCtx(), svc)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 30, Price: 500.0}
		err := repo.Update(context.Background(), svc)
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
		repo := NewServicesRepo(db)

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 30, Price: 500.0}
		err := repo.Update(clientCtx("c-1"), svc)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("staff role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 30, Price: 500.0}
		err := repo.Update(staffCtx("pro-1"), svc)
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("admin role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`UPDATE services SET`).
			WithArgs("Test", nil, 30, 500.0, true, "svc-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Update(adminCtx(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`UPDATE services SET`).
			WithArgs("Test", nil, 30, 500.0, true, "svc-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		svc := &entity.Service{ID: "svc-1", Name: "Test", DurationMinutes: 30, Price: 500.0, Active: true}
		err := repo.Update(ownerCtx(), svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestServicesRepo_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`DELETE FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(adminCtx(), "svc-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns domain.ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`DELETE FROM services WHERE id = \?`).
			WithArgs("missing").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(adminCtx(), "missing")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected domain.ErrNotFound, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`DELETE FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnError(errors.New("connection lost"))

		err := repo.Delete(adminCtx(), "svc-1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("no caller returns domain.ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		err := repo.Delete(context.Background(), "svc-1")
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
		repo := NewServicesRepo(db)

		err := repo.Delete(clientCtx("c-1"), "svc-1")
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("staff role rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		err := repo.Delete(staffCtx("pro-1"), "svc-1")
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeForbidden {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeForbidden)
		}
	})

	t.Run("admin role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`DELETE FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(adminCtx(), "svc-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectExec(`DELETE FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(ownerCtx(), "svc-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
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
			WithArgs("Corte").
			WillReturnRows(rows)

		results, err := repo.SearchFTS(adminCtx(), "Corte")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("got %d results, want 2", len(results))
		}
		if results[0].Name != "Corte" {
			t.Errorf("got first=%q, want %q", results[0].Name, "Corte")
		}
	})

	t.Run("accented query is accepted", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "description", "duration_minutes", "price",
			"is_active", "created_at", "updated_at",
		}).AddRow("svc-1", "María", nil, 30, 100.0,
			true, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT s\.id[\s\S]+FROM services s[\s\S]+JOIN services_fts`).
			WithArgs("María").
			WillReturnRows(rows)

		results, err := repo.SearchFTS(adminCtx(), "María")
		if err != nil {
			t.Fatalf("expected no error for accented query, got: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
	})

	t.Run("empty query returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		_, err := repo.SearchFTS(adminCtx(), "")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for empty query, got %v", err)
		}
	})

	t.Run("query with forbidden chars returns domain.ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewServicesRepo(db)

		_, err := repo.SearchFTS(adminCtx(), "corte* OR algo")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected domain.ErrInvalidInput for query with *, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewServicesRepo(db)

		mock.ExpectQuery(`SELECT s\.id[\s\S]+FROM services s[\s\S]+JOIN services_fts`).
			WithArgs("corte").
			WillReturnError(errors.New("FTS5 corrupt"))

		_, err := repo.SearchFTS(adminCtx(), "corte")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }

// Verify that auth.Caller is used (prevent unused import)
var _ = auth.RoleAdmin
