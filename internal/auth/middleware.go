package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// ToolRBAC maps a tool/route path to the set of roles allowed to access it.
// A nil or empty slice means "any authenticated caller".
type ToolRBAC map[string][]string

// AuthMiddleware wraps an http.Handler with authentication and authorization.
type AuthMiddleware struct {
	resolver *CallerResolver
	rbac     ToolRBAC
	logger   *slog.Logger
}

// NewAuthMiddleware creates a middleware with the given resolver, RBAC config, and logger.
// A nil logger falls back to slog.Default() so the audit log on the privileged
// path (admin/owner) never nil-derefs.
func NewAuthMiddleware(resolver *CallerResolver, rbac ToolRBAC, logger *slog.Logger) *AuthMiddleware {
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
			if errors.Is(err, ErrUnauthenticated) {
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

		// Step 4: audit log for privileged callers
		if caller.Role == RoleAdmin || caller.Role == RoleOwner {
			m.logger.Info("privileged access",
				"caller_id", caller.ID,
				"tool", tool,
			)
		}

		// Step 5: inject caller into context and call next
		ctx := WithCaller(r.Context(), caller)
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
