package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestClientsRepo_Create(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`INSERT INTO clients`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		c := &model.Client{ID: "cli-1", Name: "Juan", Phone: "+5491112345678"}
		err := repo.Create(context.Background(), c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("UNIQUE violation returns ErrConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`INSERT INTO clients`).
			WillReturnError(errors.New("UNIQUE constraint failed: clients.phone"))

		c := &model.Client{ID: "cli-2", Name: "Dup", Phone: "+5491112345678"}
		err := repo.Create(context.Background(), c)
		if !errors.Is(err, ErrConflict) {
			t.Errorf("expected ErrConflict, got %v", err)
		}
	})
}

func TestClientsRepo_Get(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", strPtr("juan@test.com"), nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE id = \?`).
			WillReturnRows(rows)

		c, err := repo.Get(context.Background(), "cli-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.Name != "Juan" {
			t.Errorf("got Name=%q, want %q", c.Name, "Juan")
		}
		if c.Phone != "+5491112345678" {
			t.Errorf("got Phone=%q, want %q", c.Phone, "+5491112345678")
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM clients WHERE id = \?`).
			WillReturnError(sql.ErrNoRows)

		_, err := repo.Get(context.Background(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestClientsRepo_GetByPhone(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", nil, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE phone = \?`).
			WillReturnRows(rows)

		c, err := repo.GetByPhone(context.Background(), "+5491112345678")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.ID != "cli-1" {
			t.Errorf("got ID=%q, want %q", c.ID, "cli-1")
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM clients WHERE phone = \?`).
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByPhone(context.Background(), "+0000000000")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestClientsRepo_GetOrCreate(t *testing.T) {
	t.Run("first call creates new client", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO clients`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", nil, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE phone = \?`).
			WillReturnRows(rows)

		c, err := repo.GetOrCreate(context.Background(), "+5491112345678", "Juan")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.ID != "cli-1" {
			t.Errorf("got ID=%q, want %q", c.ID, "cli-1")
		}
	})

	t.Run("second call returns existing client (idempotent)", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO clients`).
			WillReturnResult(sqlmock.NewResult(0, 0)) // already exists

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", nil, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE phone = \?`).
			WillReturnRows(rows)

		c, err := repo.GetOrCreate(context.Background(), "+5491112345678", "Juan Updated")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.Name != "Juan" {
			t.Errorf("got Name=%q, want %q (existing name preserved)", c.Name, "Juan")
		}
	})
}

func TestClientsRepo_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`UPDATE clients SET`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		c := &model.Client{ID: "cli-1", Name: "Updated", Phone: "+5491112345678"}
		err := repo.Update(context.Background(), c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`UPDATE clients SET`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		c := &model.Client{ID: "missing", Name: "Ghost", Phone: "+0000000000"}
		err := repo.Update(context.Background(), c)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("UNIQUE violation returns ErrConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`UPDATE clients SET`).
			WillReturnError(errors.New("UNIQUE constraint failed: clients.phone"))

		c := &model.Client{ID: "cli-1", Name: "Juan", Phone: "+5491199999999"}
		err := repo.Update(context.Background(), c)
		if !errors.Is(err, ErrConflict) {
			t.Errorf("expected ErrConflict, got %v", err)
		}
	})
}

func TestClientsRepo_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`DELETE FROM clients WHERE id = \?`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(context.Background(), "cli-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`DELETE FROM clients WHERE id = \?`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(context.Background(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestClientsRepo_SearchFTS(t *testing.T) {
	t.Run("valid query returns ranked results", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", nil, strPtr("alergia a penicilina"),
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT c\.id[\s\S]+FROM clients c[\s\S]+JOIN clients_fts`).
			WillReturnRows(rows)

		results, err := repo.SearchFTS(context.Background(), "alergia")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
		if results[0].Name != "Juan" {
			t.Errorf("got Name=%q, want %q", results[0].Name, "Juan")
		}
	})

	t.Run("empty query returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.SearchFTS(context.Background(), "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for empty query, got %v", err)
		}
	})

	t.Run("query with forbidden chars returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.SearchFTS(context.Background(), "juan* OR algo")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for query with *, got %v", err)
		}
	})
}
