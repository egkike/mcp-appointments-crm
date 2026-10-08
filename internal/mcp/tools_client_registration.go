package mcp

import (
	"context"
	"errors"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// register_client — the anonymous self-registration tool (feat-whatsapp-bot D2,
// design §1.2, REQ-MT-015 registry row).
//
// It is the only tool reachable without a resolved caller: AuthMiddleware's
// static allowlist admits its effective path BEFORE caller resolution and marks
// the request context anonymous (internal/auth/middleware.go, design §1.1), so
// the handler sees no Caller and derives no role. The identity is the
// X-Caller-Id header — the body has no phone field and an undeclared key is
// rejected by the SDK schema (additionalProperties:false) — and the use case
// owns every business rule (phone format, accounts collision, rate limit,
// placeholder name). The tool therefore carries NO ToolRBAC entry in either
// layer: its guards are the anonymous seam plus those use-case checks.
//
// Error mapping is the shared toMCPError boundary: *domain.SemanticError →
// -32002 with the Spanish message, anything else → the generic -32603.

// callerHeaderKey is the caller identity header. It mirrors the name
// AuthMiddleware step 1 and CallerResolver read; the handler trims it exactly
// the same way, so the id the use case stores is byte-identical to the one the
// resolver will look up on the next call (client-registration: "The stored
// value MUST be byte-identical to the header").
const callerHeaderKey = "X-Caller-Id"

// registerClientIn is the input of register_client: display_name and nothing
// else. There is deliberately NO phone (or caller_id/id) field — the identity
// comes from the X-Caller-Id header — and because the SDK infers
// additionalProperties:false from this struct, any other key in the payload is
// rejected before the handler runs. The schema published by tools/list therefore
// lists display_name exclusively (pinned by
// TestToolRegisterClientInputSchemaExposesOnlyDisplayName and
// TestToolRegisterClientRejectsUnknownBodyKeys).
type registerClientIn struct {
	DisplayName *string `json:"display_name,omitempty"`
}

// registerClientOut is the pinned output contract of register_client
// (REQ-MT-015): client_id is the header phone, display_name the STORED name (the
// `Cliente {phone}` placeholder when the request carried none) and created=false
// for an already-registered phone. All three keys are always present — no
// omitempty on a contract field.
type registerClientOut struct {
	ClientID    string `json:"client_id"`
	DisplayName string `json:"display_name"`
	Created     bool   `json:"created"`
}

// errNilRegistrationResult is returned when the registration port reports
// success without the result it was supposed to produce. The use case never
// does this; the guard fails closed with -32603 instead of dereferencing nil or
// emitting a half-empty payload (same class of guard as
// errNilMaintenanceEntity).
var errNilRegistrationResult = errors.New("register_client: use case returned no result")

// errRegistrationOutsideSeam is the fail-secure wiring guard: register_client is
// reachable ONLY through the anonymous allowlist seam, so a context without the
// anonymous marker means the tool was mounted outside AuthMiddleware. It is an
// internal (never client-facing) condition, so it collapses to -32603.
var errRegistrationOutsideSeam = errors.New("register_client: reached outside the anonymous allowlist seam")

// registerClientTool wires register_client onto the SDK server when the port is
// non-nil. The handler takes the phone from the request header, calls the use
// case with (phone, displayName) and maps the result onto the pinned output
// contract.
func (s *Server) registerClientTool() {
	if s.cfg.RegisterClient == nil {
		return
	}
	mcp.AddTool(s.impl, s.mcpTool("register_client", "Registra a un cliente nuevo a partir del número de teléfono del remitente. No requiere cuenta; el nombre para mostrar es opcional"),
		func(ctx context.Context, req *mcp.CallToolRequest, in registerClientIn) (*mcp.CallToolResult, registerClientOut, error) {
			// Fail closed (design §1.1): only the anonymous seam may reach this
			// handler. Without its marker the request did not pass the static
			// allowlist — a wiring bug, not a client call to serve.
			if !auth.IsAnonymous(ctx) {
				return nil, registerClientOut{}, toMCPError(errRegistrationOutsideSeam)
			}

			displayName := ""
			if in.DisplayName != nil {
				displayName = *in.DisplayName
			}

			result, err := s.cfg.RegisterClient.Execute(ctx, callerPhoneFromRequest(req), displayName)
			if err != nil {
				return nil, registerClientOut{}, toMCPError(err)
			}
			if result == nil {
				return nil, registerClientOut{}, toMCPError(errNilRegistrationResult)
			}
			return nil, registerClientOut{
				ClientID:    result.ClientID,
				DisplayName: result.DisplayName,
				Created:     result.Created,
			}, nil
		})
	s.toolNames["register_client"] = struct{}{}
}

// callerPhoneFromRequest reads the identity the anonymous seam leaves to the
// transport: the X-Caller-Id header, which the go-sdk exposes to a tool handler
// on RequestExtra.Header (the only per-request HTTP plumbing a handler can
// reach; the seam deliberately injects no Caller and no role).
//
// The value is trimmed exactly like AuthMiddleware step 1 and the resolver do,
// so the stored id matches the id looked up on the next call. An absent header
// yields "" and the use case answers its invalid-phone semantic error: the
// middleware answers the universal 401 before the seam, so this is a
// defense-in-depth path only and no caller is ever invented.
func callerPhoneFromRequest(req *mcp.CallToolRequest) string {
	if req == nil || req.Extra == nil || req.Extra.Header == nil {
		return ""
	}
	return strings.TrimSpace(req.Extra.Header.Get(callerHeaderKey))
}
