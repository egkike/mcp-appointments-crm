package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
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
//     `hermes tools list --platform <channel>`, printed verbatim and bounded
//     (maxRuntimeReportBytes, with an explicit truncation marker and the command
//     that prints the whole report).
//   - It NEVER prints an error. A failed or undecidable audit degrades to a fixed
//     failure category (binary unavailable / command failed / empty report), so a
//     filesystem path, a driver message or a stack trace can never reach the
//     console (issue #157 sanitization contract: nothing to sanitize because
//     nothing is interpolated).
//   - It NEVER hides a channel it could not audit: a partial audit names every
//     skipped channel with its category, and a total failure prints the generic
//     advisory.
//   - It NEVER claims safety it cannot prove: anything the parser does not
//     recognize as the documented minimal shape is reported as "not confirmed".

// hermesToolsListTimeout bounds the audit subprocess. The command is a local CLI
// listing: a few seconds is generous, and the flow must never hang on it.
const hermesToolsListTimeout = 5 * time.Second

// maxRuntimeReportBytes bounds the verbatim report the advisory prints. The
// runtime owns that report and can grow it as it lists tools: an unbounded print
// would flood the console the operator is reading, so the block is capped and a
// marker names where to read the whole thing.
const maxRuntimeReportBytes = 8192

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
	ctx, cancel := hermesToolsListContext(ctx)
	defer cancel()

	out, err := exec.CommandContext(ctx, hermesBinary, "tools", "list", "--platform", platform).Output() //nolint:gosec // fixed argv: literal binary, literal verb, platform is a hardcoded channel name
	if err != nil {
		return "", fmt.Errorf("hermes tools list --platform %s: %w", platform, err)
	}
	return string(out), nil
}

// hermesToolsListContext derives the probe's own budget.
//
// The parent is context.WithoutCancel, so the audit is not aborted for a reason
// that has nothing to do with it: a cancelled console flow (Ctrl-C upstream, a
// closed stdin) must not erase evidence the operator already asked for. That
// detachment is deliberate and BOUNDED, not an oversight (native review
// R4-003): a flow cancelled mid-advisory still waits for the probe, but never
// longer than hermesToolsListTimeout and never longer than a deadline the caller
// itself carried — so the worst case an operator can observe is that one probe,
// and the report it produces is exactly the evidence the cancelled flow wanted.
//
// Detaching cancellation must not detach the caller's DEADLINE (native review
// R3-withoutcancel-values): context.WithoutCancel drops Done and Deadline
// together, so the budget is the tighter of the two — the caller's deadline when
// it is earlier than hermesToolsListTimeout, the 5s budget otherwise. Values
// survive either way; only cancellation is dropped.
//
// The caller's deadline only wins while it is still USABLE (remaining > 0). An
// already-expired caller deadline, or a clock jump, would otherwise hand the probe
// a context that is dead on arrival: the command fails for a reason that has
// nothing to do with it, and the advisory reports "el comando falló", sending the
// operator to fix a command that is fine (native review R4-002). Falling through
// to the 5s budget keeps the audit evidence instead of misclassifying it.
func hermesToolsListContext(ctx context.Context) (context.Context, context.CancelFunc) {
	parent := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < hermesToolsListTimeout {
			return context.WithDeadline(parent, deadline)
		}
	}
	return context.WithTimeout(parent, hermesToolsListTimeout)
}

// auditFailure is why a channel could not be audited. It is a closed set of
// categories — never a raw error — so the advisory can tell the operator which
// action is due (install the CLI, fix the command, run it by hand) without
// interpolating a driver message, a path or a stack trace (issue #157).
type auditFailure int

const (
	auditOK auditFailure = iota
	auditBinaryUnavailable
	auditCommandFailed
	auditEmptyReport
)

// description is the fixed operator copy for one category. Literal text only:
// nothing here can carry runtime output.
func (f auditFailure) description() string {
	switch f {
	case auditBinaryUnavailable:
		return "binario `hermes` no disponible en este host"
	case auditCommandFailed:
		return "el comando `hermes tools list` falló"
	case auditEmptyReport:
		return "el comando no devolvió ningún reporte"
	case auditOK:
		return ""
	}
	return ""
}

// unauditableChannel pairs a skipped platform with its reason, so the composer
// can name it instead of letting it disappear from the output.
type unauditableChannel struct {
	platform string
	failure  auditFailure
}

// ChannelAllowlistAdvice renders the advisory for ONE channel. An empty string
// means the channel could not be audited (runner missing, command failed, no
// output at all): the composer decides whether the generic advisory is due.
func ChannelAllowlistAdvice(ctx context.Context, channel string, run ToolsListRunner) string {
	advice, _ := channelAllowlistAdvice(ctx, channel, run)
	return advice
}

