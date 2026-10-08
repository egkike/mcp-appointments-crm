package mcp

// Transport-level tests for the anonymous self-registration tool
// `register_client` (feat-whatsapp-bot TASK-3.3, design.md §1.2, REQ-MT-015
// registry row).
//
// register_client is the ONE tool reached without a resolved caller: the
// identity is the X-Caller-Id header (the body has no phone field and no other
// key may set one) and the request context is marked anonymous by the
// middleware seam (auth.MarkAnonymous, design.md §1.1). These tests drive the
// real SDK handler chain with that header, exactly as AuthMiddleware delivers
// it to the transport.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/usecase"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// Spanish registration messages the use case owns and this suite pins through
// the transport (design.md §1.2 error table). They are literals here on purpose:
// the transport must forward them verbatim, no matter where the use case keeps
// its copy.
const (
	msgRegisterInvalidPhone = "el teléfono no tiene un formato válido (se esperan entre 4 y 15 dígitos, con + opcional)."
	msgRegisterAccountPhone = "este número ya pertenece a una cuenta del negocio. Contactá al administrador."
	msgRegisterRateLimited  = "alcanzaste el límite de registros automáticos. Probá de nuevo más tarde o pedile al negocio que te registre."
	msgRegisterLegacyRow    = "este número ya está registrado con un identificador incompatible; contactá al negocio."
)

// mockRegisterClientPort is the fn-table mock of RegisterClientPort, the same
// shape the other tool mocks use (tools_test.go).
type mockRegisterClientPort struct {
	executeFn func(ctx context.Context, callerID, displayName string) (*usecase.GetOrCreateClientResult, error)
}

func (m *mockRegisterClientPort) Execute(ctx context.Context, callerID, displayName string) (*usecase.GetOrCreateClientResult, error) {
	return m.executeFn(ctx, callerID, displayName)
}

// newRegistrationToolServer wires only the registration port: the tool under
// test is the anonymous one, and the rest of the surface is pinned by the
// other suites.
func newRegistrationToolServer(t *testing.T, port *mockRegisterClientPort) *Server {
	t.Helper()
	return NewServer(Config{Version: "test", Logger: discardLogger(), RegisterClient: port})
}

