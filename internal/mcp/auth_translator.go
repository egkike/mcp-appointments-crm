package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// JSON-RPC error codes emitted by jsonrpcAuthTranslator. -32000 and -32001 are
// application-defined server errors (MCP spec reserves -32000..-32099 for
// server errors); -32603 is the standard JSON-RPC internal error.
const (
	codeAuthRequired = -32000
	codeForbidden    = -32001

	// msgAuthRequired is the fallback 401 message when AuthMiddleware wrote
	// an empty body (it normally writes "no se proporcionó X-Caller-Id" for a
	// missing header; unknown/disabled callers carry the resolver's Spanish
	// message instead — both are forwarded verbatim from the buffered body,
	// see jsonrpcAuthTranslator).
	msgAuthRequired = "no se proporcionó X-Caller-Id"
	msgForbidden    = "no tienes permiso para realizar esta acción"
	msgInternal     = "error interno del servidor"
)

// jsonrpcAuthTranslator wraps the authenticated /mcp handler and translates
// HTTP-level auth failures into JSON-RPC 2.0 error envelopes (REQ-AM-WIRED-001
// / REQ-AM-WIRED-002). The AuthMiddleware answers 401/403/500 with plain-text
// http.Error bodies; the MCP SDK client treats any non-200 status as a
// transport failure and never parses the body, so the translator re-emits
// those failures as HTTP 200 + a JSON-RPC error object with the original
// request id:
//
//	401 → -32000 with the middleware's message: "no se proporcionó
//	      X-Caller-Id" when the header is MISSING, or the resolver's Spanish
//	      message when the header is PRESENT but the caller is unknown or
//	      disabled (REQ-MT-007 "Invalid or unknown caller rejected before
//	      handler"; JD fix A-2/B-1 — the buffered body is forwarded verbatim)
//	403 → -32001 "no tienes permiso para realizar esta acción"
//	500 → -32603 "error interno del servidor"
//
// The translator is also the RBAC key bridge: AuthMiddleware authorizes by
// r.URL.Path, but every MCP tool shares the /mcp route. For tools/call
// requests the translator rewrites r.URL.Path to the tool name before calling
// the inner chain, so RBAC and the audit log see the actual tool. The body is
// buffered exactly once (same 1 MiB budget as jsonParseGuard) and restored.
func jsonrpcAuthTranslator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := json.RawMessage("null")

		if r.Method == http.MethodPost {
			body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
			if err != nil {
				// Body could not be read at all: never forward a partially
				// consumed body to the inner chain (GGA S-3).
				http.Error(w, "no se pudo leer el cuerpo de la solicitud", http.StatusBadRequest)
				return
			}
			// Always restore what was read: the inner jsonParseGuard
			// re-reads up to maxRequestBodyBytes+1 and answers 413 when
			// the budget is exceeded, so a truncated restore preserves
			// the 413 semantics. Oversized bodies keep a null id and an
			// untouched path — auth still runs first and 401/403/500 map
			// to envelopes.
			r.Body = io.NopCloser(bytes.NewReader(body))
			if len(body) <= maxRequestBodyBytes && json.Valid(body) {
				id = requestID(body)
				if name := toolCallName(body); validToolName(name) {
					r.URL.Path = name
				}
			}
		}

		rec := &statusRecorder{w: w}
		next.ServeHTTP(rec, r)

		// Forward the resolved caller's role to the outer recorder (the
		// logging middleware): AuthMiddleware annotated it on this recorder
		// where the caller was resolved, because the caller-bearing request is
		// a COPY that never propagates back up — the role travels through the
		// recorder chain instead of the request context (JD fix B-2
		// regression). Unset for auth failures (401/403/500), which log "none".
		if rec.callerRole != "" {
			if rr, ok := w.(auth.CallerRoleRecorder); ok {
				rr.RecordCallerRole(rec.callerRole)
			}
		}

		// Any status other than the three translated codes was already
		// streamed through to the client by statusRecorder (headers, status
		// and body verbatim — e.g. 200 JSON-RPC results, 405 from the SDK
		// for GET, 413/400 from jsonParseGuard).
		switch rec.status {
		case http.StatusUnauthorized:
			// Forward the middleware's own body: it distinguishes the
			// missing-header case ("no se proporcionó X-Caller-Id") from a
			// present-but-unknown/disabled caller, whose message comes from
			// the resolver (JD fix A-2/B-1). The fallback only fires for an
			// exotic 401 written without a body.
			msg := strings.TrimSpace(rec.body.String())
			if msg == "" {
				msg = msgAuthRequired
			}
			reportRealStatus(w, rec.status)
			writeJSONRPCError(w, codeAuthRequired, msg, id)
		case http.StatusForbidden:
			reportRealStatus(w, rec.status)
			writeJSONRPCError(w, codeForbidden, msgForbidden, id)
		case http.StatusInternalServerError:
			reportRealStatus(w, rec.status)
			writeJSONRPCError(w, int(jsonrpc.CodeInternalError), msgInternal, id)
		}
	})
}

