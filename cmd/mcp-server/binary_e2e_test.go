package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/db"
)

// TestBinaryEndToEnd exercises the SHIPPED wiring, not a mirror of it.
//
// Every other HTTP-level test in this repo builds its mux from
// internal/mcp's integration harness, which hand-composes the same graph as
// run(). That harness cannot see a drift in cmd/mcp-server/main.go: delete a
// use case, drop a tool registration or swap the RBAC map there and the suite
// stays green (debt L11, issue #249). This test closes that window the only way
// it can be closed from a package main test: build the real binary, start it on
// a temp database, and drive it over HTTP.
//
// What it proves end to end: config resolution (MCP_* env), the SQLite open and
// schema init, the setup-import gate, the audit HMAC key (D7 fails closed
// without it), signal-bound startup, mux mounting at /mcp, caller resolution
// against a real accounts row, the ToolRBAC gate, and one tool reaching its use
// case and repository.
func TestBinaryEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("binary E2E builds and execs the server; skipped under -short")
	}

	bin := buildServerBinary(t)
	baseURL := startServer(t, bin)

	t.Run("healthz reports ready", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			t.Fatalf("build healthz request: %v", err)
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /healthz = %d; want 200", resp.StatusCode)
		}
	})

	t.Run("authenticated tool call returns seeded data", func(t *testing.T) {
		// initialize first: the SDK requires the handshake before tool traffic.
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"binary-e2e","version":"0"}}}`
		result, code, msg := postRPC(t, baseURL, "owner-1", body)
		if code != 0 {
			t.Fatalf("initialize failed: %d %q", code, msg)
		}
		var init struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(result, &init); err != nil {
			t.Fatalf("initialize result: %v", err)
		}
		if init.ProtocolVersion != "2025-11-25" {
			t.Errorf("protocolVersion = %q; want 2025-11-25", init.ProtocolVersion)
		}

		body = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_business_profile","arguments":{}}}`
		result, code, msg = postRPC(t, baseURL, "owner-1", body)
		if code != 0 {
			t.Fatalf("tools/call failed: %d %q", code, msg)
		}
		var call struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(result, &call); err != nil {
			t.Fatalf("tools/call result: %v", err)
		}
		if len(call.Content) == 0 {
			t.Fatalf("tools/call returned no content: %s", string(result))
		}
		if !strings.Contains(call.Content[0].Text, seededBusinessName) {
			t.Errorf("tool output does not carry the seeded profile %q: %s", seededBusinessName, call.Content[0].Text)
		}
	})

	t.Run("unknown caller fails the caller resolution", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_business_profile","arguments":{}}}`
		_, code, msg := postRPC(t, baseURL, "nobody-9", body)
		if code == 0 {
			t.Fatal("tools/call with an unrecognized caller succeeded; the binary is not gating on X-Caller-Id")
		}
		// The transport surfaces auth refusals as JSON-RPC errors over HTTP 200, so
		// the assertion anchors on the message. This one is the caller resolver's
		// refusal, which is what an unrecognized caller must hit — not the map.
		if !strings.Contains(msg, "no te reconozco") {
			t.Errorf("unrecognized caller produced %q; want the caller resolver's refusal", msg)
		}
	})

	// The two subtests below are what make this E2E exercise the RBAC POLICY and
	// not just the wiring: same binary, same session, one caller that must be
	// refused by a gated row and be served by an ungated tool. An earlier version
	// only asserted "some error", which a schema or transport failure also
	// satisfies (advisory R3-weak-rbac-rejection-assertion, issue #249).
	t.Run("client role is refused an owner-only tool by the RBAC row", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_professional_schedules","arguments":{"professional_id":"p1"}}}`
		_, code, msg := postRPC(t, baseURL, clientCallerID, body)
		if code == 0 {
			t.Fatalf("a client caller reached get_professional_schedules; ToolRoles() must not allow it")
		}
		// "no tienes permiso para realizar esta acción" is the middleware's map gate
		// verbatim; the use-case gate words its refusal differently
		// (auth.RequireRole: `Rol %q no tiene permiso para esta operación`). Anchoring
		// on this exact text is what proves a ToolRoles() row refused the call rather
		// than some other layer erroring out.
		if !strings.Contains(msg, "no tienes permiso para realizar esta acción") {
			t.Errorf("client was rejected with %q; want the RBAC row's own refusal", msg)
		}
	})

	t.Run("client role is served by an ungated tool", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_clients_advanced","arguments":{}}}`
		result, code, msg := postRPC(t, baseURL, clientCallerID, body)
		if code != 0 {
			t.Fatalf("an ungated tool refused a client caller: %d %q", code, msg)
		}
		var call struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(result, &call); err != nil {
			t.Fatalf("tools/call result: %v", err)
		}
		if len(call.Content) == 0 {
			t.Fatalf("ungated tool returned no content: %s", string(result))
		}
	})

	// A client must reach its own booking lifecycle (issue #251) while the gate
	// keeps refusing what must stay closed. Asserting only the first half would
	// pass for a gate that was removed entirely.
	t.Run("client reaches the booking gate and is still refused the closed surface", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"create_booking","arguments":{"client_id":"` + clientCallerID + `","service_id":"s1","professional_id":"p1","start_datetime":"2026-12-01T10:00:00Z"}}}`
		_, code, msg := postRPC(t, baseURL, clientCallerID, body)
		// No fixtures for services/professionals exist in this database, so the use
		// case is expected to refuse on its own terms. What must NOT happen is the
		// transport refusing the role: that is the defect this test guards.
		if strings.Contains(msg, rbacRefusal) {
			t.Fatalf("the shipped gate still refuses a client its own booking: %q", msg)
		}
		if code == 0 {
			t.Logf("create_booking unexpectedly succeeded with the minimal fixture set: %q", msg)
		}

		body = `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_professional_schedules","arguments":{"professional_id":"p1"}}}`
		_, code, msg = postRPC(t, baseURL, clientCallerID, body)
		if code == 0 || !strings.Contains(msg, rbacRefusal) {
			t.Fatalf("get_professional_schedules must stay closed to a client; got code=%d msg=%q", code, msg)
		}
	})
}

