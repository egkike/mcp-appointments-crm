package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// ── find_available_slots (agent-surface T2, issue #241) ──
//
// Transport-level tests: the use case is a mock port, so these assert the WIRE
// contract only (input mapping, output mapping, error mapping, role admission).
// The availability maths belongs to the use-case suite.

func slotsResultFixture() *dto.FindAvailableSlotsResult {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		panic(err)
	}
	return &dto.FindAvailableSlotsResult{
		Slots: []dto.AvailableSlot{
			{
				StartDatetime:    time.Date(2031, time.March, 3, 9, 0, 0, 0, loc),
				ProfessionalID:   "p1",
				ProfessionalName: "Ana",
			},
			{
				StartDatetime:    time.Date(2031, time.March, 3, 10, 0, 0, 0, loc),
				ProfessionalID:   "p1",
				ProfessionalName: "Ana",
			},
		},
		TotalCandidates: 2,
		Truncated:       false,
	}
}

// slotsServer builds a server whose slots port records the input it received and
// returns the fixture (or the configured error).
func slotsServer(t *testing.T, captured *dto.FindAvailableSlotsInput, err error) *Server {
	t.Helper()
	srv, ports := newToolServer(t)
	ports.findAvailableSlots.executeFn = func(_ context.Context, in dto.FindAvailableSlotsInput) (*dto.FindAvailableSlotsResult, error) {
		if captured != nil {
			*captured = in
		}
		if err != nil {
			return nil, err
		}
		return slotsResultFixture(), nil
	}
	return srv
}