// reportRealStatus forwards the inner chain's REAL status to an outer
// recorder (the logging middleware, JD fix B-2) before the translated 200
// envelope is emitted, so the request log carries the true auth decision
// (401/403/500) instead of the envelope's 200. No-op for writers that do not
// implement realStatusReporter.
func reportRealStatus(w http.ResponseWriter, status int) {
	if r, ok := w.(realStatusReporter); ok {
		r.reportRealStatus(status)
	}
}

// requestID extracts the JSON-RPC request id without validating the rest of
// the envelope. The caller must have already checked json.Valid. ids that are
// not a JSON-RPC string/number/null (e.g. objects or booleans) are normalized
// to null (GGA S-2).
func requestID(body []byte) json.RawMessage {
	var env struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return json.RawMessage("null")
	}
	if !validJSONRPCID(env.ID) {
		return json.RawMessage("null")
	}
	return env.ID
}

// validJSONRPCID reports whether raw is a JSON-RPC 2.0 id: string, number or
// null (the spec forbids everything else).
func validJSONRPCID(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	switch raw[0] {
	case '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	case 'n':
		return bytes.Equal(raw, []byte("null"))
	default:
		return false
	}
}

// toolCallName extracts the tool name of a tools/call request. Returns "" for
// any other method or when the name is missing; the path is left untouched in
// that case and the inner chain (or the SDK) reports the problem.
func toolCallName(body []byte) string {
	var env struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	if env.Method != "tools/call" {
		return ""
	}
	return env.Params.Name
}

// validToolName reports whether the tool name is a plain identifier suitable
// for use as the RBAC path key. Only [A-Za-z0-9_] is accepted so a hostile
// name can never smuggle path separators, query characters or fragments into
// the route (GGA S-1). A well-formed but unregistered name falls through to
// unknownToolGuard, which answers -32601 "Method not found" (REQ-MT-006); the
// registered-tool allowlist lives in Server.toolNames, populated by tool
// registration (T-09).
func validToolName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isToolNameChar(name[i]) {
			return false
		}
	}
	return true
}

// isToolNameChar reports whether c is allowed inside a tool name.
func isToolNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// writeJSONRPCError emits a JSON-RPC 2.0 error envelope with HTTP 200 so the
// MCP client parses it as a protocol error instead of a transport failure.
func writeJSONRPCError(w http.ResponseWriter, code int, message string, id json.RawMessage) {
	// The inner chain may have set headers on the real writer before the
	// translated status (e.g. http.Error's Content-Type + nosniff); the
	// envelope replaces them wholesale.
	for k := range w.Header() {
		w.Header().Del(k)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// A failed encode only matters if the client is gone; nothing to do.
	_ = json.NewEncoder(w).Encode(parseErrorEnvelope{
		JSONRPC: "2.0",
		Error:   parseErrorBody{Code: code, Message: message},
		ID:      id,
	})
}

// translatedCode reports whether the status triggers the JSON-RPC envelope
// instead of a passthrough.
func translatedCode(code int) bool {
	return code == http.StatusUnauthorized ||
		code == http.StatusForbidden ||
		code == http.StatusInternalServerError
}

// realStatusReporter is implemented by statusRecorder so jsonrpcAuthTranslator
// can report the inner chain's REAL status (401/403/500) to an outer recorder
// (the logging middleware, JD fix B-2) before re-emitting the failure as a 200
// envelope: the request log then carries the true auth decision while the
// client still receives the translated envelope.
type realStatusReporter interface {
	reportRealStatus(code int)
}

// statusRecorder lets the translator decide the final status without
// buffering the whole response (GGA W-1). Headers and bodies of non-translated
// statuses stream through to the real ResponseWriter immediately — a future
// SSE response is never accumulated — and only the three auth-failure
// statuses buffer their (discarded) http.Error bodies. Flush forwards so
// streaming clients see events as they arrive.
type statusRecorder struct {
	w           http.ResponseWriter
	status      int
	reported    bool // status came from reportRealStatus (real auth status)
	wroteHeader bool
	callerRole  string
	body        bytes.Buffer
}

func (r *statusRecorder) Header() http.Header { return r.w.Header() }

// RecordCallerRole annotates the resolved caller's role for the request log
// (REQ-MT-011). AuthMiddleware calls it where the caller is resolved; the
// first annotation wins so an outer recorder's own annotation is never
// overwritten by the forwarding inner one.
func (r *statusRecorder) RecordCallerRole(role string) {
	if r.callerRole == "" {
		r.callerRole = role
	}
}

// reportRealStatus records the real status decided by the inner chain
// (reported by the JSON-RPC auth translator) without touching the wire: the
// translator follows with the 200 envelope, which must still stream through.
// The first report wins; once reported, the status is pinned for the log and
// WriteHeader/Write forward verbatim.
func (r *statusRecorder) reportRealStatus(code int) {
	if r.wroteHeader || r.status != 0 {
		return
	}
	r.status = code
	r.reported = true
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	if !r.reported {
		r.status = code
	}
	if !translatedCode(r.status) || r.reported {
		r.w.WriteHeader(code)
	}
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	if !translatedCode(r.status) || r.reported {
		return r.w.Write(p)
	}
	return r.body.Write(p)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.w.(http.Flusher); ok {
		f.Flush()
	}
}
