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

// registerTools wires the six MCP tools onto the SDK server (T-09). Tools
// whose port is nil are skipped, keeping the skeleton behavior (zero tools)
// for transport-level tests. Each registered tool also enters toolNames, the
// registry consulted by unknownToolGuard (REQ-MT-006).
func (s *Server) registerTools() {
	s.registerBookingTools()
	s.registerProfileTool()
}

// Handler returns the /mcp HTTP handler: the SDK Streamable HTTP handler
// (stateless, JSON responses) wrapped by the JSON-RPC parse guard and the
// unknown-tool guard (REQ-MT-006). This is the unauthenticated path used by
// transport-level tests.
func (s *Server) Handler() http.Handler {
	return jsonParseGuard(unknownToolGuard(s.toolNames, streamableHandler(s.impl, s.cfg.Logger)))
}

// AuthHandler returns the production /mcp HTTP handler: the unauthenticated
// Handler chain wrapped by AuthMiddleware, the per-request logging middleware
// (REQ-MT-011) and the JSON-RPC auth translator (REQ-AM-WIRED-001).
// AuthMiddleware authorizes by r.URL.Path, so the translator rewrites the
// path to the tool name for tools/call requests and translates HTTP
// 401/403/500 into JSON-RPC error envelopes (REQ-AM-WIRED-002). The logging
// middleware sits INSIDE AuthMiddleware: it observes the real auth decision
// (status before envelope translation) and the rewritten path.
func (s *Server) AuthHandler(authMW *auth.AuthMiddleware) http.Handler {
	return jsonrpcAuthTranslator(authMW.Wrap(loggingMiddleware(s.cfg.Logger, s.Handler())))
}
