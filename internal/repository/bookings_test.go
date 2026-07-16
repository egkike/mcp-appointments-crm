package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

func TestBookingsRepo_CreateBooking(t *testing.T) {
	t.Run("successful insert with no overlap", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Mock service duration lookup
		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		// Mock atomic INSERT with overlap check
		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		notes := "Test booking"
		payment := "efectivo"
		input := &CreateBookingInput{
			ClientID:       "c-1",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
			Notes:          &notes,
			PaymentMethod:  &payment,
		}

		result, err := repo.CreateBooking(adminCtx(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Booking == nil {
			t.Fatal("expected non-nil booking")
		}
		if result.Booking.ID == "" {
			t.Error("expected ID to be auto-assigned")
		}
		// end_datetime should be start + 30 minutes
		if result.Booking.EndDatetime != "2026-07-13T13:30:00.000Z" {
			t.Errorf("got EndDatetime=%q, want %q", result.Booking.EndDatetime, "2026-07-13T13:30:00.000Z")
		}
		if result.Booking.Status != model.BookingStatusPending {
			t.Errorf("got Status=%q, want %q", result.Booking.Status, model.BookingStatusPending)
		}
	})

	t.Run("overlap returns SemanticError with ErrCodeBookingOverlap", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Mock service duration lookup
		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		// Mock atomic INSERT returning 0 rows (overlap detected)
		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		input := &CreateBookingInput{
			ClientID:       "c-1",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
		}

		_, err := repo.CreateBooking(adminCtx(), input)
		if err == nil {
			t.Fatal("expected error for overlap, got nil")
		}

		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeBookingOverlap {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeBookingOverlap)
		}
	})

	t.Run("service not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Mock service duration lookup returning no rows
		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-bogus").
			WillReturnError(sql.ErrNoRows)

		input := &CreateBookingInput{
			ClientID:       "c-1",
			ProfessionalID: "p-1",
			ServiceID:      "svc-bogus",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
		}

		_, err := repo.CreateBooking(adminCtx(), input)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("empty start_datetime returns ErrInvalidInput", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		input := &CreateBookingInput{
			ClientID:       "c-1",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "",
		}

		_, err := repo.CreateBooking(adminCtx(), input)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("client creating own booking passes", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))
		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		input := &CreateBookingInput{
			ClientID:       "c-1",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
		}

		_, err := repo.CreateBooking(clientCtx("c-1"), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("client creating for another client fails", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		input := &CreateBookingInput{
			ClientID:       "c-999",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
		}

		_, err := repo.CreateBooking(clientCtx("c-1"), input)
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
		repo := NewBookingsRepo(db)

		input := &CreateBookingInput{
			ClientID:       "c-1",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
		}

		_, err := repo.CreateBooking(context.Background(), input)
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("staff can create on behalf of client", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))
		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		input := &CreateBookingInput{
			ClientID:       "c-1",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
		}

		_, err := repo.CreateBooking(staffCtx("p-1"), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestBookingsRepo_GetBooking(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		notes := "Test"
		rows := sqlmock.NewRows([]string{
			"id", "client_id", "professional_id", "service_id",
			"start_datetime", "end_datetime", "status", "notes", "payment_method",
			"created_at", "updated_at",
		}).AddRow("b-1", "c-1", "p-1", "svc-1",
			"2026-07-13T13:00:00.000Z", "2026-07-13T13:30:00.000Z",
			"pending", &notes, nil,
			"2026-07-13T12:00:00.000Z", "2026-07-13T12:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(rows)

		booking, err := repo.GetBooking(adminCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if booking.ID != "b-1" {
			t.Errorf("got ID=%q, want %q", booking.ID, "b-1")
		}
		if booking.Status != model.BookingStatusPending {
			t.Errorf("got Status=%q, want %q", booking.Status, model.BookingStatusPending)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \?`).
			WithArgs("b-bogus").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetBooking(adminCtx(), "b-bogus")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("client can see own booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "client_id", "professional_id", "service_id",
			"start_datetime", "end_datetime", "status", "notes", "payment_method",
			"created_at", "updated_at",
		}).AddRow("b-1", "c-1", "p-1", "svc-1",
			"2026-07-13T13:00:00.000Z", "2026-07-13T13:30:00.000Z",
			"pending", nil, nil,
			"2026-07-13T12:00:00.000Z", "2026-07-13T12:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnRows(rows)

		booking, err := repo.GetBooking(clientCtx("c-1"), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if booking.ClientID != "c-1" {
			t.Errorf("got ClientID=%q, want %q", booking.ClientID, "c-1")
		}
	})

	t.Run("client cannot see another clients booking returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: client query includes client_id filter, so no row matches
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetBooking(clientCtx("c-1"), "b-1")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for cross-tenant, got %v", err)
		}
	})

	t.Run("staff can see own professional booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "client_id", "professional_id", "service_id",
			"start_datetime", "end_datetime", "status", "notes", "payment_method",
			"created_at", "updated_at",
		}).AddRow("b-1", "c-1", "p-1", "svc-1",
			"2026-07-13T13:00:00.000Z", "2026-07-13T13:30:00.000Z",
			"pending", nil, nil,
			"2026-07-13T12:00:00.000Z", "2026-07-13T12:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \? AND professional_id = \?`).
			WithArgs("b-1", "p-1").
			WillReturnRows(rows)

		booking, err := repo.GetBooking(staffCtx("p-1"), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if booking.ProfessionalID != "p-1" {
			t.Errorf("got ProfessionalID=%q, want %q", booking.ProfessionalID, "p-1")
		}
	})

	t.Run("staff cannot see another professionals booking returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: staff query includes professional_id filter, so no row matches
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \? AND professional_id = \?`).
			WithArgs("b-1", "p-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetBooking(staffCtx("p-1"), "b-1")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for cross-tenant staff, got %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		_, err := repo.GetBooking(context.Background(), "b-1")
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})
}

func TestBookingsRepo_CancelBooking(t *testing.T) {
	t.Run("pending to cancelled", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: admin sees all (no extra filter)
		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.CancelBooking(adminCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("confirmed to cancelled", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("confirmed"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.CancelBooking(adminCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cancelled to cancelled returns error", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("cancelled"))

		err := repo.CancelBooking(adminCtx(), "b-1")
		if err == nil {
			t.Fatal("expected error for cancelled→cancelled transition, got nil")
		}
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T", err)
		}
		if sErr.Code != ErrCodeInvalidInput {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeInvalidInput)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-bogus").
			WillReturnError(sql.ErrNoRows)

		err := repo.CancelBooking(adminCtx(), "b-bogus")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("client cannot cancel another clients booking returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: client query includes client_id filter → no row matches
		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnError(sql.ErrNoRows)

		err := repo.CancelBooking(clientCtx("c-1"), "b-1")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for cross-tenant client, got %v", err)
		}
	})

	t.Run("client can cancel own booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.CancelBooking(clientCtx("c-1"), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("staff cannot cancel another professionals booking returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: staff query includes professional_id filter → no row matches
		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \? AND professional_id = \?`).
			WithArgs("b-1", "p-1").
			WillReturnError(sql.ErrNoRows)

		err := repo.CancelBooking(staffCtx("p-1"), "b-1")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for cross-tenant staff, got %v", err)
		}
	})

	t.Run("staff can cancel own professional booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \? AND professional_id = \?`).
			WithArgs("b-1", "p-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.CancelBooking(staffCtx("p-1"), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		err := repo.CancelBooking(context.Background(), "b-1")
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})
}

func TestBookingsRepo_RescheduleBooking(t *testing.T) {
	t.Run("successful reschedule with no overlap", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: admin sees all
		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		mock.ExpectExec(`UPDATE bookings SET start_datetime = \?, end_datetime = \?.*WHERE id = \?.*AND NOT EXISTS`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.RescheduleBooking(adminCtx(), "b-1", "2026-07-13T14:00:00.000Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("atomic overlap returns SemanticError with ErrCodeBookingOverlap", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		// Atomic UPDATE returning 0 rows (overlap detected)
		mock.ExpectExec(`UPDATE bookings SET start_datetime = \?, end_datetime = \?.*WHERE id = \?.*AND NOT EXISTS`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Fix 3: re-query status to disambiguate — status is still pending → overlap
		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		err := repo.RescheduleBooking(adminCtx(), "b-1", "2026-07-13T14:00:00.000Z")
		if err == nil {
			t.Fatal("expected error for overlap, got nil")
		}
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T", err)
		}
		if sErr.Code != ErrCodeBookingOverlap {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeBookingOverlap)
		}
	})

	t.Run("cancelled booking cannot be rescheduled", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "cancelled", "p-1"))

		err := repo.RescheduleBooking(adminCtx(), "b-1", "2026-07-13T14:00:00.000Z")
		if err == nil {
			t.Fatal("expected error for rescheduling cancelled booking, got nil")
		}
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T", err)
		}
		if sErr.Code != ErrCodeInvalidInput {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeInvalidInput)
		}
	})

	t.Run("client cannot reschedule another clients booking returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: client query includes client_id filter → no row matches
		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnError(sql.ErrNoRows)

		err := repo.RescheduleBooking(clientCtx("c-1"), "b-1", "2026-07-13T14:00:00.000Z")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for cross-tenant client, got %v", err)
		}
	})

	t.Run("client can reschedule own booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \? AND client_id = \?`).
			WithArgs("b-1", "c-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		mock.ExpectExec(`UPDATE bookings SET start_datetime = \?, end_datetime = \?.*WHERE id = \?.*AND NOT EXISTS`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.RescheduleBooking(clientCtx("c-1"), "b-1", "2026-07-13T14:00:00.000Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("staff cannot reschedule another professionals booking returns ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Dynamic WHERE: staff query includes professional_id filter → no row matches
		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \? AND professional_id = \?`).
			WithArgs("b-1", "p-1").
			WillReturnError(sql.ErrNoRows)

		err := repo.RescheduleBooking(staffCtx("p-1"), "b-1", "2026-07-13T14:00:00.000Z")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for cross-tenant staff, got %v", err)
		}
	})

	t.Run("staff can reschedule own professional booking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \? AND professional_id = \?`).
			WithArgs("b-1", "p-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		mock.ExpectExec(`UPDATE bookings SET start_datetime = \?, end_datetime = \?.*WHERE id = \?.*AND NOT EXISTS`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.RescheduleBooking(staffCtx("p-1"), "b-1", "2026-07-13T14:00:00.000Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no caller returns ErrCodeUnauthenticated", func(t *testing.T) {
		db, _ := newMockDB(t)
		repo := NewBookingsRepo(db)

		err := repo.RescheduleBooking(context.Background(), "b-1", "2026-07-13T14:00:00.000Z")
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
	})

	t.Run("empty newStartDatetime returns ErrInvalidInput", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		err := repo.RescheduleBooking(adminCtx(), "b-1", "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for empty datetime, got %v", err)
		}
	})

	t.Run("bogus newStartDatetime returns ErrInvalidInput", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		err := repo.RescheduleBooking(adminCtx(), "b-1", "bogus-datetime")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for bogus datetime, got %v", err)
		}
	})

	t.Run("invalid month newStartDatetime returns ErrInvalidInput", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		err := repo.RescheduleBooking(adminCtx(), "b-1", "2026-13-45T99:99:99Z")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for invalid datetime, got %v", err)
		}
	})

	// Fix 3: RISK-V2-3 / REL-V2-1 / RES-V2-2 — disambiguate rowsAffected==0
	t.Run("concurrent cancellation between SELECT and UPDATE returns ErrCodeInvalidInput", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		// Pre-SELECT: booking is pending
		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		// Atomic UPDATE returns 0 rows (booking was cancelled concurrently)
		mock.ExpectExec(`UPDATE bookings SET start_datetime = \?, end_datetime = \?.*WHERE id = \?.*AND NOT EXISTS`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Re-query: status is now cancelled
		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("cancelled"))

		err := repo.RescheduleBooking(adminCtx(), "b-1", "2026-07-13T14:00:00.000Z")
		if err == nil {
			t.Fatal("expected error for concurrent cancellation, got nil")
		}
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeInvalidInput {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeInvalidInput)
		}
	})

	t.Run("concurrent deletion between SELECT and UPDATE wraps ErrNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		mock.ExpectExec(`UPDATE bookings SET start_datetime = \?, end_datetime = \?.*WHERE id = \?.*AND NOT EXISTS`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Re-query: booking was deleted
		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnError(sql.ErrNoRows)

		err := repo.RescheduleBooking(adminCtx(), "b-1", "2026-07-13T14:00:00.000Z")
		if err == nil {
			t.Fatal("expected error for concurrent deletion, got nil")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound) = true, got false; err = %v", err)
		}
	})
}