// callRegisterClient performs a tools/call for register_client through the
// unauthenticated handler chain, carrying the anonymous marker the middleware
// seam sets and the X-Caller-Id header the tool reads its identity from (the
// unit-level twin of auth.WithCaller for the authenticated tools). An empty
// phone omits the header: the middleware answers that with the universal 401
// before the seam, so it is the defense-in-depth path only.
func callRegisterClient(t *testing.T, h http.Handler, phone, args string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"register_client","arguments":%s}}`, args)
	req := httptest.NewRequestWithContext(auth.MarkAnonymous(context.Background()), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if phone != "" {
		req.Header.Set("X-Caller-Id", phone)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// registrationStructured decodes the tool output and pins the exact wire shape:
// exactly the three keys {client_id, display_name, created} (REQ-MT-015).
func registrationStructured(t *testing.T, resp *toolResponse) map[string]json.RawMessage {
	t.Helper()
	var got map[string]json.RawMessage
	if err := json.Unmarshal(wantStructured(t, resp), &got); err != nil {
		t.Fatalf("unmarshal register_client output: %v", err)
	}
	want := []string{"client_id", "display_name", "created"}
	if len(got) != len(want) {
		t.Fatalf("output keys = %d (%v); want exactly %d (%v)", len(got), got, len(want), want)
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("output is missing key %q; got %v", key, got)
		}
	}
	return got
}

// toolErrorTextOf concatenates the content text of a tool isError envelope.
func toolErrorTextOf(t *testing.T, resp *toolResponse) string {
	t.Helper()
	if resp.Result == nil {
		t.Fatalf("no result envelope: %s", mustJSON(t, resp))
	}
	var sb strings.Builder
	for _, c := range resp.Result.Content {
		sb.WriteString(c.Text)
	}
	return sb.String()
}

// mustNotRun is the port body for the cases where reaching the use case is the
// bug under test: it fails the test instead of returning a value.
func mustNotRun(t *testing.T) func(context.Context, string, string) (*usecase.GetOrCreateClientResult, error) {
	t.Helper()
	return func(context.Context, string, string) (*usecase.GetOrCreateClientResult, error) {
		t.Error("the registration use case must not run for this request")
		return nil, errors.New("unreachable")
	}
}

// TestToolRegisterClientUsesHeaderPhone pins the identity contract: the phone
// is the X-Caller-Id header (trimmed exactly like AuthMiddleware and the
// resolver read it), display_name travels through verbatim — the use case owns
// the trim/placeholder policy — and the output echoes the stored result.
func TestToolRegisterClientUsesHeaderPhone(t *testing.T) {
	const phone = "+5491100999999"
	port := &mockRegisterClientPort{}
	port.executeFn = func(_ context.Context, callerID, displayName string) (*usecase.GetOrCreateClientResult, error) {
		if callerID != phone {
			t.Errorf("callerID = %q; want the trimmed X-Caller-Id header %q", callerID, phone)
		}
		if displayName != "  Ana Gómez  " {
			t.Errorf("displayName = %q; want the verbatim payload value", displayName)
		}
		return &usecase.GetOrCreateClientResult{ClientID: callerID, DisplayName: "Ana Gómez", Created: true}, nil
	}
	srv := newRegistrationToolServer(t, port)

	resp := decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "  "+phone+"  ", `{"display_name":"  Ana Gómez  "}`))
	got := registrationStructured(t, resp)
	if string(got["client_id"]) != `"`+phone+`"` {
		t.Errorf("client_id = %s; want %q", got["client_id"], phone)
	}
	if string(got["display_name"]) != `"Ana Gómez"` {
		t.Errorf("display_name = %s; want the stored name %q", got["display_name"], "Ana Gómez")
	}
	if string(got["created"]) != "true" {
		t.Errorf("created = %s; want true", got["created"])
	}

	// Without a header no caller is invented: the use case receives "" (the
	// middleware answers 401 before the seam, so this is defense in depth; the
	// empty phone is then rejected by the use case's own validation).
	var seen string
	port.executeFn = func(_ context.Context, callerID, _ string) (*usecase.GetOrCreateClientResult, error) {
		seen = callerID
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: msgRegisterInvalidPhone}
	}
	resp = decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "", `{}`))
	if seen != "" {
		t.Errorf("callerID = %q; want an empty string when no header is present", seen)
	}
	wantErrorCode(t, resp, -32002)
}

// TestToolRegisterClientDisplayNameAbsent pins the optional-name contract: an
// absent key and an empty string both reach the use case as "" so it stores the
// `Cliente {phone}` placeholder, and the response echoes that stored name back
// (client-registration "Display name is optional with a placeholder").
func TestToolRegisterClientDisplayNameAbsent(t *testing.T) {
	for _, args := range []string{`{}`, `{"display_name":""}`} {
		t.Run(args, func(t *testing.T) {
			var (
				got   string
				calls int
			)
			port := &mockRegisterClientPort{executeFn: func(_ context.Context, callerID, displayName string) (*usecase.GetOrCreateClientResult, error) {
				calls++
				got = displayName
				return &usecase.GetOrCreateClientResult{ClientID: callerID, DisplayName: "Cliente " + callerID, Created: true}, nil
			}}
			srv := newRegistrationToolServer(t, port)

			resp := decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "+5491100999999", args))
			if calls != 1 {
				t.Fatalf("use case calls = %d; want 1", calls)
			}
			if got != "" {
				t.Errorf("displayName = %q; want empty (the placeholder path)", got)
			}
			out := registrationStructured(t, resp)
			if string(out["display_name"]) != `"Cliente +5491100999999"` {
				t.Errorf("display_name = %s; want the stored placeholder", out["display_name"])
			}
		})
	}
}

// TestToolRegisterClientReportsExistingRow pins the idempotent path: created is
// false for an already-registered phone and the response echoes the STORED
// name, never the name the request tried to change.
func TestToolRegisterClientReportsExistingRow(t *testing.T) {
	port := &mockRegisterClientPort{executeFn: func(_ context.Context, callerID, _ string) (*usecase.GetOrCreateClientResult, error) {
		return &usecase.GetOrCreateClientResult{ClientID: callerID, DisplayName: "Cliente Existente", Created: false}, nil
	}}
	srv := newRegistrationToolServer(t, port)

	out := registrationStructured(t, decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "+5491100999999", `{"display_name":"Otro"}`)))
	if string(out["created"]) != "false" {
		t.Errorf("created = %s; want false for an existing row", out["created"])
	}
	if string(out["display_name"]) != `"Cliente Existente"` {
		t.Errorf("display_name = %s; want the stored name", out["display_name"])
	}
}

// TestToolRegisterClientRejectsUnknownBodyKeys pins strict decoding: a key the
// schema does not declare — including every phone-like spelling — is rejected
// before the handler runs, so the body can never set the caller identity
// (client-registration "The phone comes from the header, never from the request
// body"). The SDK infers additionalProperties:false from the typed input DTO and
// names the offending property in the tool error: the same mechanism
// TestToolGetBusinessProfileRejectsArguments and
// TestToolUpdateBusinessProfilePresenceMarkersStayOutOfSchema pin.
func TestToolRegisterClientRejectsUnknownBodyKeys(t *testing.T) {
	port := &mockRegisterClientPort{executeFn: mustNotRun(t)}
	srv := newRegistrationToolServer(t, port)

	cases := []struct {
		name         string
		args         string
		wantProperty string
	}{
		{"phone cannot be set by the body", `{"display_name":"Ana","phone":"+5491100000000"}`, `"phone"`},
		{"caller_id cannot be set by the body", `{"caller_id":"+5491100000000"}`, `"caller_id"`},
		{"id cannot be set by the body", `{"id":"+5491100000000"}`, `"id"`},
		{"unknown key", `{"foo":1}`, `"foo"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "+5491100999999", tc.args))
			wantToolError(t, resp, "unexpected additional properties")
			if text := toolErrorTextOf(t, resp); !strings.Contains(text, tc.wantProperty) {
				t.Errorf("tool error %q does not name the rejected property %s", text, tc.wantProperty)
			}
		})
	}
}

