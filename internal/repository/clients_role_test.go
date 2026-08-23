package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestClientsRepo_Writes_RequireAdminOrOwner(t *testing.T) {
	client := &entity.Client{ID: "cli-1", Name: "Juan", Phone: "+5491112345678"}

	cases := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"admin", adminCtx(), nil},
		{"owner", ownerCtx(), nil},
		{"staff", staffCtx("prof-1"), domain.ErrForbidden},
		{"client", clientCtx("cli-1"), domain.ErrForbidden},
		{"unauth", context.Background(), domain.ErrUnauthenticated},
	}

	t.Run("Save", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				db, mock := newMockDB(t)
				repo := NewClientsRepo(db)
				if tc.want == nil {
					mock.ExpectExec(`INSERT OR REPLACE INTO clients`).
						WithArgs("cli-1", "Juan", "+5491112345678", nil, nil).
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
				err := repo.Save(tc.ctx, client)
				if !errors.Is(err, tc.want) {
					t.Errorf("got %v, want %v", err, tc.want)
				}
			})
		}
	})

	t.Run("Create", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				db, mock := newMockDB(t)
				repo := NewClientsRepo(db)
				if tc.want == nil {
					mock.ExpectExec(`INSERT INTO clients`).
						WithArgs("cli-1", "Juan", "+5491112345678", nil, nil).
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
				err := repo.Create(tc.ctx, client)
				if !errors.Is(err, tc.want) {
					t.Errorf("got %v, want %v", err, tc.want)
				}
			})
		}
	})

	t.Run("Update", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				db, mock := newMockDB(t)
				repo := NewClientsRepo(db)
				if tc.want == nil {
					mock.ExpectExec(`UPDATE clients SET`).
						WithArgs("Juan", "+5491112345678", nil, nil, "cli-1").
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
				err := repo.Update(tc.ctx, client)
				if !errors.Is(err, tc.want) {
					t.Errorf("got %v, want %v", err, tc.want)
				}
			})
		}
	})

	t.Run("Delete", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				db, mock := newMockDB(t)
				repo := NewClientsRepo(db)
				if tc.want == nil {
					mock.ExpectExec(`DELETE FROM clients WHERE id = \?`).
						WithArgs("cli-1").
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
				err := repo.Delete(tc.ctx, "cli-1")
				if !errors.Is(err, tc.want) {
					t.Errorf("got %v, want %v", err, tc.want)
				}
			})
		}
	})
}

func TestClientsRepo_FindByPhone_RequiresAdminOrOwner(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"admin", adminCtx(), nil},
		{"owner", ownerCtx(), nil},
		{"staff", staffCtx("prof-1"), domain.ErrForbidden},
		{"client", clientCtx("cli-1"), domain.ErrForbidden},
		{"unauth", context.Background(), domain.ErrUnauthenticated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newMockDB(t)
			repo := NewClientsRepo(db)
			if tc.want == nil {
				rows := sqlmock.NewRows([]string{
					"id", "name", "phone", "email", "preferences", "created_at", "updated_at",
				}).AddRow("cli-1", "Juan", "+5491112345678", nil, nil,
					"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
				mock.ExpectQuery(`SELECT .+ FROM clients WHERE phone = \?`).
					WithArgs("+5491112345678").
					WillReturnRows(rows)
			}
			_, err := repo.FindByPhone(tc.ctx, "+5491112345678")
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}