// seededBusinessName is the profile written into the temp database. Its presence
// in the tool output is what proves the child process read OUR database, not a
// default path left over on the machine.
const seededBusinessName = "Negocio E2E Binario"

// clientCallerID is a caller that resolves to RoleClient: it has a clients row
// and no accounts row. The RBAC subtests need it to prove that a real row from
// ToolRoles() refuses it while an ungated tool serves it.
const clientCallerID = "client-e2e"

// rbacRefusal is the middleware's own refusal text (internal/auth/middleware.go).
// It is emitted only by the ToolRoles() gate, never by a use case, which is what
// makes it usable as an anchor: a use-case refusal words itself differently.
const rbacRefusal = "no tienes permiso para realizar esta acción"

// buildServerBinary compiles the package under test into a temp dir and returns
// the path. BUILDING the binary is the point: a test that instead called run()
// in-process would share the test binary's linkage and prove nothing about the
// artifact that ships.
func buildServerBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mcp-server-e2e")
	// The argument vector is fixed and bin is a path this test just minted with
	// t.TempDir(); no caller or environment value reaches it, so G204's concern
	// (a variable command line assembled from untrusted input) does not apply.
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".") //nolint:gosec // fixed argv, temp-dir output path
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v\n%s", err, stderr.String())
	}
	return bin
}

// startServer prepares an isolated database and operator config dir, launches
// the binary in serve mode and returns its base URL once /healthz answers.
//
// The environment is the whole contract: MCP_DB_PATH and MCP_CONFIG_DIR keep the
// run out of the operator's real ~/.local and ~/.config trees, MCP_PORT pins the
// listener, and MCP_SETUP_DIR points at an empty temp dir. SeedOnBoot never reads
// that dir here because it returns early once a business profile exists, which
// seedDatabase has already written.
func startServer(t *testing.T, bin string) string {
	t.Helper()

	workDir := t.TempDir()
	dbPath := filepath.Join(workDir, "reservas.db")
	configDir := filepath.Join(workDir, "config")
	setupDir := filepath.Join(workDir, "setup")
	for _, dir := range []string{configDir, setupDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	seedDatabase(t, dbPath)

	// Three attempts: reserving a port by listening and closing leaves a window
	// in which another process can steal it, and a lost race must not look like a
	// wiring failure.
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		port, err := reservePort(t.Context())
		if err != nil {
			t.Fatalf("reserve port: %v", err)
		}

		cmd := exec.CommandContext(t.Context(), bin)
		cmd.Env = append(os.Environ(),
			"MCP_DB_PATH="+dbPath,
			"MCP_CONFIG_DIR="+configDir,
			"MCP_SETUP_DIR="+setupDir,
			"MCP_BIND=127.0.0.1",
			fmt.Sprintf("MCP_PORT=%d", port),
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start server: %v", err)
		}

		stopped := false
		t.Cleanup(func() {
			if stopped {
				return
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})

		baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
		if err := waitForHealthz(t.Context(), baseURL+"/healthz"); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			stopped = true
			lastErr = fmt.Errorf("attempt %d: %w; server stderr:\n%s", attempt, err, stderr.String())
			continue
		}
		return baseURL
	}
	t.Fatalf("server never became healthy: %v", lastErr)
	return ""
}

// seedDatabase creates the schema and writes the minimum rows the assertions
// need: a non-empty business profile (which also makes SeedOnBoot a no-op) and
// one active owner account (which the caller resolver turns into a Caller).
func seedDatabase(t *testing.T, dbPath string) {
	t.Helper()
	ctx := t.Context()
	database, err := db.NewDatabase(ctx, dbPath)
	if err != nil {
		t.Fatalf("open temp database: %v", err)
	}
	defer database.Close() //nolint:errcheck // test cleanup

	rows := []string{
		`INSERT INTO business_profile (id, name, timezone, slot_interval_minutes) VALUES ('singleton', '` + seededBusinessName + `', 'America/Argentina/Buenos_Aires', 30)`,
		`INSERT INTO accounts (id, role, display_name, is_active) VALUES ('owner-1', 'owner', 'Owner', 1)`,
		// No accounts row: the caller resolver falls through to clients and yields
		// RoleClient, which is what the RBAC subtests need.
		`INSERT INTO clients (id, name, phone) VALUES ('` + clientCallerID + `', 'Cliente E2E', '+5491100000009')`,
	}
	for _, q := range rows {
		if _, err := database.Conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
}

// reservePort binds :0 to learn a free port and releases it for the child.
func reservePort(ctx context.Context) (int, error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

// waitForHealthz polls the readiness endpoint until the binary answers or the
// deadline passes.
func waitForHealthz(ctx context.Context, url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(20 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("healthz = %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("healthz never became ready: %w", last)
}

// postRPC sends one JSON-RPC request to the running binary and decodes the
// envelope, mirroring what an MCP client does over the streamable transport.
func postRPC(t *testing.T, baseURL, callerID, body string) (result json.RawMessage, code int64, msg string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if callerID != "" {
		req.Header.Set("X-Caller-Id", callerID)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, -1, fmt.Sprintf("status %d: %s", resp.StatusCode, string(raw))
	}

	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v; body=%s", err, string(raw))
	}
	if env.Error != nil {
		return nil, env.Error.Code, env.Error.Message
	}
	return env.Result, 0, ""
}
