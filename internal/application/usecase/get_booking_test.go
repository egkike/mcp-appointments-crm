package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestGetBookingUseCase(t *testing.T) {
	t.Run("happy path returns all 11 fields mapped correctly", func(t *testing.T) {
		notes := "Bring vaccination record"
		payment := "cash"
		now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
		booking := &entity.Booking{
			ID:             "b1",
			ClientID:       "c1",
			ProfessionalID: "p1",
			ServiceID:      "s1",
			StartDatetime:  time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC),
			EndDatetime:    time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC),
			Status:         entity.BookingStatusPending,
			Notes:          &notes,
			PaymentMethod:  &payment,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewGetBookingUseCase(bookRepo)

		result, err := uc.Execute(context.Background(), dto.GetBookingInput{
			Caller:    adminCaller(),
			BookingID: "b1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		v := result.Booking
		if v.ID != "b1" {
			t.Errorf("ID = %q; want %q", v.ID, "b1")
		}
		if v.ClientID != "c1" {
			t.Errorf("ClientID = %q; want %q", v.ClientID, "c1")
		}
		if v.ProfessionalID != "p1" {
			t.Errorf("ProfessionalID = %q; want %q", v.ProfessionalID, "p1")
		}
		if v.ServiceID != "s1" {
			t.Errorf("ServiceID = %q; want %q", v.ServiceID, "s1")
		}
		wantStart := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
		if !v.StartDatetime.Equal(wantStart) {
			t.Errorf("StartDatetime = %v; want %v", v.StartDatetime, wantStart)
		}
		wantEnd := time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC)
		if !v.EndDatetime.Equal(wantEnd) {
			t.Errorf("EndDatetime = %v; want %v", v.EndDatetime, wantEnd)
		}
		if v.Status != "pending" {
			t.Errorf("Status = %q; want %q", v.Status, "pending")
		}
		if v.Notes == nil || *v.Notes != notes {
			t.Errorf("Notes = %v; want %q", v.Notes, notes)
		}
		if v.PaymentMethod == nil || *v.PaymentMethod != payment {
			t.Errorf("PaymentMethod = %v; want %q", v.PaymentMethod, payment)
		}
		if !v.CreatedAt.Equal(now) {
			t.Errorf("CreatedAt = %v; want %v", v.CreatedAt, now)
		}
		if !v.UpdatedAt.Equal(now) {
			t.Errorf("UpdatedAt = %v; want %v", v.UpdatedAt, now)
		}
	})

	t.Run("caller not authenticated", func(t *testing.T) {
		uc := NewGetBookingUseCase(&mockBookingsRepo{})

		_, err := uc.Execute(context.Background(), dto.GetBookingInput{
			Caller:    emptyCaller(),
			BookingID: "b1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("expected errors.Is(err, ErrUnauthenticated); got %v", err)
		}
		if !strings.Contains(err.Error(), "Usuario no autenticado") {
			t.Errorf("expected Spanish message; got %q", err.Error())
		}
	})

	t.Run("booking not found", func(t *testing.T) {
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return nil, domain.ErrNotFound
			},
		}
		uc := NewGetBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.GetBookingInput{
			Caller:    adminCaller(),
			BookingID: "b-nonexistent",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeNotFound {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeNotFound)
		}
		if !strings.Contains(sem.Message, "reserva no encontrada") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("client accessing another clients booking", func(t *testing.T) {
		booking := pendingBooking()
		booking.ClientID = "c2"
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewGetBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.GetBookingInput{
			Caller:    clientCaller("c1"),
			BookingID: "b1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Cliente solo puede acceder a sus propias reservas") {
			t.Errorf("expected Spanish message; got %q", err.Error())
		}
	})

	t.Run("staff accessing different professionals booking", func(t *testing.T) {
		booking := pendingBooking()
		booking.ProfessionalID = "p2"
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewGetBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.GetBookingInput{
			Caller:    staffCaller("staff1", "p1"),
			BookingID: "b1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Personal solo puede acceder a las reservas de su profesional asignado") {
			t.Errorf("expected Spanish message; got %q", err.Error())
		}
	})

	t.Run("optional fields nil are preserved as nil", func(t *testing.T) {
		booking := &entity.Booking{
			ID:             "b1",
			ClientID:       "c1",
			ProfessionalID: "p1",
			ServiceID:      "s1",
			StartDatetime:  time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC),
			EndDatetime:    time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC),
			Status:         entity.BookingStatusPending,
			Notes:          nil,
			PaymentMethod:  nil,
		}
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewGetBookingUseCase(bookRepo)

		result, err := uc.Execute(context.Background(), dto.GetBookingInput{
			Caller:    adminCaller(),
			BookingID: "b1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Booking.Notes != nil {
			t.Errorf("expected Notes=nil; got %v", result.Booking.Notes)
		}
		if result.Booking.PaymentMethod != nil {
			t.Errorf("expected PaymentMethod=nil; got %v", result.Booking.PaymentMethod)
		}
	})
}
