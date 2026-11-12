package main

import (
	"context"
	"io"

	"github.com/egkike/mcp-appointments-crm/internal/tui"
)

// This file wires the read-only allowlist advisory of option 7 (issue #195) into
// the console flow. The mechanics and the copy live in internal/tui
// (allowlist.go); this file only supplies the injectable seam and the console
// writer, the same split the skill cascade uses (tui.RegenerateSkillForConsole).
//
// The advisory exists because the Hermes channel bundles are full access by
// default: `terminal`, `file` and `execute_code` can write the business database
// without going through the MCP tools, and the runtime only narrows a channel
// when the operator disables the extra toolsets per platform. Option 7 already
// writes the server entry, so it is the only moment where the operator is certain
// to be reading — and the only place this product can tell them what the channel
// actually exposes.

// toolsListRunner is the advisory's injectable seam: it runs
// `hermes tools list --platform <platform>` and returns the runtime's report
// verbatim. It is an alias of the tui type on purpose — one definition, injected
// by the flow and faked by the tests, so no test ever executes a real binary.
type toolsListRunner = tui.ToolsListRunner

// allowlistAudit bundles the advisory's inputs: the runner seam and the channels
// to audit. It mirrors identityDeps (the composition root constructs the
// production value once, flows receive it by value).
type allowlistAudit struct {
	runToolsList toolsListRunner
	platforms    []string
}

// defaultAllowlistAudit is the production wiring of the advisory.
//
// Channels: the two this product documents end-to-end (docs/installation.md and
// docs/gateway-contract.md) — `whatsapp` is the primary business channel and
// `telegram` is the alternative one, plus the owner bot. Option 7 always writes
// the server entry and the Telegram block is optional, so auditing both and
// letting the runtime answer is simpler and more honest than inferring which
// channel the operator "really" uses: an absent platform is reported by Hermes
// itself, and a platform whose audit fails is named in the advisory with the
// category of the failure, never dropped silently.
var defaultAllowlistAudit = allowlistAudit{
	runToolsList: tui.RunHermesToolsList,
	platforms:    []string{"whatsapp", "telegram"},
}

// writeAllowlistAdvisory prints the advisory as the last step of the flow.
//
// The step is best effort by design: config.yaml is already written when the
// advisory runs, so a console the advisory cannot write to must never turn a
// successful config write into a flow failure (native review R4-003). Its failure
// is dropped here, not propagated; the error return stays only because the caller
// returns this call directly as the flow's result.
//
// A failed audit is not an error either: the values are read-only and never
// interpolated into the output, so there is nothing to sanitize and nothing that
// can leak runtime text into the console.
func writeAllowlistAdvisory(ctx context.Context, stdout io.Writer, audit allowlistAudit) error {
	text := tui.AllowlistAdvisory(ctx, audit.platforms, audit.runToolsList)
	if text == "" {
		return nil
	}
	// Best effort, deliberately unchecked: see the doc comment above.
	_ = writeConsole(stdout, text)
	return nil
}
