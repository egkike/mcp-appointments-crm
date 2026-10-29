package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// mockSearchProfessionalsAdvancedPort is the fn-table mock for the
// search_professionals_advanced consumer port (same shape as the other port
// mocks in tools_test.go; declared here so the professionals surface keeps its
// fixtures together).
type mockSearchProfessionalsAdvancedPort struct {
	executeFn func(ctx context.Context, in dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error)
}

func (m *mockSearchProfessionalsAdvancedPort) Execute(ctx context.Context, in dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error) {
	return m.executeFn(ctx, in)
}

// mockGetProfessionalSchedulesPort is the fn-table mock for the owner-only
// agenda read port (S7b), same shape as its S7a sibling above.
type mockGetProfessionalSchedulesPort struct {
	executeFn func(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error)
}

func (m *mockGetProfessionalSchedulesPort) Execute(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
	return m.executeFn(ctx, in)
}

// ── search_professionals_advanced (S7a) ──

func TestToolSearchProfessionalsAdvanced(t *testing.T) {
	srv, ports := newToolServer(t)
	role := "Estilista"
	ports.searchProfessionals.executeFn = func(ctx context.Context, in dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error) {
		// A client caller must reach the handler: the tool has no ToolRBAC entry
		// and the booking flow runs as the client (design §8.2 D1).
		if in.Caller.ID != "client-1" || in.Caller.Role != auth.RoleClient {
			t.Errorf("input.Caller = %+v; want client-1/client", in.Caller)
		}
		if in.QueryText != "rom" || in.ServiceID != "s-lavado" {
			t.Errorf("input = %+v; want trimmed rom/s-lavado", in)
		}
		return &dto.SearchProfessionalsAdvancedResult{Results: []dto.ProfessionalSearchEntry{
			{ID: "p-romina", Name: "Romina", RoleSpecialty: &role, Specialties: []string{"s-lavado"}},
		}}, nil
	}

	caller := auth.Caller{ID: "client-1", Role: auth.RoleClient}
	resp := decodeToolResponse(t, callTool(srv.Handler(), &caller, "search_professionals_advanced",
		`{"query_text":"  rom  ","service_id":"  s-lavado  "}`))

	var out dto.SearchProfessionalsAdvancedResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].ID != "p-romina" || out.Results[0].Name != "Romina" {
		t.Fatalf("results = %+v; want one p-romina/Romina row", out.Results)
	}
	if out.Results[0].RoleSpecialty == nil || *out.Results[0].RoleSpecialty != "Estilista" {
		t.Errorf("RoleSpecialty = %v; want Estilista", out.Results[0].RoleSpecialty)
	}
	if len(out.Results[0].Specialties) != 1 || out.Results[0].Specialties[0] != "s-lavado" {
		t.Errorf("Specialties = %v; want [s-lavado]", out.Results[0].Specialties)
	}
}

func TestToolSearchProfessionalsAdvancedEmptyFilters(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.searchProfessionals.executeFn = func(ctx context.Context, in dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error) {
		if in.QueryText != "" || in.ServiceID != "" {
			t.Errorf("input = %+v; want no filters (full active catalog)", in)
		}
		return &dto.SearchProfessionalsAdvancedResult{}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_professionals_advanced", `{}`))

	var out dto.SearchProfessionalsAdvancedResult
	raw := wantStructured(t, resp)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Results) != 0 {
		t.Fatalf("results = %+v; want empty", out.Results)
	}
	if string(raw) != `{"results":[]}` {
		t.Errorf("structuredContent = %s; want {\"results\":[]}", raw)
	}
}

