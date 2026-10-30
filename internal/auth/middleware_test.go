package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// mwTestHandler is a slog.Handler that captures records for inspection.
type mwTestHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *mwTestHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *mwTestHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *mwTestHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *mwTestHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *mwTestHandler) recordsByMsg(msg string) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Message == msg {
			out = append(out, r)
		}
	}
	return out
}

// newMiddlewareWithMock sets up a CallerResolver backed by go-sqlmock and an AuthMiddleware.
func newMiddlewareWithMock(t *testing.T, rbac ToolRBAC, logger *slog.Logger) (*AuthMiddleware, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	resolver := NewCallerResolver(db)
	mw := NewAuthMiddleware(resolver, rbac, logger)
	return mw, mock
}

// expectAdminResolved sets up mock expectations for an active admin with no client row.
func expectAdminResolved(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
			AddRow("admin", nil, 1))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

// expectClientResolved sets up mock expectations for a client-only caller.
func expectClientResolved(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
}

// expectStaffResolved sets up mock expectations for an active staff with professional_id.
func expectStaffResolved(mock sqlmock.Sqlmock, id, profID string) {
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
			AddRow("staff", profID, 1))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

// expectDisabledAccount sets up mock expectations for a disabled account.
func expectDisabledAccount(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}).
			AddRow("admin", nil, 0))
}

// expectUnknownCaller sets up mock expectations for an unknown caller.
func expectUnknownCaller(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"role", "professional_id", "is_active"}))
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

// okHandler is a downstream handler that records it was called.
func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// roleRecorder wraps httptest.ResponseRecorder and captures the annotated
// caller role (CallerRoleRecorder), mirroring the statusRecorder chain the
// MCP transport builds around AuthMiddleware.
type roleRecorder struct {
	*httptest.ResponseRecorder
	recorded string
}

func (r *roleRecorder) RecordCallerRole(role string) { r.recorded = role }

// ctxHandler captures the caller from context for verification.
func ctxHandler(t *testing.T, wantID string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := FromContext(r.Context())
		if !ok {
			t.Error("downstream handler: no caller in context")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if caller.ID != wantID {
			t.Errorf("downstream handler: caller ID = %q; want %q", caller.ID, wantID)
		}
		w.WriteHeader(http.StatusOK)
	}
}

