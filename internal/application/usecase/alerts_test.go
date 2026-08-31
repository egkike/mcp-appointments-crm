package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

type mockAlertStore struct {
	inserted  []*entity.PendingAlert
	cancelled []string
	insertErr error
	cancelErr error
}

func (m *mockAlertStore) InsertForBooking(ctx context.Context, a *entity.PendingAlert) error {
	m.inserted = append(m.inserted, a)
	return m.insertErr
}

func (m *mockAlertStore) CancelByBookingID(ctx context.Context, bookingID string) error {
	m.cancelled = append(m.cancelled, bookingID)
	return m.cancelErr
}

func TestConfirmationAlertFor(t *testing.T) {
	start := time.Date(2026, 8, 3, 14, 30, 0, 0, time.UTC)
	alert := ConfirmationAlertFor("Ana", "Carlos", "b-123", start, start)

	if alert.Type != "confirmation_requested" {
		t.Errorf("Type = %q; want confirmation_requested", alert.Type)
	}
	if alert.Status != "pending" {
		t.Errorf("Status = %q; want pending", alert.Status)
	}
	if *alert.RelatedBookingID != "b-123" {
		t.Errorf("RelatedBookingID = %v; want b-123", *alert.RelatedBookingID)
	}
	want := "Confirmar reserva de Ana con Carlos el 2026-08-03T14:30:00Z"
	if alert.Message != want {
		t.Errorf("Message = %q; want %q", alert.Message, want)
	}
	if !alert.ScheduledDatetime.Equal(start.UTC()) {
		t.Errorf("ScheduledDatetime = %v; want %v", alert.ScheduledDatetime, start.UTC())
	}
}

func TestAlertBuilderBuildConfirmationMessage(t *testing.T) {
	b := AlertBuilder{}
	start := time.Date(2026, 12, 25, 9, 0, 0, 0, time.UTC)
	got := b.BuildConfirmationMessage("Ana", "Carlos", start)
	want := "Confirmar reserva de Ana con Carlos el 2026-12-25T09:00:00Z"
	if got != want {
		t.Errorf("Message = %q; want %q", got, want)
	}
}

func TestCreateBookingEmitsConfirmationAlert(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	svcRepo, bookRepo, prosRepo, bizRepo, exRepo, schedRepo, clientsRepo, validator := createBookingMocks(activeService(), nil)
	store := &mockAlertStore{}
	bookRepo.CreateFn = func(_ context.Context, _ *entity.Booking) error { return nil }
	uc := NewCreateBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, clientsRepo, validator, store, nil)

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
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(store.inserted))
	}
	alert := store.inserted[0]
	if *alert.RelatedBookingID != result.BookingID {
		t.Errorf("RelatedBookingID = %v; want %v", *alert.RelatedBookingID, result.BookingID)
	}
	if alert.ScheduledDatetime.IsZero() {
		t.Error("ScheduledDatetime is zero; want time.Now UTC")
	}
	if time.Since(alert.ScheduledDatetime) > 5*time.Second || time.Since(alert.ScheduledDatetime) < -5*time.Second {
		t.Errorf("ScheduledDatetime = %v; want close to now UTC", alert.ScheduledDatetime)
	}
}

func TestCancelBookingCancelsPendingAlert(t *testing.T) {
	booking := pendingBooking()
	bookRepo := &mockBookingsRepo{
		FindByIDFn: func(_ context.Context, _ string) (*entity.Booking, error) { return booking, nil },
		CancelFn:   func(_ context.Context, _ string) error { return nil },
	}
	store := &mockAlertStore{}
	uc := NewCancelBookingUseCase(bookRepo, store, nil)

	_, err := uc.Execute(context.Background(), dto.CancelBookingInput{
		Caller:    adminCaller(),
		BookingID: "b1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.cancelled) != 1 || store.cancelled[0] != "b1" {
		t.Errorf("cancelled = %v; want [b1]", store.cancelled)
	}
}

func TestRescheduleBookingReissuesConfirmationAlert(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 14, 0, 0, 0, time.UTC)
	booking := pendingBooking()
	bookRepo := &mockBookingsRepo{
		FindByIDFn:   func(_ context.Context, _ string) (*entity.Booking, error) { return booking, nil },
		RescheduleFn: func(_ context.Context, _ string, _, _ time.Time) error { return nil },
	}
	svcRepo := &mockServicesRepo{
		FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) { return activeService(), nil },
	}
	prosRepo, bizRepo, exRepo, schedRepo, validator := rescheduleDeps()
	store := &mockAlertStore{}
	uc := NewRescheduleBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, nil, validator, store, nil)

	_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
		Caller:       adminCaller(),
		BookingID:    "b1",
		NewStartTime: futureStart,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.cancelled) != 1 || store.cancelled[0] != "b1" {
		t.Errorf("cancelled = %v; want [b1]", store.cancelled)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 inserted alert, got %d", len(store.inserted))
	}
	if *store.inserted[0].RelatedBookingID != "b1" {
		t.Errorf("inserted RelatedBookingID = %v; want b1", *store.inserted[0].RelatedBookingID)
	}
}

func TestCreateBookingAlertFailureDoesNotFailBooking(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	svcRepo, bookRepo, prosRepo, bizRepo, exRepo, schedRepo, clientsRepo, validator := createBookingMocks(activeService(), nil)
	store := &mockAlertStore{insertErr: context.DeadlineExceeded}
	bookRepo.CreateFn = func(_ context.Context, _ *entity.Booking) error { return nil }
	uc := NewCreateBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, clientsRepo, validator, store, nil)

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
		t.Error("expected booking ID")
	}
	// Alert insert was attempted and failed, but booking still succeeded (REQ-PA-LIFE-001 resilience).
	if len(store.inserted) != 1 {
		t.Fatalf("expected alert insert attempted, got %d", len(store.inserted))
	}
}

func TestRescheduleBookingReissuesAlertOnCancelFailure(t *testing.T) {
	futureStart := time.Date(2026, 8, 3, 14, 0, 0, 0, time.UTC)
	booking := pendingBooking()
	bookRepo := &mockBookingsRepo{
		FindByIDFn:   func(_ context.Context, _ string) (*entity.Booking, error) { return booking, nil },
		RescheduleFn: func(_ context.Context, _ string, _, _ time.Time) error { return nil },
	}
	svcRepo := &mockServicesRepo{
		FindByIDFn: func(_ context.Context, _ string) (*entity.Service, error) { return activeService(), nil },
	}
	prosRepo, bizRepo, exRepo, schedRepo, validator := rescheduleDeps()
	store := &mockAlertStore{cancelErr: context.DeadlineExceeded}
	uc := NewRescheduleBookingUseCase(bookRepo, svcRepo, prosRepo, bizRepo, exRepo, schedRepo, nil, validator, store, nil)

	_, err := uc.Execute(context.Background(), dto.RescheduleBookingInput{
		Caller:       adminCaller(),
		BookingID:    "b1",
		NewStartTime: futureStart,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.cancelled) != 1 || store.cancelled[0] != "b1" {
		t.Errorf("cancelled = %v; want [b1]", store.cancelled)
	}
	// cancel failure is best-effort and does not block reissue per REQ-PA-LIFE-001
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 inserted alert despite cancel failure, got %d", len(store.inserted))
	}
	if *store.inserted[0].RelatedBookingID != "b1" {
		t.Errorf("inserted RelatedBookingID = %v; want b1", *store.inserted[0].RelatedBookingID)
	}
}
