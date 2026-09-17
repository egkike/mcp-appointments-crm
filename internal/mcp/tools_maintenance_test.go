package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ── mock ports for the eight maintenance WRITE tools (fn-table, same pattern
// as the other ports in tools_test.go) ──

type mockUpdateBusinessProfilePort struct {
	executeFn func(ctx context.Context, in dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error)
}

func (m *mockUpdateBusinessProfilePort) Execute(ctx context.Context, in dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
	return m.executeFn(ctx, in)
}

type mockCreateServicePort struct {
	executeFn func(ctx context.Context, in dto.CreateServiceInput) (*entity.Service, error)
}

func (m *mockCreateServicePort) Execute(ctx context.Context, in dto.CreateServiceInput) (*entity.Service, error) {
	return m.executeFn(ctx, in)
}

type mockUpdateServicePort struct {
	executeFn func(ctx context.Context, in dto.UpdateServiceInput) (*entity.Service, error)
}

func (m *mockUpdateServicePort) Execute(ctx context.Context, in dto.UpdateServiceInput) (*entity.Service, error) {
	return m.executeFn(ctx, in)
}

type mockDeleteServicePort struct {
	executeFn func(ctx context.Context, in dto.DeleteServiceInput) (*dto.DeleteServiceResult, error)
}

func (m *mockDeleteServicePort) Execute(ctx context.Context, in dto.DeleteServiceInput) (*dto.DeleteServiceResult, error) {
	return m.executeFn(ctx, in)
}

type mockCreateProfessionalPort struct {
	executeFn func(ctx context.Context, in dto.CreateProfessionalInput) (*entity.Professional, error)
}

func (m *mockCreateProfessionalPort) Execute(ctx context.Context, in dto.CreateProfessionalInput) (*entity.Professional, error) {
	return m.executeFn(ctx, in)
}

type mockUpdateProfessionalPort struct {
	executeFn func(ctx context.Context, in dto.UpdateProfessionalInput) (*entity.Professional, error)
}

func (m *mockUpdateProfessionalPort) Execute(ctx context.Context, in dto.UpdateProfessionalInput) (*entity.Professional, error) {
	return m.executeFn(ctx, in)
}

type mockUpsertSchedulePort struct {
	executeFn func(ctx context.Context, in dto.UpsertScheduleInput) (*entity.Schedule, error)
}

func (m *mockUpsertSchedulePort) Execute(ctx context.Context, in dto.UpsertScheduleInput) (*entity.Schedule, error) {
	return m.executeFn(ctx, in)
}

type mockDeleteSchedulePort struct {
	executeFn func(ctx context.Context, in dto.DeleteScheduleInput) (*dto.DeleteScheduleResult, error)
}

func (m *mockDeleteSchedulePort) Execute(ctx context.Context, in dto.DeleteScheduleInput) (*dto.DeleteScheduleResult, error) {
	return m.executeFn(ctx, in)
}

// ── shared helpers for the cross-cutting maintenance tests ──

// maintenanceToolArgs holds a schema-valid arguments payload per maintenance
// tool, so cross-cutting tests (missing caller, error mapping) reach the
// handler instead of failing the SDK argument validation first.
var maintenanceToolArgs = map[string]string{
	"update_business_profile": `{"name":"Nuevo Nombre"}`,
	"create_service":          `{"name":"Corte","duration_minutes":30,"price":1000}`,
	"update_service":          `{"service_id":"s1","name":"Corte"}`,
	"delete_service":          `{"service_id":"s1"}`,
	"create_professional":     `{"name":"Ana"}`,
	"update_professional":     `{"professional_id":"p1","name":"Ana"}`,
	"upsert_schedule":         `{"professional_id":"p1","day_of_week":1,"start_time":"09:00","end_time":"18:00"}`,
	"delete_schedule":         `{"professional_id":"p1","day_of_week":1}`,
}

// maintenanceToolNames returns the eight tool names in deterministic order.
func maintenanceToolNames() []string {
	names := make([]string, 0, len(maintenanceToolArgs))
	for name := range maintenanceToolArgs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// stubMaintenancePortsFail makes every maintenance port return (nil, err), so
// the shared error mapping of all eight handlers is asserted in one table.
func stubMaintenancePortsFail(p *mockToolPorts, err error) {
	p.updateProfile.executeFn = func(context.Context, dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
		return nil, err
	}
	p.createService.executeFn = func(context.Context, dto.CreateServiceInput) (*entity.Service, error) {
		return nil, err
	}
	p.updateService.executeFn = func(context.Context, dto.UpdateServiceInput) (*entity.Service, error) {
		return nil, err
	}
	p.deleteService.executeFn = func(context.Context, dto.DeleteServiceInput) (*dto.DeleteServiceResult, error) {
		return nil, err
	}
	p.createProfessional.executeFn = func(context.Context, dto.CreateProfessionalInput) (*entity.Professional, error) {
		return nil, err
	}
	p.updateProfessional.executeFn = func(context.Context, dto.UpdateProfessionalInput) (*entity.Professional, error) {
		return nil, err
	}
	p.upsertSchedule.executeFn = func(context.Context, dto.UpsertScheduleInput) (*entity.Schedule, error) {
		return nil, err
	}
	p.deleteSchedule.executeFn = func(context.Context, dto.DeleteScheduleInput) (*dto.DeleteScheduleResult, error) {
		return nil, err
	}
}

// stubMaintenancePortsNilSuccess makes every maintenance port break its
// documented contract by reporting success without a result.
func stubMaintenancePortsNilSuccess(p *mockToolPorts) {
	stubMaintenancePortsFail(p, nil)
}

// ── update_business_profile ──

func TestToolUpdateBusinessProfile(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.updateProfile.executeFn = func(ctx context.Context, in dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
		if in.Caller.ID != "owner-1" || in.Caller.Role != auth.RoleOwner {
			t.Errorf("input.Caller = %+v; want owner-1/owner", in.Caller)
		}
		if in.Name == nil || *in.Name != "Nuevo Nombre" {
			t.Errorf("input.Name = %v; want Nuevo Nombre", in.Name)
		}
		if in.PublicPhone == nil || *in.PublicPhone != "+5491100000000" {
			t.Errorf("input.PublicPhone = %v; want +5491100000000", in.PublicPhone)
		}
		// Partial merge: every field the caller omitted stays nil.
		if in.Industry != nil || in.Country != nil || in.Address != nil || in.Latitude != nil ||
			in.Longitude != nil || in.CoverPhotoURL != nil || in.MessengerPlatform != nil ||
			in.MessengerID != nil || in.ContactEmail != nil || in.WebsiteURL != nil ||
			in.GeneralDescription != nil || in.CurrencyCode != nil || in.CurrencySymbol != nil ||
			in.AcceptedPaymentMethods != nil || in.Timezone != nil || in.SlotIntervalMinutes != nil ||
			in.BusinessHours != nil {
			t.Errorf("omitted fields must stay nil (partial merge); got %+v", in)
		}
		return &entity.BusinessProfile{ID: "singleton", Name: "Nuevo Nombre", PublicPhone: in.PublicPhone}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_business_profile",
		`{"name":"Nuevo Nombre","public_phone":"+5491100000000"}`))

	var out businessProfileOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Name != "Nuevo Nombre" || out.PublicPhone == nil || *out.PublicPhone != "+5491100000000" {
		t.Errorf("profile = %+v; want the merged values (same shape as get_business_profile)", out)
	}
}

// TestToolUpdateBusinessProfileMapsEveryField pins the full 19-field mapping so
// an omitted assignment cannot silently drop a field.
func TestToolUpdateBusinessProfileMapsEveryField(t *testing.T) {
	srv, ports := newToolServer(t)
	var got dto.UpdateBusinessProfileInput
	ports.updateProfile.executeFn = func(_ context.Context, in dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
		got = in
		return &entity.BusinessProfile{ID: "singleton", Name: *in.Name}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_business_profile", `{
		"name":"N","industry":"salud","country":"AR","address":"Calle 1","latitude":-34.6,
		"longitude":-58.4,"cover_photo_url":"https://e/c.jpg","public_phone":"+5491100000000",
		"messenger_platform":"whatsapp","messenger_id":"5491100000000","contact_email":"a@b.com",
		"website_url":"https://e.com","general_description":"d","currency_code":"ARS",
		"currency_symbol":"$","accepted_payment_methods":"[\"cash\"]",
		"timezone":"America/Argentina/Buenos_Aires","slot_interval_minutes":30,
		"business_hours":"{\"1\":{\"open\":\"09:00\",\"close\":\"18:00\"}}"}`))
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %d %q", resp.Error.Code, resp.Error.Message)
	}

	if got.Caller != ownerCaller() {
		t.Errorf("input.Caller = %+v; want owner-1/owner", got.Caller)
	}

	want := dto.UpdateBusinessProfileInput{
		Name:                   strPtr("N"),
		Industry:               strPtr("salud"),
		Country:                strPtr("AR"),
		Address:                strPtr("Calle 1"),
		Latitude:               f64Ptr(-34.6),
		Longitude:              f64Ptr(-58.4),
		CoverPhotoURL:          strPtr("https://e/c.jpg"),
		PublicPhone:            strPtr("+5491100000000"),
		MessengerPlatform:      strPtr("whatsapp"),
		MessengerID:            strPtr("5491100000000"),
		ContactEmail:           strPtr("a@b.com"),
		WebsiteURL:             strPtr("https://e.com"),
		GeneralDescription:     strPtr("d"),
		CurrencyCode:           strPtr("ARS"),
		CurrencySymbol:         strPtr("$"),
		AcceptedPaymentMethods: strPtr(`["cash"]`),
		Timezone:               strPtr("America/Argentina/Buenos_Aires"),
		SlotIntervalMinutes:    intPtr(30),
		BusinessHours:          strPtr(`{"1":{"open":"09:00","close":"18:00"}}`),
	}
	if mustJSON(t, got) != mustJSON(t, want) {
		t.Errorf("input =\n%s\nwant\n%s", mustJSON(t, got), mustJSON(t, want))
	}
}

// ── create_service ──

func TestToolCreateService(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.createService.executeFn = func(ctx context.Context, in dto.CreateServiceInput) (*entity.Service, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.Name != "Corte" || in.DurationMinutes != 30 || in.Price != 1500.5 {
			t.Errorf("input = %+v; want Corte/30/1500.5", in)
		}
		if in.Description == nil || *in.Description != "Corte de pelo" {
			t.Errorf("in.Description = %v; want Corte de pelo", in.Description)
		}
		// Omitted is_active stays nil: the use case owns the default (active).
		if in.Active != nil {
			t.Errorf("in.Active = %v; want nil when the payload omits is_active", in.Active)
		}
		return &entity.Service{
			ID: "s-new", Name: in.Name, Description: in.Description,
			DurationMinutes: in.DurationMinutes, Price: in.Price, Active: true,
		}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_service",
		`{"name":"Corte","description":"Corte de pelo","duration_minutes":30,"price":1500.5}`))

	var out serviceOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ID != "s-new" || out.Name != "Corte" || out.DurationMinutes != 30 || out.Price != 1500.5 || !out.IsActive {
		t.Errorf("service = %+v; want the stored service with its generated id", out)
	}
}

func TestToolCreateServicePassesExplicitInactive(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.createService.executeFn = func(_ context.Context, in dto.CreateServiceInput) (*entity.Service, error) {
		if in.Active == nil || *in.Active {
			t.Errorf("in.Active = %v; want explicit false", in.Active)
		}
		return &entity.Service{ID: "s-new", Name: in.Name, DurationMinutes: 30, Price: 1, Active: false}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_service",
		`{"name":"Corte","duration_minutes":30,"price":1,"is_active":false}`))

	var out serviceOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.IsActive {
		t.Error("is_active = true; want false (the explicit payload value)")
	}
}

// ── update_service ──

func TestToolUpdateService(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.updateService.executeFn = func(ctx context.Context, in dto.UpdateServiceInput) (*entity.Service, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.ServiceID != "s1" {
			t.Errorf("input.ServiceID = %q; want s1", in.ServiceID)
		}
		if in.Name == nil || *in.Name != "Corte premium" {
			t.Errorf("input.Name = %v; want Corte premium", in.Name)
		}
		// Partial merge: untouched fields stay nil.
		if in.Description != nil || in.DurationMinutes != nil || in.Price != nil || in.Active != nil {
			t.Errorf("omitted fields must stay nil; got %+v", in)
		}
		return &entity.Service{ID: in.ServiceID, Name: *in.Name, DurationMinutes: 30, Price: 1000, Active: true}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_service",
		`{"service_id":"s1","name":"Corte premium"}`))

	var out serviceOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ID != "s1" || out.Name != "Corte premium" {
		t.Errorf("service = %+v; want the merged service", out)
	}
}

// ── delete_service ──

func TestToolDeleteService(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.deleteService.executeFn = func(ctx context.Context, in dto.DeleteServiceInput) (*dto.DeleteServiceResult, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.ServiceID != "s1" {
			t.Errorf("input.ServiceID = %q; want s1", in.ServiceID)
		}
		return &dto.DeleteServiceResult{ServiceID: in.ServiceID, Status: "deleted"}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "delete_service", `{"service_id":"s1"}`))

	var out dto.DeleteServiceResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ServiceID != "s1" || out.Status != "deleted" {
		t.Errorf("out = %+v; want s1/deleted", out)
	}
}

// ── create_professional ──

func TestToolCreateProfessional(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.createProfessional.executeFn = func(ctx context.Context, in dto.CreateProfessionalInput) (*entity.Professional, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.Name != "Ana" || in.Phone == nil || *in.Phone != "+5491100000001" {
			t.Errorf("input = %+v; want Ana/+5491100000001", in)
		}
		if len(in.Specialties) != 2 || in.Specialties[0] != "s1" || in.Specialties[1] != "s2" {
			t.Errorf("input.Specialties = %v; want [s1 s2] as service IDs", in.Specialties)
		}
		if in.Status != nil {
			t.Errorf("input.Status = %v; want nil when omitted (use case defaults to active)", in.Status)
		}
		stored := `["s1","s2"]`
		return &entity.Professional{ID: "p-new", Name: in.Name, Phone: in.Phone, Status: "active", Specialties: &stored}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_professional",
		`{"name":"Ana","phone":"+5491100000001","specialties":["s1","s2"]}`))

	var out professionalOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ID != "p-new" || out.Name != "Ana" || out.Status != "active" {
		t.Errorf("professional = %+v; want p-new/Ana/active", out)
	}
	if len(out.Specialties) != 2 || out.Specialties[0] != "s1" || out.Specialties[1] != "s2" {
		t.Errorf("out.Specialties = %v; want [s1 s2]", out.Specialties)
	}
}

// ── update_professional ──

func TestToolUpdateProfessional(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.updateProfessional.executeFn = func(ctx context.Context, in dto.UpdateProfessionalInput) (*entity.Professional, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.ProfessionalID != "p1" {
			t.Errorf("input.ProfessionalID = %q; want p1", in.ProfessionalID)
		}
		if in.Status == nil || *in.Status != "inactive" {
			t.Errorf("input.Status = %v; want inactive", in.Status)
		}
		// A provided list (even empty) replaces the stored specialties wholesale.
		if in.Specialties == nil || len(*in.Specialties) != 0 {
			t.Errorf("input.Specialties = %v; want a non-nil empty list", in.Specialties)
		}
		if in.Name != nil || in.RoleSpecialty != nil || in.Email != nil || in.Phone != nil {
			t.Errorf("omitted fields must stay nil; got %+v", in)
		}
		stored := `[]`
		return &entity.Professional{ID: in.ProfessionalID, Name: "Ana", Status: *in.Status, Specialties: &stored}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_professional",
		`{"professional_id":"p1","status":"inactive","specialties":[]}`))

	var out professionalOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ID != "p1" || out.Status != "inactive" {
		t.Errorf("professional = %+v; want p1/inactive", out)
	}
	if out.Specialties == nil || len(out.Specialties) != 0 {
		t.Errorf("out.Specialties = %v; want an empty list (never null)", out.Specialties)
	}
}

// ── upsert_schedule ──

func TestToolUpsertSchedule(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.upsertSchedule.executeFn = func(ctx context.Context, in dto.UpsertScheduleInput) (*entity.Schedule, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.ProfessionalID != "p1" || in.DayOfWeek != 0 {
			t.Errorf("input = %+v; want p1/day 0 (Sunday)", in)
		}
		if in.StartTime != "09:00" || in.EndTime != "18:00" {
			t.Errorf("input times = %q..%q; want 09:00..18:00", in.StartTime, in.EndTime)
		}
		return &entity.Schedule{ID: 7, ProfessionalID: in.ProfessionalID, DayOfWeek: in.DayOfWeek, StartTime: in.StartTime, EndTime: in.EndTime}, nil
	}

	// day_of_week 0 is a valid Sunday: only the raw weekday encoding is accepted.
	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "upsert_schedule",
		`{"professional_id":"p1","day_of_week":0,"start_time":"09:00","end_time":"18:00"}`))

	var out scheduleOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ID != 7 || out.ProfessionalID != "p1" || out.DayOfWeek != 0 || out.StartTime != "09:00" || out.EndTime != "18:00" {
		t.Errorf("schedule = %+v; want the stored row", out)
	}
}

// ── delete_schedule ──

func TestToolDeleteSchedule(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.deleteSchedule.executeFn = func(ctx context.Context, in dto.DeleteScheduleInput) (*dto.DeleteScheduleResult, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.ProfessionalID != "p1" || in.DayOfWeek != 6 {
			t.Errorf("input = %+v; want p1/day 6 (Saturday)", in)
		}
		return &dto.DeleteScheduleResult{ProfessionalID: in.ProfessionalID, DayOfWeek: in.DayOfWeek, Status: "deleted"}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "delete_schedule",
		`{"professional_id":"p1","day_of_week":6}`))

	var out dto.DeleteScheduleResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ProfessionalID != "p1" || out.DayOfWeek != 6 || out.Status != "deleted" {
		t.Errorf("out = %+v; want p1/6/deleted", out)
	}
}

// ── transport shape checks: day_of_week and HH:MM (the only ones the handlers
// enforce; every other rule stays in the use case/repository) ──

func TestMaintenanceScheduleShapeChecks(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		args    string
		wantMsg string
	}{
		{
			name:    "upsert rejects day_of_week 7",
			tool:    "upsert_schedule",
			args:    `{"professional_id":"p1","day_of_week":7,"start_time":"09:00","end_time":"18:00"}`,
			wantMsg: "el día debe estar entre 0 (domingo) y 6 (sábado)",
		},
		{
			name:    "upsert rejects negative day_of_week",
			tool:    "upsert_schedule",
			args:    `{"professional_id":"p1","day_of_week":-1,"start_time":"09:00","end_time":"18:00"}`,
			wantMsg: "el día debe estar entre 0 (domingo) y 6 (sábado)",
		},
		{
			name:    "upsert rejects unpadded start_time",
			tool:    "upsert_schedule",
			args:    `{"professional_id":"p1","day_of_week":1,"start_time":"9:00","end_time":"18:00"}`,
			wantMsg: "start_time debe tener formato HH:MM en 24h (ej. 09:30)",
		},
		{
			name:    "upsert rejects out-of-range start_time",
			tool:    "upsert_schedule",
			args:    `{"professional_id":"p1","day_of_week":1,"start_time":"25:00","end_time":"18:00"}`,
			wantMsg: "start_time debe tener formato HH:MM en 24h (ej. 09:30)",
		},
		{
			name:    "upsert rejects malformed end_time",
			tool:    "upsert_schedule",
			args:    `{"professional_id":"p1","day_of_week":1,"start_time":"09:00","end_time":"12:60"}`,
			wantMsg: "end_time debe tener formato HH:MM en 24h (ej. 18:00)",
		},
		{
			name:    "delete rejects day_of_week 7",
			tool:    "delete_schedule",
			args:    `{"professional_id":"p1","day_of_week":7}`,
			wantMsg: "el día debe estar entre 0 (domingo) y 6 (sábado)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			// Any port call would be a bug: the shape check must short-circuit.
			stubMaintenancePortsFail(ports, errors.New("port must not be called"))
			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), tt.tool, tt.args))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != tt.wantMsg {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, tt.wantMsg)
			}
		})
	}
}

// ── cross-cutting: caller, error mapping and fail-closed guards, per tool ──

func TestMaintenanceToolsMissingCallerUnauthenticated(t *testing.T) {
	for _, name := range maintenanceToolNames() {
		t.Run(name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubMaintenancePortsFail(ports, errors.New("port must not be called"))
			resp := decodeToolResponse(t, callTool(srv.Handler(), nil, name, maintenanceToolArgs[name]))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != "se requiere autenticación" {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, "se requiere autenticación")
			}
		})
	}
}

func TestMaintenanceToolsSemanticErrorPassthrough(t *testing.T) {
	for _, name := range maintenanceToolNames() {
		t.Run(name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubMaintenancePortsFail(ports, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "regla de negocio del use case",
			})
			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), name, maintenanceToolArgs[name]))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != "regla de negocio del use case" {
				t.Errorf("error.message = %q; want the use case message untouched", resp.Error.Message)
			}
		})
	}
}

func TestMaintenanceToolsInfraErrorMapsToInternal(t *testing.T) {
	for _, name := range maintenanceToolNames() {
		t.Run(name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubMaintenancePortsFail(ports, errors.New("sqlite: disk I/O error"))
			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), name, maintenanceToolArgs[name]))
			wantErrorCode(t, resp, -32603)
		})
	}
}

func TestMaintenanceToolsNilResultFailsClosed(t *testing.T) {
	for _, name := range maintenanceToolNames() {
		t.Run(name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubMaintenancePortsNilSuccess(ports)
			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), name, maintenanceToolArgs[name]))
			wantErrorCode(t, resp, -32603)
		})
	}
}

// ── output mapping details ──

// TestDecodeSpecialties covers the storage encoding boundary: the column holds
// a JSON array, absent/invalid values fail safe to an empty list.
func TestDecodeSpecialties(t *testing.T) {
	tests := []struct {
		name   string
		stored *string
		want   []string
	}{
		{name: "nil column", stored: nil, want: []string{}},
		{name: "empty column", stored: strPtr(""), want: []string{}},
		{name: "json null", stored: strPtr("null"), want: []string{}},
		{name: "empty array", stored: strPtr("[]"), want: []string{}},
		{name: "invalid json", stored: strPtr("not-json"), want: []string{}},
		{name: "populated", stored: strPtr(`["s1","s2"]`), want: []string{"s1", "s2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeSpecialties(tt.stored)
			if mustJSON(t, got) != mustJSON(t, tt.want) {
				t.Errorf("decodeSpecialties(%v) = %v; want %v", tt.stored, got, tt.want)
			}
		})
	}
}

func intPtr(i int) *int { return &i }
