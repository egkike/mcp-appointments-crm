package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// recordingHandler captures what the authenticated chain saw and answers a
// canned JSON-RPC 200 (POST) or 405 (any other method).
type recordingHandler struct {
	path   string
	caller *auth.Caller
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.path = r.URL.Path
	if c, ok := auth.FromContext(r.Context()); ok {
		h.caller = &c
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"jsonrpc":"2.0","result":{"ok":true},"id":7}`)
}

// buildAuthChain assembles the exact production composition under test:
// jsonrpcAuthTranslator(authMW.Wrap(inner)) with a sqlmock-backed resolver.
func buildAuthChain(t *testing.T, rbac auth.ToolRBAC, inner http.Handler) (http.Handler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw := auth.NewAuthMiddleware(auth.NewCallerResolver(db), rbac, logger)
	return jsonrpcAuthTranslator(mw.Wrap(inner)), mock
}

func postJSON(body string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// The SDK Streamable HTTP handler rejects POST without both media types
	// in Accept (HTTP 400); mirrors a real MCP client.
	req.Header.Set("Accept", "application/json, text/event-stream")
	return req
}

func doChain(t *testing.T, chain http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)
	return rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) (code int64, message string, id json.RawMessage) {
	t.Helper()
	var env struct {
		Error struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not a JSON-RPC envelope: %v; body=%q", err, rec.Body.String())
	}
	return env.Error.Code, env.Error.Message, env.ID
}

// ── 401: missing X-Caller-Id ──

func TestAuthTranslator401MissingHeader(t *testing.T) {
	chain, _ := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})

	rec := doChain(t, chain, postJSON(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"create_booking","arguments":{}}}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (auth errors become JSON-RPC envelopes)", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
	code, msg, id := decodeEnvelope(t, rec)
	if code != -32000 {
		t.Errorf("error.code = %d; want -32000", code)
	}
	if msg != "no se proporcionó X-Caller-Id" {
		t.Errorf("error.message = %q; want %q", msg, "no se proporcionó X-Caller-Id")
	}
	if string(id) != "7" {
		t.Errorf("id = %s; want 7", id)
	}
}

// ── 401: unrecognized caller id ──

func TestAuthTranslator401UnknownCaller(t *testing.T) {
	chain, mock := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := postJSON(`{"jsonrpc":"2.0","id":"abc","method":"initialize","params":{}}`)
	req.Header.Set("X-Caller-Id", "ghost")
	rec := doChain(t, chain, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	code, msg, id := decodeEnvelope(t, rec)
	if code != -32000 || msg != "no se proporcionó X-Caller-Id" {
		t.Errorf("got code=%d msg=%q; want -32000 %q", code, msg, "no se proporcionó X-Caller-Id")
	}
	if string(id) != `"abc"` {
		t.Errorf("id = %s; want \"abc\"", id)
	}
}

// ── 403: authenticated but role not allowed ──

func TestAuthTranslator403RBACDenied(t *testing.T) {
	rbac := auth.ToolRBAC{"create_booking": {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff}}
	chain, mock := buildAuthChain(t, rbac, &recordingHandler{})
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
		WithArgs("client-1").
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
		WithArgs("client-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("client-1"))

	req := postJSON(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"create_booking","arguments":{}}}`)
	req.Header.Set("X-Caller-Id", "client-1")
	rec := doChain(t, chain, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	code, msg, id := decodeEnvelope(t, rec)
	if code != -32001 {
		t.Errorf("error.code = %d; want -32001", code)
	}
	if msg != "no tienes permiso para realizar esta acción" {
		t.Errorf("error.message = %q; want %q", msg, "no tienes permiso para realizar esta acción")
	}
	if string(id) != "9" {
		t.Errorf("id = %s; want 9", id)
	}
}

// ── 500: resolver internal failure ──

func TestAuthTranslator500ResolverFailure(t *testing.T) {
	chain, mock := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
		WithArgs("boom").
		WillReturnError(errors.New("boom"))

	req := postJSON(`{"jsonrpc":"2.0","id":11,"method":"initialize","params":{}}`)
	req.Header.Set("X-Caller-Id", "boom")
	rec := doChain(t, chain, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	code, msg, id := decodeEnvelope(t, rec)
	if code != -32603 {
		t.Errorf("error.code = %d; want -32603", code)
	}
	if msg != "error interno del servidor" {
		t.Errorf("error.message = %q; want %q", msg, "error interno del servidor")
	}
	if string(id) != "11" {
		t.Errorf("id = %s; want 11", id)
	}
}

// ── 200 passthrough + path rewrite + caller injection ──

func TestAuthTranslator200PassthroughRewritesToolPath(t *testing.T) {
	inner := &recordingHandler{}
	rbac := auth.ToolRBAC{"create_booking": {auth.RoleOwner}}
	chain, mock := buildAuthChain(t, rbac, inner)
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
			AddRow("owner", nil, 1))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := postJSON(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"create_booking","arguments":{}}}`)
	req.Header.Set("X-Caller-Id", "owner-1")
	rec := doChain(t, chain, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got, want := rec.Body.String(), `{"jsonrpc":"2.0","result":{"ok":true},"id":7}`; got != want {
		t.Errorf("body = %s; want %s (transparent passthrough)", got, want)
	}
	if inner.path != "create_booking" {
		t.Errorf("inner saw path %q; want %q (RBAC keyed on tool name)", inner.path, "create_booking")
	}
	if inner.caller == nil || inner.caller.Role != auth.RoleOwner || inner.caller.ID != "owner-1" {
		t.Errorf("inner caller = %+v; want owner-1/owner", inner.caller)
	}
}

// ── non-tools/call methods leave the path untouched ──

func TestAuthTranslatorNonToolMethodKeepsPath(t *testing.T) {
	inner := &recordingHandler{}
	chain, mock := buildAuthChain(t, auth.ToolRBAC{}, inner)
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
			AddRow("owner", nil, 1))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := postJSON(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	req.Header.Set("X-Caller-Id", "owner-1")
	if rec := doChain(t, chain, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if inner.path != "/mcp" {
		t.Errorf("inner saw path %q; want /mcp", inner.path)
	}
}

// ── invalid JSON: id falls back to null, body still forwarded ──

func TestAuthTranslatorInvalidJSONNullID(t *testing.T) {
	chain, _ := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})

	rec := doChain(t, chain, postJSON(`{"broken`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	code, _, id := decodeEnvelope(t, rec)
	if code != -32000 {
		t.Errorf("error.code = %d; want -32000", code)
	}
	if string(id) != "null" {
		t.Errorf("id = %s; want null", id)
	}
}

