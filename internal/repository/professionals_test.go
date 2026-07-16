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

func TestProfessionalsRepo_Create(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		role := "Barbero"
		specs := `["svc-1","svc-2"]`

		// Mock service existence checks for specialties
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM services WHERE id = \?`).
			WithArgs("svc-2").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectExec(`INSERT INTO professionals`).
			WithArgs(sqlmock.AnyArg(), "Juan", &role, "active", nil, nil, &specs).
			WillReturnResult(sqlmock.NewResult(0, 1))

		p := &model.Professional{
			Name:          "Juan",
			RoleSpecialty: &role,
			Specialties:   &specs,
		}
		err := repo.Create(adminCtx(), p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ID == "" {
			t.Error("expected ID to be auto-assigned, got empty string")
		}
	})

	t.Run("empty name returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{Name: ""}
		err := repo.Create(adminCtx(), p)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("DB error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		mock.ExpectExec(`INSERT INTO professionals`).
			WithArgs(sqlmock.AnyArg(), "Juan", nil, "active", nil, nil, nil).
			WillReturnError(errors.New("disk full"))

		p := &model.Professional{Name: "Juan", Status: "active"}
		err := repo.Create(adminCtx(), p)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{Name: "Juan", Status: "active"}
		err := repo.Create(context.Background(), p)
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
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{Name: "Juan", Status: "active"}
		err := repo.Create(clientCtx("c-1"), p)
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
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{Name: "Juan", Status: "active"}
		err := repo.Create(staffCtx("pro-1"), p)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("owner role allowed", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		mock.ExpectExec(`INSERT INTO professionals`).
			WithArgs(sqlmock.AnyArg(), "Juan", nil, "active", nil, nil, nil).
			WillReturnResult(sqlmock.NewResult(0, 1))

		p := &model.Professional{Name: "Juan", Status: "active"}
		err := repo.Create(ownerCtx(), p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestProfessionalsRepo_Get(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		role := "Veterinario"
		specs := `["svc-1"]`
		rows := sqlmock.NewRows([]string{
			"id", "name", "role_specialty", "status", "email", "phone",
			"specialties", "created_at", "updated_at",
		}).AddRow("pro-1", "Juan", &role, "active", nil, nil,
			&specs, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM professionals WHERE id = \?`).
			WithArgs("pro-1").
			WillReturnRows(rows)

		p, err := repo.Get(adminCtx(), "pro-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name != "Juan" {
			t.Errorf("got Name=%q, want %q", p.Name, "Juan")
		}
		if p.Status != "active" {
			t.Errorf("got Status=%q, want %q", p.Status, "active")
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM professionals WHERE id = \?`).
			WithArgs("missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.Get(adminCtx(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("staff can get own professional record", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "role_specialty", "status", "email", "phone",
			"specialties", "created_at", "updated_at",
		}).AddRow("pro-1", "Juan", nil, "active", nil, nil, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM professionals WHERE id = \?`).
			WithArgs("pro-1").
			WillReturnRows(rows)

		p, err := repo.Get(staffCtx("pro-1"), "pro-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ID != "pro-1" {
			t.Errorf("got ID=%q, want %q", p.ID, "pro-1")
		}
	})

	t.Run("staff cannot get another professional record", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		_, err := repo.Get(staffCtx("pro-1"), "pro-999")
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		_, err := repo.Get(context.Background(), "pro-1")
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})
}

func TestProfessionalsRepo_GetActive(t *testing.T) {
	t.Run("returns only active professionals", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "role_specialty", "status", "email", "phone",
			"specialties", "created_at", "updated_at",
		}).
			AddRow("pro-1", "Juan", nil, "active", nil, nil, nil,
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z").
			AddRow("pro-2", "María", nil, "active", nil, nil, nil,
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM professionals WHERE status = .active. ORDER BY name`).
			WillReturnRows(rows)

		pros, err := repo.GetActive(adminCtx())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pros) != 2 {
			t.Fatalf("got %d professionals, want 2", len(pros))
		}
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "role_specialty", "status", "email", "phone",
			"specialties", "created_at", "updated_at",
		})
		mock.ExpectQuery(`SELECT .+ FROM professionals WHERE status = .active. ORDER BY name`).
			WillReturnRows(rows)

		pros, err := repo.GetActive(adminCtx())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pros) != 0 {
			t.Errorf("got %d professionals, want 0", len(pros))
		}
	})

	t.Run("staff sees only their own row", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "role_specialty", "status", "email", "phone",
			"specialties", "created_at", "updated_at",
		}).AddRow("pro-1", "Juan", nil, "active", nil, nil, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM professionals WHERE status = .active. AND id = \? ORDER BY name`).
			WithArgs("pro-1").
			WillReturnRows(rows)

		pros, err := repo.GetActive(staffCtx("pro-1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pros) != 1 {
			t.Fatalf("got %d professionals, want 1", len(pros))
		}
		if pros[0].ID != "pro-1" {
			t.Errorf("got ID=%q, want %q", pros[0].ID, "pro-1")
		}
	})

	t.Run("client can see all active professionals", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "role_specialty", "status", "email", "phone",
			"specialties", "created_at", "updated_at",
		}).
			AddRow("pro-1", "Juan", nil, "active", nil, nil, nil,
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM professionals WHERE status = .active. ORDER BY name`).
			WillReturnRows(rows)

		pros, err := repo.GetActive(clientCtx("c-1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pros) != 1 {
			t.Fatalf("got %d professionals, want 1", len(pros))
		}
	})
}

func TestProfessionalsRepo_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		role := "Estilista"
		mock.ExpectExec(`UPDATE professionals SET`).
			WithArgs("Updated", &role, "active", nil, nil, nil, "pro-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		p := &model.Professional{
			ID:            "pro-1",
			Name:          "Updated",
			RoleSpecialty: &role,
			Status:        "active",
		}
		err := repo.Update(adminCtx(), p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		mock.ExpectExec(`UPDATE professionals SET`).
			WithArgs("Ghost", nil, "active", nil, nil, nil, "missing").
			WillReturnResult(sqlmock.NewResult(0, 0))

		p := &model.Professional{ID: "missing", Name: "Ghost", Status: "active"}
		err := repo.Update(adminCtx(), p)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("empty name returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{ID: "pro-1", Name: "", Status: "active"}
		err := repo.Update(adminCtx(), p)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("invalid status returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{ID: "pro-1", Name: "Juan", Status: "invalid"}
		err := repo.Update(adminCtx(), p)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("specialty referencing non-existent service returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		specs := `["svc-999"]`
		// Mock the service existence check - returns count=0
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM services WHERE id = \?`).
			WithArgs("svc-999").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		p := &model.Professional{ID: "pro-1", Name: "Juan", Status: "active", Specialties: &specs}
		err := repo.Update(adminCtx(), p)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for non-existent service, got %v", err)
		}
	})

	t.Run("valid specialties pass validation", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		specs := `["svc-1","svc-2"]`
		// Mock service existence checks
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM services WHERE id = \?`).
			WithArgs("svc-2").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectExec(`UPDATE professionals SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		p := &model.Professional{ID: "pro-1", Name: "Juan", Status: "active", Specialties: &specs}
		err := repo.Update(adminCtx(), p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{ID: "pro-1", Name: "Juan", Status: "active"}
		err := repo.Update(context.Background(), p)
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
		repo := NewProfessionalsRepo(db)

		p := &model.Professional{ID: "pro-1", Name: "Juan", Status: "active"}
		err := repo.Update(clientCtx("c-1"), p)
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
