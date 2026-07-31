package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestRescheduleBookingUseCase(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 14, 0, 0, 0, time.UTC)

	t.Run("happy path admin reschedules pending booking", func(t *testing.T) {
		booking := pendingBooking()
		var rescheduledID string
		var gotStart, gotEnd time.Time
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
			RescheduleFn: func(_ context.Context, id string, newStart, newEnd time.Time) error {
				rescheduledID = id
				gotStart = newStart
				gotEnd = newEnd
				return nil
			},
		}
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return activeService(), nil
			},
		}
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo)

		result, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
			BookingID:    "b1",
			NewStartTime: futureStart,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.BookingID != "b1" {
			t.Errorf("result.BookingID = %q; want %q", result.BookingID, "b1")
		}
		if result.Status != string(entity.BookingStatusPending) {
			t.Errorf("result.Status = %q; want %q", result.Status, string(entity.BookingStatusPending))
		}
		if rescheduledID != "b1" {
			t.Errorf("Reschedule called with id %q; want %q", rescheduledID, "b1")
		}
		if !gotStart.Equal(futureStart) {
			t.Errorf("gotStart = %v; want %v", gotStart, futureStart)
		}
		wantEnd := futureStart.Add(60 * time.Minute)
		if !gotEnd.Equal(wantEnd) {
			t.Errorf("gotEnd = %v; want %v", gotEnd, wantEnd)
		}
	})

	t.Run("caller not authenticated", func(t *testing.T) {
		uc := NewRescheduleBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       emptyCaller(),
			BookingID:    "b1",
			NewStartTime: futureStart,
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
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
			BookingID:    "b-nonexistent",
			NewStartTime: futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
		if !strings.Contains(err.Error(), "reserva no encontrada") {
			t.Errorf("expected Spanish message; got %q", err.Error())
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
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       clientCaller("c1"),
			BookingID:    "b1",
			NewStartTime: futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "el cliente solo puede acceder a sus propias reservas") {
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
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       staffCaller("staff1", "p1"),
			BookingID:    "b1",
			NewStartTime: futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "el personal solo puede acceder a las reservas de su profesional asignado") {
			t.Errorf("expected Spanish message; got %q", err.Error())
		}
	})

	t.Run("booking in cancelled status cannot be rescheduled", func(t *testing.T) {
		booking := pendingBooking()
		booking.Status = entity.BookingStatusCancelled
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
			BookingID:    "b1",
			NewStartTime: futureStart,
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
		if !strings.Contains(sem.Message, "no puede ser reprogramada") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("service not found", func(t *testing.T) {
		booking := pendingBooking()
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return nil, domain.ErrNotFound
			},
		}
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo)

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
			BookingID:    "b1",
			NewStartTime: futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
		if !strings.Contains(err.Error(), "servicio") || !strings.Contains(err.Error(), "no encontrado") {
			t.Errorf("expected Spanish message about service; got %q", err.Error())
		}
	})

	t.Run("overlap on new time", func(t *testing.T) {
		booking := pendingBooking()
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
			RescheduleFn: func(_ context.Context, _ string, _, _ time.Time) error {
				return domain.ErrConflict
			},
		}
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return activeService(), nil
			},
		}
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo)

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
			BookingID:    "b1",
			NewStartTime: futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeBookingOverlap {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeBookingOverlap)
		}
		if !strings.Contains(sem.Message, "ya tiene una reserva en el nuevo horario") {
			t.Errorf("expected Spanish overlap message; got %q", sem.Message)
		}
	})

	t.Run("reschedule fails with generic error", func(t *testing.T) {
		booking := pendingBooking()
		repoErr := fmt.Errorf("disk full")
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
			RescheduleFn: func(_ context.Context, _ string, _, _ time.Time) error {
				return repoErr
			},
		}
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return activeService(), nil
			},
		}
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo)

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
			BookingID:    "b1",
			NewStartTime: futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "reprogramar reserva") {
			t.Errorf("expected Spanish wrapper; got %q", err.Error())
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected errors.Is(err, repoErr); got %v", err)
		}
	})
}
