package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// methodListTools is the JSON-RPC method this filter narrows. The SDK keeps its
// own constant unexported, so the wire name is declared here: it is a protocol
// value, not a policy one, and it is pinned by the integration tests.
const methodListTools = "tools/list"

// toolsListFilter returns the receiving middleware that narrows tools/list to
// the tools the resolved caller can actually invoke (agent-surface T1, issue
// #265; mcp-transport spec REQ-MT-005/006).
//
// Why a middleware and not a filter API: go-sdk v1.7.0 has no tool-visibility
// hook. Server.listTools builds the result from the registry on every request
// and the receiving chain is the only seam; the SDK hands the middleware the
// SAME *ListToolsResult pointer the handler returned and serializes it after
// the chain returns, so replacing result.Tools is what the client receives.
//
// The trap this implementation avoids: rbac is keyed on the HTTP path, and the
// auth translator rewrites that path to the tool name ONLY for tools/call. A
// filter that read the RBAC map from the tools/list request path would compile,
// keep the tools/call tests green and narrow nothing. The caller's ROLE comes
// from the request context (auth.RequireCaller), injected by AuthMiddleware on
// the HTTP request whose context — per the Streamable handler — becomes the
// SDK connection context.
//
// FAIL-OPEN is deliberate and safe in both directions, because discovery is not
// authorization:
//   - no resolved caller: the result is left untouched. Every production path
//     with a missing or unknown caller is already refused with -32000 before
//     the handler, so this case only exists in transport-level tests;
//   - a tool with no RBAC row (or an empty one): kept visible, because
//     AuthMiddleware treats an absent key as "any authenticated caller". Hiding
//     it would advertise less than the guard allows — a capability regression
//     (spec S3), not extra security.
//
// PAGINATION INVARIANT: this filter narrows the page AFTER the SDK paginated
// it, so it must not be combined with a non-zero ServerOptions.PageSize. The
// server is built without pagination (PageSize 0, one page with every tool) and
// TestToolsListFiltersByCallerRole pins the resulting surface per role;
// TestToolsListCarriesNoNextCursor pins the invariant itself — a paginated
// answer would hand the caller a cursor computed before this narrowing, and the
// page behind it would be skipped without ever naming a role. It is a
// size-sensitive pin: it fails for any PageSize smaller than the per-role
// surface and passes for one large enough to emit no cursor, which is exactly
// the pagination-inert case.
func toolsListFilter(rbac auth.ToolRBAC, logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			// Never touch tools/call, initialize, or a failed request: the filter
			// is a discovery concern and must not become a second failure mode.
			if err != nil || method != methodListTools {
				return result, err
			}
			list, ok := result.(*mcp.ListToolsResult)
			if !ok || list == nil {
				// The SDK owns both sides of this assertion. If it ever fails, the
				// filter is silently inert, so say so instead of hiding the drift.
				logger.Warn("mcp: tools/list middleware received an unexpected result type",
					slog.String("type", fmt.Sprintf("%T", result)))
				return result, err
			}
			caller, err := auth.RequireCaller(ctx)
			if err != nil {
				// Fail open on DISCOVERY only, and never silently: a caller-less
				// tools/list cannot be filtered, and every production path refuses
				// such a request with -32000 before the handler. If this line ever
				// fires in production, a transport change stopped delivering the
				// caller and the menu would otherwise degrade without a trace.
				logger.Warn("mcp: tools/list ran without a resolved caller; list left unfiltered")
				return result, nil
			}
			list.Tools = visibleTools(list.Tools, rbac, caller.Role)
			return list, nil
		}
	}
}

// visibleTools keeps the tools the role may invoke, applying the SAME predicate
// AuthMiddleware applies to tools/call — auth.CanInvoke, literally the same
// function since advisory S1 (issue #268), not a twin that must be kept in
// sync: a tool with no row, or an empty row, is open to any authenticated
// caller; otherwise the role must be listed. The deliberately ungated tools
// (UngatedTools, tool_roles.go) are therefore kept visible by construction
// rather than by a second list that could drift.
func visibleTools(tools []*mcp.Tool, rbac auth.ToolRBAC, role string) []*mcp.Tool {
	visible := make([]*mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		if auth.CanInvoke(rbac, tool.Name, role) {
			visible = append(visible, tool)
		}
	}
	return visible
}