// TestToolRegisterClientInputSchemaExposesOnlyDisplayName pins the wire schema
// tools/list publishes: display_name is the only property and it is optional, so
// no phone field exists for a client to send (REQ-MT-015 registry row).
func TestToolRegisterClientInputSchemaExposesOnlyDisplayName(t *testing.T) {
	srv := newRegistrationToolServer(t, &mockRegisterClientPort{})

	rec := callMethod(srv.Handler(), "tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
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
	if len(resp.Result.Tools) != 1 {
		t.Fatalf("tools = %d; want only register_client: %s", len(resp.Result.Tools), rec.Body.String())
	}
	tool := resp.Result.Tools[0]
	if tool.Name != "register_client" {
		t.Fatalf("tool name = %q; want register_client", tool.Name)
	}
	if len(tool.InputSchema.Properties) != 1 {
		t.Fatalf("input properties = %v; want exactly display_name", tool.InputSchema.Properties)
	}
	if _, ok := tool.InputSchema.Properties["display_name"]; !ok {
		t.Errorf("input properties = %v; want display_name", tool.InputSchema.Properties)
	}
	if len(tool.InputSchema.Required) != 0 {
		t.Errorf("required = %v; want none (display_name is optional)", tool.InputSchema.Required)
	}
}

// TestToolRegisterClientSemanticErrorPassesThrough pins the REQ-MT-015 error
// mapping: a *domain.SemanticError from the use case reaches the client as
// -32002 with its Spanish message intact.
func TestToolRegisterClientSemanticErrorPassesThrough(t *testing.T) {
	cases := []struct {
		name    string
		message string
	}{
		{"invalid phone", msgRegisterInvalidPhone},
		{"account collision", msgRegisterAccountPhone},
		{"rate limit", msgRegisterRateLimited},
		{"legacy row", msgRegisterLegacyRow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port := &mockRegisterClientPort{executeFn: func(context.Context, string, string) (*usecase.GetOrCreateClientResult, error) {
				return nil, &domain.SemanticError{Code: domain.ErrCodeConflict, Message: tc.message}
			}}
			srv := newRegistrationToolServer(t, port)

			resp := decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "+5491100999999", `{}`))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != tc.message {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, tc.message)
			}
		})
	}
}

// TestToolRegisterClientInternalErrorCollapses pins the leak boundary: a
// non-semantic failure is answered with the generic -32603, never with driver
// text or internal details.
func TestToolRegisterClientInternalErrorCollapses(t *testing.T) {
	port := &mockRegisterClientPort{executeFn: func(context.Context, string, string) (*usecase.GetOrCreateClientResult, error) {
		return nil, errors.New("registrar cliente: buscar por teléfono: sql: database is locked")
	}}
	srv := newRegistrationToolServer(t, port)

	resp := decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "+5491100999999", `{}`))
	wantErrorCode(t, resp, -32603)
	if resp.Error.Message != msgInternal {
		t.Errorf("error.message = %q; want %q", resp.Error.Message, msgInternal)
	}
}

// TestToolRegisterClientNilResultFailsClosed pins the nil-result guard: a port
// returning (nil, nil) must not dereference the result nor emit a half-empty
// payload (same class of guard as the get_business_profile nil check).
func TestToolRegisterClientNilResultFailsClosed(t *testing.T) {
	port := &mockRegisterClientPort{executeFn: func(context.Context, string, string) (*usecase.GetOrCreateClientResult, error) {
		return nil, nil
	}}
	srv := newRegistrationToolServer(t, port)

	resp := decodeToolResponse(t, callRegisterClient(t, srv.Handler(), "+5491100999999", `{}`))
	wantErrorCode(t, resp, -32603)
}

// TestToolRegisterClientFailsClosedOutsideTheSeam pins the defense in depth: the
// tool is reachable ONLY through the static anonymous allowlist, so a context
// without the anonymous marker (the tool mounted outside AuthMiddleware — a
// wiring bug) is rejected before the use case runs.
func TestToolRegisterClientFailsClosedOutsideTheSeam(t *testing.T) {
	port := &mockRegisterClientPort{executeFn: mustNotRun(t)}
	srv := newRegistrationToolServer(t, port)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"register_client","arguments":{}}}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("X-Caller-Id", "+5491100999999")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	resp := decodeToolResponse(t, rec)
	wantErrorCode(t, resp, -32603)
}
