package mcp

import (
	"encoding/json"
	"sort"
	"testing"
)

// TestToolRolesCoverTheRegisteredSurface pins the tool → role policy against the
// surface the server ACTUALLY registers, not against a hand-kept copy of it.
//
// It compares three production sources: the registered tools (observed through
// tools/list, which is what an LLM client sees), ToolRoles() and UngatedTools().
// The failure it exists to catch is silent privilege expansion: auth.ToolRBAC
// lets an absent key through, so a tool registered without a row and without a
// declaration would be reachable by every authenticated caller while every other
// test stays green.
//
// Proof it can fail (mutation evidence, issue #249): deleting any row from
// ToolRoles() turns the first check red, and adding a row for a tool the server
// never registers turns the second one red.
func TestToolRolesCoverTheRegisteredSurface(t *testing.T) {
	mux := newIntegrationMux(t)

	rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("tools/list failed: %d %q", code, msg)
	}

	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		t.Fatalf("tools/list result: %v", err)
	}
	if len(list.Tools) == 0 {
		t.Fatal("tools/list returned no tools: the coverage assertions below would be vacuous")
	}

	registered := make(map[string]struct{}, len(list.Tools))
	for _, tool := range list.Tools {
		registered[tool.Name] = struct{}{}
	}

	roles := ToolRoles()
	ungated := UngatedTools()

	for _, name := range sortedKeys(registered) {
		_, gated := roles[name]
		_, declared := ungated[name]
		if !gated && !declared {
			t.Errorf("tool %q is registered but carries no ToolRoles() row and is not declared in UngatedTools(): "+
				"an absent RBAC key means it is open to every authenticated caller", name)
		}
	}

	for _, name := range sortedKeys(roles) {
		if _, ok := registered[name]; !ok {
			t.Errorf("ToolRoles() gates %q, which the server does not register: the row is dead policy", name)
		}
	}

	for _, name := range sortedKeys(ungated) {
		if _, ok := registered[name]; !ok {
			t.Errorf("UngatedTools() declares %q, which the server does not register: the declaration is stale", name)
		}
		if reason := ungated[name]; reason == "" {
			t.Errorf("UngatedTools() declares %q with an empty reason: the declaration must say where its authorization lives", name)
		}
	}

	for name := range ungated {
		if _, both := roles[name]; both {
			t.Errorf("%q appears in both ToolRoles() and UngatedTools(): the two maps must partition the surface", name)
		}
	}

	// No count assertion here on purpose: the four loops above already force the
	// cardinality. Every registered tool carries exactly one declaration, and
	// every declaration names a registered tool, so the two maps can neither
	// overlap nor leave a hole (advisory nits, issue #271).
}

// sortedKeys returns the map keys in a stable order so failures are readable and
// reproducible.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
