package mcp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// captureHandler collects slog records for assertions (REQ-MT-011).
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

// recordAttrs flattens a record's attributes into a map for assertions.
func recordAttrs(r slog.Record) map[string]any {
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	return attrs
}

// mcpRequestRecords returns only the logging middleware's own lines
// (REQ-MT-011): the SDK server writes its activity logs to the same logger,
// so captures are filtered by the middleware's message before asserting.
func mcpRequestRecords(h *captureHandler) []slog.Record {
	var out []slog.Record
	for _, r := range h.records {
		if r.Message == "mcp request" {
			out = append(out, r)
		}
	}
	return out
}

// chainWithCallerRole wraps inner with a recorder annotation, mirroring what
// AuthMiddleware does in production: the caller role is annotated on the
// response recorder, because the caller is injected on a request COPY that
// never propagates back to the outer logging middleware (JD fix B-2
// regression fix).
func chainWithCallerRole(inner http.Handler, caller *auth.Caller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if caller != nil {
			if rr, ok := w.(auth.CallerRoleRecorder); ok {
				rr.RecordCallerRole(caller.Role)
			}
		}
		inner.ServeHTTP(w, r)
	})
}

func TestLoggingMiddlewareEmitsOneLinePerRequest(t *testing.T) {
	cap := &captureHandler{}
	logger := slog.New(cap)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	chain := loggingMiddleware(logger, chainWithCallerRole(inner, &auth.Caller{ID: "owner-1", Role: auth.RoleOwner}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/create_booking", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)
	}

	if len(cap.records) != 2 {
		t.Fatalf("records = %d; want exactly one log line per request (2)", len(cap.records))
	}
	attrs := recordAttrs(cap.records[0])

	if attrs["method"] != http.MethodPost {
		t.Errorf("method = %v; want POST", attrs["method"])
	}
	// The translator rewrites the path to the tool name before auth runs, so
	// the middleware observes the post-rewrite path (the RBAC key).
	if attrs["path"] != "/create_booking" {
		t.Errorf("path = %v; want /create_booking (post-rewrite RBAC key)", attrs["path"])
	}
	if attrs["status"] != int64(http.StatusOK) {
		t.Errorf("status = %v; want 200", attrs["status"])
	}
	if attrs["caller_role"] != "owner" {
		t.Errorf("caller_role = %v; want owner", attrs["caller_role"])
	}
	if dur, ok := attrs["duration_ms"].(int64); !ok || dur < 0 {
		t.Errorf("duration_ms = %v (%T); want non-negative int64", attrs["duration_ms"], attrs["duration_ms"])
	}
	rid, ok := attrs["request_id"].(string)
	if !ok {
		t.Fatalf("request_id missing or not a string: %v", attrs["request_id"])
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(rid) {
		t.Errorf("request_id = %q; want 32 lowercase hex chars", rid)
	}
}

func TestLoggingMiddlewareCallerRoleDefaultsToNone(t *testing.T) {
	cap := &captureHandler{}
	logger := slog.New(cap)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// No caller: the request never carried an identity.
	chain := loggingMiddleware(logger, chainWithCallerRole(inner, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
	chain.ServeHTTP(httptest.NewRecorder(), req)

	if len(cap.records) != 1 {
		t.Fatalf("records = %d; want 1", len(cap.records))
	}
	if role := recordAttrs(cap.records[0])["caller_role"]; role != "none" {
		t.Errorf("caller_role = %v; want none", role)
	}
}

// ── auth-rejected requests are logged with the REAL status (JD fix B-2) ──

func TestLoggingMiddlewareAuthDeniedLogsRealStatus(t *testing.T) {
	cap := &captureHandler{}
	logger := slog.New(cap)
	// Production composition (JD fix B-2): loggingMiddleware OUTSIDE
	// jsonrpcAuthTranslator. The inner chain rejects the request (missing
	// X-Caller-Id → 401) and the translator re-emits it as a 200 JSON-RPC
	// envelope; the log line must carry the REAL auth status (401), not the
	// envelope's 200, and caller_role must be "none".
	authReject := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no se proporcionó X-Caller-Id", http.StatusUnauthorized)
	})
	chain := loggingMiddleware(logger, jsonrpcAuthTranslator(authReject))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	// The client still receives the translated envelope (HTTP 200).
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (envelope)", rec.Code)
	}
	if len(cap.records) != 1 {
		t.Fatalf("records = %d; want 1", len(cap.records))
	}
	attrs := recordAttrs(cap.records[0])
	if attrs["status"] != int64(http.StatusUnauthorized) {
		t.Errorf("status = %v; want 401 (REAL auth status, JD fix B-2)", attrs["status"])
	}
	if attrs["caller_role"] != "none" {
		t.Errorf("caller_role = %v; want none", attrs["caller_role"])
	}
}

// ── JD fix B-2 regression: the FULL AuthHandler composition logs the REAL caller role ──

// TestAuthHandlerLogsCallerRole exercises the production AuthHandler
// composition end to end — loggingMiddleware(methodGate(jsonrpcAuthTranslator(
// authMW.Wrap(...)))) with a sqlmock-backed resolver — and asserts the logged
// line carries the resolved caller's role (REQ-MT-011). Regression from JD fix
// B-2: the logging middleware now sits OUTSIDE the auth chain, but
// authMW.Wrap injects the caller on a request COPY that flows down and never
// propagates back, so reading auth.FromContext(r.Context()) in the middleware
// yielded "none" for every authenticated request. The role must reach the log
// through the recorder chain instead of the request context.
func TestAuthHandlerLogsCallerRole(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cap := &captureHandler{}
	logger := slog.New(cap)
	srv := NewServer(Config{Version: "test", Logger: logger})
	mw := auth.NewAuthMiddleware(auth.NewCallerResolver(db), auth.ToolRBAC{}, logger)
	chain := srv.AuthHandler(mw)

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

	t.Run("authenticated request logs the resolved role", func(t *testing.T) {
		mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = \\?").
			WithArgs("owner-1").
			WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
				AddRow("owner", nil, 1))
		mock.ExpectQuery("SELECT id FROM clients WHERE id = \\?").
			WithArgs("owner-1").
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		req := postJSON(initialize)
		req.Header.Set("X-Caller-Id", "owner-1")
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
		lines := mcpRequestRecords(cap)
		if len(lines) != 1 {
			t.Fatalf("mcp request lines = %d; want 1", len(lines))
		}
		attrs := recordAttrs(lines[0])
		if attrs["caller_role"] != auth.RoleOwner {
			t.Errorf("caller_role = %v; want %q (REQ-MT-011)", attrs["caller_role"], auth.RoleOwner)
		}
		if attrs["status"] != int64(http.StatusOK) {
			t.Errorf("status = %v; want 200", attrs["status"])
		}
	})

	t.Run("denied request logs caller_role=none and the real status", func(t *testing.T) {
		// POST without X-Caller-Id: the client receives the 200 JSON-RPC
		// envelope, but the log must carry the REAL auth status (401) and
		// caller_role=none.
		req := postJSON(initialize)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200 (envelope)", rec.Code)
		}
		lines := mcpRequestRecords(cap)
		if len(lines) != 2 {
			t.Fatalf("mcp request lines = %d; want 2", len(lines))
		}
		attrs := recordAttrs(lines[1])
		if attrs["caller_role"] != "none" {
			t.Errorf("caller_role = %v; want none", attrs["caller_role"])
		}
		if attrs["status"] != int64(http.StatusUnauthorized) {
			t.Errorf("status = %v; want 401 (REAL auth status)", attrs["status"])
		}
	})
}
