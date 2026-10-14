package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ─── feat-hours-exception-tool Phase 1 (use cases + DTOs) ──────────────────
//
// The exception use cases reuse the eight maintenance use cases' shape
// (owner gate → repository → sentinel→semantic mapping → audit). Validation is
// single-sourced in the repository (design §1.3): the create use case never
// duplicates the date/time rules, so the "no write" invariant it protects is
// the owner gate (a forbidden caller must never reach the repository), not the
// repository's own pre-write validation.
//
// The mock lives here (not in mocks_test.go) because the pre-existing
// mockBusinessHoursExceptionRepo only stubs Get for the PR #B read path and
// panics on Create/List/Delete.

// mockHoursExceptionRepo is a function-table mock for
// domainrepo.BusinessHoursExceptionRepo. Each method panics if its Fn field is
// nil so an unexpected dependency path fails fast.
type mockHoursExceptionRepo struct {
	GetFn    func(ctx context.Context, date time.Time) (*entity.BusinessHoursException, error)
	CreateFn func(ctx context.Context, e *entity.BusinessHoursException) (*entity.BusinessHoursException, error)
	ListFn   func(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error)
	DeleteFn func(ctx context.Context, id int) error
}

func (m *mockHoursExceptionRepo) Get(ctx context.Context, date time.Time) (*entity.BusinessHoursException, error) {
	if m.GetFn == nil {
		panic("mockHoursExceptionRepo.GetFn not set")
	}
	return m.GetFn(ctx, date)
}

func (m *mockHoursExceptionRepo) Create(ctx context.Context, e *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
	if m.CreateFn == nil {
		panic("mockHoursExceptionRepo.CreateFn not set")
	}
	return m.CreateFn(ctx, e)
}

func (m *mockHoursExceptionRepo) List(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
	if m.ListFn == nil {
		panic("mockHoursExceptionRepo.ListFn not set")
	}
	return m.ListFn(ctx, from, to)
}

func (m *mockHoursExceptionRepo) Delete(ctx context.Context, id int) error {
	if m.DeleteFn == nil {
		panic("mockHoursExceptionRepo.DeleteFn not set")
	}
	return m.DeleteFn(ctx, id)
}

// Spanish messages pinned by design.md §4. Kept as literals so the test fails
// if the implementation drifts from the owner-approved wording.
const (
	msgExceptionInvalid  = "la excepción no es válida. Si el negocio está cerrado, no envíes open_time/close_time; si está abierto, ambos son obligatorios en formato HH:MM y open_time debe ser anterior a close_time."
	msgExceptionConflict = "ya existe una excepción para esa fecha. Borrala y creala de nuevo si querés cambiar el horario de ese día."
	msgExceptionNotFound = "no existe una excepción con ese id."
	msgExceptionRangeBad = "las fechas del rango deben tener formato YYYY-MM-DD."
)

// ─── A) create_hours_exception ─────────────────────────────────────────────

func TestCreateHoursExceptionUseCase_ClosedException(t *testing.T) {
	logger, buf := captureAuditLogger()
	var created *entity.BusinessHoursException
	repo := &mockHoursExceptionRepo{
		CreateFn: func(_ context.Context, e *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			created = e
			stored := *created
			stored.ID = 7
			return &stored, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, logger)

	result, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-12-25",
		IsClosed:      true,
		Reason:        ptr("Navidad"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created == nil {
		t.Fatal("Create not called")
	}
	if created.ExceptionDate != "2026-12-25" || !created.IsClosed {
		t.Errorf("persisted exception = %+v, want a closed 2026-12-25 exception", created)
	}
	if created.OpenTime != nil || created.CloseTime != nil {
		t.Errorf("closed exception must persist nil times, got %v/%v", created.OpenTime, created.CloseTime)
	}
	if created.Reason == nil || *created.Reason != "Navidad" {
		t.Errorf("Reason = %v, want Navidad", created.Reason)
	}
	if result == nil || result.ID != 7 {
		t.Fatalf("result = %+v, want the stored row (ID 7)", result)
	}
	assertMaintenanceAudit(t, buf, "create_hours_exception", "business_hours_exception", "7", "create")
}

func TestCreateHoursExceptionUseCase_OpenExceptionWithWindow(t *testing.T) {
	var created *entity.BusinessHoursException
	repo := &mockHoursExceptionRepo{
		CreateFn: func(_ context.Context, e *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			created = e
			stored := *created
			stored.ID = 9
			return &stored, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	open, close := "10:00", "14:00"
	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-07-13",
		IsClosed:      false,
		OpenTime:      &open,
		CloseTime:     &close,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created == nil || created.IsClosed {
		t.Fatalf("persisted exception = %+v, want an open exception", created)
	}
	if created.OpenTime == nil || *created.OpenTime != "10:00" || created.CloseTime == nil || *created.CloseTime != "14:00" {
		t.Errorf("window = %v-%v, want 10:00-14:00", created.OpenTime, created.CloseTime)
	}
}

func TestCreateHoursExceptionUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	createCalls := 0
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			createCalls++
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, logger)

	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
			Caller:        caller,
			ExceptionDate: "2026-12-25",
			IsClosed:      true,
		})
		return err
	})
	if createCalls != 0 {
		t.Errorf("repository reached for a non-owner caller (Create=%d)", createCalls)
	}
}

