package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// daySlotRequest bundles one (day, professional) grid generation request.
type daySlotRequest struct {
	day             time.Time               // business-local midnight of the day
	service         *entity.Service         // provides the step and the duration
	professional    *entity.Professional    // the candidate
	businessProfile *entity.BusinessProfile // weekly hours source for the chain
	schedule        *entity.Schedule        // that day's shift
	exception       *entity.BusinessHoursException
	overlapReader   service.BookingOverlapReader // in-memory, backed by the single range read
	duration        time.Duration
	openMin         int // combined window opening (business + shift)
	closeMin        int // combined window closing (business + shift)
}

// generateDaySlots builds one professional's candidate grid for a day and keeps
// only the candidates the shared chain accepts.
//
// Grid (S7): anchored at max(business open, shift start), stepping by the SERVICE
// DURATION (never a fixed 15/30 minutes), while the whole slot still fits in
// min(business close, shift end). Each candidate is then validated through
// service.ValidateBookingTimeSlot, whose *domain.SemanticError means "not
// available" and is dropped (S4): no reason ever reaches the client.
func generateDaySlots(ctx context.Context, req daySlotRequest) []dto.AvailableSlot {
	durationMin := int(req.duration / time.Minute)
	if durationMin <= 0 {
		return nil
	}
	shiftOpenMin, shiftCloseMin, ok := parseSlotWindow(req.schedule.StartTime, req.schedule.EndTime)
	if !ok {
		return nil
	}

	anchorMin := req.openMin
	if shiftOpenMin > anchorMin {
		anchorMin = shiftOpenMin
	}
	boundMin := req.closeMin
	if shiftCloseMin < boundMin {
		boundMin = shiftCloseMin
	}
	if anchorMin >= boundMin {
		return nil
	}

	slots := make([]dto.AvailableSlot, 0, (boundMin-anchorMin)/durationMin)
	for startMin := anchorMin; startMin+durationMin <= boundMin; startMin += durationMin {
		// B8 f3 mold: the candidate is the business-local WALL CLOCK, built with
		// time.Date in the business location (never an offset-bearing instant).
		start := time.Date(req.day.Year(), req.day.Month(), req.day.Day(),
			startMin/60, startMin%60, 0, 0, req.day.Location())

		semErr := service.ValidateBookingTimeSlot(ctx, service.SlotInput{
			ProfessionalID:  req.professional.ID,
			Service:         req.service,
			Professional:    req.professional,
			BusinessProfile: req.businessProfile,
			Schedule:        req.schedule,
			Exception:       req.exception,
			Start:           start,
		}, service.BookingTimeValidatorDeps{Bookings: req.overlapReader})
		if semErr != nil {
			continue
		}
		slots = append(slots, dto.AvailableSlot{
			StartDatetime:    start,
			ProfessionalID:   req.professional.ID,
			ProfessionalName: req.professional.Name,
		})
	}
	return slots
}

// sortAvailableSlots applies the deterministic order of REQ-AV-3: start instant
// ascending, and professional name ascending on a tie.
func sortAvailableSlots(slots []dto.AvailableSlot) {
	sort.SliceStable(slots, func(i, j int) bool {
		if !slots[i].StartDatetime.Equal(slots[j].StartDatetime) {
			return slots[i].StartDatetime.Before(slots[j].StartDatetime)
		}
		return slots[i].ProfessionalName < slots[j].ProfessionalName
	})
}
