package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
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
