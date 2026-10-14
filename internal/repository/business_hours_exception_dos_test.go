package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestBusinessHoursExceptionRepo_Create_DoSPlantingBlocked(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"staff", staffCtx("prof-1")},
		{"client", clientCtx("cli-1")},
		{"unauth", context.Background()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := newMockDB(t)
			repo := NewBusinessHoursExceptionRepo(db)
			ex := &entity.BusinessHoursException{
				ExceptionDate: "9999-12-31",
				IsClosed:      true,
			}
			_, err := repo.Create(tc.ctx, ex)
			if !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrUnauthenticated) {
				t.Errorf("got %v, want ErrForbidden or ErrUnauthenticated", err)
			}
		})
	}
}

func TestBusinessHoursExceptionRepo_Create_AdminCanPlantClosedException(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBusinessHoursExceptionRepo(db)

	mock.ExpectQuery(`INSERT INTO business_hours_exception`).
		WithArgs("9999-12-31", true, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "exception_date", "is_closed", "open_time", "close_time", "reason", "created_at"}).
			AddRow(1, "9999-12-31", true, nil, nil, nil, "2026-10-14T12:00:00Z"))

	ex := &entity.BusinessHoursException{ExceptionDate: "9999-12-31", IsClosed: true}
	if _, err := repo.Create(adminCtx(), ex); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
