package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestFindAvailableSlotsFullDayGrid(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	result, err := f.execute(t, day, day, strPtr("p1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// S7: the step is the 60-minute service duration, from the combined opening
	// (max(09:00 business, 09:00 shift)) to the combined closing (min(18:00, 17:00)).
	want := slotsAt(day, "p1", "Ana", 9, 10, 11, 12, 13, 14, 15, 16)
	wantSlots(t, result.Slots, want)
	if result.TotalCandidates != 8 {
		t.Errorf("total_candidates = %d; want 8", result.TotalCandidates)
	}
	if result.Truncated {
		t.Error("truncated = true; want false (8 candidates, default limit 10)")
	}
	if f.rangeReads != 1 {
		t.Errorf("bookings range reads = %d; want 1", f.rangeReads)
	}
	if f.overlapCalls != 0 {
		t.Errorf("repository overlap calls = %d; want 0 (every overlap question is answered in memory)", f.overlapCalls)
	}
}

func TestFindAvailableSlotsOrdersByNameOnTie(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	// Same start grid, names out of alphabetical order in the candidate list.
	f.professionals.FindAllActiveFn = func(context.Context) ([]*entity.Professional, error) {
		return []*entity.Professional{
			slotsProfessional("p2", "Zoe", "active"),
			slotsProfessional("p1", "Ana", "active"),
		}, nil
	}

	result, err := f.execute(t, day, day, nil, intPtr(4))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []dto.AvailableSlot{
		{StartDatetime: slotsAt(day, "p1", "Ana", 9)[0].StartDatetime, ProfessionalID: "p1", ProfessionalName: "Ana"},
		{StartDatetime: slotsAt(day, "p2", "Zoe", 9)[0].StartDatetime, ProfessionalID: "p2", ProfessionalName: "Zoe"},
		{StartDatetime: slotsAt(day, "p1", "Ana", 10)[0].StartDatetime, ProfessionalID: "p1", ProfessionalName: "Ana"},
		{StartDatetime: slotsAt(day, "p2", "Zoe", 10)[0].StartDatetime, ProfessionalID: "p2", ProfessionalName: "Zoe"},
	}
	wantSlots(t, result.Slots, want)
	if result.TotalCandidates != 16 || !result.Truncated {
		t.Errorf("total_candidates=%d truncated=%v; want 16/true", result.TotalCandidates, result.Truncated)
	}
}

func TestFindAvailableSlotsStepsByServiceDuration(t *testing.T) {
	loc := slotsTestLocation(t)
	day := slotsTestDay(loc, 0)

	f := newSlotsFixture(t)
	// 40-minute service: the grid must use 40 minutes, not a fixed 15/30 (S7).
	f.services.FindByIDFn = func(context.Context, string) (*entity.Service, error) {
		return &entity.Service{ID: "s1", Name: "Corte", DurationMinutes: 40, Active: true}, nil
	}

	result, err := f.execute(t, day, day, strPtr("p1"), intPtr(50))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []dto.AvailableSlot{
		{StartDatetime: time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, loc), ProfessionalID: "p1", ProfessionalName: "Ana"},
		{StartDatetime: time.Date(day.Year(), day.Month(), day.Day(), 9, 40, 0, 0, loc), ProfessionalID: "p1", ProfessionalName: "Ana"},
		{StartDatetime: time.Date(day.Year(), day.Month(), day.Day(), 10, 20, 0, 0, loc), ProfessionalID: "p1", ProfessionalName: "Ana"},
	}
	// 09:00, 09:40, 10:20, 11:00, ... up to the last start whose 40 minutes fit
	// before 17:00 (16:20). That is 12 slots, but asserting the head is enough to
	// pin the step: the second entry can only exist with a 40-minute step.
	wantSlots(t, result.Slots[:3], want)
	if result.TotalCandidates != 12 {
		t.Errorf("total_candidates = %d; want 12 (09:00 .. 16:20 step 40m)", result.TotalCandidates)
	}
}
