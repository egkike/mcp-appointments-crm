package auth

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// ToolRBAC maps a tool/route path to the set of roles allowed to access it.
// A nil or empty slice means "any authenticated caller".
type ToolRBAC map[string][]string

// CallerRoleRecorder is implemented by response writers that want the resolved
// caller's role for the request log (REQ-MT-011). AuthMiddleware resolves the
// caller here, on the request that flows DOWN through the recorder, while the
// outer logging middleware reads the annotated role from the recorder: the
// caller is injected on a request COPY that never propagates back up, so the
// request context cannot carry it to the log line.
type CallerRoleRecorder interface {
	RecordCallerRole(role string)
}

// AuthMiddleware wraps an http.Handler with authentication and authorization.
type AuthMiddleware struct {
	resolver *CallerResolver
	rbac     ToolRBAC
	logger   *slog.Logger
}

// NewAuthMiddleware creates a middleware with the given resolver, RBAC config, and logger.
// A nil logger falls back to slog.Default() so the audit log on the privileged
// path (admin/owner) never nil-derefs. A nil resolver panics at wiring time
// (fail fast): a per-request nil deref in the middle of the auth chain would
// kill the connection instead of producing the controlled 401/500 (GGA
// WARNING-2).
func NewAuthMiddleware(resolver *CallerResolver, rbac ToolRBAC, logger *slog.Logger) *AuthMiddleware {
	if resolver == nil {
		panic("auth: NewAuthMiddleware requires a non-nil CallerResolver")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthMiddleware{
		resolver: resolver,
		rbac:     rbac,
		logger:   logger,
	}
}

// Wrap returns an http.Handler that authenticates and authorizes each request.
//
// Flow:
//  1. Read X-Caller-Id (case-insensitive per RFC 7230).
//  2. If missing/empty → 401.
//  3. Resolve caller via CallerResolver; if ErrUnauthenticated → 401.
//  4. RBAC check (BEFORE next.ServeHTTP): if tool requires roles and caller lacks them → 403.
//  5. If caller.Role is admin or owner → emit audit log.
//  6. Inject caller into context and call next.ServeHTTP.
//
// RBAC precondition: authorization keys on r.URL.Path, which MUST already
// carry the tool name when this middleware guards the /mcp route — the outer
// jsonrpcAuthTranslator rewrites the path for tools/call requests BEFORE the
// middleware runs (REQ-AM-WIRED-002). Mounting Wrap standalone, or on a route
// whose path is not a tool name, keys authorization on the literal route: the
// request then matches (or misses) the RBAC entry for that exact path.
func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Step 1: read X-Caller-Id
		id := strings.TrimSpace(r.Header.Get("X-Caller-Id"))
		if id == "" {
			http.Error(w, "no se proporcionó X-Caller-Id", http.StatusUnauthorized)
			return
		}

		// Step 2: resolve caller
		caller, err := m.resolver.Resolve(r.Context(), id)
		if err != nil {
			if errors.Is(err, domain.ErrUnauthenticated) {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			// Unexpected error (DB failure, etc.) — 500
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}

		// Step 3: RBAC check BEFORE calling next
		tool := r.URL.Path
		if roles, ok := m.rbac[tool]; ok && len(roles) > 0 {
			if !roleAllowed(caller.Role, roles) {
				http.Error(w, "no tienes permiso para realizar esta acción", http.StatusForbidden)
				return
			}
		}

		// Step 4: audit log for privileged callers (hashed ID — no PII in logs)
		if caller.Role == RoleAdmin || caller.Role == RoleOwner {
			m.logger.Info("privileged access",
				"caller_hash", hashCallerID(caller.ID),
				"tool", tool,
			)
		}

		// Step 5: inject caller into context and call next
		ctx := WithCaller(r.Context(), caller)
		// Annotate the caller role on the recorder for the request log
		// (REQ-MT-011): the injected context lives on a request COPY that
		// never propagates back to the outer logging middleware, so the role
		// travels through the recorder chain instead (JD fix B-2 regression).
		if rr, ok := w.(CallerRoleRecorder); ok {
			rr.RecordCallerRole(caller.Role)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// roleAllowed checks if the given role is in the allowed set.
func roleAllowed(role string, allowed []string) bool {
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}

// hashCallerID returns the SHA-256 digest of the caller ID for audit logging:
// a stable correlation key that keeps raw PII (phone numbers, emails) out of
// log output. The digest is one-way but NOT non-reversible: caller IDs are
// low-entropy (phone numbers, emails — a ~10^10–10^11 value space), so an
// offline enumeration over the ID space can recover them from any unkeyed
// digest. Acceptable for this loopback, single-tenant deployment whose logs
// never leave the host; a keyed HMAC is required before logs are shipped
// off-host (documented deviation, defense-in-depth tradeoff).
func hashCallerID(id string) string {
	h := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%x", h[:]) // full 256-bit digest (64 hex chars)
}