// channelAllowlistAdvice is ChannelAllowlistAdvice plus the failure category, so
// the multi-channel composer can explain why a channel was skipped.
func channelAllowlistAdvice(ctx context.Context, channel string, run ToolsListRunner) (string, auditFailure) {
	if run == nil {
		// A nil seam means there is no auditor at all: same operator action as a
		// missing binary (check the host), so it reports the same category.
		return "", auditBinaryUnavailable
	}
	output, err := run(ctx, channel)
	if err != nil {
		return "", classifyToolsListFailure(err)
	}
	// A report with no bytes is not evidence of anything: claim nothing.
	if strings.TrimSpace(output) == "" {
		return "", auditEmptyReport
	}
	if channelAllowlistIsMinimal(output) {
		return fmt.Sprintf(
			"Allowlist del canal %s: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n",
			channel), auditOK
	}
	return channelAllowlistBlockedAdvice(channel, output), auditOK
}

// classifyToolsListFailure maps the runner's error to an operator category. It
// only asks whether the binary was found: the error value itself is never kept,
// formatted or printed.
func classifyToolsListFailure(err error) auditFailure {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return auditBinaryUnavailable
	}
	return auditCommandFailed
}

// AllowlistAdvisory composes the advisory the console prints after the Hermes
// config write: one section per audited channel, one explicit line per channel
// that could not be audited, and the generic advisory when no channel could be
// audited at all. The leading blank line separates it from the skill cascade
// output.
func AllowlistAdvisory(ctx context.Context, platforms []string, run ToolsListRunner) string {
	var b strings.Builder
	b.WriteString("\n")

	audited := false
	// One entry per unauditable channel, in platform order. A channel that is
	// skipped silently would read as "that one is fine", which is exactly the
	// mistake this advisory exists to prevent.
	skipped := make([]unauditableChannel, 0, len(platforms))
	for _, platform := range platforms {
		advice, failure := channelAllowlistAdvice(ctx, platform, run)
		if advice == "" {
			skipped = append(skipped, unauditableChannel{platform: platform, failure: failure})
			continue
		}
		audited = true
		b.WriteString(advice)
	}
	if !audited {
		return allowlistGenericAdvice(skipped)
	}
	for _, channel := range skipped {
		b.WriteString(channelAllowlistSkippedLine(channel))
	}
	return b.String()
}

// allowlistGenericAdvice is the fallback for every case the advisory cannot
// probe. The risk it names is NOT Hermes-specific: any agent with a channel ships
// its own tools, and this line is the only signal that operator will get. The
// parenthetical names the categories actually observed, because "the binary is
// missing" and "the command broke" send the operator to different places.
func allowlistGenericAdvice(skipped []unauditableChannel) string {
	reason := "hermes no está disponible en este host o el comando falló"
	if reasons := auditFailureDescriptions(skipped); len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}
	return "\n" +
		"Allowlist del canal: no se pudo auditar (" + reason + ").\n" +
		"Si usás Hermes o cualquier otro agente con canal (WhatsApp, Telegram, Discord, …), verificá que ese canal exponga sólo los tools del CRM y ninguna ejecución de código o shell: cualquier tool extra puede escribir la base del negocio sin pasar por los MCP del CRM.\n" +
		"Detalle: " + installationDocPointer + ".\n"
}

// auditFailureDescriptions returns the distinct failure categories in a stable
// order (the order of the categories themselves), so the same host always prints
// the same sentence.
func auditFailureDescriptions(skipped []unauditableChannel) []string {
	var descriptions []string
	for _, failure := range []auditFailure{auditBinaryUnavailable, auditCommandFailed, auditEmptyReport} {
		if slices.ContainsFunc(skipped, func(c unauditableChannel) bool { return c.failure == failure }) {
			descriptions = append(descriptions, failure.description())
		}
	}
	return descriptions
}

// channelAllowlistSkippedLine names one channel that could not be audited. It
// reuses the generic advisory's wording, shortened: the operator must verify that
// channel by hand rather than assume the audited one speaks for both.
func channelAllowlistSkippedLine(channel unauditableChannel) string {
	return fmt.Sprintf("Allowlist del canal %s: no se pudo auditar (%s).\n",
		channel.platform, channel.failure.description()) +
		"Verificá a mano que ese canal exponga sólo los tools del CRM y ninguna ejecución de código o shell.\n"
}

// channelAllowlistBlockedAdvice renders the actionable block: the runtime's own
// report printed as-is but bounded (never parsed into a summary the operator
// cannot verify), the recommended list, the exact command, and the doc pointer.
func channelAllowlistBlockedAdvice(channel, output string) string {
	return fmt.Sprintf("Allowlist del canal %s: no puedo confirmar que sea la mínima ⚠\n", channel) +
		fmt.Sprintf("Salida literal de `%s tools list --platform %s`:\n", hermesBinary, channel) +
		boundedRuntimeReport(channel, output) + "\n" +
		"\nAllowlist mínima recomendada: " + minimalAllowlistSummary + "\n" +
		"Aplicala con:\n" +
		fmt.Sprintf("%s tools disable --platform %s %s\n", hermesBinary, channel, strings.Join(disableCommandToolsets(output), " ")) +
		"Detalle y verificación: " + installationDocPointer + ".\n"
}

