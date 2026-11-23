package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// Bounds of the date-range slot search (REQ-AV-3 / agent-surface T2).
const (
	// defaultAvailableSlotsLimit is the applied limit when the caller omits it.
	defaultAvailableSlotsLimit = 10
	// maxAvailableSlotsLimit is the hard cap; a larger value is rejected, not
	// clamped, so the caller learns the bound instead of silently losing slots.
	maxAvailableSlotsLimit = 50
	// maxAvailableSlotsRangeDays bounds the INCLUSIVE range (from and to both
	// count). Without it a six-month range would be a pointless scan under
	// SQLite's busy_timeout.
	maxAvailableSlotsRangeDays = 14
	// slotsDateLayout is the YYYY-MM-DD form of both range bounds and of
	// business_hours_exception.exception_date, so the two index cleanly.
	slotsDateLayout = "2006-01-02"
)

// bookingRangeReader is the consumer-side port for the ONE range read this use
// case performs. It is deliberately narrow (only ListBookingsForRange): the use
// case never asks a caller-scoped question, so it must not be handed a dependency
// shaped like one (issue #243).
//
// The interface alone does NOT prevent the defect — the filtered BookingsRepo
// satisfies it too (trap 3). What prevents it is the composition root wiring the
// role-neutral repository.AvailabilityBookingsReader, proved end-to-end by
// TestIntegrationAvailabilityIgnoresCallerScope.
type bookingRangeReader interface {
	ListBookingsForRange(ctx context.Context, start, end time.Time) ([]*entity.Booking, error)
}

// FindAvailableSlotsUseCase answers "which slots are free for this service
// between these dates?" in one call (agent-surface T2, issue #241).
//
// It reuses the availability rule instead of duplicating it: every candidate
// slot goes through service.ValidateBookingTimeSlot — the same helper
// CheckAvailability delegates to (REQ-AV-2) — injected with a BookingOverlapReader
// backed by the range's bookings read ONCE (S9). A candidate the chain rejects is
// simply not returned: this tool FILTERS, it never reports why (REQ-AV-3), which
// is the deliberate difference from check_availability.
//
// The range is read through the role-neutral availability adapter (issue #243):
// with the caller-scoped repo a client caller lost every other client's booking
// and saw taken slots as free.
//
// Role: any authenticated caller. It carries no ToolRBAC entry in the
// composition root (same mold as check_availability) and this use case
// re-asserts auth.RequireAuthenticated.
type FindAvailableSlotsUseCase struct {
	services      domainrepo.ServicesRepo
	professionals domainrepo.ProfessionalsRepo
	profile       domainrepo.BusinessProfileRepo
	exceptions    domainrepo.BusinessHoursExceptionRepo
	schedules     domainrepo.SchedulesRepo
	bookings      bookingRangeReader
	clock         Clock
}

// NewFindAvailableSlotsUseCase constructs the use case. Every dependency except
// bookings is a domain port; bookings is the narrow range reader the
// composition root satisfies with the role-neutral availability adapter.
func NewFindAvailableSlotsUseCase(
	services domainrepo.ServicesRepo,
	professionals domainrepo.ProfessionalsRepo,
	profile domainrepo.BusinessProfileRepo,
	exceptions domainrepo.BusinessHoursExceptionRepo,
	schedules domainrepo.SchedulesRepo,
	bookings bookingRangeReader,
) *FindAvailableSlotsUseCase {
	return &FindAvailableSlotsUseCase{
		services:      services,
		professionals: professionals,
		profile:       profile,
		exceptions:    exceptions,
		schedules:     schedules,
		bookings:      bookings,
		clock:         systemClock{},
	}
}

// SetClock replaces the clock. It is intended for tests, and it is what makes
// the "today" clamp deterministic: a test pins the clock to the same instant it
// used to derive its own dates, so the two can never disagree across midnight.
func (uc *FindAvailableSlotsUseCase) SetClock(clock Clock) {
	uc.clock = clock
}