func TestCreateHoursExceptionUseCase_ClosedWithTimesRejected(t *testing.T) {
	open, close := "10:00", "14:00"
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return nil, fmt.Errorf("crear excepción: si está cerrado, no se deben especificar horarios: %w", domain.ErrInvalidInput)
		},
		GetFn: func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
			t.Fatal("read-back must not run after a rejected write")
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-12-25",
		IsClosed:      true,
		OpenTime:      &open,
		CloseTime:     &close,
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, msgExceptionInvalid)
}

func TestCreateHoursExceptionUseCase_OpenWithoutTimesRejected(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return nil, fmt.Errorf("crear excepción: si está abierto, se deben especificar hora de apertura y cierre: %w", domain.ErrInvalidInput)
		},
		GetFn: func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
			t.Fatal("read-back must not run after a rejected write")
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-07-13",
		IsClosed:      false,
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, msgExceptionInvalid)
}

func TestCreateHoursExceptionUseCase_OpenTimeNotBeforeCloseTimeRejected(t *testing.T) {
	open, close := "15:00", "09:00"
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return nil, fmt.Errorf("crear excepción: la hora de apertura (%s) debe ser anterior a la hora de cierre (%s): %w",
				open, close, domain.ErrInvalidInput)
		},
		GetFn: func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
			t.Fatal("read-back must not run after a rejected write")
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-07-13",
		IsClosed:      false,
		OpenTime:      &open,
		CloseTime:     &close,
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, msgExceptionInvalid)
}

func TestCreateHoursExceptionUseCase_MalformedDatePassthrough(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			// Shape produced by repository.validateExceptionDate.
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la fecha debe tener formato YYYY-MM-DD.",
				Cause:   fmt.Errorf("la fecha debe tener formato YYYY-MM-DD, se recibió: %q: %w", "25/12/2026", domain.ErrInvalidInput),
			}
		},
		GetFn: func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
			t.Fatal("read-back must not run after a rejected write")
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "25/12/2026",
		IsClosed:      true,
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "la fecha debe tener formato YYYY-MM-DD.")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("cause chain must keep domain.ErrInvalidInput, got %v", err)
	}
}

func TestCreateHoursExceptionUseCase_CalendarInvalidDatePassthrough(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la fecha no es una fecha válida (formato YYYY-MM-DD).",
				Cause:   fmt.Errorf("la fecha %q no es una fecha válida (formato YYYY-MM-DD): %w", "2026-02-30", domain.ErrInvalidInput),
			}
		},
		GetFn: func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
			t.Fatal("read-back must not run after a rejected write")
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-02-30",
		IsClosed:      true,
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "la fecha no es una fecha válida (formato YYYY-MM-DD).")
}

func TestCreateHoursExceptionUseCase_DuplicateDateConflict(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return nil, fmt.Errorf("crear excepción: la fecha %s ya existe: %w", "2026-12-25", domain.ErrConflict)
		},
		GetFn: func(context.Context, time.Time) (*entity.BusinessHoursException, error) {
			t.Fatal("read-back must not run after a rejected write")
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-12-25",
		IsClosed:      true,
	})
	assertSemanticError(t, err, domain.ErrCodeConflict, msgExceptionConflict)
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("cause chain must keep domain.ErrConflict, got %v", err)
	}
}

