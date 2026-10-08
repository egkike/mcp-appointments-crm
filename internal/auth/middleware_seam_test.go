package auth

// Tests for the anonymous allowlist seam (feat-whatsapp-bot, TASK-3.1,
// design.md §1.1). The seam admits the `register_client` tool path without
// caller resolution, AFTER the universal empty-header check and BEFORE the
// resolver. Every other path keeps the existing 401 behavior.
//
// The effective tool path is the raw tool name: jsonrpcAuthTranslator rewrites
// r.URL.Path to the tool name for tools/call requests before AuthMiddleware
// runs (internal/mcp/auth_translator.go), so "register_client" — not
// "/tools/register_client" — is the allowlist key.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

// allowlistEntry is the single tool path admitted by the anonymous seam.
const allowlistEntry = "register_client"

// anonymousProbe records how the downstream handler saw the request context.
type anonymousProbe struct {
	called    bool
	anonymous bool
	hasCaller bool
}

// probeHandler captures the anonymous marker and caller presence from the
// request context and answers 200.
func probeHandler(p *anonymousProbe) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.called = true
		p.anonymous = IsAnonymous(r.Context())
		_, p.hasCaller = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}
}

// newToolRequest builds a request whose URL.Path carries the effective tool
// name (the form the JSON-RPC auth translator produces for tools/call).
func newToolRequest(tool, callerID string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
	req.URL.Path = tool
	if callerID != "" {
		req.Header.Set("X-Caller-Id", callerID)
	}
	return req
}

// TestMiddleware_AnonymousAllowlist_SkipsResolution pins scenario
// "Allowlisted path skips resolution": an unknown phone on the allowlisted
// path reaches the handler with zero resolver queries and no caller with a
// role, and the context is marked anonymous.
func TestMiddleware_AnonymousAllowlist_SkipsResolution(t *testing.T) {
	t.Parallel()
	h := &mwTestHandler{}
	logger := slog.New(h)
	// No sqlmock expectations on purpose: any resolver query is an unexpected
	// call (sqlmock errors), which would surface as a 500 and a skipped handler.
	mw, mock := newMiddlewareWithMock(t, nil, logger)

	var probe anonymousProbe
	req := newToolRequest(allowlistEntry, "+5491100999999")
	rec := httptest.NewRecorder()

	mw.Wrap(probeHandler(&probe)).ServeHTTP(rec, req)

	if !probe.called {
		t.Fatal("downstream handler was not called for the allowlisted path")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
	if !probe.anonymous {
		t.Error("context was not marked anonymous for the allowlisted path")
	}
	if probe.hasCaller {
		t.Error("context must NOT carry a caller with a role on the anonymous seam")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("resolver must not query on the allowlisted path: %v", err)
	}
}

// TestMiddleware_AnonymousAllowlist_EmptyHeaderUniversal401 pins the
// precedence scenario: a missing/empty/whitespace-only header on the
// allowlisted path answers the universal 401 BEFORE the allowlist is
// evaluated, with zero resolver queries and no invented anonymous marker.
func TestMiddleware_AnonymousAllowlist_EmptyHeaderUniversal401(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		header *string
	}{
		{name: "absent"},
		{name: "empty"},
		{name: "whitespace"},
	}
	empty, blank := "", "   "
	cases[1].header = &empty
	cases[2].header = &blank

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &mwTestHandler{}
			logger := slog.New(h)
			mw, mock := newMiddlewareWithMock(t, nil, logger)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
			req.URL.Path = allowlistEntry
			if tc.header != nil {
				req.Header.Set("X-Caller-Id", *tc.header)
			}
			rec := httptest.NewRecorder()

			var probe anonymousProbe
			mw.Wrap(probeHandler(&probe)).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
			}
			if !strings.Contains(rec.Body.String(), "no se proporcionó X-Caller-Id") {
				t.Errorf("body = %q; want to contain %q", rec.Body.String(), "no se proporcionó X-Caller-Id")
			}
			if probe.called {
				t.Error("downstream handler must NOT run when the header is missing/empty")
			}
			if probe.anonymous {
				t.Error("no anonymous marker may be invented when the header is missing/empty")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("the allowlist seam must not run before the empty-header check: %v", err)
			}
		})
	}
}

