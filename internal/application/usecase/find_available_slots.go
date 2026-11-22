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
// Role: any authenticated caller. It carries no ToolRBAC entry in the
// composition root (same mold as check_availability) and this use case
// re-asserts auth.RequireAuthenticated.
type FindAvailableSlotsUseCase struct {
	services      domainrepo.ServicesRepo
	professionals domainrepo.ProfessionalsRepo
	profile       domainrepo.BusinessProfileRepo
	exceptions    domainrepo.BusinessHoursExceptionRepo
	schedules     domainrepo.SchedulesRepo
	bookings      domainrepo.BookingsRepo
}

// NewFindAvailableSlotsUseCase constructs the use case. Every dependency is a
// domain port; the composition root injects the same repository handles the
// availability read path already consumes.
func NewFindAvailableSlotsUseCase(
	services domainrepo.ServicesRepo,
	professionals domainrepo.ProfessionalsRepo,
	profile domainrepo.BusinessProfileRepo,
	exceptions domainrepo.BusinessHoursExceptionRepo,
	schedules domainrepo.SchedulesRepo,
	bookings domainrepo.BookingsRepo,
) *FindAvailableSlotsUseCase {
	return &FindAvailableSlotsUseCase{
		services:      services,
		professionals: professionals,
		profile:       profile,
		exceptions:    exceptions,
		schedules:     schedules,
		bookings:      bookings,
	}
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
func (uc *FindAvailableSlotsUseCase) Execute(ctx context.Context, input dto.FindAvailableSlotsInput) (*dto.FindAvailableSlotsResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	ctx = auth.WithCaller(ctx, input.Caller)

	limit, err := resolveSlotsLimit(input.Limit)
	if err != nil {
		return nil, err
	}
	from, to, days, err := resolveSlotsRange(input.From, input.To)
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

	professionals, err := uc.resolveCandidateProfessionals(ctx, input.ProfessionalID, svc.ID)
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
// the INCLUSIVE window. Both bounds are calendar dates, so the day count is an
// exact multiple of 24h once parsed.
func resolveSlotsRange(fromBound, toBound string) (from, to time.Time, days int, err error) {
	from, err = parseExceptionDateBound(fromBound)
	if err != nil {
		return time.Time{}, time.Time{}, 0, err
	}
	to, err = parseExceptionDateBound(toBound)
	if err != nil {
		return time.Time{}, time.Time{}, 0, err
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, 0, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el rango de fechas es inválido: 'to' no puede ser anterior a 'from'.",
			Cause:   fmt.Errorf("rango invertido %s..%s: %w", fromBound, toBound, domain.ErrInvalidInput),
		}
	}
	days = int(to.Sub(from)/(24*time.Hour)) + 1
	if days > maxAvailableSlotsRangeDays {
		return time.Time{}, time.Time{}, 0, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("el rango de fechas no puede superar los %d días (se pidieron %d).", maxAvailableSlotsRangeDays, days),
			Cause:   fmt.Errorf("rango %s..%s = %d días > %d: %w", fromBound, toBound, days, maxAvailableSlotsRangeDays, domain.ErrInvalidInput),
		}
	}
	return from, to, days, nil
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
// With no name the candidate set is every ACTIVE professional offering the
// service: inactive staff are excluded silently, and a professional with no
// declared specialty is excluded too (fail-closed, same rule as
// search_professionals_advanced: an empty/unreadable specialties value never
// widens the catalog).
func (uc *FindAvailableSlotsUseCase) resolveCandidateProfessionals(ctx context.Context, professionalID *string, serviceID string) ([]*entity.Professional, error) {
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
	candidates := make([]*entity.Professional, 0, len(all))
	for _, pro := range all {
		if pro == nil || !pro.IsActive() || !pro.HasSpecialty(serviceID) {
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
