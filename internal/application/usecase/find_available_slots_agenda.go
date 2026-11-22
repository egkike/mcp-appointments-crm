package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// slotsClockLayout is the HH:MM form of every stored open/close bound.
const slotsClockLayout = "15:04"

// agendaCache reads each candidate's shift ONCE per day of week and caches the
// answer, so a multi-day range costs at most seven reads per professional instead
// of one per day (the schedules analogue of S9; S9 itself constrains the bookings
// read, and the per-slot path never touches a repository).
//
// It deliberately uses SchedulesRepo.FindByProfessionalAndDay — the same read
// CheckAvailability performs — and not FindByProfessional: the weekly read is
// owner-only at the adapter (feat-owner-chat S7b, "the agenda is
// business-operational data"), and this tool must work for ANY authenticated
// caller (S2). The day read is open to every authenticated caller and scopes a
// staff caller to its own row, so the cache verifies identity before trusting a
// row (see forDay).
type agendaCache struct {
	schedules domainrepo.SchedulesRepo
	byPro     map[string]map[entity.ScheduleWeekday]*entity.Schedule
}

// newAgendaCache returns an empty cache over the schedules port.
func newAgendaCache(schedules domainrepo.SchedulesRepo) *agendaCache {
	return &agendaCache{
		schedules: schedules,
		byPro:     make(map[string]map[entity.ScheduleWeekday]*entity.Schedule),
	}
}

// forDay returns the professional's shift for that day, or nil when there is
// none (a nil answer is a cached "no shift", never a cache miss).
//
// Fail-closed identity check: the day read overrides professionalID with the
// caller's own identity for staff callers, so a returned row may describe
// ANOTHER professional. Trusting it would publish the staff member's shift as
// someone else's availability — silently wrong and a cross-professional leak — so
// a row whose ProfessionalID does not match the requested professional is
// discarded. The consequence for a staff caller is the safe one: the roster is
// limited to their own agenda.
func (a *agendaCache) forDay(ctx context.Context, professional *entity.Professional, day entity.ScheduleWeekday) (*entity.Schedule, error) {
	week, ok := a.byPro[professional.ID]
	if !ok {
		week = make(map[entity.ScheduleWeekday]*entity.Schedule, 7)
		a.byPro[professional.ID] = week
	}
	if schedule, cached := week[day]; cached {
		return schedule, nil
	}

	schedule, err := a.schedules.FindByProfessionalAndDay(ctx, professional.ID, day)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, fmt.Errorf("find_available_slots: consultar horario del profesional %s: %w", professional.ID, err)
	}
	if schedule != nil && schedule.ProfessionalID != professional.ID {
		schedule = nil
	}
	week[day] = schedule
	return schedule, nil
}

// indexExceptionsByDate keys the range's exceptions by their calendar date. The
// exception table is date-scoped (no professional scoping, no weekday rule), so
// one lookup per day of the range is enough.
func indexExceptionsByDate(exceptions []*entity.BusinessHoursException) map[string]*entity.BusinessHoursException {
	byDate := make(map[string]*entity.BusinessHoursException, len(exceptions))
	for _, exception := range exceptions {
		if exception == nil || exception.ExceptionDate == "" {
			continue
		}
		byDate[exception.ExceptionDate] = exception
	}
	return byDate
}

// businessWindow resolves the day's business opening/closing in minutes since
// midnight. open=false means the business is closed that day, so no candidate is
// generated.
//
// This resolves the ITERATION BOUNDS only: the authority on whether a slot is
// valid remains service.ValidateBookingTimeSlot, which runs on every candidate
// and re-reads the same exception and weekly hours. A disagreement can therefore
// only cost iterations, never change a returned slot.
func businessWindow(profile *entity.BusinessProfile, exception *entity.BusinessHoursException, day time.Time) (openMin, closeMin int, open bool, err error) {
	if exception != nil {
		if exception.IsClosedDay() {
			// A declared closure is data, not corruption: the day is closed.
			return 0, 0, false, nil
		}
		if excOpen, excClose, hasHours := exception.EffectiveHours(); hasHours {
			start, end, ok := parseSlotWindow(excOpen, excClose)
			if !ok {
				// Same fail-closed posture as the weekly hours below: an open
				// exception declaring malformed or inverted hours is an internal
				// problem, never a silently empty day.
				return 0, 0, false, &domain.SemanticError{
					Code:    domain.ErrCodeInternal,
					Message: "la excepción de horario del negocio no pudo leerse; contactá al administrador.",
					Cause: fmt.Errorf("excepción de horario %s con horas %q-%q inválidas: %w",
						exception.ExceptionDate, excOpen, excClose, domain.ErrInvalidInput),
				}
			}
			return start, end, true, nil
		}
	}

	openHHMM, closeHHMM, hasHours, parseErr := profile.GetOpenClose(entity.ProfileDayKey(day.Weekday()))
	if parseErr != nil {
		// Same fail-closed posture as the chain's step 2: an unreadable
		// business_hours value is an internal problem, never a "closed day".
		return 0, 0, false, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "el horario del negocio no pudo leerse; contactá al administrador.",
			Cause:   parseErr,
		}
	}
	if !hasHours || openHHMM == "" || closeHHMM == "" {
		return 0, 0, false, nil
	}
	start, end, ok := parseSlotWindow(openHHMM, closeHHMM)
	if !ok {
		return 0, 0, false, nil
	}
	return start, end, true, nil
}

// parseSlotWindow converts a stored open/close pair to minutes since midnight.
// A malformed or inverted pair is reported as not-a-window: the chain already
// fails closed on such a value per slot, so the generator cannot invent hours
// out of it.
func parseSlotWindow(openHHMM, closeHHMM string) (openMin, closeMin int, ok bool) {
	openMin, openOK := clockMinutes(openHHMM)
	closeMin, closeOK := clockMinutes(closeHHMM)
	if !openOK || !closeOK || closeMin <= openMin {
		return 0, 0, false
	}
	return openMin, closeMin, true
}

// clockMinutes converts an HH:MM bound to minutes since midnight.
//
// entity.ValidHHMM stays the single format rule; the conversion lives here
// because the domain/service helper (hhmmToMinutes) is unexported inside the
// package the availability chain is frozen in (design log L8).
func clockMinutes(value string) (int, bool) {
	if !entity.ValidHHMM(value) {
		return 0, false
	}
	parsed, err := time.Parse(slotsClockLayout, value)
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}