func TestToolSearchProfessionalsAdvancedNilResult(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.searchProfessionals.executeFn = func(ctx context.Context, in dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error) {
		return nil, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_professionals_advanced", `{"service_id":"s1"}`))
	raw := wantStructured(t, resp)
	if string(raw) != `{"results":[]}` {
		t.Errorf("structuredContent = %s; want {\"results\":[]} (nil result must never render null)", raw)
	}
}

// TestToolSearchProfessionalsAdvancedHasNoPII pins design §8.2 D1: the tool is
// readable by clients, so email/phone MUST NOT appear in the wire shape.
func TestToolSearchProfessionalsAdvancedHasNoPII(t *testing.T) {
	srv, ports := newToolServer(t)
	role := "Barbero"
	ports.searchProfessionals.executeFn = func(ctx context.Context, in dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error) {
		return &dto.SearchProfessionalsAdvancedResult{Results: []dto.ProfessionalSearchEntry{
			{ID: "p-santiago", Name: "Santiago", RoleSpecialty: &role, Specialties: []string{}},
		}}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_professionals_advanced", `{}`))
	raw := wantStructured(t, resp)

	var row map[string]any
	var envelope struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(envelope.Results) != 1 {
		t.Fatalf("results = %d; want 1", len(envelope.Results))
	}
	row = envelope.Results[0]
	for _, forbidden := range []string{"email", "phone"} {
		if _, ok := row[forbidden]; ok {
			t.Errorf("wire row carries %q; search_professionals_advanced must stay PII-free (D1): %s", forbidden, raw)
		}
	}
	for _, want := range []string{"id", "name", "specialties"} {
		if _, ok := row[want]; !ok {
			t.Errorf("wire row missing %q: %s", want, raw)
		}
	}
}

func TestToolSearchProfessionalsAdvancedSemanticError(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.searchProfessionals.executeFn = func(ctx context.Context, in dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error) {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeNotFound,
			Message: "el servicio indicado no existe",
			Cause:   domain.ErrNotFound,
		}
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_professionals_advanced", `{"service_id":"s-missing"}`))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != "el servicio indicado no existe" {
		t.Errorf("message = %q; want the semantic Spanish message", resp.Error.Message)
	}
}

// ── get_professional_schedules (S7b) ──

// professionalSchedulesWire mirrors the pinned output contract of
// get_professional_schedules so the assertions below fail on a wire-shape
// change, not on a compile error. Agenda rows carry the FOUR field agenda row
// (issue #153): per-row professional_id was REMOVED because the response
// already scopes every row to professional.id at the top level.
type professionalSchedulesWire struct {
	Professional struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"professional"`
	Schedules []struct {
		ID        int    `json:"id"`
		DayOfWeek int    `json:"day_of_week"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	} `json:"schedules"`
}

func TestToolGetProfessionalSchedules(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.getProfessionalSchedules.executeFn = func(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
		// The caller travels from the resolved request context into the use case:
		// the transport never trusts the arguments for identity.
		if in.Caller.ID != "owner-1" || in.Caller.Role != auth.RoleOwner {
			t.Errorf("input.Caller = %+v; want owner-1/owner", in.Caller)
		}
		if in.ProfessionalID != "p-romina" {
			t.Errorf("input.ProfessionalID = %q; want the trimmed p-romina", in.ProfessionalID)
		}
		return &dto.GetProfessionalSchedulesResult{
			Professional: dto.ProfessionalScheduleSubject{ID: "p-romina", Name: "Romina", Status: "active"},
			Schedules: []*entity.Schedule{
				{ID: 1, ProfessionalID: "p-romina", DayOfWeek: 1, StartTime: "09:00", EndTime: "13:00"},
				{ID: 2, ProfessionalID: "p-romina", DayOfWeek: 3, StartTime: "14:00", EndTime: "18:00"},
			},
		}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_professional_schedules", `{"professional_id":"  p-romina  "}`))
	raw := wantStructured(t, resp)

	var out professionalSchedulesWire
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Professional.ID != "p-romina" || out.Professional.Name != "Romina" || out.Professional.Status != "active" {
		t.Errorf("professional = %+v; want p-romina/Romina/active", out.Professional)
	}
	if len(out.Schedules) != 2 {
		t.Fatalf("schedules = %+v; want the two rows", out.Schedules)
	}
	first := out.Schedules[0]
	if first.ID != 1 || first.DayOfWeek != 1 || first.StartTime != "09:00" || first.EndTime != "13:00" {
		t.Errorf("row 0 = %+v; want id=1 day=1 09:00-13:00", first)
	}
	// The redundant per-row professional_id banned from the agenda read (issue
	// #153): the response is already scoped by professional.id, so the key must
	// not appear anywhere in the serialized payload.
	if bytes.Contains(raw, []byte("professional_id")) {
		t.Errorf("wire carries redundant per-row professional_id; the response is scoped by professional.id: %s", raw)
	}
	// No client or booking data may cross this wire: only the agenda of the
	// requested professional (spec S7b).
	for _, forbidden := range []string{"email", "phone", "client", "booking"} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Errorf("wire carries %q; get_professional_schedules must expose no client/booking/staff contact data: %s", forbidden, raw)
		}
	}
}

