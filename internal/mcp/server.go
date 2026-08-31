package mcp

import (
	"log/slog"
	"net/http"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server owns the MCP SDK server and its Streamable HTTP handler
// (REQ-ARCH-INTMCP-001). Tool handlers are registered on the SDK server in
// PR 2 (T-09); toolNames is the registry the transport guard consults to
// answer tools/call for unknown tools with -32601 (REQ-MT-006).
type Server struct {
	impl      *mcp.Server
	cfg       Config
	toolNames map[string]struct{}
}

// NewServer builds the MCP server for the given configuration. Version feeds
// serverInfo; Logger receives SDK activity logs. Capabilities advertise the
// tools feature with listChanged disabled (no notifications in this phase).
func NewServer(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	impl := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-appointments-crm",
		Version: cfg.Version,
	}, &mcp.ServerOptions{
		Logger: cfg.Logger,
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{ListChanged: false},
		},
	})
	srv := &Server{
		impl:      impl,
		cfg:       cfg,
		toolNames: make(map[string]struct{}),
	}
	srv.registerTools()
	return srv
}

// registerTools wires the eight MCP tools onto the SDK server (T-09). Tools
// whose port is nil are skipped, keeping the skeleton behavior (zero tools)
// for transport-level tests. Each registered tool also enters toolNames, the
// registry consulted by unknownToolGuard (REQ-MT-006).
func (s *Server) registerTools() {
	s.registerBookingTools()
	s.registerProfileTool()
	s.registerSearchTools()
	s.registerAlertTools()
}

// Handler returns the /mcp HTTP handler: the SDK Streamable HTTP handler
// (stateless, JSON responses) wrapped by the JSON-RPC parse guard and the
// unknown-tool guard (REQ-MT-006). This is the unauthenticated path used by
// transport-level tests.
func (s *Server) Handler() http.Handler {
	return jsonParseGuard(unknownToolGuard(s.toolNames, streamableHandler(s.impl, s.cfg.Logger)))
}

// AuthHandler returns the production /mcp HTTP handler: the unauthenticated
// Handler chain wrapped by AuthMiddleware, the JSON-RPC auth translator
// (REQ-AM-WIRED-001) and the per-request logging middleware (REQ-MT-011).
//
// Method gate (REQ-MT-002, JD fix A-3): POST is the sole MCP endpoint.
// Non-POST requests bypass the auth chain and go straight to the SDK
// Streamable HTTP handler, which answers 405 Method Not Allowed — an
// unauthenticated GET /mcp must never produce a 200 JSON-RPC envelope.
// POST requests run the authenticated chain, composed from the outside as:
//
//	loggingMiddleware(methodGate(jsonrpcAuthTranslator(authMW.Wrap(handler))))
//
// AuthMiddleware authorizes by r.URL.Path, so the translator rewrites the
// path to the tool name for tools/call requests and translates HTTP
// 401/403/500 into JSON-RPC error envelopes (REQ-AM-WIRED-002). The logging
// middleware sits OUTSIDE the translator (JD fix B-2): auth-rejected requests
// still produce their structured log line with the REAL status — the
// translator reports it through statusRecorder before re-emitting the failure
// as a 200 envelope. The caller role for the log line is annotated on the
// recorder by AuthMiddleware where the caller is resolved and forwarded
// through the recorder chain (the caller is injected on a request COPY that
// never propagates back to the middleware), else "none".
func (s *Server) AuthHandler(authMW *auth.AuthMiddleware) http.Handler {
	if authMW == nil {
		panic("mcp: AuthHandler requires a non-nil AuthMiddleware")
	}
	postChain := jsonrpcAuthTranslator(authMW.Wrap(s.Handler()))
	methodGate := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.Handler().ServeHTTP(w, r)
			return
		}
		postChain.ServeHTTP(w, r)
	})
	return loggingMiddleware(s.cfg.Logger, methodGate)
}
