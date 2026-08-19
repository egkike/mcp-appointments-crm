package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// maxRequestBodyBytes bounds the JSON-RPC request body buffered by
// jsonParseGuard. Verified against go-sdk v1.4.1: StreamableHTTPOptions has
// no MaxRequestBodyBytes field and servePOST reads the body unbounded, so
// the 1 MiB budget is enforced here, at the outermost layer of /mcp
// (design §4, R4-003; re-verified on the v1.2.0 → v1.4.1 security bump). If
// a future SDK grows a native body limit, prefer it and drop this half of
// the guard.
const maxRequestBodyBytes = 1 << 20

// codeBusinessError is the application-level JSON-RPC code for all
// domain.SemanticError failures (MCP spec reserves -32000..-32099 for server
// errors; auth uses -32000/-32001, business rules use -32002).
const codeBusinessError = -32002

// toMCPError maps a tool-handler error to a JSON-RPC protocol error
// (REQ-MT-015). Returning *jsonrpc.Error from a typed SDK handler makes the
// SDK emit a JSON-RPC error object instead of packing the error into
// CallToolResult.Content:
//
//   - *domain.SemanticError (or any error wrapping one) → -32002 with the
//     semantic Spanish message; the LLM client can act on it directly.
//   - anything else → -32603 "error interno del servidor": infrastructure
//     failures never leak internals to the client.
//
// A nil error stays nil (handlers call this only when err != nil, but the
// guard is free).
func toMCPError(err error) *jsonrpc.Error {
	if err == nil {
		return nil
	}
	var sem *domain.SemanticError
	if errors.As(err, &sem) {
		return &jsonrpc.Error{Code: codeBusinessError, Message: sem.Message}
	}
	return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: msgInternal}
}

// jsonParseGuard wraps the streamable handler with two JSON-RPC transport
// safeguards go-sdk v1.4.1 does not provide (REQ-MT-003):
//   - payloads larger than 1 MiB are rejected with HTTP 413;
//   - malformed JSON is answered with a JSON-RPC 2.0 -32700 Parse error
//     envelope instead of the SDK's plain-text HTTP 400.
//
// The body is buffered exactly once and restored for the inner handler.
func jsonParseGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only POST carries a JSON-RPC body; GET/DELETE pass through to the
		// SDK handler untouched (e.g. GET /mcp must still answer 405).
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if len(body) > maxRequestBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if !json.Valid(body) {
			writeParseError(w)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

// parseErrorBody is the JSON-RPC error object of the parse-error envelope.
type parseErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// parseErrorEnvelope is the REQ-MT-003 response body. ID is marshaled as the
// literal null (a malformed request carries no id).
type parseErrorEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	Error   parseErrorBody  `json:"error"`
	ID      json.RawMessage `json:"id"`
}

// writeParseError emits the JSON-RPC 2.0 parse-error envelope for a body that
// is not valid JSON (REQ-MT-003).
func writeParseError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	// A failed encode only matters if the client is gone; nothing to do.
	_ = json.NewEncoder(w).Encode(parseErrorEnvelope{
		JSONRPC: "2.0",
		Error: parseErrorBody{
			Code:    jsonrpc.CodeParseError,
			Message: "Parse error",
		},
		ID: json.RawMessage("null"),
	})
}