func TestToolGetProfessionalSchedulesEmptyAgenda(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.getProfessionalSchedules.executeFn = func(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
		return &dto.GetProfessionalSchedulesResult{
			Professional: dto.ProfessionalScheduleSubject{ID: "p-inactive", Name: "Joaquin", Status: "inactive"},
			Schedules:    nil,
		}, nil
	}

	raw := wantStructured(t, decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_professional_schedules", `{"professional_id":"p-inactive"}`)))

	var out professionalSchedulesWire
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	// Fail-closed wire contract: an empty agenda renders [] and the inactive
	// professional still comes back with its status (the delete gate needs to
	// know against whom it is confirming).
	if !bytes.Contains(raw, []byte(`"schedules":[]`)) {
		t.Errorf("empty agenda must render \"schedules\":[] (not null): %s", raw)
	}
	if out.Professional.Status != "inactive" {
		t.Errorf("status = %q; want inactive", out.Professional.Status)
	}
}

func TestToolGetProfessionalSchedulesNilResultFailsClosed(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.getProfessionalSchedules.executeFn = func(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
		return nil, nil
	}

	// A port that reports success with no result must not degrade into a
	// zero-value identity: the gate would read "unknown professional, empty
	// week" as a real answer. Fail closed to -32603 like every other tool.
	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_professional_schedules", `{"professional_id":"p-romina"}`))
	wantErrorCode(t, resp, -32603)
	if strings.Contains(resp.Error.Message, "maintenance tool") {
		t.Errorf("internal guard text leaked to the client: %q", resp.Error.Message)
	}
}

func TestToolGetProfessionalSchedulesForbiddenIsSemantic(t *testing.T) {
	srv, ports := newToolServer(t)
	// Second enforcement layer: the composition-root RBAC gate lives in
	// cmd/mcp-server, so the transport must still map the use case's forbidden
	// error to -32002 with the semantic Spanish message (never -32603).
	ports.getProfessionalSchedules.executeFn = func(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeForbidden,
			Message: "no tienes permiso para realizar esta acción",
			Cause:   domain.ErrForbidden,
		}
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_professional_schedules", `{"professional_id":"p-romina"}`))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != "no tienes permiso para realizar esta acción" {
		t.Errorf("message = %q; want the semantic Spanish message", resp.Error.Message)
	}
}

func TestToolGetProfessionalSchedulesInternalErrorLeaksNothing(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.getProfessionalSchedules.executeFn = func(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
		return nil, errors.New("sql: database is locked at /home/owner/appointments.db")
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_professional_schedules", `{"professional_id":"p-romina"}`))
	wantErrorCode(t, resp, -32603)
	if strings.Contains(resp.Error.Message, "appointments.db") || strings.Contains(resp.Error.Message, "locked") {
		t.Errorf("internal detail leaked to the client: %q", resp.Error.Message)
	}
}

func TestToolGetProfessionalSchedulesNilRowFailsClosed(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.getProfessionalSchedules.executeFn = func(ctx context.Context, in dto.GetProfessionalSchedulesInput) (*dto.GetProfessionalSchedulesResult, error) {
		return &dto.GetProfessionalSchedulesResult{
			Professional: dto.ProfessionalScheduleSubject{ID: "p-romina", Name: "Romina", Status: "active"},
			Schedules: []*entity.Schedule{
				{ID: 1, ProfessionalID: "p-romina", DayOfWeek: 1, StartTime: "09:00", EndTime: "13:00"},
				nil,
			},
		}, nil
	}

	// An incomplete agenda is worse than an error: the delete gate would verify
	// a week that is not the stored one. Skip-nothing, fail closed.
	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_professional_schedules", `{"professional_id":"p-romina"}`))
	wantErrorCode(t, resp, -32603)
}
