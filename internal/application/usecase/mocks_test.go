package usecase

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// Hand-rolled function-table mocks for repository interfaces.
// No third-party mocking libraries. Each method panics if its Fn field is nil,
// ensuring tests fail fast when a dependency is not wired up.

// --- mockBookingsRepo ---

type mockBookingsRepo struct {
	FindByIDFn             func(ctx context.Context, id string) (*entity.Booking, error)
	CreateFn               func(ctx context.Context, b *entity.Booking) error
	UpdateFn               func(ctx context.Context, b *entity.Booking) error
	CancelFn               func(ctx context.Context, id string) error
	RescheduleFn           func(ctx context.Context, id string, newStart, newEnd time.Time) error
	FindOverlappingFn      func(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error)
	FindByStaffAndRangeFn  func(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error)
	ListBookingsForRangeFn func(ctx context.Context, start, end time.Time) ([]*entity.Booking, error)
	SearchByNotesFn        func(ctx context.Context, q string) ([]*entity.Booking, error)
	UpdateStatusFn         func(ctx context.Context, id string, status entity.BookingStatus) error
}

func (m *mockBookingsRepo) FindByID(ctx context.Context, id string) (*entity.Booking, error) {
	if m.FindByIDFn == nil {
		panic("mockBookingsRepo.FindByIDFn not set")
	}
	return m.FindByIDFn(ctx, id)
}

func (m *mockBookingsRepo) Create(ctx context.Context, b *entity.Booking) error {
	if m.CreateFn == nil {
		panic("mockBookingsRepo.CreateFn not set")
	}
	return m.CreateFn(ctx, b)
}

func (m *mockBookingsRepo) Update(ctx context.Context, b *entity.Booking) error {
	if m.UpdateFn == nil {
		panic("mockBookingsRepo.UpdateFn not set")
	}
	return m.UpdateFn(ctx, b)
}

func (m *mockBookingsRepo) Cancel(ctx context.Context, id string) error {
	if m.CancelFn == nil {
		panic("mockBookingsRepo.CancelFn not set")
	}
	return m.CancelFn(ctx, id)
}

func (m *mockBookingsRepo) Reschedule(ctx context.Context, id string, newStart, newEnd time.Time) error {
	if m.RescheduleFn == nil {
		panic("mockBookingsRepo.RescheduleFn not set")
	}
	return m.RescheduleFn(ctx, id, newStart, newEnd)
}

func (m *mockBookingsRepo) FindOverlapping(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error) {
	if m.FindOverlappingFn == nil {
		panic("mockBookingsRepo.FindOverlappingFn not set")
	}
	return m.FindOverlappingFn(ctx, staffID, start, end)
}

func (m *mockBookingsRepo) FindByStaffAndRange(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error) {
	if m.FindByStaffAndRangeFn == nil {
		panic("mockBookingsRepo.FindByStaffAndRangeFn not set")
	}
	return m.FindByStaffAndRangeFn(ctx, staffID, start, end)
}

func (m *mockBookingsRepo) ListBookingsForRange(ctx context.Context, start, end time.Time) ([]*entity.Booking, error) {
	if m.ListBookingsForRangeFn == nil {
		panic("mockBookingsRepo.ListBookingsForRangeFn not set")
	}
	return m.ListBookingsForRangeFn(ctx, start, end)
}

func (m *mockBookingsRepo) SearchByNotes(ctx context.Context, q string) ([]*entity.Booking, error) {
	if m.SearchByNotesFn == nil {
		panic("mockBookingsRepo.SearchByNotesFn not set")
	}
	return m.SearchByNotesFn(ctx, q)
}

func (m *mockBookingsRepo) UpdateStatus(ctx context.Context, id string, status entity.BookingStatus) error {
	if m.UpdateStatusFn == nil {
		panic("mockBookingsRepo.UpdateStatusFn not set")
	}
	return m.UpdateStatusFn(ctx, id, status)
}

// --- mockServicesRepo ---

type mockServicesRepo struct {
	FindByIDFn   func(ctx context.Context, id string) (*entity.Service, error)
	FindActiveFn func(ctx context.Context) ([]*entity.Service, error)
	SaveFn       func(ctx context.Context, s *entity.Service) error
	UpdateFn     func(ctx context.Context, s *entity.Service) error
	DeleteFn     func(ctx context.Context, id string) error
}

func (m *mockServicesRepo) FindByID(ctx context.Context, id string) (*entity.Service, error) {
	if m.FindByIDFn == nil {
		panic("mockServicesRepo.FindByIDFn not set")
	}
	return m.FindByIDFn(ctx, id)
}

func (m *mockServicesRepo) FindActive(ctx context.Context) ([]*entity.Service, error) {
	if m.FindActiveFn == nil {
		panic("mockServicesRepo.FindActiveFn not set")
	}
	return m.FindActiveFn(ctx)
}

func (m *mockServicesRepo) Save(ctx context.Context, s *entity.Service) error {
	if m.SaveFn == nil {
		panic("mockServicesRepo.SaveFn not set")
	}
	return m.SaveFn(ctx, s)
}

func (m *mockServicesRepo) Update(ctx context.Context, s *entity.Service) error {
	if m.UpdateFn == nil {
		panic("mockServicesRepo.UpdateFn not set")
	}
	return m.UpdateFn(ctx, s)
}

func (m *mockServicesRepo) Delete(ctx context.Context, id string) error {
	if m.DeleteFn == nil {
		panic("mockServicesRepo.DeleteFn not set")
	}
	return m.DeleteFn(ctx, id)
}

// --- mockAvailabilityChecker ---

type mockAvailabilityChecker struct {
	CheckAvailabilityFn func(ctx context.Context, params *service.CheckAvailabilityParams, deps service.AvailabilityDeps) (*service.CheckAvailabilityResult, error)
}

func (m *mockAvailabilityChecker) CheckAvailability(ctx context.Context, params *service.CheckAvailabilityParams, deps service.AvailabilityDeps) (*service.CheckAvailabilityResult, error) {
	if m.CheckAvailabilityFn == nil {
		panic("mockAvailabilityChecker.CheckAvailabilityFn not set")
	}
	return m.CheckAvailabilityFn(ctx, params, deps)
}

// --- Test helpers ---

func ptr(s string) *string { return &s }

func emptyCaller() auth.Caller {
	return auth.Caller{}
}

func adminCaller() auth.Caller {
	return auth.Caller{ID: "admin1", Role: auth.RoleAdmin}
}

func clientCaller(id string) auth.Caller {
	return auth.Caller{ID: id, Role: auth.RoleClient, ClientID: ptr(id)}
}

func staffCaller(id, proID string) auth.Caller {
	return auth.Caller{ID: id, Role: auth.RoleStaff, ProfessionalID: ptr(proID)}
}

func activeService() *entity.Service {
	return &entity.Service{
		ID:              "s1",
		Name:            "Corte de pelo",
		DurationMinutes: 60,
		Active:          true,
	}
}

func pendingBooking() *entity.Booking {
	return &entity.Booking{
		ID:             "b1",
		ClientID:       "c1",
		ProfessionalID: "p1",
		ServiceID:      "s1",
		StartDatetime:  time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC),
		EndDatetime:    time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC),
		Status:         entity.BookingStatusPending,
	}
}