// Execute searches the free slots of one service over an inclusive day range.
//
// Order of work matters for the observable contract: input bounds are validated
// BEFORE any read, so a rejected range or limit performs no booking read at all
// (S6, and the "a rejected input changes nothing" rule in the feature doc).
//
// Semantics (REQ-AV-3 / S11):
//   - unknown or inactive service_id → semantic error (input problem, propagates);
//   - professional_id named and unknown/inactive → semantic error;
//   - professional_id omitted → candidates are the ACTIVE professionals that
//     offer the service; inactive ones are excluded silently;
//   - a date with a closed-hours exception produces no slot (an exception with
//     its own window replaces the weekly hours);
//   - no free slot in the range → empty list, not an error.
//
// A window that starts before today is narrowed to today before any range read.
// Past days cannot produce a slot (the chain rejects every past candidate in its
// step 1), so the answer is identical and only the work disappears; the empty
// result of a fully past range is a result, never an error.
func (uc *FindAvailableSlotsUseCase) Execute(ctx context.Context, input dto.FindAvailableSlotsInput) (*dto.FindAvailableSlotsResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	ctx = auth.WithCaller(ctx, input.Caller)

	limit, err := resolveSlotsLimit(input.Limit)
	if err != nil {
		return nil, err
	}
	from, to, err := resolveSlotsRange(input.From, input.To)
	if err != nil {
		return nil, err
	}

	svc, err := uc.resolveService(ctx, input.ServiceID)
	if err != nil {
		return nil, err
	}
	if svc.Duration() <= 0 {
		// The grid steps by the service duration, so a non-positive value would
		// loop forever. Service.Validate guarantees a positive duration on every
		// write path, so this is a fail-closed programmer/bad-data guard.
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al leer la duración del servicio.",
			Cause:   fmt.Errorf("servicio %s con duración no positiva: %w", svc.ID, domain.ErrInvalidInput),
		}
	}

	professionals, err := uc.resolveCandidateProfessionals(ctx, input.ProfessionalID)
	if err != nil {
		return nil, err
	}

	profile, loc, err := uc.resolveProfile(ctx)
	if err != nil {
		return nil, err
	}

	// The range is built as business-local midnight boundaries: the wall clock is
	// what business hours and weekly schedules are expressed in, never an offset.
	rangeStart := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	rangeEnd := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc)

	// S11 efficiency invariant, not a rejection: clamp the scan to today in the
	// business location, then recompute the day count so the loop below and both
	// range reads agree on the same window. A fully past range narrows to zero
	// days and returns the empty result WITHOUT reading the range at all.
	if today := businessDay(uc.clock.Now(), loc); rangeStart.Before(today) {
		rangeStart = today
	}
	days := civilDays(rangeStart, rangeEnd)
	if days <= 0 {
		return emptyAvailableSlotsResult(), nil
	}

	exceptions, err := uc.exceptions.List(ctx, rangeStart, rangeEnd)
	if err != nil {
		return nil, fmt.Errorf("find_available_slots: consultar excepciones del rango: %w", err)
	}
	exceptionsByDate := indexExceptionsByDate(exceptions)

	// S9: the WHOLE range is read exactly once. The in-memory reader below
	// answers every per-slot overlap question from this snapshot, so the loop
	// never re-enters the repository (no N+1 per slot or per professional).
	bookings, err := uc.bookings.ListBookingsForRange(ctx, rangeStart.UTC(), rangeEnd.AddDate(0, 0, 1).UTC())
	if err != nil {
		return nil, fmt.Errorf("find_available_slots: consultar reservas del rango: %w", err)
	}
	overlapReader := newSlotBookingsReader(bookings)

	agendas := newAgendaCache(uc.schedules)

	duration := svc.Duration()
	slots := make([]dto.AvailableSlot, 0, days*len(professionals))
	for dayIndex := 0; dayIndex < days; dayIndex++ {
		day := rangeStart.AddDate(0, 0, dayIndex)
		exception := exceptionsByDate[day.Format(slotsDateLayout)]

		openMin, closeMin, open, err := businessWindow(profile, exception, day)
		if err != nil {
			return nil, err
		}
		if !open {
			// Closed that day (no weekly hours, or a closed exception): every
			// candidate would fail the chain, so no candidate is generated.
			continue
		}

		for _, professional := range professionals {
			schedule, err := agendas.forDay(ctx, professional, entity.ScheduleDay(day.Weekday()))
			if err != nil {
				return nil, err
			}
			if schedule == nil {
				continue
			}
			slots = append(slots, generateDaySlots(ctx, daySlotRequest{
				day:             day,
				service:         svc,
				professional:    professional,
				businessProfile: profile,
				schedule:        schedule,
				exception:       exception,
				overlapReader:   overlapReader,
				duration:        duration,
				openMin:         openMin,
				closeMin:        closeMin,
			})...)
		}
	}

	sortAvailableSlots(slots)

	result := &dto.FindAvailableSlotsResult{
		Slots:           slots,
		TotalCandidates: len(slots),
		Truncated:       len(slots) > limit,
	}
	if result.Truncated {
		result.Slots = result.Slots[:limit]
	}
	if result.Slots == nil {
		// Empty range renders as [] on the wire, never null.
		result.Slots = []dto.AvailableSlot{}
	}
	return result, nil
}

