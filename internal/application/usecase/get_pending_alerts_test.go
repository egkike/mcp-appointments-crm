package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time { return c.now }

func TestGetPendingAlertsUseCase(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	id1, id2 := "b-1", "b-2"

	alerts := []*entity.PendingAlert{
		{
			ID:                1,
			Type:              "confirmation_requested",
			Message:           "msg1",
			ScheduledDatetime: now.Add(-2 * time.Hour),
			Status:            "pending",
			RelatedBookingID:  &id1,
			CreatedAt:         now.Add(-3 * time.Hour).Format(time.RFC3339),
		},
		{
			ID:                2,
			Type:              "confirmation_requested",
			Message:           "msg2",
			ScheduledDatetime: now.Add(-1 * time.Hour),
			Status:            "pending",
			RelatedBookingID:  &id2,
			CreatedAt:         now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
	}

	repo := &mockPendingAlertsRepo{findPendingResult: alerts}
	uc := NewGetPendingAlertsUseCase(repo)
	uc.clock = fakeClock{now: now}

	result, err := uc.Execute(context.Background(), dto.GetPendingAlertsInput{Caller: adminCaller()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(result.Alerts))
	}
	if result.Alerts[0].AlertID != 1 {
		t.Errorf("first alert ID = %d; want 1", result.Alerts[0].AlertID)
	}
	if result.Alerts[1].AlertID != 2 {
		t.Errorf("second alert ID = %d; want 2", result.Alerts[1].AlertID)
	}

	_, err = uc.Execute(context.Background(), dto.GetPendingAlertsInput{Caller: staffCaller("s1", "p1")})
	if err == nil {
		t.Fatal("expected error for staff caller")
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) || sem.Code != domain.ErrCodeForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestGetPendingAlertsUseCaseEmpty(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	repo := &mockPendingAlertsRepo{findPendingResult: nil}
	uc := NewGetPendingAlertsUseCase(repo)
	uc.clock = fakeClock{now: now}

	result, err := uc.Execute(context.Background(), dto.GetPendingAlertsInput{Caller: adminCaller()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Alerts) != 0 {
		t.Errorf("expected empty alerts, got %d", len(result.Alerts))
	}
}
