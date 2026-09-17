package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ─── Shared helpers for the T2 maintenance use-case tests ───────────────────

// maintenanceNonOwners returns the roles the owner-only gate (ADR-0015
// Decision 2) must reject: admin is explicitly out of scope for this MVP, and
// staff/client are never allowed.
func maintenanceNonOwners() []auth.Caller {
	return []auth.Caller{adminCaller(), staffCaller("s1", "p1"), clientCaller("c1")}
}

// assertSemanticError asserts err is a *domain.SemanticError with the given
// code, and with the given message when message is non-empty.
func assertSemanticError(t *testing.T, err error, code domain.ErrCode, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", code)
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
	}
	if sem.Code != code {
		t.Fatalf("code = %s, want %s (message %q)", sem.Code, code, sem.Message)
	}
	if message != "" && sem.Message != message {
		t.Fatalf("message = %q, want %q", sem.Message, message)
	}
}

// captureAuditLogger returns a logger writing JSON records into buf.
func captureAuditLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, nil)), buf
}

// assertMaintenanceAudit asserts the mutation emitted exactly ONE structured
// audit record with the expected attributes and no caller PII.
func assertMaintenanceAudit(t *testing.T, buf *bytes.Buffer, tool, entityName, entityID, action string) {
	t.Helper()
	records := auditRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("audit records = %d, want exactly 1 (%v)", len(records), records)
	}
	rec := records[0]
	if rec["msg"] != maintenanceAuditMessage {
		t.Errorf("msg = %v, want %q", rec["msg"], maintenanceAuditMessage)
	}
	if rec["tool"] != tool {
		t.Errorf("tool = %v, want %q", rec["tool"], tool)
	}
	if rec["entity"] != entityName {
		t.Errorf("entity = %v, want %q", rec["entity"], entityName)
	}
	if rec["action"] != action {
		t.Errorf("action = %v, want %q", rec["action"], action)
	}
	if rec["role"] != auth.RoleOwner {
		t.Errorf("role = %v, want %q", rec["role"], auth.RoleOwner)
	}
	if entityID != "" && rec["entity_id"] != entityID {
		t.Errorf("entity_id = %v, want %q", rec["entity_id"], entityID)
	}
	ts, ok := rec["ts"].(string)
	if !ok {
		t.Fatalf("ts missing or not a string: %v", rec["ts"])
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("ts %q is not RFC3339Nano: %v", ts, err)
	}
	if strings.Contains(buf.String(), "owner1") {
		t.Error("audit record leaked the caller ID (PII)")
	}
}

// auditRecords decodes every JSON line captured by captureAuditLogger.
func auditRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(buf.String())
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("audit record is not valid JSON: %v (%s)", err, line)
		}
		records = append(records, rec)
	}
	return records
}

// assertOwnerOnly runs execute for every non-owner role and asserts the
// owner-only gate denied each one with the shared forbidden message and without
// emitting an audit record.
func assertOwnerOnly(t *testing.T, buf *bytes.Buffer, execute func(caller auth.Caller) error) {
	t.Helper()
	for _, caller := range maintenanceNonOwners() {
		err := execute(caller)
		assertSemanticError(t, err, domain.ErrCodeForbidden, "no tienes permiso para realizar esta acción")
	}
	if records := auditRecords(t, buf); len(records) != 0 {
		t.Fatalf("denied mutations emitted %d audit records, want 0", len(records))
	}
}

// storedProfile is a minimal persisted business profile for merge tests.
func storedProfile() *entity.BusinessProfile {
	return &entity.BusinessProfile{
		ID:            "singleton",
		Name:          "Peluquería Vieja",
		Timezone:      "UTC",
		CurrencyCode:  "ARS",
		BusinessHours: `{"1":{"open":"09:00","close":"18:00"}}`,
	}
}

// ─── A) update_business_profile ────────────────────────────────────────────

func TestUpdateBusinessProfileUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var saved *entity.BusinessProfile
	repo := &mockBusinessProfileRepo{
		GetFn: func(context.Context) (*entity.BusinessProfile, error) { return storedProfile(), nil },
		UpdateFn: func(_ context.Context, p *entity.BusinessProfile) error {
			saved = p
			return nil
		},
	}
	uc := NewUpdateBusinessProfileUseCase(repo, logger)

	newName := "Peluquería Nueva"
	newHours := `{"2":{"open":"10:00","close":"19:00"}}`
	result, err := uc.Execute(context.Background(), dto.UpdateBusinessProfileInput{
		Caller:        ownerCaller(),
		Name:          &newName,
		BusinessHours: &newHours,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if saved == nil {
		t.Fatal("Update not called")
	}
	if saved.Name != newName {
		t.Errorf("Name = %q, want %q", saved.Name, newName)
	}
	if saved.BusinessHours != newHours {
		t.Errorf("BusinessHours = %q, want %q", saved.BusinessHours, newHours)
	}
	// Fields absent from the payload keep their stored value.
	if saved.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want untouched %q", saved.Timezone, "UTC")
	}
	if saved.CurrencyCode != "ARS" {
		t.Errorf("CurrencyCode = %q, want untouched %q", saved.CurrencyCode, "ARS")
	}
	if result != saved {
		t.Error("result is not the merged entity that was persisted")
	}
	assertMaintenanceAudit(t, buf, "update_business_profile", "business_profile", "singleton", "update")
}

func TestUpdateBusinessProfileUseCase_RejectsEmptyUpdate(t *testing.T) {
	repo := &mockBusinessProfileRepo{
		GetFn: func(context.Context) (*entity.BusinessProfile, error) { return storedProfile(), nil },
		UpdateFn: func(context.Context, *entity.BusinessProfile) error {
			t.Fatal("Update must not be called for an empty update")
			return nil
		},
	}
	uc := NewUpdateBusinessProfileUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.UpdateBusinessProfileInput{Caller: ownerCaller()})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "no se proporcionaron campos para actualizar")
}

func TestUpdateBusinessProfileUseCase_NotFound(t *testing.T) {
	repo := &mockBusinessProfileRepo{
		GetFn: func(context.Context) (*entity.BusinessProfile, error) { return nil, domain.ErrNotFound },
	}
	uc := NewUpdateBusinessProfileUseCase(repo, nil)

	name := "X"
	_, err := uc.Execute(context.Background(), dto.UpdateBusinessProfileInput{Caller: ownerCaller(), Name: &name})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "el perfil del negocio no existe")
}

func TestUpdateBusinessProfileUseCase_InvalidInput(t *testing.T) {
	repo := &mockBusinessProfileRepo{
		GetFn:    func(context.Context) (*entity.BusinessProfile, error) { return storedProfile(), nil },
		UpdateFn: func(context.Context, *entity.BusinessProfile) error { return domain.ErrInvalidInput },
	}
	uc := NewUpdateBusinessProfileUseCase(repo, nil)

	hours := "{invalid"
	_, err := uc.Execute(context.Background(), dto.UpdateBusinessProfileInput{Caller: ownerCaller(), BusinessHours: &hours})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("cause chain must keep the repository error, got %v", err)
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
	}
	if strings.Contains(sem.Message, "invalid input") {
		t.Errorf("message must stay semantic, got %q", sem.Message)
	}
}

func TestUpdateBusinessProfileUseCase_Conflict(t *testing.T) {
	repo := &mockBusinessProfileRepo{
		GetFn:    func(context.Context) (*entity.BusinessProfile, error) { return storedProfile(), nil },
		UpdateFn: func(context.Context, *entity.BusinessProfile) error { return domain.ErrConflict },
	}
	uc := NewUpdateBusinessProfileUseCase(repo, nil)

	name := "X"
	_, err := uc.Execute(context.Background(), dto.UpdateBusinessProfileInput{Caller: ownerCaller(), Name: &name})
	assertSemanticError(t, err, domain.ErrCodeConflict, "conflicto al actualizar el perfil del negocio")
}

func TestUpdateBusinessProfileUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockBusinessProfileRepo{
		GetFn:    func(context.Context) (*entity.BusinessProfile, error) { called = true; return storedProfile(), nil },
		UpdateFn: func(context.Context, *entity.BusinessProfile) error { called = true; return nil },
	}
	uc := NewUpdateBusinessProfileUseCase(repo, logger)

	name := "X"
	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.UpdateBusinessProfileInput{Caller: caller, Name: &name})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── B) create_service ─────────────────────────────────────────────────────

func TestCreateServiceUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var saved *entity.Service
	repo := &mockServicesRepo{
		SaveFn: func(_ context.Context, s *entity.Service) error {
			saved = s
			return nil
		},
	}
	uc := NewCreateServiceUseCase(repo, logger)

	description := "Corte clásico"
	result, err := uc.Execute(context.Background(), dto.CreateServiceInput{
		Caller:          ownerCaller(),
		Name:            "Corte",
		Description:     &description,
		DurationMinutes: 30,
		Price:           1500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == nil {
		t.Fatal("Save not called")
	}
	if len(saved.ID) != 36 {
		t.Errorf("ID = %q, want a UUID v4", saved.ID)
	}
	if saved.ID != result.ID {
		t.Errorf("result ID = %q, want the persisted ID %q", result.ID, saved.ID)
	}
	// An omitted is_active creates an active, bookable service.
	if !saved.Active {
		t.Error("Active = false, want default true when is_active is omitted")
	}
	if saved.Name != "Corte" || saved.DurationMinutes != 30 || saved.Price != 1500 {
		t.Errorf("unexpected persisted service: %+v", saved)
	}
	assertMaintenanceAudit(t, buf, "create_service", "service", saved.ID, "create")
}

func TestCreateServiceUseCase_ExplicitInactive(t *testing.T) {
	var saved *entity.Service
	repo := &mockServicesRepo{
		SaveFn: func(_ context.Context, s *entity.Service) error {
			saved = s
			return nil
		},
	}
	uc := NewCreateServiceUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateServiceInput{
		Caller:          ownerCaller(),
		Name:            "Servicio pausado",
		DurationMinutes: 15,
		Price:           100,
		Active:          ptrBool(false),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == nil || saved.Active {
		t.Fatalf("Active = %v, want false when is_active=false", saved)
	}
}

func TestCreateServiceUseCase_InvalidInput(t *testing.T) {
	repo := &mockServicesRepo{
		SaveFn: func(context.Context, *entity.Service) error { return domain.ErrInvalidInput },
	}
	uc := NewCreateServiceUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateServiceInput{Caller: ownerCaller(), Name: "", DurationMinutes: 0, Price: 0})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "")
}

func TestCreateServiceUseCase_Conflict(t *testing.T) {
	repo := &mockServicesRepo{
		SaveFn: func(context.Context, *entity.Service) error { return domain.ErrConflict },
	}
	uc := NewCreateServiceUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateServiceInput{Caller: ownerCaller(), Name: "Corte", DurationMinutes: 30, Price: 100})
	assertSemanticError(t, err, domain.ErrCodeConflict, "ya existe un servicio con esos datos")
}

func TestCreateServiceUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockServicesRepo{
		SaveFn: func(context.Context, *entity.Service) error { called = true; return nil },
	}
	uc := NewCreateServiceUseCase(repo, logger)

	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.CreateServiceInput{Caller: caller, Name: "Corte", DurationMinutes: 30, Price: 100})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── C) update_service ─────────────────────────────────────────────────────

func TestUpdateServiceUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var saved *entity.Service
	repo := &mockServicesRepo{
		FindByIDFn: func(_ context.Context, id string) (*entity.Service, error) {
			if id != "s1" {
				t.Fatalf("FindByID id = %q, want s1", id)
			}
			return &entity.Service{ID: "s1", Name: "Corte", DurationMinutes: 30, Price: 1500, Active: true}, nil
		},
		UpdateFn: func(_ context.Context, s *entity.Service) error {
			saved = s
			return nil
		},
	}
	uc := NewUpdateServiceUseCase(repo, logger)

	newName := "Corte premium"
	newPrice := 2000.0
	result, err := uc.Execute(context.Background(), dto.UpdateServiceInput{
		Caller:    ownerCaller(),
		ServiceID: "s1",
		Name:      &newName,
		Price:     &newPrice,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == nil {
		t.Fatal("Update not called")
	}
	if saved.Name != newName || saved.Price != newPrice {
		t.Errorf("merge failed: %+v", saved)
	}
	if saved.DurationMinutes != 30 || !saved.Active {
		t.Errorf("untouched fields drifted: %+v", saved)
	}
	if result == nil || result.Price != newPrice {
		t.Errorf("result = %+v, want the merged service", result)
	}
	assertMaintenanceAudit(t, buf, "update_service", "service", "s1", "update")
}

func TestUpdateServiceUseCase_NotFound(t *testing.T) {
	repo := &mockServicesRepo{
		FindByIDFn: func(context.Context, string) (*entity.Service, error) { return nil, domain.ErrNotFound },
		UpdateFn: func(context.Context, *entity.Service) error {
			t.Fatal("Update must not be called when the service does not exist")
			return nil
		},
	}
	uc := NewUpdateServiceUseCase(repo, nil)

	name := "Corte"
	_, err := uc.Execute(context.Background(), dto.UpdateServiceInput{Caller: ownerCaller(), ServiceID: "missing", Name: &name})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "el servicio no existe")
}

