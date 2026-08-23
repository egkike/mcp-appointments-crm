package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestClientsRepo_GetOrCreate_Roles(t *testing.T) {
	t.Run("admin unrestricted", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO clients`).
			WithArgs(sqlmock.AnyArg(), "Juan", "+5491112345678").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE phone = \?`).
			WithArgs("+5491112345678").
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
			}).AddRow("cli-1", "Juan", "+5491112345678", nil, nil,
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z"))

		c, err := repo.GetOrCreate(adminCtx(), "+5491112345678", "Juan")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.ID != "cli-1" {
			t.Errorf("got ID=%q, want cli-1", c.ID)
		}
	})

	t.Run("client own-phone success", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewClientsRepo(db)

		mock.ExpectExec(`INSERT OR IGNORE INTO clients`).
			WithArgs(sqlmock.AnyArg(), "Juan", "+5491112345678").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT .+ FROM clients WHERE phone = \?`).
			WithArgs("+5491112345678").
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
			}).AddRow("cli-1", "Juan", "+5491112345678", nil, nil,
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z"))

		c, err := repo.GetOrCreate(clientCtx("+5491112345678"), "+5491112345678", "Juan")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.ID != "cli-1" {
			t.Errorf("got ID=%q, want cli-1", c.ID)
		}
	})

	t.Run("client foreign phone rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.GetOrCreate(clientCtx("cli-1"), "+5491199999999", "Juan")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("staff rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.GetOrCreate(staffCtx("prof-1"), "+5491112345678", "Juan")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("unauth rejected", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewClientsRepo(db)

		_, err := repo.GetOrCreate(context.Background(), "+5491112345678", "Juan")
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("got %v, want ErrUnauthenticated", err)
		}
	})
}
