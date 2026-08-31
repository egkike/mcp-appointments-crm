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
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
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
		prosRepo, bizRepo, exRepo, schedRepo, validator := rescheduleDeps()
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, nil, validator, nil, nil)

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
		uc := NewRescheduleBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

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
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

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

	t.Run("empty booking id rejected before repo dispatch", func(t *testing.T) {
		uc := NewRescheduleBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
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
		if !strings.Contains(sem.Message, "Identificador de reserva requerido") {
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
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       clientCaller("c1"),
			BookingID:    "b1",
			NewStartTime: futureStart,
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
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       staffCaller("staff1", "p1"),
			BookingID:    "b1",
			NewStartTime: futureStart,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Personal solo puede acceder a las reservas de su profesional asignado") {
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
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

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

	t.Run("zero NewStartTime returns ErrCodeInvalidInput", func(t *testing.T) {
		booking := pendingBooking()
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return booking, nil
			},
		}
		uc := NewRescheduleBookingUseCase(bookRepo, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

		_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
			Caller:       adminCaller(),
			BookingID:    "b1",
			NewStartTime: time.Time{}, // zero value
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
		if !strings.Contains(sem.Message, "nueva fecha") {
			t.Errorf("expected message to mention 'nueva fecha'; got %q", sem.Message)
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
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, nil, &mockBookingValidator{}, nil, nil)

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
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return pendingBooking(), nil
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
		prosRepo, bizRepo, exRepo, schedRepo, validator := rescheduleDeps()
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, nil, validator, nil, nil)

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
		repoErr := fmt.Errorf("disk full")
		bookRepo := &mockBookingsRepo{
			FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
				return pendingBooking(), nil
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
		prosRepo, bizRepo, exRepo, schedRepo, validator := rescheduleDeps()
		uc := NewRescheduleBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, nil, validator, nil, nil)

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

// rescheduleDeps returns the five PR #C entity-resolution dependencies (pro,
// business profile, exception, schedule) plus a validator that passes (returns
// nil). Callers override these as needed for specific subtests.
func rescheduleDeps() (
	*mockProfessionalsRepo, *mockBusinessProfileRepo,
	*mockBusinessHoursExceptionRepo, *mockSchedulesRepo, *mockBookingValidator,
) {
	prosRepo := &mockProfessionalsRepo{
		FindByIDFn: func(_ context.Context, _ string) (*entity.Professional, error) { return activeProfessional(), nil },
	}
	bizRepo := &mockBusinessProfileRepo{
		GetFn: func(_ context.Context) (*entity.BusinessProfile, error) { return businessProfileUTC(), nil },
	}
	exRepo := &mockBusinessHoursExceptionRepo{
		GetFn: func(_ context.Context, _ time.Time) (*entity.BusinessHoursException, error) {
			return nil, domain.ErrNotFound
		},
	}
	schedRepo := &mockSchedulesRepo{
		FindByProfessionalAndDayFn: func(_ context.Context, _ string, _ int) (*entity.Schedule, error) {
			return nil, domain.ErrNotFound
		},
	}
	validator := &mockBookingValidator{
		OnValidate: func(_ context.Context, _ service.ValidateBookingInput) *domain.SemanticError { return nil },
	}
	return prosRepo, bizRepo, exRepo, schedRepo, validator
}

// TestRescheduleBookingUseCase_Execute exercises the 9-row validation matrix
// from design.md §4.3 and the bookings delta spec (REQ-BK-9, REQ-BK-10,
// REQ-BK-11, REQ-BK-12), adapted to the reschedule shape: the matrix runs
// after the existing-booking load + CanReschedule check.
//
//   - Rows 2–6 prove the use case propagates validator *domain.SemanticError
//     unchanged (REQ-BK-10, REQ-BK-11): the repo Reschedule is never reached,
//     and there is no semantic-error → domain.ErrConflict mapping.
//   - Row 7 (service_not_active) and row 8 (professional_not_active) prove the
//     use case owns the active-status checks BEFORE the validator: the
//     validator is not called at all.
//   - Row 9 (toctou_repo_overlap) proves the repo atomic overlap guard stays
//     reachable: the validator passes yet the repo returns domain.ErrConflict,
//     which the use case maps to ErrCodeBookingOverlap (REQ-BK-12 defense-in-depth).
func TestRescheduleBookingUseCase_Execute(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 14, 0, 0, 0, time.UTC) // Monday

	tests := []struct {
		name         string
		inactiveSvc  bool
		inactivePro  bool
		validatorRet *domain.SemanticError
		repoRet      error
		wantErr      bool
		wantCode     domain.ErrCode
		wantSuccess  bool
	}{
		{name: "happy_path", wantSuccess: true},
		{name: "past_slot", validatorRet: &domain.SemanticError{Code: domain.ErrCodeSlotInPast, Message: "No se puede reservar en el pasado."}, wantErr: true, wantCode: domain.ErrCodeSlotInPast},
		{name: "business_closed", validatorRet: &domain.SemanticError{Code: domain.ErrCodeBusinessClosed, Message: "Negocio cerrado."}, wantErr: true, wantCode: domain.ErrCodeBusinessClosed},
		{name: "professional_not_working", validatorRet: &domain.SemanticError{Code: domain.ErrCodeProfessionalNotWorking, Message: "Profesional no trabaja."}, wantErr: true, wantCode: domain.ErrCodeProfessionalNotWorking},
		{name: "slot_out_of_hours", validatorRet: &domain.SemanticError{Code: domain.ErrCodeSlotOutOfHours, Message: "Fuera de horario."}, wantErr: true, wantCode: domain.ErrCodeSlotOutOfHours},
		{name: "overlap", validatorRet: &domain.SemanticError{Code: domain.ErrCodeBookingOverlap, Message: "Ya tiene una reserva."}, wantErr: true, wantCode: domain.ErrCodeBookingOverlap},
		{name: "service_not_active", inactiveSvc: true, wantErr: true, wantCode: domain.ErrCodeServiceNotActive},
		{name: "professional_not_active", inactivePro: true, wantErr: true, wantCode: domain.ErrCodeProfessionalNotActive},
		{name: "toctou_repo_overlap", validatorRet: nil, repoRet: domain.ErrConflict, wantErr: true, wantCode: domain.ErrCodeBookingOverlap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := activeService()
			if tt.inactiveSvc {
				svc.Active = false
			}
			validatorCalled := false
			bookRepo := &mockBookingsRepo{
				FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) {
					return pendingBooking(), nil
				},
			}
			svcRepo := &mockServicesRepo{
				FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) { return svc, nil },
			}
			prosRepo, bizRepo, exRepo, schedRepo, validator := rescheduleDeps()
			if tt.inactivePro {
				prosRepo.FindByIDFn = func(_ context.Context, _ string) (*entity.Professional, error) {
					return &entity.Professional{ID: "p1", Name: "Ana", Status: "inactive"}, nil
				}
			}
			validator.OnValidate = func(_ context.Context, _ service.ValidateBookingInput) *domain.SemanticError {
				validatorCalled = true
				return tt.validatorRet
			}
			var rescheduledCalled bool
			bookRepo.RescheduleFn = func(_ context.Context, _ string, _, _ time.Time) error {
				rescheduledCalled = true
				return tt.repoRet
			}
			uc := NewRescheduleBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, nil, validator, nil, nil)

			result, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
				Caller:       adminCaller(),
				BookingID:    "b1",
				NewStartTime: futureStart,
			})

			if tt.wantSuccess {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.BookingID != "b1" {
					t.Fatalf("result.BookingID = %q; want %q", result.BookingID, "b1")
				}
				if !validatorCalled {
					t.Error("validator MUST be called on the happy path")
				}
				if !rescheduledCalled {
					t.Error("repo Reschedule MUST be reached on the happy path")
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var sem *domain.SemanticError
			if !errors.As(err, &sem) {
				t.Fatalf("expected *domain.SemanticError; got %T: %v", err, err)
			}
			if sem.Code != tt.wantCode {
				t.Errorf("code = %q; want %q", sem.Code, tt.wantCode)
			}

			switch tt.name {
			case "service_not_active", "professional_not_active":
				if validatorCalled {
					t.Error("validator MUST NOT be called when service/professional is inactive (use case owns the check)")
				}
				if rescheduledCalled {
					t.Error("repo Reschedule MUST NOT be called when service/professional is inactive")
				}
			case "toctou_repo_overlap":
				if !validatorCalled {
					t.Error("validator MUST be called before the repo in the TOCTOU row")
				}
				if !rescheduledCalled {
					t.Error("repo Reschedule MUST be reached in the TOCTOU row (validator passed)")
				}
			default:
				// Validator-rejected rows: the repo must never be reached.
				if !validatorCalled {
					t.Errorf("validator MUST be called (row %q)", tt.name)
				}
				if rescheduledCalled {
					t.Errorf("repo Reschedule MUST NOT be reached when the validator rejects the slot (row %q)", tt.name)
				}
			}
		})
	}
}
