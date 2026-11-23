package auth

// CanInvoke reports whether role may invoke tool under rbac. It is the ONE
// predicate behind both authorization decisions of the /mcp surface:
//
//   - the RBAC step of AuthMiddleware.Wrap, which refuses tools/call with 403
//     before the handler runs;
//   - the tools/list discovery filter (internal/mcp/tool_filter.go), which
//     narrows the advertised menu to the tools the caller can actually invoke.
//
// Why it lives here (advisory S1, issue #268): the predicate used to exist
// twice — inline in the middleware and as a canInvoke copy in internal/mcp —
// with a comment claiming the two were "kept in one place". They were not: an
// edit on either side was a silent divergence, and the discovery copy could
// have widened or narrowed the menu without ever touching the guard. Sharing
// the function turns that promise into structure, which is also why a
// "consumers agree" test is no longer meaningful (it would compare a function
// with itself): the guarantee is now the single definition, and the table test
// below pins its truth table while the per-role integration tests
// (internal/mcp/tool_filter_integration_test.go) pin the two observable halves
// — the menu each role receives and the -32001 refusal on a hidden tool.
//
// The rule, preserved byte for byte from both former copies:
//   - absent row: allowed. An omitted key means "no gate", which is exactly why
//     the deliberately ungated tools are DECLARED in mcp.UngatedTools() instead
//     of simply being forgotten;
//   - empty row ({} or a row with no entries): allowed, same reasoning — the
//     shape a future edit is most likely to translate wrongly, so it is
//     asserted explicitly;
//   - populated row: the role must be listed. The comparison is
//     case-sensitive, as it always was: roles are the Role* constants and no
//     layer case-folds caller input on the way in.
func CanInvoke(rbac ToolRBAC, tool, role string) bool {
	roles := rbac[tool]
	if len(roles) == 0 {
		return true
	}
	return roleAllowed(role, roles)
}

// roleAllowed checks if the given role is in the allowed set. Case-sensitive on
// purpose: the allowed set is policy written from the Role* constants, never
// caller-supplied text (a case-fold here would be an authorization change).
func roleAllowed(role string, allowed []string) bool {
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}
