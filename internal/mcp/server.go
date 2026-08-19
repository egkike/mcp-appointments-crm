package mcp

import (
	"log/slog"
	"net/http"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server owns the MCP SDK server and its Streamable HTTP handler
// (REQ-ARCH-INTMCP-001). Tool handlers are registered on the SDK server in
// PR 2; the skeleton answers initialize and tools/list (0 tools).
type Server struct {
	impl *mcp.Server
	cfg  Config
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
	return &Server{impl: impl, cfg: cfg}
}

// Handler returns the /mcp HTTP handler: the SDK Streamable HTTP handler
// (stateless, JSON responses) wrapped by the JSON-RPC parse guard. This is the
// unauthenticated path used by transport-level tests.
func (s *Server) Handler() http.Handler {
	return jsonParseGuard(streamableHandler(s.impl, s.cfg.Logger))
}

// AuthHandler returns the production /mcp HTTP handler: the unauthenticated
// Handler chain wrapped by AuthMiddleware and the JSON-RPC auth translator
// (REQ-AM-WIRED-001). AuthMiddleware authorizes by r.URL.Path, so the
// translator rewrites the path to the tool name for tools/call requests and
// translates HTTP 401/403/500 into JSON-RPC error envelopes (REQ-AM-WIRED-002).
func (s *Server) AuthHandler(authMW *auth.AuthMiddleware) http.Handler {
	return jsonrpcAuthTranslator(authMW.Wrap(s.Handler()))
}
