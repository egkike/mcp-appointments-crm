package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

func TestToolSearchClientsAdvanced(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.searchClients.executeFn = func(ctx context.Context, in dto.SearchClientsAdvancedInput) (*dto.SearchClientsAdvancedResult, error) {
		if in.Caller.ID != "owner-1" || in.QueryText != "juan" {
			t.Errorf("input = %+v", in)
		}
		return &dto.SearchClientsAdvancedResult{
			Results: []dto.ClientSearchEntry{{ID: "c1", Name: "Juan", Phone: "+5491112345678"}},
		}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_clients_advanced",
		`{"query_text":"juan"}`))
	var out dto.SearchClientsAdvancedResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].ID != "c1" {
		t.Errorf("got %+v", out)
	}
}

func TestToolSearchClientsAdvancedRejectsEmptyQuery(t *testing.T) {
	srv, _ := newToolServer(t)
	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_clients_advanced", `{"query_text":""}`))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != msgQueryTextRequired {
		t.Errorf("error.message = %q, want %q", resp.Error.Message, msgQueryTextRequired)
	}
}

func TestToolSearchClientsAdvancedCallerPropagated(t *testing.T) {
	srv, ports := newToolServer(t)
	var captured auth.Caller
	ports.searchClients.executeFn = func(ctx context.Context, in dto.SearchClientsAdvancedInput) (*dto.SearchClientsAdvancedResult, error) {
		captured = in.Caller
		return &dto.SearchClientsAdvancedResult{}, nil
	}
	caller := auth.Caller{ID: "client-1", Role: auth.RoleClient, ClientID: strPtr("c1")}
	decodeToolResponse(t, callTool(srv.Handler(), &caller, "search_clients_advanced", `{"query_text":"x"}`))
	if captured.ID != "client-1" || captured.Role != auth.RoleClient {
		t.Errorf("captured caller = %+v", captured)
	}
}

func TestToolSearchServicesAdvanced(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.searchServices.executeFn = func(ctx context.Context, in dto.SearchServicesAdvancedInput) (*dto.SearchServicesAdvancedResult, error) {
		if in.QueryText != "corte" {
			t.Errorf("query = %q, want corte", in.QueryText)
		}
		return &dto.SearchServicesAdvancedResult{
			Results: []dto.ServiceSearchEntry{{ID: "s1", Name: "Corte", DurationMinutes: 30, Price: 500.0, IsActive: true}},
		}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_services_advanced",
		`{"query_text":"corte"}`))
	var out dto.SearchServicesAdvancedResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].ID != "s1" {
		t.Errorf("got %+v", out)
	}
}

func TestToolSearchServicesAdvancedRejectsEmptyQuery(t *testing.T) {
	srv, _ := newToolServer(t)
	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_services_advanced", `{"query_text":""}`))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != msgQueryTextRequired {
		t.Errorf("error.message = %q, want %q", resp.Error.Message, msgQueryTextRequired)
	}
}

func TestToolSearchServicesAdvancedRoleRejection(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.searchServices.executeFn = func(ctx context.Context, in dto.SearchServicesAdvancedInput) (*dto.SearchServicesAdvancedResult, error) {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción"}
	}
	caller := auth.Caller{ID: "staff-1", Role: auth.RoleStaff, ProfessionalID: strPtr("p1")}
	resp := decodeToolResponse(t, callTool(srv.Handler(), &caller, "search_services_advanced",
		`{"query_text":"corte"}`))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != "no tienes permiso para realizar esta acción" {
		t.Errorf("error.message = %q, want %q", resp.Error.Message, "no tienes permiso para realizar esta acción")
	}
}

func TestToolSearchServicesAdvancedUnauthenticated(t *testing.T) {
	srv, _ := newToolServer(t)
	resp := decodeToolResponse(t, callTool(srv.Handler(), nil, "search_services_advanced",
		`{"query_text":"corte"}`))
	wantErrorCode(t, resp, -32002)
}

func TestSearchToolsUseCaseErrorPropagates(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.searchClients.executeFn = func(ctx context.Context, in dto.SearchClientsAdvancedInput) (*dto.SearchClientsAdvancedResult, error) {
		return nil, errors.New("db down")
	}
	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "search_clients_advanced",
		`{"query_text":"juan"}`))
	wantErrorCode(t, resp, -32603)
}

func TestMaxFTSQueryLenMatchesRepository(t *testing.T) {
	if maxFTSQueryLen != repository.MaxFTSQueryLen {
		t.Errorf("mcp.maxFTSQueryLen (%d) != repository.MaxFTSQueryLen (%d); keep transport and repository limits in sync", maxFTSQueryLen, repository.MaxFTSQueryLen)
	}
	if maxFTSQueryLen != 200 {
		t.Errorf("mcp.maxFTSQueryLen = %d; want 200", maxFTSQueryLen)
	}
	if repository.MaxFTSQueryLen != 200 {
		t.Errorf("repository.MaxFTSQueryLen = %d; want 200", repository.MaxFTSQueryLen)
	}
}

// TestIntegrationSearchBadFTSQueryIsSemantic proves the F2 contract end to end
// through the real mux: a query that passes the transport's own checks but is
// rejected by the repository FTS validation reaches the LLM as JSON-RPC -32002
// with the Spanish invalid-input message, not the generic -32603. Before the
// fix the repository returned plain fmt.Errorf wraps, so the client saw
// -32603 "error interno del servidor".
func TestIntegrationSearchBadFTSQueryIsSemantic(t *testing.T) {
	mux := newIntegrationMux(t)

	const wantMsg = "la consulta de búsqueda contiene caracteres no permitidos."
	tests := []struct {
		name string
		tool string
	}{
		{"clients", "search_clients_advanced"},
		{"services", "search_services_advanced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":{"query_text":"juan*"}}}`, tt.tool)
			_, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, "owner-1", body))
			if code != -32002 {
				t.Errorf("code = %d; want -32002 (msg=%q)", code, msg)
			}
			if msg != wantMsg {
				t.Errorf("message = %q; want %q", msg, wantMsg)
			}
		})
	}
}
