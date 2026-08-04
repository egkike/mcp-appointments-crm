package usecase

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
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

// --- mockBookingValidator (PR #B) ---

// mockBookingValidator is a function-table mock for *service.BookingValidator.
// Each test sets OnValidate to return the *domain.SemanticError the use case
// must propagate unchanged, or nil to let the use case reach the repo.
type mockBookingValidator struct {
	OnValidate func(ctx context.Context, input service.ValidateBookingInput) *domain.SemanticError
}

func (m *mockBookingValidator) Validate(ctx context.Context, input service.ValidateBookingInput) *domain.SemanticError {
	if m.OnValidate == nil {
		panic("mockBookingValidator.OnValidate not set")
	}
	return m.OnValidate(ctx, input)
}

// --- mockProfessionalsRepo (PR #B entity resolution) ---

type mockProfessionalsRepo struct {
	FindByIDFn func(ctx context.Context, id string) (*entity.Professional, error)
}

func (m *mockProfessionalsRepo) FindByID(ctx context.Context, id string) (*entity.Professional, error) {
	if m.FindByIDFn == nil {
		panic("mockProfessionalsRepo.FindByIDFn not set")
	}
	return m.FindByIDFn(ctx, id)
}

// FindActive, Save, Update are not exercised by the PR #B use case path but
// must be implemented to satisfy repository.ProfessionalsRepo. They panic if
// called to surface unexpected dependencies in tests.
func (m *mockProfessionalsRepo) FindActive(ctx context.Context) ([]*entity.Professional, error) {
	panic("mockProfessionalsRepo.FindActive: not expected in PR #B tests")
}
func (m *mockProfessionalsRepo) Save(ctx context.Context, p *entity.Professional) error {
	panic("mockProfessionalsRepo.Save: not expected in PR #B tests")
}
func (m *mockProfessionalsRepo) Update(ctx context.Context, p *entity.Professional) error {
	panic("mockProfessionalsRepo.Update: not expected in PR #B tests")
}

// --- mockBusinessProfileRepo (PR #B entity resolution) ---

type mockBusinessProfileRepo struct {
	GetFn func(ctx context.Context) (*entity.BusinessProfile, error)
}

func (m *mockBusinessProfileRepo) Get(ctx context.Context) (*entity.BusinessProfile, error) {
	if m.GetFn == nil {
		panic("mockBusinessProfileRepo.GetFn not set")
	}
	return m.GetFn(ctx)
}

// Update is not exercised by the PR #B use case path but must be implemented
// to satisfy repository.BusinessProfileRepo. It panics if called.
func (m *mockBusinessProfileRepo) Update(ctx context.Context, p *entity.BusinessProfile) error {
	panic("mockBusinessProfileRepo.Update: not expected in PR #B tests")
}

// --- mockBusinessHoursExceptionRepo (PR #B entity resolution) ---

type mockBusinessHoursExceptionRepo struct {
	GetFn func(ctx context.Context, date time.Time) (*entity.BusinessHoursException, error)
}

func (m *mockBusinessHoursExceptionRepo) Get(ctx context.Context, date time.Time) (*entity.BusinessHoursException, error) {
	if m.GetFn == nil {
		panic("mockBusinessHoursExceptionRepo.GetFn not set")
	}
	return m.GetFn(ctx, date)
}

// Create, List, Delete are not exercised by the PR #B use case path but must
// be implemented to satisfy repository.BusinessHoursExceptionRepo. They panic
// if called.
func (m *mockBusinessHoursExceptionRepo) Create(ctx context.Context, e *entity.BusinessHoursException) error {
	panic("mockBusinessHoursExceptionRepo.Create: not expected in PR #B tests")
}
func (m *mockBusinessHoursExceptionRepo) List(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
	panic("mockBusinessHoursExceptionRepo.List: not expected in PR #B tests")
}
func (m *mockBusinessHoursExceptionRepo) Delete(ctx context.Context, id int) error {
	panic("mockBusinessHoursExceptionRepo.Delete: not expected in PR #B tests")
}

// --- mockSchedulesRepo (PR #B entity resolution) ---

type mockSchedulesRepo struct {
	FindByProfessionalAndDayFn func(ctx context.Context, professionalID string, day int) (*entity.Schedule, error)
}

func (m *mockSchedulesRepo) FindByProfessionalAndDay(ctx context.Context, professionalID string, day int) (*entity.Schedule, error) {
	if m.FindByProfessionalAndDayFn == nil {
		panic("mockSchedulesRepo.FindByProfessionalAndDayFn not set")
	}
	return m.FindByProfessionalAndDayFn(ctx, professionalID, day)
}

// Upsert, Delete are not exercised by the PR #B use case path but must be
// implemented to satisfy repository.SchedulesRepo. They panic if called.
func (m *mockSchedulesRepo) Upsert(ctx context.Context, s *entity.Schedule) error {
	panic("mockSchedulesRepo.Upsert: not expected in PR #B tests")
}
func (m *mockSchedulesRepo) Delete(ctx context.Context, professionalID string, day int) error {
	panic("mockSchedulesRepo.Delete: not expected in PR #B tests")
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

// activeProfessional returns a staff member with active status (PR #B).
func activeProfessional() *entity.Professional {
	return &entity.Professional{
		ID:     "p1",
		Name:   "Ana",
		Status: "active",
	}
}

// businessProfileUTC returns a business profile whose timezone is UTC so the
// use case's timezone conversion is a no-op in tests (deterministic instants).
func businessProfileUTC() *entity.BusinessProfile {
	return &entity.BusinessProfile{
		ID:            "singleton",
		Name:          "Negocio de prueba",
		Timezone:      "UTC",
		BusinessHours: `{"1":{"open":"09:00","close":"18:00"}}`,
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