// TestMiddleware_NonAllowlistedPaths_UnknownCaller401 pins scenario
// "Non-allowlisted path keeps the existing 401 behavior" over several real
// tool paths, plus near-miss names that must NOT be admitted by the seam.
func TestMiddleware_NonAllowlistedPaths_UnknownCaller401(t *testing.T) {
	t.Parallel()
	tools := []string{
		"create_booking",
		"cancel_booking",
		"reschedule_booking",
		"get_booking",
		"update_business_profile",
		// Near-misses that a sloppy prefix/loose match could wrongly admit.
		"register_clients",
		"register_client_extra",
		"/tools/register_client",
		"registerclient",
	}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			h := &mwTestHandler{}
			logger := slog.New(h)
			mw, mock := newMiddlewareWithMock(t, nil, logger)

			id := "+5491100099999"
			expectUnknownCaller(mock, id)

			var probe anonymousProbe
			req := newToolRequest(tool, id)
			rec := httptest.NewRecorder()
			mw.Wrap(probeHandler(&probe)).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
			}
			if !strings.Contains(rec.Body.String(), "reconozco") {
				t.Errorf("body = %q; want to contain 'reconozco'", rec.Body.String())
			}
			if probe.called {
				t.Error("downstream handler must NOT run for an unknown caller on a non-allowlisted path")
			}
			if probe.anonymous {
				t.Error("non-allowlisted path must not be marked anonymous")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet resolver expectations: %v", err)
			}
		})
	}
}

// TestAnonymousAllowlist_ExactlyOneEntry is the runtime shape check: the
// allowlist is a static map with exactly the register_client entry.
func TestAnonymousAllowlist_ExactlyOneEntry(t *testing.T) {
	t.Parallel()
	if len(anonymousAllowedTools) != 1 {
		t.Fatalf("anonymous allowlist must have exactly 1 entry; got %d (%v)", len(anonymousAllowedTools), anonymousAllowedTools)
	}
	if _, ok := anonymousAllowedTools[allowlistEntry]; !ok {
		t.Fatalf("anonymous allowlist is missing %q; got %v", allowlistEntry, anonymousAllowedTools)
	}
}

// TestAnonymousAllowlist_IsStaticCodeOwned is the structural check: the
// allowlist is declared once, as a single-entry string-keyed literal, it is
// unexported, and no code path mutates it or hands it to a callee (delete,
// clear, or any function argument). Combined with Go's package visibility,
// this pins "cannot be extended from outside".
func TestAnonymousAllowlist_IsStaticCodeOwned(t *testing.T) {
	t.Parallel()

	const ident = "anonymousAllowedTools"

	fset := token.NewFileSet()
	dirEntries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read auth package dir: %v", err)
	}

	var (
		declCount  int
		entries    []string
		violations []string
	)
	for _, dirEntry := range dirEntries {
		name := dirEntry.Name()
		if dirEntry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ValueSpec:
				for i, name := range x.Names {
					if name.Name != ident {
						continue
					}
					declCount++
					cl, ok := x.Values[i].(*ast.CompositeLit)
					if !ok {
						violations = append(violations, fset.Position(n.Pos()).String()+": non-literal initializer")
						return true
					}
					for _, elt := range cl.Elts {
						kv, ok := elt.(*ast.KeyValueExpr)
						if !ok {
							violations = append(violations, fset.Position(n.Pos()).String()+": non-keyed entry")
							continue
						}
						lit, ok := kv.Key.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							violations = append(violations, fset.Position(n.Pos()).String()+": non-string key")
							continue
						}
						key, err := strconv.Unquote(lit.Value)
						if err != nil {
							violations = append(violations, fset.Position(n.Pos()).String()+": unquotable key")
							continue
						}
						entries = append(entries, key)
					}
				}
			case *ast.AssignStmt:
				for _, lhs := range x.Lhs {
					if exprRefersTo(lhs, ident) {
						violations = append(violations, fset.Position(n.Pos()).String()+": assignment to allowlist")
					}
				}
			case *ast.CallExpr:
				funIdent, isIdent := x.Fun.(*ast.Ident)
				readOnlyBuiltin := isIdent && (funIdent.Name == "len" || funIdent.Name == "cap")
				for _, arg := range x.Args {
					if !exprRefersTo(arg, ident) {
						continue
					}
					switch {
					case readOnlyBuiltin:
						// len/cap do not mutate the map.
					case isIdent && (funIdent.Name == "delete" || funIdent.Name == "clear"):
						violations = append(violations, fset.Position(n.Pos()).String()+": "+funIdent.Name+" on allowlist")
					case isIdent:
						violations = append(violations, fset.Position(n.Pos()).String()+": allowlist passed to "+funIdent.Name)
					default:
						violations = append(violations, fset.Position(n.Pos()).String()+": allowlist passed as call argument")
					}
				}
			}
			return true
		})
	}

	if declCount != 1 {
		t.Errorf("allowlist declarations = %d; want exactly 1", declCount)
	}
	if len(entries) != 1 || entries[0] != allowlistEntry {
		t.Errorf("allowlist literal entries = %v; want exactly [%s]", entries, allowlistEntry)
	}
	for _, v := range violations {
		t.Errorf("allowlist mutation vector: %s", v)
	}
}

// exprRefersTo reports whether expr directly contains an identifier with the
// given name. Nested function literals are NOT descended into: passing a
// closure to a function cannot let the callee mutate the map by aliasing it.
func exprRefersTo(expr ast.Expr, name string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}
