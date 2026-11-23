package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file pins the agent-surface T1 contract (issue #265, spec S2/S3):
// tools/list answers the tools THE CALLER CAN INVOKE, not the whole registry.
//
// The expectations are derived from the single policy pair — ToolRoles() and
// UngatedTools() — instead of a copied literal, so a policy row and the filter
// can never disagree silently. The literal counts are the deliberate pin: they
// make an unintended broadening of the surface visible in the diff.

// The wire contract per role, derived from the policy on 2026-11-23:
// 20 gated rows (owner 20, admin 8, staff 5, client 5) + 6 ungated tools.
const (
	expectedOwnerToolCount  = expectedToolCount // 26
	expectedAdminToolCount  = 14
	expectedStaffToolCount  = 11
	expectedClientToolCount = 11
)

// policyVisibleTools returns the tool names the given role may invoke, sorted,
// as the union of its ToolRoles() rows and every UngatedTool.
//
// UngatedTools() carries two shapes (see tool_roles.go): downstream-enforced
// tools, open to any authenticated caller, and register_client, the anonymous
// allowlist seam. tools/list is only reachable with a resolved caller (a
// missing header is refused before the handler), so "visible to an
// authenticated caller" is the whole rule here: the seam's caller resolution
// happens downstream, not in the discovery surface.
func policyVisibleTools(role string) []string {
	names := make([]string, 0, expectedToolCount)
	for tool, roles := range ToolRoles() {
		if slices.Contains(roles, role) {
			names = append(names, tool)
		}
	}
	for tool := range UngatedTools() {
		names = append(names, tool)
	}
	sort.Strings(names)
	return names
}

// listToolsForCaller issues tools/list over the real mux and returns the
// advertised tool names, sorted. It fails the test on a JSON-RPC error.
func listToolsForCaller(t *testing.T, h http.Handler, callerID string) []string {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	result, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, h, callerID, body))
	if code != 0 {
		t.Fatalf("caller %q tools/list failed: %d %q", callerID, code, msg)
	}
	var payload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	names := make([]string, 0, len(payload.Tools))
	for _, tool := range payload.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// TestToolsListFiltersByCallerRole is the T1 wire contract: every role gets
// exactly the tools the policy allows it to invoke, owner included.
//
// Before the filter landed, all four callers received the same 26-tool
// surface (the measured defect: 20.151 B for staff and client too). Owner is
// asserted as well, not just the narrowed roles, because a filter that hides
// something the owner can invoke is a capability regression (spec S3).
func TestToolsListFiltersByCallerRole(t *testing.T) {
	mux := newIntegrationMux(t)

	cases := []struct {
		caller    string
		role      string
		wantCount int
	}{
		{"owner-1", auth.RoleOwner, expectedOwnerToolCount},
		{"admin-1", auth.RoleAdmin, expectedAdminToolCount},
		{"staff-1", auth.RoleStaff, expectedStaffToolCount},
		{"c1", auth.RoleClient, expectedClientToolCount},
	}

	for _, tc := range cases {
		t.Run(tc.caller, func(t *testing.T) {
			got := listToolsForCaller(t, mux, tc.caller)
			want := policyVisibleTools(tc.role)

			if !slices.Equal(got, want) {
				t.Errorf("tools/list for %s (%s) advertised:\n got %v\nwant %v", tc.caller, tc.role, got, want)
			}
			if len(got) != tc.wantCount {
				t.Errorf("tools/list for %s (%s) = %d tools; want %d (the pinned wire contract)",
					tc.caller, tc.role, len(got), tc.wantCount)
			}
		})
	}
}

