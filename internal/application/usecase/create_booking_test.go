package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestCreateBookingUseCase(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

	t.Run("happy path admin creates for any client", func(t *testing.T) {
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return activeService(), nil
			},
		}
		var createdBooking *entity.Booking
		bookRepo := &mockBookingsRepo{
			CreateFn: func(_ context.Context, b *entity.Booking) error {
				createdBooking = b
				return nil
			},
		}
		uc := NewCreateBookingUseCase(bookRepo, svcRepo)

		result, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "c1",
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartTime:      futureStart,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.BookingID == "" {
			t.Fatal("expected non-empty BookingID")
		}
		if createdBooking == nil {
			t.Fatal("expected Create to be called")
		}
		if createdBooking.ClientID != "c1" {
			t.Errorf("booking.ClientID = %q; want %q", createdBooking.ClientID, "c1")
		}
		if createdBooking.ProfessionalID != "p1" {
			t.Errorf("booking.ProfessionalID = %q; want %q", createdBooking.ProfessionalID, "p1")
		}
		if createdBooking.Status != entity.BookingStatusPending {
			t.Errorf("booking.Status = %q; want %q", createdBooking.Status, entity.BookingStatusPending)
		}
		if !createdBooking.StartDatetime.Equal(futureStart) {
			t.Errorf("booking.StartDatetime = %v; want %v", createdBooking.StartDatetime, futureStart)
		}
		// Duration = 60 min from service
		wantEnd := futureStart.Add(60 * time.Minute)
		if !createdBooking.EndDatetime.Equal(wantEnd) {
			t.Errorf("booking.EndDatetime = %v; want %v", createdBooking.EndDatetime, wantEnd)
		}
	})

	t.Run("happy path client creates for themselves", func(t *testing.T) {
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return activeService(), nil
			},
		}
		bookRepo := &mockBookingsRepo{
			CreateFn: func(_ context.Context, _ *entity.Booking) error { return nil },
		}
		uc := NewCreateBookingUseCase(bookRepo, svcRepo)

		result, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         clientCaller("c1"),
			ClientID:       "c1",
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartTime:      futureStart,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.BookingID == "" {
			t.Fatal("expected non-empty BookingID")
		}
	})

	t.Run("caller not authenticated", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:    auth.Caller{}, // empty ID
			ClientID:  "c1",
			ServiceID: "s1",
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

	t.Run("client role creating for another client", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:    clientCaller("c1"),
			ClientID:  "c2", // different from caller's ClientID
			ServiceID: "s1",
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
		if !strings.Contains(sem.Message, "el cliente solo puede crear reservas para") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("staff role for different professional", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{})

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         staffCaller("staff1", "p1"),
			ClientID:       "c1",
			ServiceID:      "s1",
			ProfessionalID: "p2", // different from caller's ProfessionalID
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
		if !strings.Contains(sem.Message, "el personal solo puede crear reservas para su profesional asignado") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("service not found", func(t *testing.T) {
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return nil, domain.ErrNotFound
			},
		}
		bookRepo := &mockBookingsRepo{}
		uc := NewCreateBookingUseCase(bookRepo, svcRepo)

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "c1",
			ServiceID:      "s-nonexistent",
			ProfessionalID: "p1",
			StartTime:      futureStart,
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
		if !strings.Contains(sem.Message, "servicio") || !strings.Contains(sem.Message, "no encontrado") {
			t.Errorf("expected Spanish message about service not found; got %q", sem.Message)
		}
	})

	t.Run("service not active", func(t *testing.T) {
		inactive := activeService()
		inactive.Active = false
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return inactive, nil
			},
		}
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, svcRepo)

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "c1",
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartTime:      futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeServiceNotActive {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeServiceNotActive)
		}
		if !strings.Contains(sem.Message, "no está activo") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("booking overlap", func(t *testing.T) {
		svcRepo := &mockServicesRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) {
				return activeService(), nil
			},
		}
		bookRepo := &mockBookingsRepo{
			CreateFn: func(_ context.Context, _ *entity.Booking) error {
				return domain.ErrConflict
			},
		}
		uc := NewCreateBookingUseCase(bookRepo, svcRepo)

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "c1",
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartTime:      futureStart,
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
		if !strings.Contains(sem.Message, "ya tiene una reserva en ese horario") {
			t.Errorf("expected Spanish overlap message; got %q", sem.Message)
		}
	})
}
