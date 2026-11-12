package tui

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// This file implements the read-only allowlist advisory of the "Configurar
// Hermes" capability (issue #195). Hermes channel bundles are full access by
// default — the same terminal/file/execute_code surface that can write the
// business database without going through the MCP tools — and the runtime only
// narrows a channel when the operator runs `hermes tools disable` per platform
// (docs/installation.md → "Allowlist mínima recomendada"). Option 7 already
// writes the server entry, so it is the one moment where the operator is
// guaranteed to be looking: the advisory reports the channel's current state and,
// when it cannot be proven minimal, the exact commands to fix it.
//
// Hard boundaries of this file:
//   - It NEVER writes the Hermes config. config.yaml is runtime-owned; the
//     advisory only reads the runtime's own report and prints it.
//   - It NEVER reimplements the resolver. The state comes from
//     `hermes tools list --platform <channel>`, printed verbatim.
//   - It NEVER prints an error. A failed or undecidable audit degrades to the
//     generic advisory, so a filesystem path, a driver message or a stack trace
//     can never reach the console (issue #157 sanitization contract: nothing to
//     sanitize because nothing is interpolated).
//   - It NEVER claims safety it cannot prove: anything the parser does not
//     recognize as the documented minimal shape is reported as "not confirmed".

// hermesToolsListTimeout bounds the audit subprocess. The command is a local CLI
// listing: a few seconds is generous, and the flow must never hang on it.
const hermesToolsListTimeout = 5 * time.Second

// hermesBinary is the runtime CLI the advisory queries. It is a literal, never
// operator input: the channel is a fixed platform name from
// defaultAllowlistAudit and the argv is built by this package.
const hermesBinary = "hermes"

// minimalAllowlistSummary is the recommended allowlist, worded exactly as the
// installation guide lists it.
const minimalAllowlistSummary = "mcp-appointments, skills, clarify, memory, todo"

// installationDocPointer is the single doc reference every state prints, so the
// operator can read the full procedure (audit → disable → verify) instead of
// trusting one command.
const installationDocPointer = `docs/installation.md → "Allowlist mínima recomendada"`

// minimalChannelToolsets are the four bundled toolsets a CRM-only channel needs.
// mcp-appointments is an MCP server, not a toolset, so it is asserted from the
// report's MCP section instead.
var minimalChannelToolsets = []string{"skills", "todo", "memory", "clarify"}

// dangerousChannelToolsets is every toolset/plugin the installation guide tells
// the operator to disable: each one can execute code, read/write files or reach
// the network, i.e. each one can write the business database behind the MCP
// tools' back. The order matches the documented command.
var dangerousChannelToolsets = []string{
	"code_execution",
	"terminal",
	"file",
	"browser",
	"web",
	"computer_use",
	"delegation",
	"cronjob",
	"image_gen",
	"tts",
	"vision",
	"session_search",
	"connections",
	"homeassistant",
}

// ToolsListRunner runs the runtime's tool inventory for one platform and returns
// its stdout verbatim. It is the injectable seam of the advisory: production uses
// RunHermesToolsList, tests inject fakes so no test ever executes a real binary.
type ToolsListRunner func(ctx context.Context, platform string) (string, error)

// RunHermesToolsList is the production ToolsListRunner. The command is bounded by
// hermesToolsListTimeout, runs without a shell (no interpolation, no quoting
// surface) and only its stdout is used: stderr belongs to the runtime and is
// never echoed to the operator.
func RunHermesToolsList(ctx context.Context, platform string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, hermesToolsListTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, hermesBinary, "tools", "list", "--platform", platform).Output() //nolint:gosec // fixed argv: literal binary, literal verb, platform is a hardcoded channel name
	if err != nil {
		return "", fmt.Errorf("hermes tools list --platform %s: %w", platform, err)
	}
	return string(out), nil
}

// ChannelAllowlistAdvice renders the advisory for ONE channel. An empty string
// means the channel could not be audited (runner missing, command failed, no
// output at all): the composer decides whether the generic advisory is due.
func ChannelAllowlistAdvice(ctx context.Context, channel string, run ToolsListRunner) string {
	if run == nil {
		return ""
	}
	output, err := run(ctx, channel)
	if err != nil {
		return ""
	}
	// A report with no bytes is not evidence of anything: claim nothing.
	if strings.TrimSpace(output) == "" {
		return ""
	}
	if channelAllowlistIsMinimal(output) {
		return fmt.Sprintf(
			"Allowlist del canal %s: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n",
			channel)
	}
	return channelAllowlistBlockedAdvice(channel, output)
}