// ── notification without id: envelope id is null ──

func TestAuthTranslatorNoIDNull(t *testing.T) {
	chain, _ := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})

	rec := doChain(t, chain, postJSON(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))

	code, _, id := decodeEnvelope(t, rec)
	if code != -32000 {
		t.Errorf("error.code = %d; want -32000", code)
	}
	if string(id) != "null" {
		t.Errorf("id = %s; want null", id)
	}
}

// ── non-401/403/500 statuses pass through untouched ──

func TestAuthTranslator405Passthrough(t *testing.T) {
	inner := &recordingHandler{}
	chain, mock := buildAuthChain(t, auth.ToolRBAC{}, inner)
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
			AddRow("owner", nil, 1))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/mcp", nil)
	req.Header.Set("X-Caller-Id", "owner-1")
	rec := doChain(t, chain, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d; want 405 passthrough", rec.Code)
	}
}

// ── Server.AuthHandler composition with the real SDK handler ──

func TestAuthHandlerComposition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(Config{Version: "test", Logger: logger})
	rbac := auth.ToolRBAC{"create_booking": {auth.RoleOwner}}
	mw := auth.NewAuthMiddleware(auth.NewCallerResolver(db), rbac, logger)
	chain := srv.AuthHandler(mw)

	t.Run("missing header maps to auth envelope", func(t *testing.T) {
		rec := doChain(t, chain, postJSON(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"create_booking","arguments":{}}}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
		code, msg, id := decodeEnvelope(t, rec)
		if code != -32000 || msg != "no se proporcionó X-Caller-Id" {
			t.Errorf("got code=%d msg=%q; want -32000 %q", code, msg, "no se proporcionó X-Caller-Id")
		}
		if string(id) != "5" {
			t.Errorf("id = %s; want 5", id)
		}
	})

	t.Run("authorized call reaches the SDK and returns its error", func(t *testing.T) {
		mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
			WithArgs("owner-1").
			WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
				AddRow("owner", nil, 1))
		mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
			WithArgs("owner-1").
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		req := postJSON(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"create_booking","arguments":{}}}`)
		req.Header.Set("X-Caller-Id", "owner-1")
		rec := doChain(t, chain, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
		code, _, _ := decodeEnvelope(t, rec)
		// PR 1 ships zero tools, so the SDK answers "unknown tool" with
		// -32602; proving the SDK handler runs behind translator + auth.
		if code != -32602 {
			t.Errorf("error.code = %d; want -32602 (SDK unknown tool passthrough)", code)
		}
	})
}

func TestAuthTranslatorUnauthenticatedGETMapsToEnvelope(t *testing.T) {
	chain, _ := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})

	rec := doChain(t, chain, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/mcp", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	code, _, _ := decodeEnvelope(t, rec)
	if code != -32000 {
		t.Errorf("error.code = %d; want -32000", code)
	}
}

// ── body read failure short-circuits with 400 (GGA S-3) ──

type failingBodyReader struct{}

func (failingBodyReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestAuthTranslatorReadError400(t *testing.T) {
	chain, _ := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})

	req := postJSON(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	req.Body = io.NopCloser(failingBodyReader{})
	rec := doChain(t, chain, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

// ── invalid JSON-RPC id shapes are normalized to null (GGA S-2) ──

func TestAuthTranslatorObjectIDNormalizedToNull(t *testing.T) {
	chain, _ := buildAuthChain(t, auth.ToolRBAC{}, &recordingHandler{})

	rec := doChain(t, chain, postJSON(`{"jsonrpc":"2.0","id":{"a":1},"method":"initialize","params":{}}`))

	code, _, id := decodeEnvelope(t, rec)
	if code != -32000 {
		t.Errorf("error.code = %d; want -32000", code)
	}
	if string(id) != "null" {
		t.Errorf("id = %s; want null (JSON-RPC ids are string|number|null)", id)
	}
}

// ── hostile tool names never reach the RBAC path key (GGA S-1) ──

func TestAuthTranslatorHostileToolNameNotRewritten(t *testing.T) {
	inner := &recordingHandler{}
	rbac := auth.ToolRBAC{"create_booking": {auth.RoleOwner}}
	chain, mock := buildAuthChain(t, rbac, inner)
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
			AddRow("owner", nil, 1))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := postJSON(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"../create_booking","arguments":{}}}`)
	req.Header.Set("X-Caller-Id", "owner-1")
	if rec := doChain(t, chain, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if inner.path != "/mcp" {
		t.Errorf("inner saw path %q; want /mcp (hostile name must not reach the RBAC key)", inner.path)
	}
}
