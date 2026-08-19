package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// loggingMiddleware emits exactly one structured log line per /mcp request
// (REQ-MT-011). It composes OUTSIDE jsonrpcAuthTranslator (JD fix B-2) so
// auth-rejected requests still produce their log line: for 401/403/500 the
// translator reports the inner chain's REAL status through statusRecorder
// before re-emitting the failure as a 200 envelope, and the recorder keeps
// that real status for the log while streaming the envelope to the client.
// For passthrough statuses the recorder observes the final status directly.
//
//   - request_id: 32-hex-char random identifier (crypto/rand; the design's
//     google/uuid v1.6.0 would add a dependency for a single field — stdlib
//     keeps go.mod untouched, deviation documented);
//   - method, path: the path is observed AFTER the JSON-RPC auth translator
//     rewrote it to the tool name for tools/call — the RBAC key;
//   - status: the REAL status the auth chain decided (401/403/500 reported by
//     the translator) or the final passthrough status (200/405/413/400);
//   - duration_ms: handler wall time;
//   - caller_role: the resolved caller's role, "none" when the request
//     carried no identity (auth failures included).
func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{w: w}
		next.ServeHTTP(rec, r)
		role := "none"
		if c, ok := auth.FromContext(r.Context()); ok {
			role = string(c.Role)
		}
		logger.Info("mcp request",
			"request_id", newRequestID(),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"caller_role", role,
		)
	})
}

// newRequestID returns a 32-hex-char random identifier. A failed rand.Read
// falls back to a zero id rather than failing the request: a log field must
// never break the transport (the odds of a CSPRNG failure are negligible).
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b[:])
}
