package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestBusinessHoursExceptionRepo_Create_RequiresAdminOrOwner(t *testing.T) {
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
			repo := NewBusinessHoursExceptionRepo(db)
			if tc.want == nil {
				mock.ExpectExec(`INSERT INTO business_hours_exception`).
					WithArgs("2026-12-25", true, nil, nil, nil).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}
			ex := &entity.BusinessHoursException{ExceptionDate: "2026-12-25", IsClosed: true}
			err := repo.Create(tc.ctx, ex)
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBusinessHoursExceptionRepo_Delete_RequiresAdminOrOwner(t *testing.T) {
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
			repo := NewBusinessHoursExceptionRepo(db)
			if tc.want == nil {
				mock.ExpectExec(`DELETE FROM business_hours_exception WHERE id = \?`).
					WithArgs(1).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			err := repo.Delete(tc.ctx, 1)
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBusinessHoursExceptionRepo_Get_AnyAuthenticatedCaller(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"admin", adminCtx(), nil},
		{"owner", ownerCtx(), nil},
		{"staff", staffCtx("prof-1"), nil},
		{"client", clientCtx("cli-1"), nil},
		{"unauth", context.Background(), domain.ErrUnauthenticated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newMockDB(t)
			repo := NewBusinessHoursExceptionRepo(db)
			if tc.want == nil {
				rows := sqlmock.NewRows([]string{
					"id", "exception_date", "is_closed", "open_time", "close_time", "reason", "created_at",
				}).AddRow(1, "2026-12-25", true, nil, nil, nil, "2026-01-01T00:00:00.000Z")
				mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date = \?`).
					WithArgs("2026-12-25").
					WillReturnRows(rows)
			}
			_, err := repo.Get(tc.ctx, time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC))
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBusinessHoursExceptionRepo_List_AnyAuthenticatedCaller(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"admin", adminCtx(), nil},
		{"owner", ownerCtx(), nil},
		{"staff", staffCtx("prof-1"), nil},
		{"client", clientCtx("cli-1"), nil},
		{"unauth", context.Background(), domain.ErrUnauthenticated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newMockDB(t)
			repo := NewBusinessHoursExceptionRepo(db)
			if tc.want == nil {
				mock.ExpectQuery(`SELECT .+ FROM business_hours_exception WHERE exception_date >= \? AND exception_date <= \? ORDER BY exception_date`).
					WithArgs("2026-12-01", "2026-12-31").
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "exception_date", "is_closed", "open_time", "close_time", "reason", "created_at",
					}).AddRow(1, "2026-12-25", true, nil, nil, nil, "2026-01-01T00:00:00.000Z"))
			}
			from := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
			to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
			_, err := repo.List(tc.ctx, from, to)
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}