// TestToolsListKeepsUngatedToolsVisibleForEveryAuthenticatedCaller pins the
// SECOND half of the policy (spec S2): a tool without a ToolRBAC row is not a
// tool to hide. Narrowing these to nobody would break the client booking flow
// and the anonymous registration seam, and the role table above would not
// notice, because it derives its expectation from the same maps.
func TestToolsListKeepsUngatedToolsVisibleForEveryAuthenticatedCaller(t *testing.T) {
	mux := newIntegrationMux(t)

	ungated := UngatedTools()
	if len(ungated) == 0 {
		t.Fatal("UngatedTools() is empty; the policy pair is broken, not the filter")
	}

	for _, caller := range []string{"owner-1", "admin-1", "staff-1", "c1"} {
		visible := listToolsForCaller(t, mux, caller)
		for tool := range ungated {
			if !slices.Contains(visible, tool) {
				t.Errorf("caller %s does not see ungated tool %q (reason: %s)", caller, tool, ungated[tool])
			}
		}
	}
}

// TestToolsCallOnAFilteredToolStaysForbidden is the defense-in-depth half of
// T1 (spec S3): filtering is DISCOVERY, never authorization. A staff caller
// does not see delete_service, and calling it by name must still be refused by
// the RBAC layer with the same -32001 and the same message as before the
// filter existed. If this ever passes, the filter replaced the guard instead of
// narrowing the menu.
func TestToolsCallOnAFilteredToolStaysForbidden(t *testing.T) {
	mux := newIntegrationMux(t)

	const hidden = "delete_service"
	if slices.Contains(listToolsForCaller(t, mux, "staff-1"), hidden) {
		t.Fatalf("precondition failed: staff-1 sees %q; the filter is not narrowing the surface", hidden)
	}

	for _, tool := range []string{"delete_service", "update_business_profile", "get_pending_alerts"} {
		if slices.Contains(listToolsForCaller(t, mux, "staff-1"), tool) {
			t.Fatalf("precondition failed: staff-1 sees %q", tool)
		}
		_, code, msg := callMCPTool(t, mux, "staff-1", tool, `{"service_id":"s1","id":"s1"}`)
		if code != -32001 {
			t.Errorf("staff-1 tools/call %q code = %d; want -32001 (msg=%q)", tool, code, msg)
		}
		if msg != auth.MsgForbiddenTool {
			t.Errorf("staff-1 tools/call %q message = %q; want %q", tool, msg, auth.MsgForbiddenTool)
		}
	}
}

// TestToolsListStillRefusesAMissingCallerHeader guards the boundary the filter
// must not touch: without X-Caller-Id there is no role to filter by, and the
// answer stays the pre-existing -32000 refusal rather than an empty or full
// surface.
func TestToolsListStillRefusesAMissingCallerHeader(t *testing.T) {
	mux := newIntegrationMux(t)

	_, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if code != codeAuthRequired {
		t.Errorf("tools/list without X-Caller-Id code = %d; want %d (msg=%q)", code, codeAuthRequired, msg)
	}
	if msg != msgAuthRequired {
		t.Errorf("tools/list without X-Caller-Id message = %q; want %q", msg, msgAuthRequired)
	}
}

// TestVisibleToolsMirrorsTheTransportGate pins the predicate the filter shares
// with AuthMiddleware (auth.CanInvoke, internal/auth/can_invoke.go — one
// function since advisory S1, issue #268): an absent row or an EMPTY row is
// open to any role, a populated row admits only its listed roles, and
// order/duplicates of the registry survive unchanged. The empty-row case cannot
// occur in today's policy, which is exactly why it is asserted here: it is the
// shape a future edit is most likely to translate wrongly.
//
// This is the filter-side view of the shared predicate. It is NOT a
// cross-consumer equivalence test — the two gates now literally call the same
// function — so what it pins is the wiring: visibleTools must route through
// auth.CanInvoke instead of re-deriving the rule locally (the mutation that
// proves it: make visibleTools return every tool and this test goes red while
// the tools/call gate stays green).
func TestVisibleToolsMirrorsTheTransportGate(t *testing.T) {
	rbac := auth.ToolRBAC{
		"owner_only": {auth.RoleOwner},
		"shared":     {auth.RoleOwner, auth.RoleStaff},
		"empty_row":  {},
	}
	registry := []*sdkmcp.Tool{
		{Name: "owner_only"},
		{Name: "shared"},
		{Name: "empty_row"},
		{Name: "no_row"},
		{Name: "shared"},
	}

	cases := []struct {
		role string
		want []string
	}{
		{auth.RoleOwner, []string{"owner_only", "shared", "empty_row", "no_row", "shared"}},
		{auth.RoleStaff, []string{"shared", "empty_row", "no_row", "shared"}},
		{auth.RoleClient, []string{"empty_row", "no_row"}},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			got := make([]string, 0, len(registry))
			for _, tool := range visibleTools(registry, rbac, tc.role) {
				got = append(got, tool.Name)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("visibleTools(%s) = %v; want %v", tc.role, got, tc.want)
			}
		})
	}
}