// resolveSlotsLimit validates the requested limit, applying the default when the
// caller omitted it. Out-of-range values are rejected (semantic error), never
// clamped.
func resolveSlotsLimit(limit *int) (int, error) {
	if limit == nil {
		return defaultAvailableSlotsLimit, nil
	}
	if *limit < 1 || *limit > maxAvailableSlotsLimit {
		return 0, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("el límite debe estar entre 1 y %d.", maxAvailableSlotsLimit),
			Cause:   fmt.Errorf("limit %d fuera del rango 1..%d: %w", *limit, maxAvailableSlotsLimit, domain.ErrInvalidInput),
		}
	}
	return *limit, nil
}

// resolveSlotsRange parses both inclusive bounds and enforces the 14-day cap on
// the INCLUSIVE requested window. It returns no day count on purpose: the number
// of days actually scanned is decided LATER, from the window clamped to today
// (see Execute), and a single day-count source keeps the loop and both range
// reads in agreement.
func resolveSlotsRange(fromBound, toBound string) (from, to time.Time, err error) {
	from, err = parseExceptionDateBound(fromBound)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err = parseExceptionDateBound(toBound)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el rango de fechas es inválido: 'to' no puede ser anterior a 'from'.",
			Cause:   fmt.Errorf("rango invertido %s..%s: %w", fromBound, toBound, domain.ErrInvalidInput),
		}
	}
	days := civilDays(from, to)
	if days > maxAvailableSlotsRangeDays {
		return time.Time{}, time.Time{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("el rango de fechas no puede superar los %d días (se pidieron %d).", maxAvailableSlotsRangeDays, days),
			Cause:   fmt.Errorf("rango %s..%s = %d días > %d: %w", fromBound, toBound, days, maxAvailableSlotsRangeDays, domain.ErrInvalidInput),
		}
	}
	return from, to, nil
}

// businessDay returns business-local midnight of now's calendar date: the anchor
// every business-hours and schedule comparison uses.
func businessDay(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// civilDays counts the INCLUSIVE calendar days between two dates. Only the date
// part matters, so both ends are re-anchored on UTC midnight before subtracting:
// a daylight-saving hour must never shift the count and make the loop and the
// range reads disagree.
func civilDays(from, to time.Time) int {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(end.Sub(start)/(24*time.Hour)) + 1
}

// emptyAvailableSlotsResult is the observable shape of "no slot in the range":
// an empty list (never null), zero candidates, not truncated.
func emptyAvailableSlotsResult() *dto.FindAvailableSlotsResult {
	return &dto.FindAvailableSlotsResult{Slots: []dto.AvailableSlot{}}
}

// resolveService resolves the service and enforces the active gate. An inactive
// service is an input problem and propagates, exactly like check_availability.
func (uc *FindAvailableSlotsUseCase) resolveService(ctx context.Context, serviceID string) (*entity.Service, error) {
	svc, err := uc.services.FindByID(ctx, strings.TrimSpace(serviceID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "el servicio indicado no existe",
				Cause:   err,
			}
		}
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, fmt.Errorf("find_available_slots: consultar servicio: %w", err)
	}
	if svc == nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al consultar el servicio.",
			Cause:   fmt.Errorf("servicio %s sin fila: %w", serviceID, domain.ErrNotFound),
		}
	}
	if !svc.IsActive() {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeServiceNotActive,
			Message: fmt.Sprintf("Servicio %s no está activo.", svc.Name),
		}
	}
	return svc, nil
}

