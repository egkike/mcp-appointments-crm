package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestMarkAlertAsSentUseCase(t *testing.T) {
	repo := &mockPendingAlertsRepo{}
	uc := NewMarkAlertAsSentUseCase(repo)

	result, err := uc.Execute(context.Background(), dto.MarkAlertAsSentInput{Caller: adminCaller(), AlertID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlertID != 42 {
		t.Errorf("AlertID = %d; want 42", result.AlertID)
	}
	if repo.markAsSentID != 42 {
		t.Errorf("MarkAsSent called with %d; want 42", repo.markAsSentID)
	}

	_, err = uc.Execute(context.Background(), dto.MarkAlertAsSentInput{Caller: staffCaller("s1", "p1"), AlertID: 1})
	if err == nil {
		t.Fatal("expected error for staff caller")
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) || sem.Code != domain.ErrCodeForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestMarkAlertAsSentUseCaseUnauthenticated(t *testing.T) {
	repo := &mockPendingAlertsRepo{}
	uc := NewMarkAlertAsSentUseCase(repo)

	_, err := uc.Execute(context.Background(), dto.MarkAlertAsSentInput{AlertID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
	}
	if sem.Code != domain.ErrCodeUnauthenticated && sem.Code != domain.ErrCodeForbidden {
		t.Fatalf("expected unauthenticated or forbidden, got %v", sem.Code)
	}
}

func TestMarkAlertAsSentUseCase_NotFound(t *testing.T) {
	repo := &mockPendingAlertsRepo{markAsSentErr: domain.ErrNotFound}
	uc := NewMarkAlertAsSentUseCase(repo)

	_, err := uc.Execute(context.Background(), dto.MarkAlertAsSentInput{Caller: adminCaller(), AlertID: 99})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
	}
	if sem.Code != domain.ErrCodeNotFound {
		t.Fatalf("expected ErrCodeNotFound, got %v", sem.Code)
	}
	if sem.Message != "alerta no encontrada o no pendiente" {
		t.Fatalf("unexpected message %q", sem.Message)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected errors.Is ErrNotFound, got %v", err)
	}
}

func TestMarkAlertAsSentUseCase_CancelledReturnsNotFound(t *testing.T) {
	repo := &mockPendingAlertsRepo{markAsSentErr: domain.ErrNotFound}
	uc := NewMarkAlertAsSentUseCase(repo)

	_, err := uc.Execute(context.Background(), dto.MarkAlertAsSentInput{Caller: adminCaller(), AlertID: 42})
	if err == nil {
		t.Fatal("expected error")
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) || sem.Code != domain.ErrCodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}