func TestBookingsRepo_OwnerRole(t *testing.T) {
	t.Run("owner can GetBooking without filter", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		rows := sqlmock.NewRows([]string{
			"id", "client_id", "professional_id", "service_id",
			"start_datetime", "end_datetime", "status", "notes", "payment_method",
			"created_at", "updated_at",
		}).AddRow("b-1", "c-1", "p-1", "svc-1",
			"2026-07-13T13:00:00.000Z", "2026-07-13T13:30:00.000Z",
			"pending", nil, nil,
			"2026-07-13T12:00:00.000Z", "2026-07-13T12:00:00.000Z")
		mock.ExpectQuery(`SELECT .+ FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(rows)

		booking, err := repo.GetBooking(ownerCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if booking.ID != "b-1" {
			t.Errorf("got ID=%q, want %q", booking.ID, "b-1")
		}
	})

	t.Run("owner can CancelBooking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT status FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

		mock.ExpectExec(`UPDATE bookings SET status = .cancelled.`).
			WithArgs("b-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.CancelBooking(ownerCtx(), "b-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner can RescheduleBooking", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT service_id, status, professional_id FROM bookings WHERE id = \?`).
			WithArgs("b-1").
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "status", "professional_id"}).
				AddRow("svc-1", "pending", "p-1"))

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))

		mock.ExpectExec(`UPDATE bookings SET start_datetime = \?, end_datetime = \?.*WHERE id = \?.*AND NOT EXISTS`).
			WithArgs("2026-07-13T14:00:00.000Z", "2026-07-13T14:30:00.000Z", "b-1", "b-1", "p-1", "2026-07-13T14:30:00.000Z", "2026-07-13T14:00:00.000Z").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.RescheduleBooking(ownerCtx(), "b-1", "2026-07-13T14:00:00.000Z")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("owner can CreateBooking for any client", func(t *testing.T) {
		db, mock := newMockDB(t)
		repo := NewBookingsRepo(db)

		mock.ExpectQuery(`SELECT duration_minutes FROM services WHERE id = \?`).
			WithArgs("svc-1").
			WillReturnRows(sqlmock.NewRows([]string{"duration_minutes"}).AddRow(30))
		mock.ExpectExec(`INSERT INTO bookings.*SELECT.*WHERE NOT EXISTS`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		input := &CreateBookingInput{
			ClientID:       "c-any",
			ProfessionalID: "p-1",
			ServiceID:      "svc-1",
			StartDatetime:  "2026-07-13T13:00:00.000Z",
		}

		result, err := repo.CreateBooking(ownerCtx(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Booking == nil {
			t.Fatal("expected non-nil booking")
		}
	})
}