func TestCreateHoursExceptionUseCase_NilStoredRowStaysInternal(t *testing.T) {
	// Defensive contract of Create...RETURNING: a conforming implementation
	// never returns (nil, nil); if a future one does, the use case must fail
	// closed instead of logging/dereferencing a nil id.
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return nil, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-12-25",
		IsClosed:      true,
	})
	assertInternalErrorPrefix(t, err, "create_hours_exception:")
}

func TestCreateHoursExceptionUseCase_UnknownErrorStaysInternal(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return nil, errors.New("disk I/O error")
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-12-25",
		IsClosed:      true,
	})
	assertInternalErrorPrefix(t, err, "create_hours_exception:")
}

func TestCreateHoursExceptionUseCase_NilLoggerDoesNotPanic(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		CreateFn: func(context.Context, *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
			return &entity.BusinessHoursException{ID: 1, ExceptionDate: "2026-12-25", IsClosed: true}, nil
		},
	}
	uc := NewCreateHoursExceptionUseCase(repo, nil)

	if _, err := uc.Execute(context.Background(), dto.CreateHoursExceptionInput{
		Caller:        ownerCaller(),
		ExceptionDate: "2026-12-25",
		IsClosed:      true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// assertInternalErrorPrefix asserts err is NOT a *domain.SemanticError (it must
// collapse to -32603 at the transport) and keeps the use-case context prefix.
func assertInternalErrorPrefix(t *testing.T, err error, prefix string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with prefix %q, got nil", prefix)
	}
	var sem *domain.SemanticError
	if errors.As(err, &sem) {
		t.Fatalf("unclassified error must stay internal, got semantic %s: %q", sem.Code, sem.Message)
	}
	if !strings.Contains(err.Error(), prefix) {
		t.Errorf("error %q must keep the use-case context prefix %q", err, prefix)
	}
}

// ─── B) delete_hours_exception ─────────────────────────────────────────────

func TestDeleteHoursExceptionUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	deletedID := -1
	repo := &mockHoursExceptionRepo{
		DeleteFn: func(_ context.Context, id int) error {
			deletedID = id
			return nil
		},
	}
	uc := NewDeleteHoursExceptionUseCase(repo, logger)

	result, err := uc.Execute(context.Background(), dto.DeleteHoursExceptionInput{Caller: ownerCaller(), ExceptionID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedID != 42 {
		t.Errorf("Delete id = %d, want 42", deletedID)
	}
	if result == nil || result.ExceptionID != 42 || result.Status != "deleted" {
		t.Errorf("result = %+v, want {42 deleted}", result)
	}
	assertMaintenanceAudit(t, buf, "delete_hours_exception", "business_hours_exception", "42", "delete")
}

func TestDeleteHoursExceptionUseCase_NotFound(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		DeleteFn: func(context.Context, int) error {
			return fmt.Errorf("eliminar excepción: %w", domain.ErrNotFound)
		},
	}
	uc := NewDeleteHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.DeleteHoursExceptionInput{Caller: ownerCaller(), ExceptionID: 999})
	assertSemanticError(t, err, domain.ErrCodeNotFound, msgExceptionNotFound)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cause chain must keep domain.ErrNotFound, got %v", err)
	}
}

func TestDeleteHoursExceptionUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	deleteCalls := 0
	repo := &mockHoursExceptionRepo{
		DeleteFn: func(context.Context, int) error {
			deleteCalls++
			return nil
		},
	}
	uc := NewDeleteHoursExceptionUseCase(repo, logger)

	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.DeleteHoursExceptionInput{Caller: caller, ExceptionID: 42})
		return err
	})
	if deleteCalls != 0 {
		t.Errorf("repository reached for a non-owner caller (Delete=%d)", deleteCalls)
	}
}

func TestDeleteHoursExceptionUseCase_UnknownErrorStaysInternal(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		DeleteFn: func(context.Context, int) error { return errors.New("disk I/O error") },
	}
	uc := NewDeleteHoursExceptionUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.DeleteHoursExceptionInput{Caller: ownerCaller(), ExceptionID: 42})
	assertInternalErrorPrefix(t, err, "delete_hours_exception:")
}

// ─── C) list_hours_exceptions ──────────────────────────────────────────────