// TestToolsListStaffSurfaceMatchesTheSpec pins the staff surface by NAME, not
// only by count: the per-role test derives its expectation from the live policy
// maps, so a row edit there would move the expectation and the assertion
// together. This literal is the other half of that pair — it is the exact list
// openspec/specs/mcp-transport/spec.md REQ-MT-005 publishes, so a policy change
// that narrows or widens what staff can discover has to be a deliberate edit in
// three places (policy, test, spec) instead of a silently stale document.
func TestToolsListStaffSurfaceMatchesTheSpec(t *testing.T) {
	mux := newIntegrationMux(t)

	want := []string{
		"cancel_booking",
		"check_availability",
		"create_booking",
		"find_available_slots",
		"get_booking",
		"get_business_profile",
		"register_client",
		"reschedule_booking",
		"search_clients_advanced",
		"search_professionals_advanced",
		"search_services_advanced",
	}

	if got := listToolsForCaller(t, mux, "staff-1"); !slices.Equal(got, want) {
		t.Errorf("staff tools/list advertised:\n got %v\nwant %v (REQ-MT-005)", got, want)
	}
}

// TestToolsListFilterWithoutACallerLogsAndDoesNotFilter pins the fail-open
// branch that REQ-MT-005 now publishes as deliberate. The branch is unreachable
// through AuthMiddleware (a caller-less request is refused with -32000 before
// the SDK handler), so this test drives the middleware directly: the point is
// that the surface is left intact AND the condition is loud. Without the log
// assertion the requirement would be unproven, and a silent fail-open is
// exactly the failure mode the requirement exists to forbid.
func TestToolsListFilterWithoutACallerLogsAndDoesNotFilter(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	inner := func(context.Context, string, sdkmcp.Request) (sdkmcp.Result, error) {
		return &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{
			{Name: "owner_only"},
			{Name: "shared"},
		}}, nil
	}
	filtered := toolsListFilter(auth.ToolRBAC{"owner_only": {auth.RoleOwner}}, logger)(inner)

	// A context with no injected caller: exactly the condition under test.
	result, err := filtered(context.Background(), methodListTools, &sdkmcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	list, ok := result.(*sdkmcp.ListToolsResult)
	if !ok || list == nil {
		t.Fatalf("filter returned %T; want *mcp.ListToolsResult", result)
	}
	if len(list.Tools) != 2 {
		t.Errorf("tools = %d; want the unfiltered 2 (fail-open)", len(list.Tools))
	}
	if !strings.Contains(logs.String(), "without a resolved caller") {
		t.Errorf("expected a warn log naming the missing caller; got %q", logs.String())
	}
}

// capturingLogger mirrors discardLogger (shutdown_test.go) for the tests that
// must READ the records instead of dropping them: same one-line construction,
// but the handler keeps them (captureHandler, logging_test.go).
func capturingLogger() (*slog.Logger, *captureHandler) {
	cap := &captureHandler{}
	return slog.New(cap), cap
}

// warnRecords returns the records at Warn or above, in emission order: the
// level an operator actually sees without turning anything on.
func warnRecords(h *captureHandler) []slog.Record {
	var out []slog.Record
	for _, r := range h.records {
		if r.Level >= slog.LevelWarn {
			out = append(out, r)
		}
	}
	return out
}

