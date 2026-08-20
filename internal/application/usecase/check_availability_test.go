package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// mondayFuture is 2026-08-03 10:00 UTC, a Monday within business hours.
var mondayFuture = time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

func TestCheckAvailabilityUseCase(t *testing.T) {
	t.Run("happy path returns available true", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				return &service.CheckAvailabilityResult{Available: true}, nil
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		result, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Available {
			t.Error("expected Available=true")
		}
	})

	t.Run("zero StartDatetime returns ErrCodeInvalidInput", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				t.Fatal("checker should not be called when StartDatetime is zero")
				return nil, nil
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		result, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  time.Time{},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if result != nil {
			t.Errorf("expected nil result; got %+v", result)
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if semErr.Code != domain.ErrCodeInvalidInput {
			t.Errorf("expected ErrCodeInvalidInput; got %v", semErr.Code)
		}
		if !strings.Contains(semErr.Message, "start_datetime") {
			t.Errorf("expected message to mention start_datetime; got %q", semErr.Message)
		}
	})

	t.Run("empty caller does not trigger auth error", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				return &service.CheckAvailabilityResult{Available: true}, nil
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		result, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			Caller:         emptyCaller(),
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err != nil {
			t.Fatalf("expected no error for empty caller (non-auth use case); got %v", err)
		}
		if !result.Available {
			t.Error("expected Available=true")
		}
	})

	t.Run("maps input to service params correctly", func(t *testing.T) {
		var gotParams *service.CheckAvailabilityParams
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, params *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				gotParams = params
				return &service.CheckAvailabilityResult{Available: true}, nil
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotParams == nil {
			t.Fatal("expected CheckAvailability to be called with params")
		}
		if gotParams.ServiceID != "s1" {
			t.Errorf("params.ServiceID = %q; want %q", gotParams.ServiceID, "s1")
		}
		if gotParams.ProfessionalID != "p1" {
			t.Errorf("params.ProfessionalID = %q; want %q", gotParams.ProfessionalID, "p1")
		}
		if gotParams.StartDatetime != mondayFuture.Format(time.RFC3339) {
			t.Errorf("params.StartDatetime = %q; want %q", gotParams.StartDatetime, mondayFuture.Format(time.RFC3339))
		}
	})

	t.Run("business closed error propagates", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				return nil, &domain.SemanticError{
					Code:    domain.ErrCodeBusinessClosed,
					Message: "el negocio está cerrado el lunes (feriado).",
				}
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeBusinessClosed {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeBusinessClosed)
		}
		if !strings.Contains(sem.Message, "cerrado") {
			t.Errorf("expected Spanish closed message; got %q", sem.Message)
		}
	})

	t.Run("slot out of hours error propagates", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				return nil, &domain.SemanticError{
					Code:    domain.ErrCodeSlotOutOfHours,
					Message: "el horario de atención comienza a las 09:00.",
				}
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		earlyTime := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  earlyTime,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeSlotOutOfHours {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeSlotOutOfHours)
		}
	})

	t.Run("slot in the past error propagates", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				return nil, &domain.SemanticError{
					Code:    domain.ErrCodeSlotInPast,
					Message: "no se puede reservar en el pasado.",
				}
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		pastTime := time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC)
		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  pastTime,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sem *domain.SemanticError
		if !errors.As(err, &sem) {
			t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
		}
		if sem.Code != domain.ErrCodeSlotInPast {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeSlotInPast)
		}
		if !strings.Contains(sem.Message, "pasado") {
			t.Errorf("expected Spanish past message; got %q", sem.Message)
		}
	})

	t.Run("overlap error propagates", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				return nil, &domain.SemanticError{
					Code: domain.ErrCodeBookingOverlap,
					// Mirrors the real domain template (booking_time_validator.go:
					// "Profesional %s ya tiene una reserva de %s a %s." interpolates
					// the professional's display name) — no leading "el", RFC3339
					// UTC slot bounds.
					Message: "Profesional Juan ya tiene una reserva de 2026-08-03T10:00:00Z a 2026-08-03T11:00:00Z.",
				}
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
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
		if !strings.Contains(sem.Message, "ya tiene una reserva") {
			t.Errorf("expected Spanish overlap message; got %q", sem.Message)
		}
	})

	t.Run("service not found error wraps with context", func(t *testing.T) {
		checker := &mockAvailabilityChecker{
			CheckAvailabilityFn: func(_ context.Context, _ *service.CheckAvailabilityParams, _ service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
				return nil, domain.ErrNotFound
			},
		}
		uc := NewCheckAvailabilityUseCase(checker, service.AvailabilityDeps{})

		_, err := uc.Execute(context.Background(), dto.CheckAvailabilityInput{
			ServiceID:      "s-nonexistent",
			ProfessionalID: "p1",
			StartDatetime:  mondayFuture,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected errors.Is(err, ErrNotFound); got %v", err)
		}
		if !strings.Contains(err.Error(), "consultar disponibilidad") {
			t.Errorf("expected error to mention availability context; got %q", err.Error())
		}
	})
}