func TestUpdateServiceUseCase_InvalidInputFromRepository(t *testing.T) {
	repo := &mockServicesRepo{
		FindByIDFn: func(context.Context, string) (*entity.Service, error) {
			return &entity.Service{ID: "s1", Name: "Corte", DurationMinutes: 30, Price: 1500, Active: true}, nil
		},
		UpdateFn: func(context.Context, *entity.Service) error { return domain.ErrInvalidInput },
	}
	uc := NewUpdateServiceUseCase(repo, nil)

	duration := 0
	_, err := uc.Execute(context.Background(), dto.UpdateServiceInput{Caller: ownerCaller(), ServiceID: "s1", DurationMinutes: &duration})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "")
}

func TestUpdateServiceUseCase_Conflict(t *testing.T) {
	repo := &mockServicesRepo{
		FindByIDFn: func(context.Context, string) (*entity.Service, error) {
			return &entity.Service{ID: "s1", Name: "Corte", DurationMinutes: 30, Price: 1500, Active: true}, nil
		},
		UpdateFn: func(context.Context, *entity.Service) error { return domain.ErrConflict },
	}
	uc := NewUpdateServiceUseCase(repo, nil)

	name := "Corte"
	_, err := uc.Execute(context.Background(), dto.UpdateServiceInput{Caller: ownerCaller(), ServiceID: "s1", Name: &name})
	assertSemanticError(t, err, domain.ErrCodeConflict, "conflicto al actualizar el servicio")
}

func TestUpdateServiceUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockServicesRepo{
		FindByIDFn: func(context.Context, string) (*entity.Service, error) { called = true; return nil, nil },
		UpdateFn:   func(context.Context, *entity.Service) error { called = true; return nil },
	}
	uc := NewUpdateServiceUseCase(repo, logger)

	name := "Corte"
	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.UpdateServiceInput{Caller: caller, ServiceID: "s1", Name: &name})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── D) delete_service ─────────────────────────────────────────────────────

func TestDeleteServiceUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var deleted string
	repo := &mockServicesRepo{
		DeleteFn: func(_ context.Context, id string) error {
			deleted = id
			return nil
		},
	}
	uc := NewDeleteServiceUseCase(repo, logger)

	result, err := uc.Execute(context.Background(), dto.DeleteServiceInput{Caller: ownerCaller(), ServiceID: "s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != "s1" {
		t.Errorf("Delete called with %q, want s1", deleted)
	}
	if result == nil || result.ServiceID != "s1" || result.Status != "deleted" {
		t.Errorf("result = %+v, want {s1 deleted}", result)
	}
	assertMaintenanceAudit(t, buf, "delete_service", "service", "s1", "delete")
}

func TestDeleteServiceUseCase_NotFound(t *testing.T) {
	repo := &mockServicesRepo{
		DeleteFn: func(context.Context, string) error { return domain.ErrNotFound },
	}
	uc := NewDeleteServiceUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.DeleteServiceInput{Caller: ownerCaller(), ServiceID: "missing"})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "el servicio no existe")
}

// TestDeleteServiceUseCase_ForeignKeyConflict pins the delete_service FK path:
// bookings.service_id is ON DELETE RESTRICT (internal/db/schema.go), so the
// repository classifies the resulting SQLite violation as domain.ErrConflict and
// the use case must answer with the conflict message instead of leaking an
// internal error (-32603).
func TestDeleteServiceUseCase_ForeignKeyConflict(t *testing.T) {
	repo := &mockServicesRepo{
		DeleteFn: func(context.Context, string) error {
			// Same shape the repository produces once the classifier is wired.
			return fmt.Errorf("eliminar servicio: %w", domain.ErrConflict)
		},
	}
	uc := NewDeleteServiceUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.DeleteServiceInput{Caller: ownerCaller(), ServiceID: "s1"})
	assertSemanticError(t, err, domain.ErrCodeConflict, "no se puede eliminar el servicio porque tiene reservas asociadas")
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("cause chain must keep domain.ErrConflict, got %v", err)
	}
}

// TestDeleteServiceUseCase_UnknownErrorStaysInternal pins the layering rule: the
// repository is the only layer allowed to translate a SQLite code, so a raw
// (unclassified) driver error must stay an internal error here. A toMCPError call
// would emit -32002 for an infrastructure failure if the use case guessed.
func TestDeleteServiceUseCase_UnknownErrorStaysInternal(t *testing.T) {
	repo := &mockServicesRepo{
		DeleteFn: func(context.Context, string) error { return errors.New("FOREIGN KEY constraint failed") },
	}
	uc := NewDeleteServiceUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.DeleteServiceInput{Caller: ownerCaller(), ServiceID: "s1"})
	if err == nil {
		t.Fatal("expected error")
	}
	var sem *domain.SemanticError
	if errors.As(err, &sem) {
		t.Fatalf("unclassified error must stay an internal error, got semantic %s: %q", sem.Code, sem.Message)
	}
	if !strings.Contains(err.Error(), "delete_service:") {
		t.Errorf("error %q must keep the use-case context prefix", err)
	}
}

func TestDeleteServiceUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockServicesRepo{
		DeleteFn: func(context.Context, string) error { called = true; return nil },
	}
	uc := NewDeleteServiceUseCase(repo, logger)

	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.DeleteServiceInput{Caller: caller, ServiceID: "s1"})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── E) create_professional ────────────────────────────────────────────────

func TestCreateProfessionalUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var saved *entity.Professional
	repo := &mockProfessionalsRepo{
		SaveFn: func(_ context.Context, p *entity.Professional) error {
			p.ID = "p-nuevo" // mirrors ProfessionalsRepo.Save (UUID v4)
			saved = p
			return nil
		},
	}
	uc := NewCreateProfessionalUseCase(repo, logger)

	specialty := "colorista"
	result, err := uc.Execute(context.Background(), dto.CreateProfessionalInput{
		Caller:        ownerCaller(),
		Name:          "Ana",
		RoleSpecialty: &specialty,
		Specialties:   []string{"s1", "s2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == nil {
		t.Fatal("Save not called")
	}
	if saved.ID != "p-nuevo" || result.ID != "p-nuevo" {
		t.Errorf("ID = %q / result ID = %q, want p-nuevo", saved.ID, result.ID)
	}
	if saved.Status != defaultProfessionalStatus {
		t.Errorf("Status = %q, want default %q", saved.Status, defaultProfessionalStatus)
	}
	if saved.Specialties == nil || *saved.Specialties != `["s1","s2"]` {
		t.Errorf("Specialties = %v, want the JSON array [\"s1\",\"s2\"]", saved.Specialties)
	}
	assertMaintenanceAudit(t, buf, "create_professional", "professional", "p-nuevo", "create")
}

func TestCreateProfessionalUseCase_ExplicitStatusAndNoSpecialties(t *testing.T) {
	var saved *entity.Professional
	repo := &mockProfessionalsRepo{
		SaveFn: func(_ context.Context, p *entity.Professional) error {
			saved = p
			return nil
		},
	}
	uc := NewCreateProfessionalUseCase(repo, nil)

	inactive := "inactive"
	_, err := uc.Execute(context.Background(), dto.CreateProfessionalInput{
		Caller: ownerCaller(),
		Name:   "Beto",
		Status: &inactive,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.Status != "inactive" {
		t.Errorf("Status = %q, want inactive", saved.Status)
	}
	if saved.Specialties != nil {
		t.Errorf("Specialties = %v, want nil when none were provided", saved.Specialties)
	}
}

func TestCreateProfessionalUseCase_MissingSpecialtyService(t *testing.T) {
	repo := &mockProfessionalsRepo{
		SaveFn: func(context.Context, *entity.Professional) error { return domain.ErrNotFound },
	}
	uc := NewCreateProfessionalUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateProfessionalInput{
		Caller:      ownerCaller(),
		Name:        "Ana",
		Specialties: []string{"ghost"},
	})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "uno de los servicios indicados en las especialidades no existe")
}

func TestCreateProfessionalUseCase_InvalidInput(t *testing.T) {
	repo := &mockProfessionalsRepo{
		SaveFn: func(context.Context, *entity.Professional) error { return domain.ErrInvalidInput },
	}
	uc := NewCreateProfessionalUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateProfessionalInput{Caller: ownerCaller(), Name: ""})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "")
}

func TestCreateProfessionalUseCase_Conflict(t *testing.T) {
	repo := &mockProfessionalsRepo{
		SaveFn: func(context.Context, *entity.Professional) error { return domain.ErrConflict },
	}
	uc := NewCreateProfessionalUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.CreateProfessionalInput{Caller: ownerCaller(), Name: "Ana"})
	assertSemanticError(t, err, domain.ErrCodeConflict, "ya existe un profesional con esos datos")
}

func TestCreateProfessionalUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockProfessionalsRepo{
		SaveFn: func(context.Context, *entity.Professional) error { called = true; return nil },
	}
	uc := NewCreateProfessionalUseCase(repo, logger)

	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.CreateProfessionalInput{Caller: caller, Name: "Ana"})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── F) update_professional ────────────────────────────────────────────────

func TestUpdateProfessionalUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var saved *entity.Professional
	repo := &mockProfessionalsRepo{
		FindByIDFn: func(_ context.Context, id string) (*entity.Professional, error) {
			if id != "p1" {
				t.Fatalf("FindByID id = %q, want p1", id)
			}
			return &entity.Professional{ID: "p1", Name: "Ana", Status: "active", Specialties: ptr(`["s1"]`)}, nil
		},
		UpdateFn: func(_ context.Context, p *entity.Professional) error {
			saved = p
			return nil
		},
	}
	uc := NewUpdateProfessionalUseCase(repo, logger)

	newName := "Ana María"
	inactive := "inactive"
	result, err := uc.Execute(context.Background(), dto.UpdateProfessionalInput{
		Caller:         ownerCaller(),
		ProfessionalID: "p1",
		Name:           &newName,
		Status:         &inactive,
		Specialties:    &[]string{"s2", "s3"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == nil {
		t.Fatal("Update not called")
	}
	if saved.Name != newName || saved.Status != "inactive" {
		t.Errorf("merge failed: %+v", saved)
	}
	if saved.Specialties == nil || *saved.Specialties != `["s2","s3"]` {
		t.Errorf("Specialties = %v, want wholesale replacement", saved.Specialties)
	}
	if result == nil || result.Name != newName {
		t.Errorf("result = %+v, want the merged professional", result)
	}
	assertMaintenanceAudit(t, buf, "update_professional", "professional", "p1", "update")
}

func TestUpdateProfessionalUseCase_KeepsSpecialtiesWhenOmitted(t *testing.T) {
	var saved *entity.Professional
	repo := &mockProfessionalsRepo{
		FindByIDFn: func(context.Context, string) (*entity.Professional, error) {
			return &entity.Professional{ID: "p1", Name: "Ana", Status: "active", Specialties: ptr(`["s1"]`)}, nil
		},
		UpdateFn: func(_ context.Context, p *entity.Professional) error {
			saved = p
			return nil
		},
	}
	uc := NewUpdateProfessionalUseCase(repo, nil)

	newName := "Ana"
	if _, err := uc.Execute(context.Background(), dto.UpdateProfessionalInput{
		Caller:         ownerCaller(),
		ProfessionalID: "p1",
		Name:           &newName,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.Specialties == nil || *saved.Specialties != `["s1"]` {
		t.Errorf("Specialties = %v, want the stored list untouched", saved.Specialties)
	}
}

func TestUpdateProfessionalUseCase_NotFound(t *testing.T) {
	repo := &mockProfessionalsRepo{
		FindByIDFn: func(context.Context, string) (*entity.Professional, error) { return nil, domain.ErrNotFound },
		UpdateFn: func(context.Context, *entity.Professional) error {
			t.Fatal("Update must not be called when the professional does not exist")
			return nil
		},
	}
	uc := NewUpdateProfessionalUseCase(repo, nil)

	name := "Ana"
	_, err := uc.Execute(context.Background(), dto.UpdateProfessionalInput{Caller: ownerCaller(), ProfessionalID: "missing", Name: &name})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "el profesional no existe")
}

func TestUpdateProfessionalUseCase_MissingSpecialtyService(t *testing.T) {
	repo := &mockProfessionalsRepo{
		FindByIDFn: func(context.Context, string) (*entity.Professional, error) {
			return &entity.Professional{ID: "p1", Name: "Ana", Status: "active"}, nil
		},
		UpdateFn: func(context.Context, *entity.Professional) error { return domain.ErrNotFound },
	}
	uc := NewUpdateProfessionalUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.UpdateProfessionalInput{
		Caller:         ownerCaller(),
		ProfessionalID: "p1",
		Specialties:    &[]string{"ghost"},
	})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "uno de los servicios indicados en las especialidades no existe")
}

func TestUpdateProfessionalUseCase_Conflict(t *testing.T) {
	repo := &mockProfessionalsRepo{
		FindByIDFn: func(context.Context, string) (*entity.Professional, error) {
			return &entity.Professional{ID: "p1", Name: "Ana", Status: "active"}, nil
		},
		UpdateFn: func(context.Context, *entity.Professional) error { return domain.ErrConflict },
	}
	uc := NewUpdateProfessionalUseCase(repo, nil)

	name := "Ana"
	_, err := uc.Execute(context.Background(), dto.UpdateProfessionalInput{Caller: ownerCaller(), ProfessionalID: "p1", Name: &name})
	assertSemanticError(t, err, domain.ErrCodeConflict, "conflicto al actualizar el profesional")
}

func TestUpdateProfessionalUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockProfessionalsRepo{
		FindByIDFn: func(context.Context, string) (*entity.Professional, error) { called = true; return nil, nil },
		UpdateFn:   func(context.Context, *entity.Professional) error { called = true; return nil },
	}
	uc := NewUpdateProfessionalUseCase(repo, logger)

	name := "Ana"
	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.UpdateProfessionalInput{Caller: caller, ProfessionalID: "p1", Name: &name})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── G) upsert_schedule ────────────────────────────────────────────────────

func TestUpsertScheduleUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var upserted *entity.Schedule
	repo := &mockSchedulesRepo{
		UpsertFn: func(_ context.Context, s *entity.Schedule) error {
			upserted = s
			return nil
		},
		FindByProfessionalAndDayFn: func(_ context.Context, professionalID string, day int) (*entity.Schedule, error) {
			return &entity.Schedule{ID: 7, ProfessionalID: professionalID, DayOfWeek: day, StartTime: "09:00", EndTime: "13:00"}, nil
		},
	}
	uc := NewUpsertScheduleUseCase(repo, logger)

	result, err := uc.Execute(context.Background(), dto.UpsertScheduleInput{
		Caller:         ownerCaller(),
		ProfessionalID: "p1",
		DayOfWeek:      3,
		StartTime:      "09:00",
		EndTime:        "13:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upserted == nil {
		t.Fatal("Upsert not called")
	}
	if upserted.ProfessionalID != "p1" || upserted.DayOfWeek != 3 || upserted.StartTime != "09:00" || upserted.EndTime != "13:00" {
		t.Errorf("unexpected upserted slot: %+v", upserted)
	}
	// The store assigns the surrogate key, so the result must be the read-back.
	if result == nil || result.ID != 7 {
		t.Errorf("result = %+v, want the stored row (ID 7)", result)
	}
	assertMaintenanceAudit(t, buf, "upsert_schedule", "schedule", "p1:3", "upsert")
}

func TestUpsertScheduleUseCase_InvalidInput(t *testing.T) {
	repo := &mockSchedulesRepo{
		UpsertFn: func(context.Context, *entity.Schedule) error { return domain.ErrInvalidInput },
	}
	uc := NewUpsertScheduleUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.UpsertScheduleInput{
		Caller:         ownerCaller(),
		ProfessionalID: "p1",
		DayOfWeek:      9,
		StartTime:      "09:00",
		EndTime:        "08:00",
	})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "")
}

func TestUpsertScheduleUseCase_ProfessionalNotFound(t *testing.T) {
	repo := &mockSchedulesRepo{
		UpsertFn: func(context.Context, *entity.Schedule) error { return domain.ErrNotFound },
	}
	uc := NewUpsertScheduleUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.UpsertScheduleInput{
		Caller:         ownerCaller(),
		ProfessionalID: "ghost",
		DayOfWeek:      1,
		StartTime:      "09:00",
		EndTime:        "13:00",
	})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "el profesional no existe")
}

// TestUpsertScheduleUseCase_ForeignKeyConflict pins the upsert_schedule FK path:
// schedules.professional_id references professionals(id) (internal/db/schema.go),
// so a slot for a professional that does not exist is a repository-classified
// domain.ErrConflict. Because that foreign key can only mean a missing parent row,
// the use case answers ErrCodeNotFound with a message naming the professional,
// and stays free of SQL internals.
func TestUpsertScheduleUseCase_ForeignKeyConflict(t *testing.T) {
	repo := &mockSchedulesRepo{
		UpsertFn: func(context.Context, *entity.Schedule) error {
			// Same shape the repository produces once the classifier is wired.
			return fmt.Errorf("upsert horario: %w", domain.ErrConflict)
		},
	}
	uc := NewUpsertScheduleUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.UpsertScheduleInput{
		Caller:         ownerCaller(),
		ProfessionalID: "ghost",
		DayOfWeek:      1,
		StartTime:      "09:00",
		EndTime:        "13:00",
	})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "el profesional indicado no existe")
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("cause chain must keep the repository sentinel domain.ErrConflict, got %v", err)
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
	}
	if strings.Contains(sem.Message, "FOREIGN KEY") || strings.Contains(sem.Message, "(1811)") {
		t.Errorf("semantic message must not leak driver internals, got %q", sem.Message)
	}
}

func TestUpsertScheduleUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockSchedulesRepo{
		UpsertFn:                   func(context.Context, *entity.Schedule) error { called = true; return nil },
		FindByProfessionalAndDayFn: func(context.Context, string, int) (*entity.Schedule, error) { called = true; return nil, nil },
	}
	uc := NewUpsertScheduleUseCase(repo, logger)

	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.UpsertScheduleInput{
			Caller:         caller,
			ProfessionalID: "p1",
			DayOfWeek:      1,
			StartTime:      "09:00",
			EndTime:        "13:00",
		})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── H) delete_schedule ────────────────────────────────────────────────────

func TestDeleteScheduleUseCase_HappyPath(t *testing.T) {
	logger, buf := captureAuditLogger()
	var deletedDay int
	repo := &mockSchedulesRepo{
		DeleteFn: func(_ context.Context, professionalID string, day int) error {
			if professionalID != "p1" {
				t.Fatalf("Delete professionalID = %q, want p1", professionalID)
			}
			deletedDay = day
			return nil
		},
	}
	uc := NewDeleteScheduleUseCase(repo, logger)

	result, err := uc.Execute(context.Background(), dto.DeleteScheduleInput{Caller: ownerCaller(), ProfessionalID: "p1", DayOfWeek: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedDay != 3 {
		t.Errorf("Delete day = %d, want 3", deletedDay)
	}
	if result == nil || result.ProfessionalID != "p1" || result.DayOfWeek != 3 || result.Status != "deleted" {
		t.Errorf("result = %+v, want {p1 3 deleted}", result)
	}
	assertMaintenanceAudit(t, buf, "delete_schedule", "schedule", "p1:3", "delete")
}

func TestDeleteScheduleUseCase_NotFound(t *testing.T) {
	repo := &mockSchedulesRepo{
		DeleteFn: func(context.Context, string, int) error { return domain.ErrNotFound },
	}
	uc := NewDeleteScheduleUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.DeleteScheduleInput{Caller: ownerCaller(), ProfessionalID: "p1", DayOfWeek: 3})
	assertSemanticError(t, err, domain.ErrCodeNotFound, "el profesional no tiene un horario ese día")
}

func TestDeleteScheduleUseCase_InvalidDay(t *testing.T) {
	repo := &mockSchedulesRepo{
		DeleteFn: func(context.Context, string, int) error { return domain.ErrInvalidInput },
	}
	uc := NewDeleteScheduleUseCase(repo, nil)

	_, err := uc.Execute(context.Background(), dto.DeleteScheduleInput{Caller: ownerCaller(), ProfessionalID: "p1", DayOfWeek: 9})
	assertSemanticError(t, err, domain.ErrCodeInvalidInput, "el día debe estar entre 0 (domingo) y 6 (sábado)")
}

func TestDeleteScheduleUseCase_Forbidden(t *testing.T) {
	logger, buf := captureAuditLogger()
	called := false
	repo := &mockSchedulesRepo{
		DeleteFn: func(context.Context, string, int) error { called = true; return nil },
	}
	uc := NewDeleteScheduleUseCase(repo, logger)

	assertOwnerOnly(t, buf, func(caller auth.Caller) error {
		_, err := uc.Execute(context.Background(), dto.DeleteScheduleInput{Caller: caller, ProfessionalID: "p1", DayOfWeek: 3})
		return err
	})
	if called {
		t.Error("repository was reached for a non-owner caller")
	}
}

// ─── Nil-logger safety ─────────────────────────────────────────────────────

// TestMaintenanceUseCases_NilLoggerDoesNotPanic covers the wiring mistake where
// the composition root passes a nil logger: every constructor must fall back to
// slog.Default() instead of nil-dereferencing on the mutation path.
func TestMaintenanceUseCases_NilLoggerDoesNotPanic(t *testing.T) {
	// Keep the fallback sink silent: the point is that the constructors do not
	// nil-dereference, not that they write to stderr.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(previous)

	service := &mockServicesRepo{DeleteFn: func(context.Context, string) error { return nil }}
	if _, err := NewDeleteServiceUseCase(service, nil).Execute(context.Background(), dto.DeleteServiceInput{Caller: ownerCaller(), ServiceID: "s1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	schedules := &mockSchedulesRepo{DeleteFn: func(context.Context, string, int) error { return nil }}
	if _, err := NewDeleteScheduleUseCase(schedules, nil).Execute(context.Background(), dto.DeleteScheduleInput{Caller: ownerCaller(), ProfessionalID: "p1", DayOfWeek: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
