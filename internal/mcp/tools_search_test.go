package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
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
