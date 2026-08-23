package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestClientsRepo_FindByID_Scoped(t *testing.T) {
	t.Run("admin reads any client", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-2", "Maria", "+5491199999999", nil, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE id = \?`).
			WithArgs("cli-2").
			WillReturnRows(rows)

		c, err := repo.FindByID(adminCtx(), "cli-2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.ID != "cli-2" {
			t.Errorf("got ID=%q, want cli-2", c.ID)
		}
	})

	t.Run("client reads own row", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", nil, strPtr("alergia"),
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE id = \? AND id = \?`).
			WithArgs("cli-1", "cli-1").
			WillReturnRows(rows)

		c, err := repo.FindByID(clientCtx("cli-1"), "cli-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.Preferences == nil || *c.Preferences != "alergia" {
			t.Errorf("got preferences=%v, want alergia", c.Preferences)
		}
	})

	t.Run("client cross-tenant collapses to ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM clients WHERE id = \? AND id = \?`).
			WithArgs("cli-2", "cli-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.FindByID(clientCtx("cli-1"), "cli-2")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("client missing id collapses to ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM clients WHERE id = \? AND id = \?`).
			WithArgs("missing", "cli-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.FindByID(clientCtx("cli-1"), "missing")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("staff rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.FindByID(staffCtx("prof-1"), "cli-1")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("unauth rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.FindByID(context.Background(), "cli-1")
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("got %v, want ErrUnauthenticated", err)
		}
	})
}

func TestClientsRepo_SearchFTS_Scoped(t *testing.T) {
	t.Run("admin returns full ranked results", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", nil, strPtr("alergia"),
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z").
			AddRow("cli-2", "Maria", "+5491199999999", nil, strPtr("alergia"),
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT c\.id[\s\S]+FROM clients c[\s\S]+JOIN clients_fts`).
			WithArgs("alergia").
			WillReturnRows(rows)

		results, err := repo.SearchFTS(adminCtx(), "alergia")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2", len(results))
		}
	})

	t.Run("client returns only own row", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
		}).AddRow("cli-1", "Juan", "+5491112345678", nil, strPtr("alergia"),
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
		mock.ExpectQuery(`SELECT c\.id[\s\S]+FROM clients c[\s\S]+JOIN clients_fts[\s\S]+AND id = \?`).
			WithArgs("alergia", "cli-1").
			WillReturnRows(rows)

		results, err := repo.SearchFTS(clientCtx("cli-1"), "alergia")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 || results[0].ID != "cli-1" {
			t.Errorf("got %v", results)
		}
	})

	t.Run("client no own match returns empty", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectQuery(`SELECT c\.id[\s\S]+FROM clients c[\s\S]+JOIN clients_fts[\s\S]+AND id = \?`).
			WithArgs("alergia", "cli-1").
			WillReturnRows(sqlmock.NewRows(nil))

		results, err := repo.SearchFTS(clientCtx("cli-1"), "alergia")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("got %d results, want 0", len(results))
		}
	})

	t.Run("staff rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.SearchFTS(staffCtx("prof-1"), "alergia")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("unauth rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.SearchFTS(context.Background(), "alergia")
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("got %v, want ErrUnauthenticated", err)
		}
	})
}
