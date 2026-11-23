package mcp

import (
	"encoding/json"
	"testing"
)

// TestIntegrationSearchClientsRoleScoped proves that search_clients_advanced
// obeys the role matrix at the transport level with a real SQLite database.
func TestIntegrationSearchClientsRoleScoped(t *testing.T) {
	mux := newIntegrationMux(t)

	// Owner sees all seeded clients.
	rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_clients_advanced","arguments":{"query_text":"Cliente"}}}`)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("owner search failed: %d %q", code, msg)
	}
	var ownerOut struct {
		StructuredContent struct {
			Results []map[string]any `json:"results"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &ownerOut); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ownerOut.StructuredContent.Results) != 2 {
		t.Errorf("owner results = %d; want 2", len(ownerOut.StructuredContent.Results))
	}

	// Staff without a linked booking sees none (no bookings seeded).
	rec = postMCPCaller(t, mux, "staff-1", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_clients_advanced","arguments":{"query_text":"Cliente"}}}`)
	result, code, msg = decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("staff search failed: %d %q", code, msg)
	}
	var staffOut struct {
		StructuredContent struct {
			Results []map[string]any `json:"results"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &staffOut); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(staffOut.StructuredContent.Results) != 0 {
		t.Errorf("staff results = %d; want 0", len(staffOut.StructuredContent.Results))
	}
}

// TestIntegrationSearchServicesRequiresAuth proves the gate that REMAINS on
// search_services_advanced: an authenticated caller. Role no longer rejects
// anyone (issue #255 gives every authenticated caller a catalog view -- the
// maintenance FTS view for owner/admin, the active-only booking catalog for the
// rest), so the former "non-owner/admin is rejected" contract is gone by design.
func TestIntegrationSearchServicesRequiresAuth(t *testing.T) {
	mux := newIntegrationMux(t)

	rec := postMCPCaller(t, mux, "nobody-7", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_services_advanced","arguments":{"query_text":"Consulta"}}}`)
	_, code, msg := decodeRPCEnvelope(t, rec)
	if code == 0 {
		t.Errorf("an unrecognized caller reached the service catalog (msg=%q)", msg)
	}
}
