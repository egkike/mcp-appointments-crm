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
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

func TestCreateBookingUseCase(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

	t.Run("happy path admin creates for any client", func(t *testing.T) {
		svcRepo, bookRepo, prosRepo, bizRepo, exRepo, schedRepo, validator := createBookingMocks(activeService(), nil)
		var createdBooking *entity.Booking
		bookRepo.CreateFn = func(_ context.Context, b *entity.Booking) error {
			createdBooking = b
			return nil
		}
		uc := NewCreateBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, validator)

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
		svcRepo, bookRepo, prosRepo, bizRepo, exRepo, schedRepo, validator := createBookingMocks(activeService(), nil)
		bookRepo.CreateFn = func(_ context.Context, _ *entity.Booking) error { return nil }
		uc := NewCreateBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, validator)

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
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

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
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

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
		if sem.Code != domain.ErrCodeForbidden {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeForbidden)
		}
		if !strings.Contains(sem.Message, "Cliente solo puede crear reservas para") {
			t.Errorf("expected Spanish message; got %q", sem.Message)
		}
	})

	t.Run("staff role for different professional", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

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
		if sem.Code != domain.ErrCodeForbidden {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeForbidden)
		}
		if !strings.Contains(sem.Message, "Personal solo puede crear reservas para su profesional asignado") {
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
		uc := NewCreateBookingUseCase(bookRepo, svcRepo, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

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
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, svcRepo, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

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
		svcRepo, bookRepo, prosRepo, bizRepo, exRepo, schedRepo, validator := createBookingMocks(activeService(), nil)
		bookRepo.CreateFn = func(_ context.Context, _ *entity.Booking) error {
			return domain.ErrConflict
		}
		uc := NewCreateBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, validator)

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

	t.Run("empty client_id returns invalid input", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "",
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
		if sem.Code != domain.ErrCodeInvalidInput {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeInvalidInput)
		}
		if !strings.Contains(sem.Message, "Cliente") {
			t.Errorf("expected message to mention client; got %q", sem.Message)
		}
	})

	t.Run("empty service_id returns invalid input", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "c1",
			ServiceID:      "",
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
		if sem.Code != domain.ErrCodeInvalidInput {
			t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeInvalidInput)
		}
		if !strings.Contains(sem.Message, "Servicio") {
			t.Errorf("expected message to mention service; got %q", sem.Message)
		}
	})

	t.Run("empty professional_id returns invalid input", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "c1",
			ServiceID:      "s1",
			ProfessionalID: "",
			StartTime:      futureStart,
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
		if !strings.Contains(sem.Message, "Profesional") {
			t.Errorf("expected message to mention professional; got %q", sem.Message)
		}
	})

	t.Run("zero start_time returns invalid input", func(t *testing.T) {
		uc := NewCreateBookingUseCase(&mockBookingsRepo{}, &mockServicesRepo{}, &mockProfessionalsRepo{}, &mockBusinessProfileRepo{}, &mockBusinessHoursExceptionRepo{}, &mockSchedulesRepo{}, &mockBookingValidator{})

		_, err := uc.Execute(context.Background(), dto.CreateBookingInput{
			Caller:         adminCaller(),
			ClientID:       "c1",
			ServiceID:      "s1",
			ProfessionalID: "p1",
			StartTime:      time.Time{}, // zero value
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
		if !strings.Contains(sem.Message, "fecha") || !strings.Contains(sem.Message, "hora") {
			t.Errorf("expected message to mention date/time; got %q", sem.Message)
		}
	})
}

// createBookingMocks wires the six dependencies the PR #B use case resolves
// BEFORE calling the validator. The caller may override bookRepo.CreateFn
// (and any other Fn) afterwards — the mock reads the field at call time.
func createBookingMocks(svc *entity.Service, validatorRet *domain.SemanticError) (
	*mockServicesRepo, *mockBookingsRepo, *mockProfessionalsRepo,
	*mockBusinessProfileRepo, *mockBusinessHoursExceptionRepo,
	*mockSchedulesRepo, *mockBookingValidator,
) {
	svcRepo := &mockServicesRepo{
		FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) { return svc, nil },
	}
	bookRepo := &mockBookingsRepo{}
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
		FindByProfessionalAndDayFn: func(_ context.Context, _ string, _ int) (*entity.Schedule, error) { return nil, domain.ErrNotFound },
	}
	validator := &mockBookingValidator{
		OnValidate: func(_ context.Context, _ service.ValidateBookingInput) *domain.SemanticError { return validatorRet },
	}
	return svcRepo, bookRepo, prosRepo, bizRepo, exRepo, schedRepo, validator
}

// TestCreateBookingUseCase_Execute exercises the 8-row validation matrix from
// design.md §4.2 and the bookings delta spec (REQ-BK-9, REQ-BK-10, REQ-BK-11,
// REQ-BK-12).
//
//   - Rows 2–6 prove the use case propagates validator *domain.SemanticError
//     unchanged (REQ-BK-10, REQ-BK-11): the repo Create is never reached, and
//     there is no semantic-error → domain.ErrConflict mapping.
//   - Row 7 (service_not_active) proves the use case owns the active-status
//     check BEFORE the validator: the validator is not called at all.
//   - Row 8 (toctou_repo_overlap) proves the repo atomic overlap guard stays
//     reachable: the validator passes yet the repo returns domain.ErrConflict,
//     which the use case maps to ErrCodeBookingOverlap (REQ-BK-12 defense-in-depth).
func TestCreateBookingUseCase_Execute(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC) // Monday

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
			svcRepo, bookRepo, prosRepo, bizRepo, exRepo, schedRepo, validator := createBookingMocks(svc, tt.validatorRet)
			if tt.inactivePro {
				prosRepo.FindByIDFn = func(_ context.Context, _ string) (*entity.Professional, error) {
					return &entity.Professional{ID: "p1", Name: "Ana", Status: "inactive"}, nil
				}
			}
			validator.OnValidate = func(_ context.Context, _ service.ValidateBookingInput) *domain.SemanticError {
				validatorCalled = true
				return tt.validatorRet
			}
			var createdCalled bool
			bookRepo.CreateFn = func(_ context.Context, _ *entity.Booking) error {
				createdCalled = true
				return tt.repoRet
			}
			uc := NewCreateBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, validator)

			result, err := uc.Execute(context.Background(), dto.CreateBookingInput{
				Caller:         adminCaller(),
				ClientID:       "c1",
				ServiceID:      "s1",
				ProfessionalID: "p1",
				StartTime:      futureStart,
			})

			if tt.wantSuccess {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.BookingID == "" {
					t.Fatal("expected non-empty BookingID")
				}
				if !validatorCalled {
					t.Error("validator MUST be called on the happy path")
				}
				if !createdCalled {
					t.Error("repo Create MUST be reached on the happy path")
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
				if createdCalled {
					t.Error("repo Create MUST NOT be called when service/professional is inactive")
				}
			case "toctou_repo_overlap":
				if !validatorCalled {
					t.Error("validator MUST be called before the repo in the TOCTOU row")
				}
				if !createdCalled {
					t.Error("repo Create MUST be reached in the TOCTOU row (validator passed)")
				}
			default:
				// Validator-rejected rows: the repo must never be reached.
				if !validatorCalled {
					t.Errorf("validator MUST be called (row %q)", tt.name)
				}
				if createdCalled {
					t.Errorf("repo Create MUST NOT be reached when the validator rejects the slot (row %q)", tt.name)
				}
			}
		})
	}
}
