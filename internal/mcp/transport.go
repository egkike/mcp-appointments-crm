package mcp

import (
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// streamableHandler builds the SDK Streamable HTTP handler in stateless mode
// (REQ-MT-002): GET /mcp returns 405, no Mcp-Session-Id is issued, and
// sessions are ephemeral per-request — no goroutine leak from abandoned
// sessions. JSONResponse keeps responses as application/json instead of SSE.
func streamableHandler(impl *mcp.Server, logger *slog.Logger) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return impl
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		Logger:       logger,
	})
}
