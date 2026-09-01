package service

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Hand-rolled mock implementations using function-table style.
// No third-party mocking libraries. Only methods called by AvailabilityService
// have configurable behavior; unused interface methods return zero values.

type mockServicesRepo struct {
	OnFindByID func(context.Context, string) (*entity.Service, error)
}

func (m *mockServicesRepo) FindByID(ctx context.Context, id string) (*entity.Service, error) {
	return m.OnFindByID(ctx, id)
}
func (m *mockServicesRepo) FindActive(context.Context) ([]*entity.Service, error) { return nil, nil }
func (m *mockServicesRepo) Save(context.Context, *entity.Service) error           { return nil }
func (m *mockServicesRepo) Update(context.Context, *entity.Service) error         { return nil }
func (m *mockServicesRepo) Delete(context.Context, string) error                  { return nil }
func (m *mockServicesRepo) SearchFTS(context.Context, string) ([]*entity.Service, error) {
	return nil, nil
}

type mockProfessionalsRepo struct {
	OnFindByID func(context.Context, string) (*entity.Professional, error)
}

func (m *mockProfessionalsRepo) FindByID(ctx context.Context, id string) (*entity.Professional, error) {
	return m.OnFindByID(ctx, id)
}
func (m *mockProfessionalsRepo) FindActive(context.Context) ([]*entity.Professional, error) {
	return nil, nil
}
func (m *mockProfessionalsRepo) Save(context.Context, *entity.Professional) error   { return nil }
func (m *mockProfessionalsRepo) Update(context.Context, *entity.Professional) error { return nil }

type mockBusinessProfileRepo struct {
	OnGet func(context.Context) (*entity.BusinessProfile, error)
}

func (m *mockBusinessProfileRepo) Get(ctx context.Context) (*entity.BusinessProfile, error) {
	return m.OnGet(ctx)
}
func (m *mockBusinessProfileRepo) Update(context.Context, *entity.BusinessProfile) error { return nil }

type mockBusinessHoursExceptionRepo struct {
	OnGet func(context.Context, time.Time) (*entity.BusinessHoursException, error)
}

func (m *mockBusinessHoursExceptionRepo) Get(ctx context.Context, d time.Time) (*entity.BusinessHoursException, error) {
	return m.OnGet(ctx, d)
}
func (m *mockBusinessHoursExceptionRepo) Create(context.Context, *entity.BusinessHoursException) error {
	return nil
}
func (m *mockBusinessHoursExceptionRepo) List(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
	return nil, nil
}
func (m *mockBusinessHoursExceptionRepo) Delete(context.Context, int) error { return nil }

type mockSchedulesRepo struct {
	OnFindByProfessionalAndDay func(context.Context, string, int) (*entity.Schedule, error)
}

func (m *mockSchedulesRepo) FindByProfessionalAndDay(ctx context.Context, pid string, day int) (*entity.Schedule, error) {
	return m.OnFindByProfessionalAndDay(ctx, pid, day)
}
func (m *mockSchedulesRepo) Upsert(context.Context, *entity.Schedule) error { return nil }
func (m *mockSchedulesRepo) Delete(context.Context, string, int) error      { return nil }

type mockBookingsRepo struct {
	OnFindOverlapping func(context.Context, string, time.Time, time.Time) ([]*entity.Booking, error)
}

func (m *mockBookingsRepo) FindByID(context.Context, string) (*entity.Booking, error) {
	return nil, nil
}
func (m *mockBookingsRepo) Create(context.Context, *entity.Booking) error { return nil }
func (m *mockBookingsRepo) Update(context.Context, *entity.Booking) error { return nil }
func (m *mockBookingsRepo) Cancel(context.Context, string) error          { return nil }
func (m *mockBookingsRepo) Reschedule(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (m *mockBookingsRepo) FindOverlapping(ctx context.Context, sid string, s, e time.Time) ([]*entity.Booking, error) {
	return m.OnFindOverlapping(ctx, sid, s, e)
}
func (m *mockBookingsRepo) FindByStaffAndRange(context.Context, string, time.Time, time.Time) ([]*entity.Booking, error) {
	return nil, nil
}
func (m *mockBookingsRepo) ListBookingsForRange(context.Context, time.Time, time.Time) ([]*entity.Booking, error) {
	return nil, nil
}
func (m *mockBookingsRepo) SearchByNotes(context.Context, string) ([]*entity.Booking, error) {
	return nil, nil
}
func (m *mockBookingsRepo) UpdateStatus(context.Context, string, entity.BookingStatus) error {
	return nil
}
func (m *mockBookingsRepo) AggregateByClient(context.Context, time.Time, time.Time, int) ([]domainrepo.ClientBookingCount, error) {
	return nil, nil
}