func TestListHoursExceptionsUseCase_InclusiveRangePassthrough(t *testing.T) {
	var gotFrom, gotTo time.Time
	stored := []*entity.BusinessHoursException{
		{ID: 1, ExceptionDate: "2026-12-24", IsClosed: false},
		{ID: 2, ExceptionDate: "2026-12-25", IsClosed: true},
		{ID: 3, ExceptionDate: "2026-12-26", IsClosed: true},
	}
	repo := &mockHoursExceptionRepo{
		ListFn: func(_ context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
			gotFrom, gotTo = from, to
			return stored, nil
		},
	}
	uc := NewListHoursExceptionsUseCase(repo)

	rows, err := uc.Execute(context.Background(), dto.ListHoursExceptionsInput{
		Caller: ownerCaller(),
		From:   "2026-12-24",
		To:     "2026-12-26",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Bounds are forwarded verbatim (inclusive on both ends).
	if gotFrom.Format("2006-01-02") != "2026-12-24" || gotTo.Format("2006-01-02") != "2026-12-26" {
		t.Errorf("List bounds = %s..%s, want 2026-12-24..2026-12-26", gotFrom.Format("2006-01-02"), gotTo.Format("2006-01-02"))
	}
	if len(rows) != 3 || rows[0].ExceptionDate != "2026-12-24" || rows[2].ExceptionDate != "2026-12-26" {
		t.Errorf("rows = %+v, want the three stored exceptions in order", rows)
	}
	// No audit assertion is needed here: the list use case has no logger
	// dependency at all (read-only, mirrors get_business_profile), so it cannot
	// emit a maintenance-mutation record.
}

func TestListHoursExceptionsUseCase_EmptyResultIsNotAnError(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		ListFn: func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
			return nil, nil
		},
	}
	uc := NewListHoursExceptionsUseCase(repo)

	rows, err := uc.Execute(context.Background(), dto.ListHoursExceptionsInput{
		Caller: ownerCaller(),
		From:   "2026-01-01",
		To:     "2026-01-31",
	})
	if err != nil {
		t.Fatalf("empty range must not be an error, got %v", err)
	}
	if rows == nil {
		t.Error("rows = nil, want a non-nil empty slice ([] on the wire, not null)")
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0", len(rows))
	}
}

func TestListHoursExceptionsUseCase_MalformedFromRejected(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		ListFn: func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
			t.Fatal("List must not be called for a malformed bound")
			return nil, nil
		},
	}
	uc := NewListHoursExceptionsUseCase(repo)

	_, err := uc.Execute(context.Background(), dto.ListHoursExceptionsInput{
		Caller: ownerCaller(),
		From:   "24/12/2026",
		To:     "2026-12-26",
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, msgExceptionRangeBad)
}

func TestListHoursExceptionsUseCase_MalformedToRejected(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		ListFn: func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
			t.Fatal("List must not be called for a malformed bound")
			return nil, nil
		},
	}
	uc := NewListHoursExceptionsUseCase(repo)

	_, err := uc.Execute(context.Background(), dto.ListHoursExceptionsInput{
		Caller: ownerCaller(),
		From:   "2026-12-24",
		To:     "2026-13-40",
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, msgExceptionRangeBad)
}

func TestListHoursExceptionsUseCase_Forbidden(t *testing.T) {
	listCalls := 0
	repo := &mockHoursExceptionRepo{
		ListFn: func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
			listCalls++
			return nil, nil
		},
	}
	uc := NewListHoursExceptionsUseCase(repo)

	for _, caller := range maintenanceNonOwners() {
		_, err := uc.Execute(context.Background(), dto.ListHoursExceptionsInput{
			Caller: caller,
			From:   "2026-12-24",
			To:     "2026-12-26",
		})
		assertSemanticError(t, err, domain.ErrCodeForbidden, "no tienes permiso para realizar esta acción")
	}
	if listCalls != 0 {
		t.Errorf("repository reached for a non-owner caller (List=%d)", listCalls)
	}
}

func TestListHoursExceptionsUseCase_UnknownErrorStaysInternal(t *testing.T) {
	repo := &mockHoursExceptionRepo{
		ListFn: func(context.Context, time.Time, time.Time) ([]*entity.BusinessHoursException, error) {
			return nil, errors.New("disk I/O error")
		},
	}
	uc := NewListHoursExceptionsUseCase(repo)

	_, err := uc.Execute(context.Background(), dto.ListHoursExceptionsInput{
		Caller: ownerCaller(),
		From:   "2026-12-24",
		To:     "2026-12-26",
	})
	assertInternalErrorPrefix(t, err, "list_hours_exceptions:")
}
