package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callerHeaderTransport injects X-Caller-Id on every request the SDK client
// makes. The SDK transport has no header hook; this mirrors the production
// Hermes client (REQ-AM-WIRED-001) and keeps the e2e chain fully
// authenticated.
type callerHeaderTransport struct {
	base     http.RoundTripper
	callerID string
}

func (t *callerHeaderTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("X-Caller-Id", t.callerID)
	return t.base.RoundTrip(r)
}

// TestE2EMockClient drives the production /mcp endpoint with the real go-sdk
// client over Streamable HTTP (REQ-MT-014): initialize handshake happens
// inside Connect; ListTools proves the six tools are discoverable; CallTool
// proves check_availability resolves a valid slot end-to-end against the
// seeded SQLite file. DisableStandaloneSSE matches the stateless JSON server
// (GET /mcp answers 405).
func TestE2EMockClient(t *testing.T) {
	mux := newIntegrationMux(t)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "test"}, &mcp.ClientOptions{})
	transport := &mcp.StreamableClientTransport{
		Endpoint: ts.URL + "/mcp",
		HTTPClient: &http.Client{
			Transport: &callerHeaderTransport{base: http.DefaultTransport, callerID: "owner-1"},
		},
		DisableStandaloneSSE: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, &mcp.ClientSessionOptions{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 6 {
		t.Errorf("tools = %d; want 6", len(tools.Tools))
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_availability",
		Arguments: map[string]any{
			"service_id":      "s1",
			"professional_id": "p1",
			"start_datetime":  "2026-08-24T10:00:00-03:00",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("call tool returned IsError; content=%+v", result.Content)
	}
	sc, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T; want map[string]any", result.StructuredContent)
	}
	if avail, _ := sc["available"].(bool); !avail {
		t.Errorf("available = %v; want true (valid Monday slot)", sc["available"])
	}
}
