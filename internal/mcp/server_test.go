package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testServerVersion = "1.2.3"

// newTestServer boots a real httptest server with the transport skeleton.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := NewServer(Config{Version: testServerVersion})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// postMCP issues a JSON-RPC POST to /mcp with the headers the SDK requires
// and returns the HTTP status plus the response body. The response body is
// closed inside the helper.
func postMCP(t *testing.T, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url+"/mcp", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

// TestServerInitialize covers REQ-MT-004: initialize negotiates protocol
// version 2025-11-25 and returns serverInfo plus capabilities.tools.
func TestServerInitialize(t *testing.T) {
	ts := newTestServer(t)
	status, body := postMCP(t, ts.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0"}}}`)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", status, http.StatusOK, body)
	}
	var envelope struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
			Capabilities struct {
				Tools *struct {
					ListChanged bool `json:"listChanged"`
				} `json:"tools"`
			} `json:"capabilities"`
		} `json:"result"`
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("response %q is not JSON: %v", body, err)
	}
	if envelope.Error != nil {
		t.Fatalf("initialize returned JSON-RPC error code %d", envelope.Error.Code)
	}
	if envelope.Result.ProtocolVersion != "2025-11-25" {
		t.Errorf("protocolVersion = %q, want %q", envelope.Result.ProtocolVersion, "2025-11-25")
	}
	if envelope.Result.ServerInfo.Name != "mcp-appointments-crm" {
		t.Errorf("serverInfo.name = %q, want %q", envelope.Result.ServerInfo.Name, "mcp-appointments-crm")
	}
	if envelope.Result.ServerInfo.Version != testServerVersion {
		t.Errorf("serverInfo.version = %q, want %q", envelope.Result.ServerInfo.Version, testServerVersion)
	}
	if envelope.Result.Capabilities.Tools == nil {
		t.Error("capabilities.tools is missing, want the tools capability advertised")
	}
}

// TestServerToolsListEmpty covers the PR 1 skeleton contract: tools/list
// returns an empty array (tools are registered in PR 2).
func TestServerToolsListEmpty(t *testing.T) {
	ts := newTestServer(t)
	status, body := postMCP(t, ts.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", status, http.StatusOK, body)
	}
	var envelope struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("response %q is not JSON: %v", body, err)
	}
	if envelope.Error != nil {
		t.Fatalf("tools/list returned JSON-RPC error code %d", envelope.Error.Code)
	}
	if envelope.Result.Tools == nil {
		t.Fatal("result.tools is null, want an empty array")
	}
	if len(envelope.Result.Tools) != 0 {
		t.Fatalf("result.tools has %d entries, want 0 (PR 1 skeleton)", len(envelope.Result.Tools))
	}
}

// TestServerGetMethodNotAllowed covers REQ-MT-002: in stateless mode the SDK
// answers GET /mcp with 405.
func TestServerGetMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

// TestServerMalformedJSON covers REQ-MT-003: a body that is not valid JSON
// answers with the JSON-RPC -32700 Parse error envelope, id null.
func TestServerMalformedJSON(t *testing.T) {
	ts := newTestServer(t)
	status, body := postMCP(t, ts.URL, `{"jsonrpc":"2.0","method":`)

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body %s)", status, http.StatusBadRequest, body)
	}
	var envelope struct {
		JSONRPC string `json:"jsonrpc"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		ID *json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("response %q is not JSON: %v", body, err)
	}
	if envelope.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want %q", envelope.JSONRPC, "2.0")
	}
	if envelope.Error.Code != -32700 {
		t.Errorf("error.code = %d, want %d", envelope.Error.Code, -32700)
	}
	if envelope.Error.Message != "Parse error" {
		t.Errorf("error.message = %q, want %q", envelope.Error.Message, "Parse error")
	}
	if envelope.ID != nil {
		t.Errorf("id = %s, want null", *envelope.ID)
	}
}
