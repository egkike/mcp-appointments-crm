package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// TestIntegrationClientCatalogReads pins the two reads a booking flow needs and
// the asymmetry that makes them safe (issue #255):
//
//   - a `client` can read the business profile and the service catalog;
//   - the client's catalog carries ONLY active services, even when the client
//     searches for a retired one BY NAME;
//   - the owner keeps seeing retired services through the SAME tool, so opening
//     the read to clients did not flatten the maintenance view into it.
//
// The retired service is the discriminator: without it, a client catalog built on
// the maintenance search would pass every assertion here.
func TestIntegrationClientCatalogReads(t *testing.T) {
	mux, dbConn := newIntegrationMuxWithDB(t)
	defer dbConn.Close() //nolint:errcheck // test cleanup

	const retiredName = "Servicio Retirado"
	if _, err := dbConn.ExecContext(t.Context(),
		`INSERT INTO services (id, name, description, duration_minutes, price, is_active) VALUES ('s-retired', ?, 'ya no se ofrece', 45, 250.0, 0)`,
		retiredName); err != nil {
		t.Fatalf("seed retired service: %v", err)
	}

	t.Run("client reads the business profile", func(t *testing.T) {
		result, code, msg := callToolAs(t, mux, "c1", "get_business_profile", `{}`)
		if code != 0 {
			t.Fatalf("a client could not read the business profile: %d %q", code, msg)
		}
		if text := toolText(t, result); !strings.Contains(text, "Mi Negocio") {
			t.Errorf("business profile payload does not carry the seeded name: %s", text)
		}
	})

	t.Run("client catalog is the active set", func(t *testing.T) {
		names := serviceNamesAs(t, mux, "c1", "")
		if len(names) == 0 {
			t.Fatal("a client received an empty service catalog with an empty term")
		}
		if containsName(names, "Retirado") {
			t.Errorf("client catalog exposes a retired service: %v", names)
		}
		if !containsName(names, "Consulta") {
			t.Errorf("client catalog is missing the active service: %v", names)
		}
	})

	t.Run("client cannot find a retired service by name", func(t *testing.T) {
		names := serviceNamesAs(t, mux, "c1", "Retirado")
		if len(names) != 0 {
			t.Errorf("a client searching the retired service by name got it back: %v", names)
		}
	})

	t.Run("owner still needs a term for the maintenance search", func(t *testing.T) {
		// The spec scenario pinned in this change says the maintenance FTS view
		// rejects a missing or empty term while the booking view accepts it. The
		// client half is covered above; this is the half that has no catalog to
		// fall back to, so it must refuse rather than empty out.
		_, code, msg := callToolAs(t, mux, "owner-1", "search_services_advanced", `{"query_text":""}`)
		if code == 0 {
			t.Fatal("owner search with an empty term succeeded; the maintenance view must require a term")
		}
		if !strings.Contains(msg, "vacía") {
			t.Errorf("refusal = %q; want the semantic empty-query error", msg)
		}
	})

	t.Run("owner still sees the retired service", func(t *testing.T) {
		names := serviceNamesAs(t, mux, "owner-1", "Retirado")
		if !containsName(names, "Retirado") {
			t.Errorf("owner search lost the retired service; the maintenance view regressed: %v", names)
		}
	})

	t.Run("client still cannot reach the maintenance writes", func(t *testing.T) {
		_, code, msg := callToolAs(t, mux, "c1", "create_service", `{"name":"x","duration_minutes":30,"price":10}`)
		if code == 0 || !strings.Contains(msg, auth.MsgForbiddenTool) {
			t.Fatalf("create_service must stay owner-only; got code=%d msg=%q", code, msg)
		}
	})
}

// callToolAs performs one tools/call through the transport as callerID and
// decodes the envelope. (The package already has a callTool helper with a
// different signature; this one takes a caller id header.)
func callToolAs(t *testing.T, mux http.Handler, callerID, tool, args string) (result json.RawMessage, code int64, msg string) {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tool, args)
	rec := postMCPCaller(t, mux, callerID, body)
	return decodeRPCEnvelope(t, rec)
}

// toolText returns the first text block of a tools/call result.
func toolText(t *testing.T, result json.RawMessage) string {
	t.Helper()
	var content struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &content); err != nil {
		t.Fatalf("tools/call result: %v", err)
	}
	if len(content.Content) == 0 {
		t.Fatalf("tools/call returned no content: %s", string(result))
	}
	return content.Content[0].Text
}

// serviceNamesAs runs search_services_advanced as callerID and returns the names
// the caller received.
func serviceNamesAs(t *testing.T, mux http.Handler, callerID, query string) []string {
	t.Helper()
	args := fmt.Sprintf(`{"query_text":%q}`, query)
	result, code, msg := callToolAs(t, mux, callerID, "search_services_advanced", args)
	if code != 0 {
		t.Fatalf("search_services_advanced as %s failed: %d %q", callerID, code, msg)
	}
	var payload struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(toolText(t, result)), &payload); err != nil {
		t.Fatalf("search_services_advanced payload: %v", err)
	}
	names := make([]string, 0, len(payload.Results))
	for _, entry := range payload.Results {
		names = append(names, entry.Name)
	}
	return names
}

// containsName reports whether any name carries the given substring.
func containsName(names []string, want string) bool {
	for _, name := range names {
		if strings.Contains(name, want) {
			return true
		}
	}
	return false
}
