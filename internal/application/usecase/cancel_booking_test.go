package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestCancelBookingUseCase(t *testing.T) {
	t.Run("happy path admin cancels pending booking", func(t *testing.T) {
		booking := pendingBooking()
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
			CancelFn: func(_ context.Context, id string) error {
				if id != "b1" {
					return fmt.Errorf("unexpected booking ID: %s", id)
				}
				return nil
			},
		}
		uc := NewCancelBookingUseCase(bookRepo)

		result, err := uc.Execute(context.Background(), dto.CancelBookingInput{
			Caller:    adminCaller(),
			BookingID: "b1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.BookingID != "b1" {
			t.Errorf("result.BookingID = %q; want %q", result.BookingID, "b1")
		}
		if result.Status != string(entity.BookingStatusCancelled) {
			t.Errorf("result.Status = %q; want %q", result.Status, string(entity.BookingStatusCancelled))
		}
	})

	t.Run("caller not authenticated", func(t *testing.T) {
		uc := NewCancelBookingUseCase(&mockBookingsRepo{})

		_, err := uc.Execute(context.Background(), dto.CancelBookingInput{
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
		uc := NewCancelBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.CancelBookingInput{
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
		booking.ClientID = "c2" // different from caller
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewCancelBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.CancelBookingInput{
			Caller:    clientCaller("c1"),
			BookingID: "b1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeUnauthenticated)
		}
		if !strings.Contains(sem.Message, "el cliente solo puede acceder a sus propias reservas") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("staff accessing different professionals booking", func(t *testing.T) {
		booking := pendingBooking()
		booking.ProfessionalID = "p2" // different from caller
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewCancelBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.CancelBookingInput{
			Caller:    staffCaller("staff1", "p1"),
			BookingID: "b1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeUnauthenticated)
		}
		if !strings.Contains(sem.Message, "el personal solo puede acceder a las reservas de su profesional asignado") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("booking in cancelled status cannot be cancelled", func(t *testing.T) {
		booking := pendingBooking()
		booking.Status = entity.BookingStatusCancelled
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewCancelBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.CancelBookingInput{
			Caller:    adminCaller(),
			BookingID: "b1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeInvalidInput {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeInvalidInput)
		}
		if !strings.Contains(sem.Message, "no puede ser cancelada") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("cancel fails in repo with generic error", func(t *testing.T) {
		booking := pendingBooking()
		repoErr := fmt.Errorf("database connection lost")
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
			CancelFn: func(_ context.Context, _ string) error {
				return repoErr
			},
		}
		uc := NewCancelBookingUseCase(bookRepo)

		_, err := uc.Execute(context.Background(), dto.CancelBookingInput{
			Caller:    adminCaller(),
			BookingID: "b1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "cancelar reserva") {
			t.Errorf("expected Spanish wrapper; got %q", err.Error())
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected errors.Is(err, repoErr); got %v", err)
		}
	})
}
