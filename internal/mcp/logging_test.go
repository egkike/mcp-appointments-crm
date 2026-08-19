package mcp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

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

// chainWithCaller wraps inner with a caller-injected context, simulating what
// AuthMiddleware does in production (the logging middleware sits inside it).
func chainWithCaller(inner http.Handler, caller *auth.Caller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if caller != nil {
			r = r.WithContext(auth.WithCaller(r.Context(), *caller))
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
	chain := chainWithCaller(loggingMiddleware(logger, inner), &auth.Caller{ID: "owner-1", Role: auth.RoleOwner})

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
	// No caller in the context: the request never carried an identity.
	chain := chainWithCaller(loggingMiddleware(logger, inner), nil)

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
