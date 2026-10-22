package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/db"
)

// Hermes MCP client contract (defect-outputschema-array-root-contract, v0.9.1):
// the pydantic-based MCP client validates the ROOT of every tool's
// outputSchema as Literal["object"], so a tool whose typed output is a bare
// slice — the JSON Schema generator then emits "type": ["null", "array"] —
// poisons the whole ListToolsResult and the server stays parked without tools.
//
// This test is the catalog-level guard: it boots the REAL composition root
// (run) in-process, asks the live SDK server for tools/list and asserts, for
// every registered tool that declares an outputSchema, that its root type is
// exactly the JSON string "object". It is the only place the full 25-tool
// surface is inspected at once, so a new bare-slice output cannot ship
// unnoticed (the array shape is a legal MCP schema, which is why no handler
// test can catch it).

// wiredToolCount mirrors internal/mcp's expectedToolCount: the composition root
// wires one non-nil port per tool, so serve mode must advertise exactly this
// many. Kept as a literal here on purpose — importing the mock helpers is not
// possible across packages, and a drift makes this contract test fail loudly.
const wiredToolCount = 25

// toolSchema is the slice of a tools/list entry this contract cares about.
type toolSchema struct {
	Name         string          `json:"name"`
	OutputSchema json.RawMessage `json:"outputSchema"`
}

// TestServeModeToolOutputSchemaRootIsObject pins the Hermes MCP client contract
// across the whole tool catalog.
func TestServeModeToolOutputSchemaRootIsObject(t *testing.T) {
	baseURL := startServeMode(t)

	tools := listTools(t, baseURL, "owner-1")
	if len(tools) != wiredToolCount {
		t.Fatalf("tools = %d; want %d", len(tools), wiredToolCount)
	}

	names := make([]string, 0, len(tools))
	absent := make([]string, 0, len(tools))
	offenders := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
		if len(tool.OutputSchema) == 0 {
			// No typed output ⇒ no outputSchema on the wire ⇒ nothing for the
			// client to validate. Recorded so the set is visible in the log line.
			absent = append(absent, tool.Name)
			continue
		}
		rawType, ok := outputSchemaRootType(t, tool)
		if !ok {
			continue
		}
		if string(rawType) != `"object"` {
			offenders = append(offenders, fmt.Sprintf("%s (root type %s)", tool.Name, rawType))
		}
	}
	sort.Strings(names)

	if len(offenders) > 0 {
		t.Errorf("tools whose outputSchema root is not {\"type\":\"object\"}: %s", strings.Join(offenders, ", "))
	}
	// The catalog is the evidence for the report on every run, green or red.
	t.Logf("outputSchema roots: %d tools checked, %d object, %d without outputSchema (%s); tools: %s",
		len(tools), len(tools)-len(offenders)-len(absent), len(absent), strings.Join(absent, ", "), strings.Join(names, ", "))
}

// outputSchemaRootType extracts the raw JSON token of the outputSchema "type"
// property, so the comparison is exact: "object" passes, "array" and type
// arrays such as ["null","array"] fail.
func outputSchemaRootType(t *testing.T, tool toolSchema) (json.RawMessage, bool) {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(tool.OutputSchema, &root); err != nil {
		t.Errorf("%s: unmarshal outputSchema: %v; schema=%s", tool.Name, err, tool.OutputSchema)
		return nil, false
	}
	rawType, ok := root["type"]
	if !ok {
		t.Errorf("%s: outputSchema has no root type; schema=%s", tool.Name, tool.OutputSchema)
		return nil, false
	}
	return rawType, true
}

// startServeMode boots the production composition root in-process: run() opens
// its own database, wires every port, and serves /mcp + /healthz on a free
// loopback port. The database is pre-marked as configured so the setup seeder
// is a no-op, and one active owner is seeded so tools/list can be authorized
// through the real AuthMiddleware.
func startServeMode(t *testing.T) string {
	t.Helper()

	// run() logs through slog.Default(); keep the test output readable.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	dbPath := filepath.Join(t.TempDir(), "schema-contract.db")
	seedConfiguredDatabase(t, dbPath, "owner-1")
	port := freeLoopbackPort(t)

	// run() resolves both from the environment (ADR-0007 precedence); t.Setenv
	// keeps the mutation scoped to this test.
	t.Setenv("MCP_DB_PATH", dbPath)
	t.Setenv("MCP_BIND", "127.0.0.1")
	t.Setenv("MCP_PORT", port)

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-served:
			if err != nil {
				t.Errorf("serve mode returned error on shutdown: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("serve mode did not stop within 30s of the context cancel")
		}
	})

	baseURL := "http://" + net.JoinHostPort("127.0.0.1", port)
	waitHealthy(t, baseURL)
	return baseURL
}

// seedConfiguredDatabase creates the SQLite file with the canonical schema and
// the two rows serve mode needs to be reachable: a business profile whose name
// is set (so SeedOnBoot skips the wizard import) and an active owner account
// (so AuthMiddleware resolves the X-Caller-Id header).
func seedConfiguredDatabase(t *testing.T, dbPath, ownerID string) {
	t.Helper()
	ctx := context.Background()
	database, err := db.NewDatabase(ctx, dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Fatalf("close seeded database: %v", err)
		}
	}()

	statements := []struct {
		query string
		args  []any
	}{
		{
			// A non-empty name means "already configured" for config.SeedOnBoot.
			query: `INSERT OR IGNORE INTO business_profile (id, name) VALUES ('singleton', 'Contrato de esquema')`,
		},
		{
			query: `INSERT INTO accounts (id, role, display_name, is_active) VALUES (?, 'owner', 'Owner', 1)`,
			args:  []any{ownerID},
		},
	}
	for _, statement := range statements {
		if _, err := database.Conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed %q: %v", statement.query, err)
		}
	}
}

// freeLoopbackPort reserves and releases a loopback port, so serve mode can bind
// a concrete address the test can poll. The window between Close and bind is
// acceptable for a local test; a collision fails fast in waitHealthy.
func freeLoopbackPort(t *testing.T) string {
	t.Helper()
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address = %T; want *net.TCPAddr", listener.Addr())
	}
	port := fmt.Sprintf("%d", addr.Port)
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}
	return port
}

// waitHealthy blocks until /healthz answers 200 or the boot budget elapses.
func waitHealthy(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = probeHealthz(client, baseURL)
		if lastErr == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("serve mode never became healthy at %s: %v", baseURL, lastErr)
}

// probeHealthz performs one /healthz GET with the caller's context.
func probeHealthz(client *http.Client, baseURL string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("build healthz request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz status %d", resp.StatusCode)
	}
	return nil
}

// listTools performs an authenticated tools/list round trip over /mcp and
// returns the declared tool schemas.
func listTools(t *testing.T, baseURL, callerID string) []toolSchema {
	t.Helper()
	const body = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build tools/list request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("X-Caller-Id", callerID)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("tools/list request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read tools/list response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list status = %d; want 200; body=%s", resp.StatusCode, raw)
	}

	var envelope struct {
		Result struct {
			Tools []toolSchema `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list: %v; body=%s", err, raw)
	}
	if envelope.Error != nil {
		t.Fatalf("tools/list failed: %d %q", envelope.Error.Code, envelope.Error.Message)
	}
	return envelope.Result.Tools
}
