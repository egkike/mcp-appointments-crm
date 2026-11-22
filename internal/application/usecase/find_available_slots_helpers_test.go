package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ── find_available_slots (agent-surface T2, issue #241) ──
//
// The fixture pins a deterministic geometry so every assertion can name the
// complete observable effect:
//
//	business hours   09:00-18:00 every day (business_hours keys 1..7)
//	shift            09:00-17:00 every day (so the bound is the shift, not the business)
//	service duration 60 minutes  → slots at 09,10,...,16 (8 per day, 1 professional)
//
// The business timezone is America/Argentina/Buenos_Aires, not UTC, so the
// business-local wall clock is exercised for real (trap 4).

const slotsAllWeekHours = `{"1":{"open":"09:00","close":"18:00"},"2":{"open":"09:00","close":"18:00"},` +
	`"3":{"open":"09:00","close":"18:00"},"4":{"open":"09:00","close":"18:00"},` +
	`"5":{"open":"09:00","close":"18:00"},"6":{"open":"09:00","close":"18:00"},` +
	`"7":{"open":"09:00","close":"18:00"}}`

// slotsTestLocation loads the business timezone used across the suite.
func slotsTestLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return loc
}

// slotsTestDay returns business-local midnight of the base test date plus offset
// days. 2031-03-03 is arbitrary and far in the future: the chain's step 1 rejects
// past slots, so fixtures must always look forward.
func slotsTestDay(loc *time.Location, offsetDays int) time.Time {
	return time.Date(2031, time.March, 3, 0, 0, 0, 0, loc).AddDate(0, 0, offsetDays)
}

// slotsTestToday returns business-local midnight of the real current date. The
// S11 clamp reads the wall clock, so its tests cannot pin a fixed date the way
// the rest of the suite does.
func slotsTestToday(t *testing.T, loc *time.Location) time.Time {
	t.Helper()
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}

// slotsProfessional builds a staff member. Specialties carries the service id so
// the "no professional_id" path (HasSpecialty, fail-closed) admits it.
func slotsProfessional(id, name, status string) *entity.Professional {
	return &entity.Professional{
		ID:          id,
		Name:        name,
		Status:      status,
		Specialties: ptr(`["s1"]`),
	}
}

// slotsShift builds the shift row the schedules double answers for a given
// (professional, day) pair.
func slotsShift(professionalID string, day entity.ScheduleWeekday, startHour, endHour int) *entity.Schedule {
	return &entity.Schedule{
		ID:             int(day) + 1,
		ProfessionalID: professionalID,
		DayOfWeek:      int(day),
		StartTime:      fmt.Sprintf("%02d:00", startHour),
		EndTime:        fmt.Sprintf("%02d:00", endHour),
	}
}

// mockSlotsExceptionsRepo is the range read double: the shared exceptions mock
// cannot answer List (it panics by design), and this use case needs the single
// range query instead of one Get per day. It records no counter of its own: the
// exception-read budget is asserted by the tests that pin it, from their own
// ListFn closure, so nothing here can imply a budget nobody checks.
type mockSlotsExceptionsRepo struct {
	*mockBusinessHoursExceptionRepo
	ListFn func(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error)
}

func (m *mockSlotsExceptionsRepo) List(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
	return m.ListFn(ctx, from, to)
}

// slotsFixture wires the use case over function-table doubles and records the
// reads that S9 constrains: the range must be read ONCE and the per-slot overlap
// question must never reach the repository.
type slotsFixture struct {
	uc            *FindAvailableSlotsUseCase
	loc           *time.Location
	services      *mockServicesRepo
	professionals *mockProfessionalsRepo
	profile       *mockBusinessProfileRepo
	exceptions    *mockSlotsExceptionsRepo
	schedules     *mockSchedulesRepo
	bookings      *mockBookingsRepo

	// rangeReads pins HOW MANY range reads happened; rangeReadFrom/rangeReadTo
	// pin WHICH window was read (S9 with the S11 clamp: the same bounds must reach
	// both range reads, and a past window must collapse to today's).
	rangeReads    int
	rangeReadFrom time.Time
	rangeReadTo   time.Time
	overlapCalls  int
	scheduleReads int
}