// recordMessages flattens records to their messages so a count failure shows
// WHAT was logged instead of only how many lines there were.
func recordMessages(records []slog.Record) []string {
	messages := make([]string, 0, len(records))
	for _, r := range records {
		messages = append(messages, r.Message)
	}
	return messages
}

// TestServerRegistersNoDiscoveryFilterWithoutAPolicy pins S2 (advisory, issue
// #268) through both halves of its contract:
//
//  1. exactly ONE Warn at construction when ToolRBAC is nil, naming the missing
//     policy — a partial Config must not degrade discovery in silence; and
//  2. NOTHING registered, observed through the log the filter alone can print: a
//     caller-less tools/list through the unauthenticated chain emits no
//     "without a resolved caller" Warn. Half 1 alone could not tell
//     "conditional registration" from "registered but inert on a nil map",
//     which is the same surface with a middleware doing nothing.
func TestServerRegistersNoDiscoveryFilterWithoutAPolicy(t *testing.T) {
	t.Run("nil policy warns once and registers nothing", func(t *testing.T) {
		logger, cap := capturingLogger()
		srv := NewServer(Config{Version: "test", Logger: logger})

		warns := warnRecords(cap)
		if len(warns) != 1 {
			t.Fatalf("construction emitted %d warn records; want exactly 1 (S2): %v",
				len(warns), recordMessages(warns))
		}
		if msg := warns[0].Message; !strings.Contains(msg, "ToolRBAC") {
			t.Errorf("construction warn = %q; want it to name the missing ToolRBAC policy", msg)
		}

		// No X-Caller-Id: only a REGISTERED filter answers this request, and it
		// answers with its own fail-open Warn.
		rec := postMCPCaller(t, srv.Handler(), "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200 (body=%s)", rec.Code, rec.Body.String())
		}
		if got := len(warnRecords(cap)); got != 1 {
			t.Errorf("warn records after a caller-less tools/list = %d; want still 1: "+
				"the discovery filter must not be registered without a policy (%v)",
				got, recordMessages(warnRecords(cap)))
		}
	})

	t.Run("a policy registers the filter and stays quiet", func(t *testing.T) {
		logger, cap := capturingLogger()
		NewServer(Config{
			Version:  "test",
			Logger:   logger,
			ToolRBAC: auth.ToolRBAC{"create_booking": {auth.RoleOwner}},
		})

		if warns := warnRecords(cap); len(warns) != 0 {
			t.Errorf("construction with a policy emitted %d warns; want 0: %v",
				len(warns), recordMessages(warns))
		}
	})
}

// TestToolsListCarriesNoNextCursor pins the pagination invariant of the shared
// discovery surface (S3, advisory, issue #268) on the wire instead of in a
// comment: the filter narrows the result AFTER the SDK paginated it, so
// ServerOptions.PageSize must stay 0 and tools/list must never answer with a
// nextCursor. A non-zero page size would compute the cursor without knowing the
// caller's role: the client would follow it into the next page and the tools
// this filter kept for that role would be unreachable — a capability hole
// dressed as pagination.
func TestToolsListCarriesNoNextCursor(t *testing.T) {
	mux := newIntegrationMux(t)

	for _, caller := range []string{"owner-1", "admin-1", "staff-1", "c1"} {
		t.Run(caller, func(t *testing.T) {
			result, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, caller,
				`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			if code != 0 {
				t.Fatalf("caller %s tools/list failed: %d %q", caller, code, msg)
			}

			var page map[string]json.RawMessage
			if err := json.Unmarshal(result, &page); err != nil {
				t.Fatalf("unmarshal tools/list result: %v; raw=%s", err, result)
			}
			// Guard against a vacuous pass: the cursor keys can only be absent
			// if the envelope is the one this test thinks it is.
			if _, ok := page["tools"]; !ok {
				t.Fatalf("tools/list result carries no tools key; raw=%s", result)
			}
			if _, ok := page["nextCursor"]; ok {
				t.Errorf("tools/list for %s carries a nextCursor (S3: the filter narrows after pagination); raw=%s",
					caller, result)
			}
		})
	}
}