// resolveCandidateProfessionals returns the professionals the grid is built for.
//
// A named professional is resolved strictly: unknown or inactive is a semantic
// error (the caller asked about someone specific, so silence would be a lie).
// With no name the candidate set is every ACTIVE professional: inactive staff are
// excluded silently.
//
// Specialties do NOT narrow either path (owner decision 2026-11-23, issue #261):
// they are catalog/search metadata (search_professionals_advanced still filters on
// them) and no other layer validates them, so filtering here made availability
// disagree with what booking actually allows.
func (uc *FindAvailableSlotsUseCase) resolveCandidateProfessionals(ctx context.Context, professionalID *string) ([]*entity.Professional, error) {
	if professionalID != nil {
		if named := strings.TrimSpace(*professionalID); named != "" {
			pro, err := uc.professionals.FindByID(ctx, named)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return nil, &domain.SemanticError{
						Code:    domain.ErrCodeNotFound,
						Message: "el profesional indicado no existe",
						Cause:   err,
					}
				}
				var sem *domain.SemanticError
				if errors.As(err, &sem) {
					return nil, sem
				}
				return nil, fmt.Errorf("find_available_slots: consultar profesional: %w", err)
			}
			if pro == nil || !pro.IsActive() {
				name := ""
				if pro != nil {
					name = pro.Name
				}
				return nil, &domain.SemanticError{
					Code:    domain.ErrCodeProfessionalNotActive,
					Message: fmt.Sprintf("Profesional %s no está activo.", name),
				}
			}
			return []*entity.Professional{pro}, nil
		}
	}

	// A staff caller MUST name its professional (issue #259). For staff the day
	// read overrides the id with the caller's own and the cache drops foreign rows,
	// so an unnamed call would discard every candidate and be indistinguishable
	// from "no availability" — the agent would tell the client there are no slots
	// when the caller simply had no permission. Refusing names the cause and the
	// way out instead.
	staffCaller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if staffCaller.Role == auth.RoleStaff {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeForbidden,
			Message: "Personal debe indicar su professional_id: solo puede consultar su propia disponibilidad.",
		}
	}

	// FindAllActive is deliberately unscoped (a staff caller must still see the
	// whole roster): the question is "who can serve this service?", not "who am I".
	all, err := uc.professionals.FindAllActive(ctx)
	if err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, fmt.Errorf("find_available_slots: listar profesionales activos: %w", err)
	}
	// Specialties do NOT restrict availability (owner decision, 2026-11-23, issue
	// #261): they are catalog/search metadata. This path used to drop every
	// professional whose specialty list did not include the requested service, which
	// made availability disagree with booking -- no other layer validates the
	// specialty, so a client could book a professional this filter hid, and an
	// installation with no specialties loaded got an empty grid for everyone.
	candidates := make([]*entity.Professional, 0, len(all))
	for _, pro := range all {
		if pro == nil || !pro.IsActive() {
			continue
		}
		candidates = append(candidates, pro)
	}
	return candidates, nil
}

// resolveProfile loads the singleton business profile and its timezone.
func (uc *FindAvailableSlotsUseCase) resolveProfile(ctx context.Context) (*entity.BusinessProfile, *time.Location, error) {
	profile, err := uc.profile.Get(ctx)
	if err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, nil, sem
		}
		return nil, nil, fmt.Errorf("find_available_slots: consultar zona horaria: %w", err)
	}
	if profile == nil {
		return nil, nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al consultar el perfil del negocio.",
			Cause:   fmt.Errorf("perfil del negocio sin fila: %w", domain.ErrNotFound),
		}
	}
	loc, err := service.ParseBusinessTimezone(profile.Timezone)
	if err != nil {
		return nil, nil, fmt.Errorf("find_available_slots: %w", err)
	}
	return profile, loc, nil
}
