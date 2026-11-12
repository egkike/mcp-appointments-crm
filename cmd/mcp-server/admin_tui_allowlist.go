package main

import (
	"context"
	"io"
	"os"

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

// allowlistAudit bundles the advisory's inputs: the runner seam, the channels
// to audit and the fallback console. It mirrors identityDeps (the composition
// root constructs the production value once, flows receive it by value).
type allowlistAudit struct {
	runToolsList toolsListRunner
	platforms    []string
	// fallback is the console the advisory retries on when the operator console
	// refuses the write. Production points it at stderr; a nil fallback keeps the
	// step a plain best-effort write, which is what a unit test with no real
	// stderr and no second writer wants (native review R4-002).
	fallback io.Writer
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
//
// The fallback console is stderr: a security notice the operator never reads is
// a silent loss, so a refused stdout write is retried once on the other stream
// the same process owns (native review R4-002).
var defaultAllowlistAudit = allowlistAudit{
	runToolsList: tui.RunHermesToolsList,
	platforms:    []string{"whatsapp", "telegram"},
	fallback:     os.Stderr,
}

// writeAllowlistAdvisory prints the advisory as the last step of the flow.
//
// The step is best effort by design: config.yaml is already written when the
// advisory runs, so a console the advisory cannot write to must never turn a
// successful config write into a flow failure (native review R4-003). It is not
// silent, though: a refused write is retried on the fallback console, and only a
// failure of both is dropped, because at that point there is no operator action
// left to offer (native review R4-002). The error return stays only because the
// caller returns this call directly as the flow's result.
//
// A failed audit is not an error either: the values are read-only and never
// interpolated into the output, so there is nothing to sanitize and nothing that
// can leak runtime text into the console.
func writeAllowlistAdvisory(ctx context.Context, stdout io.Writer, audit allowlistAudit) error {
	text := tui.AllowlistAdvisory(ctx, audit.platforms, audit.runToolsList)
	if text == "" {
		return nil
	}
	writeAdvisoryText(stdout, audit.fallback, text)
	return nil
}

// writeAdvisoryText writes the advisory to the operator console and, when that
// console refuses the write, retries the SAME text on the fallback console so the
// security notice cannot vanish with nobody learning it was lost (native review
// R4-002). A failure of both is absorbed: the config write already succeeded and
// this is the last, non-essential step of the flow.
func writeAdvisoryText(primary, fallback io.Writer, text string) {
	if err := writeConsole(primary, text); err == nil {
		return
	}
	if fallback == nil {
		return
	}
	// Best effort by contract: if stderr is gone too, the write is dropped.
	_ = writeConsole(fallback, text)
}
