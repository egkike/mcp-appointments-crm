package mcp

import (
	"encoding/json"
	"net/http"
)

// Healthz returns a liveness-only handler (REQ-MT-014): HTTP 200 with
// {"status":"ok","version":"<version>"}. It deliberately does not probe
// SQLite — a DB failure surfaces as -32603 on the next tools/call, which is
// the client-visible signal (liveness-only semantics, see spec).
func Healthz(version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// A failed encode only matters if the client is gone; nothing to do.
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
	})
}