// AllowlistAdvisory composes the advisory the console prints after the Hermes
// config write: one section per audited channel, and the generic advisory when no
// channel could be audited at all. The leading blank line separates it from the
// skill cascade output.
func AllowlistAdvisory(ctx context.Context, platforms []string, run ToolsListRunner) string {
	var b strings.Builder
	b.WriteString("\n")

	audited := false
	for _, platform := range platforms {
		advice := ChannelAllowlistAdvice(ctx, platform, run)
		if advice == "" {
			// Skipped silently: a channel that cannot be audited contributes no
			// evidence, and the generic advisory covers the unknown case.
			continue
		}
		audited = true
		b.WriteString(advice)
	}
	if !audited {
		return allowlistGenericAdvice
	}
	return b.String()
}

// allowlistGenericAdvice is the fallback for every case the advisory cannot
// probe. The risk it names is NOT Hermes-specific: any agent with a channel ships
// its own tools, and this line is the only signal that operator will get.
const allowlistGenericAdvice = "\n" +
	"Allowlist del canal: no se pudo auditar (hermes no está disponible en este host o el comando falló).\n" +
	"Si usás Hermes o cualquier otro agente con canal (WhatsApp, Telegram, Discord, …), verificá que ese canal exponga sólo los tools del CRM y ninguna ejecución de código o shell: cualquier tool extra puede escribir la base del negocio sin pasar por los MCP del CRM.\n" +
	"Detalle: " + installationDocPointer + ".\n"

// channelAllowlistBlockedAdvice renders the actionable block: the runtime's own
// report printed as-is (never parsed into a summary the operator cannot verify),
// the recommended list, the exact command, and the doc pointer.
func channelAllowlistBlockedAdvice(channel, output string) string {
	return fmt.Sprintf("Allowlist del canal %s: no puedo confirmar que sea la mínima ⚠\n", channel) +
		fmt.Sprintf("Salida literal de `%s tools list --platform %s`:\n", hermesBinary, channel) +
		strings.TrimRight(output, " \t\n") + "\n" +
		"\nAllowlist mínima recomendada: " + minimalAllowlistSummary + "\n" +
		"Aplicala con:\n" +
		fmt.Sprintf("%s tools disable --platform %s %s\n", hermesBinary, channel, strings.Join(dangerousChannelToolsets, " ")) +
		"Detalle y verificación: " + installationDocPointer + ".\n"
}

// channelAllowlistIsMinimal decides the "already minimal" state conservatively.
// It only answers true for the exact documented shape of a narrowed channel:
// an enabled-toolset line that lists precisely the four minimal toolsets, no
// extra names, and an MCP section that still carries mcp-appointments with its
// tools enabled. Every other report — a different layout, an unknown toolset, a
// missing MCP server — answers false, because the expensive mistake here is
// telling an operator their channel is safe when it is not.
func channelAllowlistIsMinimal(output string) bool {
	enabled := enabledChannelToolsets(output)
	if len(enabled) != len(minimalChannelToolsets) {
		return false
	}
	for _, name := range enabled {
		if !slices.Contains(minimalChannelToolsets, name) {
			return false
		}
	}
	for _, want := range minimalChannelToolsets {
		if !slices.Contains(enabled, want) {
			return false
		}
	}
	// The business tools must still be there: a minimal allowlist without the
	// MCP server would break the bot instead of protecting it.
	return strings.Contains(output, "mcp-appointments") &&
		strings.Contains(output, "all tools enabled")
}

// enabledChannelToolsets extracts the toolset names from the report's
// enabled-toolset lines (`✓ enabled  skills · todo · memory · clarify`). Lines
// that do not match that shape are ignored, and an output without any such line
// yields no names — which the caller reads as "not minimal".
func enabledChannelToolsets(output string) []string {
	var enabled []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		body, ok := strings.CutPrefix(trimmed, "✓")
		if !ok {
			continue
		}
		fields := strings.Fields(body)
		if len(fields) < 2 || fields[0] != "enabled" {
			continue
		}
		names := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body), "enabled"))
		for _, name := range strings.Split(names, "·") {
			if tool := strings.TrimSpace(name); tool != "" {
				enabled = append(enabled, tool)
			}
		}
	}
	return enabled
}