func newSlotsFixture(t *testing.T) *slotsFixture {
	t.Helper()
	loc := slotsTestLocation(t)
	f := &slotsFixture{loc: loc}

	f.services = &mockServicesRepo{FindByIDFn: func(_ context.Context, id string) (*entity.Service, error) {
		if id != "s1" {
			return nil, fmt.Errorf("servicio %s: %w", id, domain.ErrNotFound)
		}
		return &entity.Service{ID: "s1", Name: "Corte", DurationMinutes: 60, Active: true}, nil
	}}
	f.professionals = &mockProfessionalsRepo{
		FindByIDFn: func(_ context.Context, id string) (*entity.Professional, error) {
			switch id {
			case "p1":
				return slotsProfessional("p1", "Ana", "active"), nil
			case "p2":
				return slotsProfessional("p2", "Beto", "active"), nil
			case "p3":
				return slotsProfessional("p3", "Caro", "inactive"), nil
			default:
				return nil, fmt.Errorf("profesional %s: %w", id, domain.ErrNotFound)
			}
		},
		FindAllActiveFn: func(context.Context) ([]*entity.Professional, error) {
			return []*entity.Professional{
				slotsProfessional("p1", "Ana", "active"),
				slotsProfessional("p3", "Caro", "inactive"),
			}, nil
		},
	}
	f.profile = &mockBusinessProfileRepo{GetFn: func(context.Context) (*entity.BusinessProfile, error) {
		return &entity.BusinessProfile{
			ID:            "singleton",
			Name:          "Negocio",
			Timezone:      loc.String(),
			BusinessHours: slotsAllWeekHours,
		}, nil
	}}
	f.exceptions = &mockSlotsExceptionsRepo{
		mockBusinessHoursExceptionRepo: &mockBusinessHoursExceptionRepo{},
		ListFn: func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
			return nil, nil
		},
	}
	// The day read (not the owner-only weekly read) is what the use case may
	// consume: mockSchedulesRepo.FindByProfessional panics on purpose, so a
	// regression that switches back to it fails loudly instead of hiding behind
	// a FORBIDDEN error for non-owner callers.
	f.schedules = &mockSchedulesRepo{FindByProfessionalAndDayFn: func(_ context.Context, professionalID string, day entity.ScheduleWeekday) (*entity.Schedule, error) {
		f.scheduleReads++
		return slotsShift(professionalID, day, 9, 17), nil
	}}
	f.bookings = &mockBookingsRepo{
		ListBookingsForRangeFn: func(_ context.Context, from, to time.Time) ([]*entity.Booking, error) {
			f.rangeReads++
			f.rangeReadFrom, f.rangeReadTo = from, to
			return nil, nil
		},
		FindOverlappingFn: func(context.Context, string, time.Time, time.Time) ([]*entity.Booking, error) {
			f.overlapCalls++
			return nil, nil
		},
	}
	f.uc = NewFindAvailableSlotsUseCase(f.services, f.professionals, f.profile, f.exceptions, f.schedules, f.bookings)
	return f
}

// execute runs the use case for a named professional ("p1" by default) over the
// given inclusive range.
func (f *slotsFixture) execute(t *testing.T, from, to time.Time, professionalID *string, limit *int) (*dto.FindAvailableSlotsResult, error) {
	t.Helper()
	return f.uc.Execute(context.Background(), dto.FindAvailableSlotsInput{
		Caller:         ownerCaller(),
		ServiceID:      "s1",
		ProfessionalID: professionalID,
		From:           from.Format("2006-01-02"),
		To:             to.Format("2006-01-02"),
		Limit:          limit,
	})
}

func strPtr(value string) *string { return &value }

// slotsAt builds the expected slot list for one day at the given local hours.
func slotsAt(day time.Time, professionalID, professionalName string, hours ...int) []dto.AvailableSlot {
	out := make([]dto.AvailableSlot, 0, len(hours))
	for _, hour := range hours {
		out = append(out, dto.AvailableSlot{
			StartDatetime:    time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location()),
			ProfessionalID:   professionalID,
			ProfessionalName: professionalName,
		})
	}
	return out
}

// wantSlots asserts the complete slot content: count, every start instant in the
// business wall clock, and the professional identity of each entry.
func wantSlots(t *testing.T, got, want []dto.AvailableSlot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slots = %d; want %d\ngot:  %v\nwant: %v", len(got), len(want), slang(got), slang(want))
	}
	for index := range want {
		if !got[index].StartDatetime.Equal(want[index].StartDatetime) {
			t.Errorf("slot[%d].start = %s; want %s", index, got[index].StartDatetime, want[index].StartDatetime)
		}
		if got[index].ProfessionalID != want[index].ProfessionalID {
			t.Errorf("slot[%d].professional_id = %q; want %q", index, got[index].ProfessionalID, want[index].ProfessionalID)
		}
		if got[index].ProfessionalName != want[index].ProfessionalName {
			t.Errorf("slot[%d].professional_name = %q; want %q", index, got[index].ProfessionalName, want[index].ProfessionalName)
		}
	}
}

// slang renders slots compactly for a failure message.
func slang(slots []dto.AvailableSlot) []string {
	out := make([]string, 0, len(slots))
	for _, slot := range slots {
		out = append(out, slot.StartDatetime.Format("2006-01-02T15:04")+"/"+slot.ProfessionalID)
	}
	return out
}

// wantSemantic asserts the error is the expected *domain.SemanticError code.
func wantSemantic(t *testing.T, err error, code domain.ErrCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected semantic error %q, got nil", code)
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("error %v (%T) is not a *domain.SemanticError", err, err)
	}
	if sem.Code != code {
		t.Errorf("error code = %q; want %q (msg=%q)", sem.Code, code, sem.Message)
	}
}

// wantNoReads asserts a rejected input touched no counter: the range was never
// read, no agenda was read and no overlap question was asked.
func wantNoReads(t *testing.T, f *slotsFixture) {
	t.Helper()
	if f.rangeReads != 0 {
		t.Errorf("bookings range reads = %d; want 0 (rejected input must not read)", f.rangeReads)
	}
	if f.scheduleReads != 0 {
		t.Errorf("schedule reads = %d; want 0 (rejected input must not read)", f.scheduleReads)
	}
	if f.overlapCalls != 0 {
		t.Errorf("repository overlap calls = %d; want 0 (overlap is answered in memory)", f.overlapCalls)
	}
}
