package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthz covers REQ-MT-014: GET /healthz returns 200 with
// {"status":"ok","version":"<version>"}.
func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(Healthz("1.2.3"))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body %q is not JSON: %v", body, err)
	}
	if got["status"] != "ok" {
		t.Errorf("status field = %q, want %q", got["status"], "ok")
	}
	if got["version"] != "1.2.3" {
		t.Errorf("version field = %q, want %q", got["version"], "1.2.3")
	}
}