func TestMiddleware_HeaderPresent(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100000000"
	expectAdminResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	mw.Wrap(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMiddleware_HeaderAbsent(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, _ := newMiddlewareWithMock(t, nil, logger)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	// No X-Caller-Id header
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "no se proporcionó X-Caller-Id") {
		t.Errorf("body = %q; want to contain %q", rec.Body.String(), "no se proporcionó X-Caller-Id")
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

func TestMiddleware_HeaderEmpty(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, _ := newMiddlewareWithMock(t, nil, logger)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", "   ")
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "no se proporcionó X-Caller-Id") {
		t.Errorf("body = %q; want to contain %q", rec.Body.String(), "no se proporcionó X-Caller-Id")
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

func TestMiddleware_HeaderCaseInsensitive(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100000000"
	expectAdminResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("x-caller-id", id) // lowercase
	rec := httptest.NewRecorder()

	mw.Wrap(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMiddleware_AdminResolved(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100000000"
	expectAdminResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	mw.Wrap(ctxHandler(t, id)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_StaffResolved(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100002222"
	profID := "p-001"
	expectStaffResolved(mock, id, profID)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	mw.Wrap(ctxHandler(t, id)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_DisabledAccount(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100000000"
	expectDisabledAccount(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "deshabilitada") {
		t.Errorf("body = %q; want to contain 'deshabilitada'", rec.Body.String())
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

func TestMiddleware_ClientResolved(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100003333"
	expectClientResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	mw.Wrap(ctxHandler(t, id)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_UnknownCaller(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100099999"
	expectUnknownCaller(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "reconozco") {
		t.Errorf("body = %q; want to contain 'reconozco'", rec.Body.String())
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

func TestMiddleware_CallerInDownstreamCtx(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100000000"
	expectAdminResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	var gotCaller Caller
	var gotOK bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCaller, gotOK = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if !gotOK {
		t.Fatal("downstream handler: no caller in context")
	}
	if gotCaller.ID != id {
		t.Errorf("caller ID = %q; want %q", gotCaller.ID, id)
	}
	if gotCaller.Role != RoleAdmin {
		t.Errorf("caller Role = %q; want %q", gotCaller.Role, RoleAdmin)
	}
}

func TestMiddleware_RBAC_AdminAllowed(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	rbac := ToolRBAC{"/tools/admin-only": {RoleAdmin}}
	mw, mock := newMiddlewareWithMock(t, rbac, logger)

	id := "+5491100000000"
	expectAdminResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/admin-only", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("downstream handler should have been called for admin")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_RBAC_ClientDenied(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	rbac := ToolRBAC{"/tools/admin-only": {RoleAdmin}}
	mw, mock := newMiddlewareWithMock(t, rbac, logger)

	id := "+5491100003333"
	expectClientResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/admin-only", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
	if !strings.Contains(rec.Body.String(), "no tienes permiso") {
		t.Errorf("body = %q; want to contain 'no tienes permiso'", rec.Body.String())
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

// TestMiddleware_RBAC_DeniedStillRecordsCallerRole covers the S-1 follow-up:
// a 403 RBAC denial must STILL annotate the RESOLVED caller's role on the
// recorder — the annotation happens right after resolve, BEFORE the RBAC gate
// — so the request log shows the real role instead of "none".
func TestMiddleware_RBAC_DeniedStillRecordsCallerRole(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	rbac := ToolRBAC{"/tools/admin-only": {RoleAdmin}}
	mw, mock := newMiddlewareWithMock(t, rbac, logger)

	id := "+5491100003333"
	expectClientResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/admin-only", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := &roleRecorder{ResponseRecorder: httptest.NewRecorder()}

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
	if rec.recorded != RoleClient {
		t.Errorf("RecordCallerRole called with %q; want %q (resolved role)", rec.recorded, RoleClient)
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

func TestMiddleware_RBAC_StaffAllowed(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	rbac := ToolRBAC{"/tools/staff-tool": {RoleStaff, RoleAdmin}}
	mw, mock := newMiddlewareWithMock(t, rbac, logger)

	id := "+5491100002222"
	expectStaffResolved(mock, id, "p-001")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/staff-tool", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("downstream handler should have been called for staff")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_NoRBAC_AnyCallerAllowed(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	// No RBAC — nil map means any authenticated caller is fine
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100003333"
	expectClientResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/any", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("downstream handler should have been called (no RBAC)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_AuditLog_Admin(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)
	// Pin the keyed digest (ADR-0021) instead of relying on the process-wide
	// one: the assertion below is a fixed HMAC vector, not a recomputation of
	// whatever the middleware happened to use.
	mw.digester = testAuditDigester(t)

	id := testAuditCallerID
	expectAdminResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/admin-tool", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	mw.Wrap(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}

	// Check audit log was emitted
	recs := h.recordsByMsg("privileged access")
	if len(recs) == 0 {
		t.Fatal("expected audit log 'privileged access'; got none")
	}

	// Verify attributes
	var attrs map[string]any
	recs[0].Attrs(func(a slog.Attr) bool {
		if attrs == nil {
			attrs = make(map[string]any)
		}
		attrs[a.Key] = a.Value.Any()
		return true
	})
	if attrs["caller_hash"] != testAuditDigestPinned {
		t.Errorf("audit caller_hash = %v; want the pinned keyed digest %q", attrs["caller_hash"], testAuditDigestPinned)
	}
	// The audit hash is the FULL 256-bit digest (64 hex chars), never a
	// truncated prefix (GGA WARNING-1).
	if got := attrs["caller_hash"].(string); len(got) != 64 {
		t.Errorf("audit caller_hash length = %d; want 64 (full HMAC-SHA-256 digest)", len(got))
	}
	if attrs["tool"] != "/tools/admin-tool" {
		t.Errorf("audit tool = %v; want %q", attrs["tool"], "/tools/admin-tool")
	}
	// Spec mandates 'timestamp ISO 8601 UTC' in the audit record.
	// slog's default handler adds the time to the record itself.
	if recs[0].Time.IsZero() {
		t.Error("audit log: time should be set (slog default adds ISO 8601 time); got zero value")
	}
}

// TestMiddleware_AuditLog_DigestUnconfiguredFailsClosed pins the ADR-0021
// fail-closed seam on BOTH shapes of "not configured": an explicitly absent
// digester and the production default digester with nothing installed at boot.
// In both cases the audit line is still emitted, but it carries NO caller_hash —
// never an empty placeholder and never the enumerable unkeyed SHA-256 — and the
// degradation is logged so the wiring bug cannot pass unnoticed.
//
// The test is sequential on purpose: it clears the process-wide digest, and the
// package's parallel tests resume only after every sequential test (cleanup
// included) has finished.
func TestMiddleware_AuditLog_DigestUnconfiguredFailsClosed(t *testing.T) {
	seams := map[string]func(t *testing.T, mw *AuthMiddleware){
		"nil digester": func(_ *testing.T, mw *AuthMiddleware) {
			mw.digester = nil
		},
		"default digester, nothing configured at boot": func(t *testing.T, _ *AuthMiddleware) {
			preserveAuditDigest(t)
			clearAuditDigest()
		},
	}

	for name, breakSeam := range seams {
		t.Run(name, func(t *testing.T) {
			h := &mwTestHandler{}
			logger := slog.New(h)
			mw, mock := newMiddlewareWithMock(t, nil, logger)
			breakSeam(t, mw)

			id := testAuditCallerID
			expectAdminResolved(mock, id)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/admin-tool", nil)
			req.Header.Set("X-Caller-Id", id)
			rec := httptest.NewRecorder()

			mw.Wrap(okHandler()).ServeHTTP(rec, req)

			recs := h.recordsByMsg("privileged access")
			if len(recs) == 0 {
				t.Fatal("expected the 'privileged access' line even without a digest; got none")
			}
			recs[0].Attrs(func(a slog.Attr) bool {
				if a.Key == "caller_hash" {
					t.Errorf("audit line carries caller_hash = %v without a configured digest; must omit it", a.Value.Any())
				}
				return true
			})

			unkeyed := sha256.Sum256([]byte(id))
			if got := hex.EncodeToString(unkeyed[:]); strings.Contains(recordsText(recs), got) {
				t.Error("audit line leaked the retired unkeyed SHA-256 digest")
			}

			degraded := h.recordsByMsg("auth: audit digest unconfigured, caller_hash omitted")
			if len(degraded) != 1 {
				t.Fatalf("digest-degradation records = %d; want exactly 1 (fail closed must be visible)", len(degraded))
			}
		})
	}
}

// recordsText renders the attribute values of the given records, so an
// assertion can search for a value without depending on slog's text format.
func recordsText(recs []slog.Record) string {
	var sb strings.Builder
	for _, r := range recs {
		r.Attrs(func(a slog.Attr) bool {
			sb.WriteString(a.Value.String())
			sb.WriteString(" ")
			return true
		})
	}
	return sb.String()
}

// TestMiddleware_ResolverDBError_Returns500 covers F-REL-1: when the resolver
// returns a non-ErrUnauthenticated error (e.g. DB connection lost), the
// middleware MUST respond 500 — not 401, not 200.
func TestMiddleware_ResolverDBError_Returns500(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100000000"
	mock.ExpectQuery("SELECT role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnError(errors.New("connection refused"))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw.Wrap(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusInternalServerError)
	}
	if called {
		t.Error("downstream handler should NOT have been called on resolver DB error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestMiddleware_HeaderTrimsValidID covers F-REL-4: a valid id surrounded by
// whitespace (e.g. "  +5491100000000  ") MUST be trimmed before resolution
// (spec scenario 'Header presente con valor no vacío' requires 'sin espacios
// al inicio/final').
func TestMiddleware_HeaderTrimsValidID(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100000000"
	expectAdminResolved(mock, id)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/test", nil)
	req.Header.Set("X-Caller-Id", "  "+id+"  ")
	rec := httptest.NewRecorder()

	mw.Wrap(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMiddleware_NoAuditLog_Staff(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	id := "+5491100002222"
	expectStaffResolved(mock, id, "p-001")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tools/staff-tool", nil)
	req.Header.Set("X-Caller-Id", id)
	rec := httptest.NewRecorder()

	mw.Wrap(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}

	// Staff should NOT trigger audit log
	recs := h.recordsByMsg("privileged access")
	if len(recs) != 0 {
		t.Errorf("staff should not trigger audit log; got %d records", len(recs))
	}
}

// ── fail-fast wiring guards (GGA WARNING-2) ──

func TestNewAuthMiddlewarePanicsOnNilResolver(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewAuthMiddleware(nil, ...) did not panic; must fail fast at wiring time, not per-request")
		}
	}()
	NewAuthMiddleware(nil, ToolRBAC{}, slog.Default())
}

// TestMiddlewareAuditAttrs_TypeIsTypedSlogAttr pins the typed-attrs contract
// for the middleware seam too (GGA round 3): the audit attr builder returns
// []slog.Attr rather than raw key-value []any.
func TestMiddlewareAuditAttrs_TypeIsTypedSlogAttr(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw, _ := newMiddlewareWithMock(t, nil, logger)
	attrs := mw.auditAttrs([]slog.Attr{}, "x", "tool")
	if got := reflect.TypeOf(attrs); got != reflect.TypeOf([]slog.Attr{}) {
		t.Fatalf("middleware.auditAttrs type = %T, want []slog.Attr", attrs)
	}
}