// toolsetNamePattern is the shape a report-derived name must have before it is
// appended to the remediation command. That command is COPY-PASTED INTO A SHELL
// by the operator, and the names come from the runtime report — data this
// program does not control — so a name carrying spaces, `;`, backticks, `$(`,
// quotes or newlines would turn a copy-paste into code execution on the
// operator's host (native review R1-report-derived-command). Lowercase ASCII
// alphanumerics plus `-` and `_` is all a real toolset name needs
// (code_execution, computer_use, session_search, …).
var toolsetNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// isSafeToolsetName reports whether a report-derived name may be printed inside
// the remediation command.
func isSafeToolsetName(name string) bool {
	return toolsetNamePattern.MatchString(name)
}

// disableCommandToolsets is the exact list the remediation command prints: the
// documented dangerous toolsets first — so the documented command stays
// recognizable — then every extra toolset the runtime report shows as enabled
// beyond the minimal set, in report order. Deriving the extras from the report is
// what keeps the printed command actionable when the runtime ships a toolset the
// documented list does not name yet: the operator disables what the report
// actually shows instead of a stale list.
//
// A derived name is only kept when it looks like a toolset name
// (isSafeToolsetName). Anything else is dropped from the COMMAND only: the report
// itself is still printed verbatim right above it (bounded), so the operator
// loses nothing the runtime said and gains a command that cannot execute the
// report's content.
func disableCommandToolsets(output string) []string {
	names := slices.Clone(dangerousChannelToolsets)
	for _, enabled := range enabledChannelToolsets(output) {
		if !isSafeToolsetName(enabled) {
			continue
		}
		if slices.Contains(minimalChannelToolsets, enabled) || slices.Contains(names, enabled) {
			continue
		}
		names = append(names, enabled)
	}
	return names
}

// boundedRuntimeReport caps the verbatim report at maxRuntimeReportBytes, cutting
// on a line boundary when one exists so no tool entry is left half-printed, and
// appends a marker that names the truncation and the command that prints the
// whole report. A silent cut would be indistinguishable from a complete report.
//
// A report that survives the cut as nothing (it opens with bytes that are not
// UTF-8, so no valid prefix can be printed) gets one fixed explanation line
// instead of an empty body (native review R4-001): a header, a blank line and a
// marker would read as a formatting bug rather than as a report the console
// cannot render.
func boundedRuntimeReport(channel, output string) string {
	trimmed := strings.TrimRight(output, " \t\n")
	if len(trimmed) <= maxRuntimeReportBytes {
		return trimmed
	}
	cut := validUTF8Prefix(trimmed[:maxRuntimeReportBytes])
	if lastNewline := strings.LastIndexByte(cut, '\n'); lastNewline > 0 {
		cut = cut[:lastNewline]
	}
	if cut == "" {
		return runtimeReportUnprintableLine(channel) + "\n" + runtimeReportTruncationMarker(channel, 0)
	}
	return cut + "\n" + runtimeReportTruncationMarker(channel, len(cut))
}

// runtimeReportUnprintableLine replaces an empty report body. Fixed copy only:
// nothing from the runtime is interpolated, and the operator still learns where
// the whole report lives.
func runtimeReportUnprintableLine(channel string) string {
	return fmt.Sprintf(
		"[el reporte no se pudo mostrar porque no es texto UTF-8; corré `%s tools list --platform %s` para verlo completo]",
		hermesBinary, channel)
}

// runtimeReportTruncationMarker names the cut, the number of report bytes that
// were ACTUALLY kept (native review R3-marker-precision: naming the cap told the
// operator a larger number than the block shows) and where the whole report
// lives.
func runtimeReportTruncationMarker(channel string, kept int) string {
	return fmt.Sprintf(
		"[… reporte truncado: se muestran sólo los primeros %d bytes; corré `%s tools list --platform %s` para verlo completo]",
		kept, hermesBinary, channel)
}

// validUTF8Prefix drops a partial rune left by a byte-boundary cut, so the report
// printed to a terminal is always valid UTF-8.
//
// A UTF-8 encoded rune is at most utf8.UTFMax (4) bytes, so a byte-boundary cut
// can leave at most three bytes of ONE broken rune at the tail: walking back over
// them is enough (native review R3-unbounded-validutf8 — the former loop
// re-validated the whole prefix after every byte it dropped, i.e. O(n²)). The
// remaining validity check is a single pass, so the cost is one scan of the
// prefix instead of one scan per dropped byte.
//
// A prefix that is still invalid after the backwalk was never a valid encoding cut
// mid-rune: an invalid byte sits somewhere in the middle of the report. Discarding
// the whole report there would throw away everything the runtime said up to that
// byte, so the forward walk keeps the longest valid prefix — the operator loses the
// corrupted tail, not the report (native review R4-001). A report whose FIRST bytes
// are invalid still yields the empty prefix, which the caller renders as the fixed
// unprintable-report line.
func validUTF8Prefix(s string) string {
	for dropped := 0; dropped < utf8.UTFMax-1 && s != ""; dropped++ {
		if r, size := utf8.DecodeLastRuneInString(s); r != utf8.RuneError || size > 1 {
			break
		}
		s = s[:len(s)-1]
	}
	if utf8.ValidString(s) {
		return s
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return s[:i]
		}
		i += size
	}
	return s
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