func TestFindAvailableSlotsToolRegistered(t *testing.T) {
	srv, _ := newToolServer(t)
	rec := callMethod(srv.Handler(), "tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					Type       string                     `json:"type"`
					Properties map[string]json.RawMessage `json:"properties"`
					Required   []string                   `json:"required"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v; body=%s", err, rec.Body.String())
	}

	var found bool
	for _, tool := range resp.Result.Tools {
		if tool.Name != "find_available_slots" {
			continue
		}
		found = true
		if tool.Description == "" {
			t.Error("description is empty; the LLM needs the tool's purpose and the 14-day bound")
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("inputSchema.type = %q; want object", tool.InputSchema.Type)
		}
		for _, property := range []string{"service_id", "professional_id", "from", "to", "limit"} {
			if _, ok := tool.InputSchema.Properties[property]; !ok {
				t.Errorf("inputSchema is missing property %q", property)
			}
		}
		required := map[string]bool{}
		for _, name := range tool.InputSchema.Required {
			required[name] = true
		}
		for _, name := range []string{"service_id", "from", "to"} {
			if !required[name] {
				t.Errorf("%q must be required; required = %v", name, tool.InputSchema.Required)
			}
		}
		for _, name := range []string{"professional_id", "limit"} {
			if required[name] {
				t.Errorf("%q must be optional; required = %v", name, tool.InputSchema.Required)
			}
		}
	}
	if !found {
		t.Fatalf("tools/list does not expose find_available_slots; got %s", mustJSON(t, resp.Result.Tools))
	}
}

func TestFindAvailableSlotsToolOutput(t *testing.T) {
	srv := slotsServer(t, nil, nil)

	rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
		`{"service_id":"s1","professional_id":"p1","from":"2031-03-03","to":"2031-03-03"}`)
	resp := decodeToolResponse(t, rec)
	structured := wantStructured(t, resp)

	var out dto.FindAvailableSlotsResult
	if err := json.Unmarshal(structured, &out); err != nil {
		t.Fatalf("unmarshal structuredContent: %v; raw=%s", err, structured)
	}
	if len(out.Slots) != 2 || out.TotalCandidates != 2 || out.Truncated {
		t.Fatalf("out = %s; want 2 slots, 2 candidates, truncated false", structured)
	}
	if out.Slots[0].ProfessionalID != "p1" || out.Slots[0].ProfessionalName != "Ana" {
		t.Errorf("slot[0] = %+v; want the professional identity", out.Slots[0])
	}

	// The wire keys are the contract (REQ-MT-015): start_datetime,
	// professional_id, professional_name, total_candidates, truncated.
	var raw struct {
		Slots []map[string]json.RawMessage `json:"slots"`
		Total *int                         `json:"total_candidates"`
		Trunc *bool                        `json:"truncated"`
	}
	if err := json.Unmarshal(structured, &raw); err != nil {
		t.Fatalf("unmarshal raw shape: %v", err)
	}
	if raw.Total == nil || raw.Trunc == nil {
		t.Errorf("missing total_candidates/truncated in %s", structured)
	}
	for _, key := range []string{"start_datetime", "professional_id", "professional_name"} {
		if _, ok := raw.Slots[0][key]; !ok {
			t.Errorf("slot is missing wire key %q; got %s", key, structured)
		}
	}
	// S5: no client identity may travel, even though the slot was filtered by a
	// client booking inside the use case.
	for _, forbidden := range []string{"client_id", "client_name", "phone", "booking_id"} {
		if _, ok := raw.Slots[0][forbidden]; ok {
			t.Errorf("slot exposes %q; the response must carry no client identity", forbidden)
		}
	}
}

func TestFindAvailableSlotsToolInputMapping(t *testing.T) {
	t.Run("optional fields absent", func(t *testing.T) {
		var captured dto.FindAvailableSlotsInput
		srv := slotsServer(t, &captured, nil)

		rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
			`{"service_id":"s1","from":"2031-03-03","to":"2031-03-04"}`)
		decodeToolResponse(t, rec)

		if captured.ServiceID != "s1" || captured.From != "2031-03-03" || captured.To != "2031-03-04" {
			t.Errorf("captured = %+v; want the service and both date bounds", captured)
		}
		if captured.ProfessionalID != nil {
			t.Errorf("ProfessionalID = %v; want nil (absent means every candidate)", *captured.ProfessionalID)
		}
		if captured.Limit != nil {
			t.Errorf("Limit = %v; want nil (absent means the default)", *captured.Limit)
		}
		if captured.Caller.ID != ownerCaller().ID {
			t.Errorf("Caller.ID = %q; want %q (the resolved caller must reach the use case)", captured.Caller.ID, ownerCaller().ID)
		}
	})

	t.Run("optional fields present", func(t *testing.T) {
		var captured dto.FindAvailableSlotsInput
		srv := slotsServer(t, &captured, nil)

		rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
			`{"service_id":"s1","professional_id":"p2","from":"2031-03-03","to":"2031-03-04","limit":5}`)
		decodeToolResponse(t, rec)

		if captured.ProfessionalID == nil || *captured.ProfessionalID != "p2" {
			t.Errorf("ProfessionalID = %v; want p2", captured.ProfessionalID)
		}
		if captured.Limit == nil || *captured.Limit != 5 {
			t.Errorf("Limit = %v; want 5", captured.Limit)
		}
	})
}

// TestFindAvailableSlotsToolAdmitsAnyAuthenticatedRole pins S2: the tool is "any
// authenticated", so every resolved role reaches the use case.
func TestFindAvailableSlotsToolAdmitsAnyAuthenticatedRole(t *testing.T) {
	clientID := "c1"
	professionalID := "p1"
	roles := map[string]auth.Caller{
		"owner":  {ID: "owner-1", Role: auth.RoleOwner},
		"admin":  {ID: "admin-1", Role: auth.RoleAdmin},
		"staff":  {ID: "staff-1", Role: auth.RoleStaff, ProfessionalID: &professionalID},
		"client": {ID: "client-1", Role: auth.RoleClient, ClientID: &clientID},
	}

	for name, caller := range roles {
		t.Run(name, func(t *testing.T) {
			srv := slotsServer(t, nil, nil)
			callerCopy := caller
			rec := callTool(srv.Handler(), &callerCopy, "find_available_slots",
				`{"service_id":"s1","from":"2031-03-03","to":"2031-03-03"}`)
			resp := decodeToolResponse(t, rec)
			if resp.Error != nil {
				t.Fatalf("role %q rejected at the transport: %d %q", name, resp.Error.Code, resp.Error.Message)
			}
			wantStructured(t, resp)
		})
	}
}

func TestFindAvailableSlotsToolErrors(t *testing.T) {
	t.Run("semantic error surfaces its Spanish message", func(t *testing.T) {
		srv := slotsServer(t, nil, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el rango de fechas no puede superar los 14 días (se pidieron 15).",
		})
		rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
			`{"service_id":"s1","from":"2031-03-03","to":"2031-03-17"}`)
		resp := decodeToolResponse(t, rec)
		wantErrorCode(t, resp, -32002)
		if resp.Error.Message != "el rango de fechas no puede superar los 14 días (se pidieron 15)." {
			t.Errorf("message = %q; want the semantic message verbatim", resp.Error.Message)
		}
	})

	t.Run("semantic error wrapped by the use case still surfaces", func(t *testing.T) {
		wrapped := errors.Join(errors.New("find_available_slots: consultar reservas"), &domain.SemanticError{
			Code:    domain.ErrCodeNotFound,
			Message: "el servicio indicado no existe",
		})
		srv := slotsServer(t, nil, wrapped)
		rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
			`{"service_id":"missing","from":"2031-03-03","to":"2031-03-03"}`)
		resp := decodeToolResponse(t, rec)
		wantErrorCode(t, resp, -32002)
		if resp.Error.Message != "el servicio indicado no existe" {
			t.Errorf("message = %q; want the wrapped semantic message", resp.Error.Message)
		}
	})

	t.Run("infrastructure error collapses to the generic internal error", func(t *testing.T) {
		srv := slotsServer(t, nil, errors.New("disk I/O error: /var/lib/secret.db"))
		rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
			`{"service_id":"s1","from":"2031-03-03","to":"2031-03-03"}`)
		resp := decodeToolResponse(t, rec)
		wantErrorCode(t, resp, -32603)
		if resp.Error.Message != msgInternal {
			t.Errorf("message = %q; want the generic %q (no internals may leak)", resp.Error.Message, msgInternal)
		}
	})

	t.Run("missing caller fails closed", func(t *testing.T) {
		// Unit level: AuthMiddleware resolves the caller before the handler in
		// production, so this path is the fail-closed guard. auth.RequireCaller
		// answers with its own semantic error (same shape as every other tool).
		srv := slotsServer(t, nil, nil)
		rec := callTool(srv.Handler(), nil, "find_available_slots",
			`{"service_id":"s1","from":"2031-03-03","to":"2031-03-03"}`)
		resp := decodeToolResponse(t, rec)
		wantErrorCode(t, resp, -32002)
		if resp.Error.Message != "se requiere autenticación" {
			t.Errorf("error.message = %q; want %q", resp.Error.Message, "se requiere autenticación")
		}
	})

	t.Run("missing required field is rejected by the SDK schema", func(t *testing.T) {
		srv := slotsServer(t, nil, nil)
		rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
			`{"service_id":"s1","from":"2031-03-03"}`)
		resp := decodeToolResponse(t, rec)
		wantToolError(t, resp, "missing properties")
	})
}

// TestFindAvailableSlotsToolNilPortUnregistered keeps the nil-port-skips-tool
// contract: a Config without the port registers nothing for this tool.
func TestFindAvailableSlotsToolNilPortUnregistered(t *testing.T) {
	srv := NewServer(Config{Version: testServerVersion})
	if got := srv.ToolCount(); got != 0 {
		t.Errorf("ToolCount() = %d; want 0", got)
	}
	rec := callTool(srv.Handler(), ownerCallerPtr(), "find_available_slots",
		`{"service_id":"s1","from":"2031-03-03","to":"2031-03-03"}`)
	resp := decodeToolResponse(t, rec)
	wantErrorCode(t, resp, -32601)
}
