package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/tui"
	"github.com/mattn/go-isatty"
)

// runAdminTUI is the entry point of `mcp-server admin tui`, the operator-facing
// identity and accounts interface (ADR-0016 §5, ADR-0010).
//
// Two presentations share the same reviewed core (internal/admin), and the
// process streams decide which one runs:
//
//   - an interactive terminal gets the Bubble Tea TUI (T7). It owns the whole
//     operator flow, including the first-owner seed gateway as its first screen,
//     and it reads the keys the program needs to render and navigate with.
//   - a non-interactive invocation (piped stdin or stdout, CI, scripted runs)
//     keeps the line-based flow below, which is the only presentation that works
//     without a terminal. It is not dead code: it is the fallback path, and the
//     binary-level dispatch test locks its seed-gateway output.
//
// Both paths validate the configuration and SQLite in the same order, build the
// identity repositories through the shared construction site (newIdentityDeps)
// and render the shared operator facts through the presentation helpers in
// internal/admin (account label, role menu, successor list, recovery hint), so
// the two entry points cannot drift.
func runAdminTUI(ctx context.Context) error {
	if interactiveTerminal() {
		return runAdminTUIProgram(ctx)
	}
	return runAdminTUIFlow(ctx, os.Stdin, os.Stdout)
}

// runAdminTUIProgram opens the interactive Bubble Tea program over the identity
// repositories. The HTTP transport is deliberately not validated: ADR-0016
// Decision 3.5 keeps the TUI independent from it (shared *sql.DB, logger and
// repos only).
func runAdminTUIProgram(ctx context.Context) error {
	deps, err := openCommandDependencies(ctx)
	if err != nil {
		return err
	}
	defer deps.close()

	identity := newIdentityDeps(deps.database, deps.logger)

	// The Hermes bootstrap is resolved lazily, inside "Configurar Hermes", never
	// at startup. The cached resolver pays a successful resolution once and lets
	// a failed one be retried on the next open (R4-001), so an unresolvable home
	// or an invalid MCP_BIND can neither block the whole admin TUI nor stick for
	// the rest of the session.
	resolveHermes := tui.NewCachedHermesResolver(func() (tui.HermesConfig, error) {
		return resolveHermesBootstrap(deps.config.Bind, deps.config.Port)
	})

	// tui.Run keeps a background context on purpose: the program is interactive
	// and a termination signal must not cancel an in-flight operator write
	// mid-flow. ctx only bounded the shared database open above.
	return tui.Run(context.Background(), tui.Deps{
		Accounts:        identity.accounts,
		Professionals:   identity.professionals,
		Clients:         identity.clients,
		BusinessProfile: identity.businessProfiles,
		Hermes:          tui.NewHermesBootstrap(resolveHermes),
		Audit:           identity.audit,
	})
}

// resolveHermesBootstrap composes the two facts of the Hermes bootstrap from the
// already-resolved runtime configuration (ADR-0017 Decision 3): the standard
// ~/.hermes/config.yaml path and the MCP endpoint URL derived from
// MCP_BIND/MCP_PORT with the fixed /mcp path. It returns the one shared
// tui.HermesConfig shape both presentations consume, so the path and the URL
// cannot drift between them (R2-02).
func resolveHermesBootstrap(bind, port string) (tui.HermesConfig, error) {
	path, err := admin.HermesConfigPath()
	if err != nil {
		return tui.HermesConfig{}, err
	}
	endpointURL, err := admin.HermesEndpointURL(bind, port)
	if err != nil {
		return tui.HermesConfig{}, err
	}
	return tui.HermesConfig{Path: path, EndpointURL: endpointURL}, nil
}

// interactiveTerminal reports whether both process streams are attached to a
// terminal, which is what the Bubble Tea program needs to render a frame and
// read keys. Mintty/ConEmu pseudo-terminals on Windows are recognized through
// IsCygwinTerminal, so the TUI does not silently degrade there.
func interactiveTerminal() bool {
	return isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout)
}

// isTerminalFile reports whether one stream is a terminal.
func isTerminalFile(file *os.File) bool {
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// runAdminTUIFlow implements the non-interactive (line-based) presentation: the
// T2 owner seed gateway plus the T3-T6 operator menu (ADR-0016 Decision 1). The
// interactive presentation is the Bubble Tea program in internal/tui, which
// drives the same core; this flow stays because a piped invocation has no
// terminal to render a TUI on.
//
// It:
//
//  1. validates the dependencies serve mode also validates at startup
//     (configuration + SQLite), so a broken install fails here instead of
//     half-opening a TUI;
//  2. constructs the identity repositories through the shared construction
//     site (newIdentityDeps), so the flows inherit production wiring;
//  3. decides whether the first owner must be created — if an active owner
//     already exists it prints a semantic message and opens the menu, never
//     duplicating the account (ownership transfer is T5);
//  4. drives a line-based questionnaire (bufio.Scanner) that re-prompts on
//     invalid input, creates the owner through admin.Seed and writes
//     <config-dir>/caller-id;
//  5. once the seed gate is resolved, opens the numbered operator menu where
//     each capability (Add Staff, Deactivate, List views) is a self-contained
//     action.
//
// One input is shared by the questionnaire and the menu on purpose: two
// scanners over the same stream would let the first one buffer the answers the
// second one needs.
//
// The HTTP transport is deliberately not validated: ADR-0016 Decision 3.5
// keeps this flow independent from it (shared *sql.DB, logger and repos only).
// The interactive presentation (internal/tui, T7) drives the same core with the
// same startup validation; this one exists for the invocations that have no
// terminal to render on.
func runAdminTUIFlow(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	deps, err := openCommandDependencies(ctx)
	if err != nil {
		return err
	}
	defer deps.close()

	identity := newIdentityDeps(deps.database, deps.logger)

	// The console flow is interactive: its admin calls run on a background
	// context so a termination signal never cancels an in-flight write mid-flow.
	// ctx only bounded the shared database open above.
	flowCtx := context.Background()
	input := newConsoleInput(stdin)

	// The Hermes bootstrap facts come from the running server configuration
	// (MCP_BIND/MCP_PORT), never from a hardcoded path or URL, and they are
	// resolved on demand when the operator opens "Configurar Hermes". A bad bind
	// or an unresolvable home therefore only surfaces as the menu's "Error: ..."
	// line, no longer blocks the whole console, and is retried on the next open
	// through the same cached resolver the Bubble Tea program uses (R4-001).
	resolveHermes := tui.NewCachedHermesResolver(func() (tui.HermesConfig, error) {
		return resolveHermesBootstrap(deps.config.Bind, deps.config.Port)
	})

	needsSeed, err := admin.NeedsSeed(flowCtx, identity.accounts)
	if err != nil {
		return err
	}

	if !needsSeed {
		// R4-partial-seed-wedge: a previous run may have created the owner and
		// then failed to write the caller-id file. Repair it before reporting a
		// clean exit, or Hermes keeps failing to resolve a caller id.
		repaired, err := admin.EnsureCallerID(flowCtx, identity.accounts)
		if err != nil {
			return err
		}
		if err := writeConsole(stdout,
			"Ya existe un owner activo: no se crea ninguna cuenta nueva.\n"+
				"La transferencia de propiedad llega en una tarea posterior.\n"); err != nil {
			return err
		}
		if repaired {
			if err := writeConsole(stdout, "Se reparó el archivo caller-id para el owner existente.\n"); err != nil {
				return err
			}
		}
		return runAdminMenu(flowCtx, identity, input, stdout, resolveHermes)
	}

	// R4-deactivated-owner-seed-deadend: "no ACTIVE owner" does not mean "no
	// owner row". When deactivated owner rows exist, the seed questionnaire
	// stalls on the accounts.id PRIMARY KEY the moment the operator retypes that
	// phone, so the reactivation path is offered first.
	inactiveOwners, err := admin.InactiveOwners(flowCtx, identity.accounts)
	if err != nil {
		return err
	}
	if len(inactiveOwners) > 0 {
		if err := runSeedDeadendRecovery(flowCtx, identity, input, stdout, inactiveOwners); err != nil {
			return err
		}
		return runAdminMenu(flowCtx, identity, input, stdout, resolveHermes)
	}

	if err := writeConsole(stdout,
		"No hay ningún owner activo: vamos a crear el primero.\n"+
			"El administrador del sistema operativo es el gatekeeper de este paso (ADR-0010).\n"); err != nil {
		return err
	}

	seedInput, err := promptSeedInput(input, stdout)
	if err != nil {
		return err
	}

	if err := admin.Seed(flowCtx, identity.accounts, seedInput); err != nil {
		if errors.Is(err, admin.ErrOwnerAlreadyExists) {
			// Idempotent outcome: an owner appeared between the decision and
			// the write. Report it semantically and exit clean — never
			// duplicate, never surface a driver error.
			if err := writeConsole(stdout, fmt.Sprintf("No se creó ninguna cuenta: %v\n", err)); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	path, err := admin.WriteCallerID(seedInput.Phone)
	if err != nil {
		return err
	}

	if err := writeConsole(stdout,
		fmt.Sprintf("Owner creado: %s (%s)\n", seedInput.DisplayName, seedInput.Phone)); err != nil {
		return err
	}
	if err := writeConsole(stdout, fmt.Sprintf("caller-id escrito en %s\n", path)); err != nil {
		return err
	}
	if err := writeConsole(stdout, "Ya podés usar `mcp-server hermes chat` con ese caller id.\n"); err != nil {
		return err
	}

	return runAdminMenu(flowCtx, identity, input, stdout, resolveHermes)
}

// runSeedDeadendRecovery closes finding R4-deactivated-owner-seed-deadend: the
// installation has zero ACTIVE owners but its deactivated owner row(s) still
// exist. Blindly re-running the seed questionnaire would stall on the
// accounts.id PRIMARY KEY as soon as the operator retypes that phone, so the
// operator chooses explicitly between reactivating an existing row and creating
// a new owner with a different phone.
//
// The loop is the consent gate: a declined confirmation, or a phone that turns
// out to be taken, re-renders the choice instead of exiting with the system
// still unusable. Only a successful reactivation or creation leaves the loop,
// so the operator menu always opens with an ACTIVE owner in place.
func runSeedDeadendRecovery(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer, inactiveOwners []admin.AccountView) error {
	createKey := len(inactiveOwners) + 1

	for {
		if err := writeConsole(stdout, fmt.Sprintf(
			"No hay ningún owner activo, pero hay %d cuenta(s) de owner desactivada(s).\n"+
				"Podés reactivar una de ellas o crear un owner nuevo con otro teléfono.\n",
			len(inactiveOwners))); err != nil {
			return err
		}
		for i, view := range inactiveOwners {
			if err := writeConsole(stdout,
				fmt.Sprintf("  [%d] Reactivar %s\n", i+1, admin.AccountLabel(view))); err != nil {
				return err
			}
		}
		if err := writeConsole(stdout,
			fmt.Sprintf("  [%d] Crear un owner nuevo con otro teléfono\n", createKey)); err != nil {
			return err
		}

		index, err := promptIndex(input, stdout, "Elegí una opción", createKey)
		if err != nil {
			return err
		}

		if index != createKey {
			done, err := runReactivateOwnerChoice(ctx, identity, input, stdout, inactiveOwners[index-1])
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}

		done, err := runNewOwnerSeed(ctx, identity, input, stdout, inactiveOwners)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// runReactivateOwnerChoice reactivates one deactivated owner row after an
// explicit confirmation and rewrites the caller-id file, because the id Hermes
// must send is the account id of the newly active owner (ADR-0012). done is
// false when the operator declined: nothing was written and the recovery menu
// re-renders.
func runReactivateOwnerChoice(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer, candidate admin.AccountView) (done bool, err error) {
	confirmed, err := promptConfirm(input, stdout,
		fmt.Sprintf("Se reactivará %s como owner activo. ¿Confirmar?", admin.AccountLabel(candidate)))
	if err != nil {
		return false, err
	}
	if !confirmed {
		return false, writeConsole(stdout, "Operación cancelada: no se reactivó ninguna cuenta.\n")
	}

	reactivated, err := admin.ReactivateOwner(ctx, identity.accounts, candidate.ID)
	if err != nil {
		return false, err
	}

	path, err := admin.WriteCallerID(reactivated.ID)
	if err != nil {
		return false, err
	}
	if err := writeConsole(stdout,
		fmt.Sprintf("Owner reactivado: %s (%s)\n", reactivated.DisplayName, reactivated.ID)); err != nil {
		return false, err
	}
	if err := writeConsole(stdout, fmt.Sprintf("caller-id escrito en %s\n", path)); err != nil {
		return false, err
	}
	return true, writeConsole(stdout, "Ya podés usar `mcp-server hermes chat` con ese caller id.\n")
}

// runNewOwnerSeed runs the seed questionnaire inside the deadend recovery. done
// is false when the phone the operator typed already belongs to an account
// (almost always the deactivated owner row): the semantic conflict plus the
// reactivation guidance re-render the choice, so the operator can reactivate
// that account in one step instead of hunting for a different phone.
func runNewOwnerSeed(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer, inactiveOwners []admin.AccountView) (done bool, err error) {
	seedInput, err := promptSeedInput(input, stdout)
	if err != nil {
		return false, err
	}

	if err := admin.Seed(ctx, identity.accounts, seedInput); err != nil {
		if errors.Is(err, admin.ErrOwnerAlreadyExists) {
			if err := writeConsole(stdout, fmt.Sprintf("No se creó ninguna cuenta: %v\n", err)); err != nil {
				return false, err
			}
			if hint := admin.ReactivationHint(inactiveOwners, seedInput.Phone); hint != "" {
				if err := writeConsole(stdout, fmt.Sprintf("%s\n", hint)); err != nil {
					return false, err
				}
			}
			return false, nil
		}
		return false, err
	}

	path, err := admin.WriteCallerID(seedInput.Phone)
	if err != nil {
		return false, err
	}
	if err := writeConsole(stdout,
		fmt.Sprintf("Owner creado: %s (%s)\n", seedInput.DisplayName, seedInput.Phone)); err != nil {
		return false, err
	}
	if err := writeConsole(stdout, fmt.Sprintf("caller-id escrito en %s\n", path)); err != nil {
		return false, err
	}
	return true, writeConsole(stdout, "Ya podés usar `mcp-server hermes chat` con ese caller id.\n")
}

// menuExitKey leaves the operator menu (ADR-0016 §5 keeps `q` as the quit
// binding of the console flow).
const menuExitKey = "q"

// adminMenuAction runs one menu capability. It receives the operator streams so
// each action owns its own prompts. A returned error is rendered by
// runAdminMenu as "Error: <detalle>" and the menu reopens: a business failure
// (conflict, dangling professional, DB error) must never abort the session.
type adminMenuAction func(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error

// adminMenuOption is one numbered capability of the operator menu.
type adminMenuOption struct {
	key    string
	label  string
	action adminMenuAction
}

// adminMenuOptions is the single dispatch table of the console menu. Landing
// T5-T7 appends an entry here instead of adding branches to runAdminTUIFlow, so
// the menu stays trivial to grow. The Hermes entry is the only capability whose
// inputs come from the runtime configuration rather than from the operator
// database, so it resolves them on demand through resolveHermes instead of
// reading them from a package-level path. resolveHermes is the shared
// tui.HermesResolver, so the console and the Bubble Tea program consume the same
// shape and the same retry contract (R2-02, R4-001).
func adminMenuOptions(resolveHermes tui.HermesResolver) []adminMenuOption {
	return []adminMenuOption{
		{key: "1", label: "Add Staff", action: runAddStaffFlow},
		{key: "2", label: "Desactivar cuenta", action: runDeactivateAccountFlow},
		{key: "3", label: "Listar cuentas", action: runListAllAccountsFlow},
		{key: "4", label: "Listar por rol", action: runListByRoleFlow},
		{key: "5", label: "Transferir ownership", action: runTransferOwnershipFlow},
		{key: "6", label: "Agregarme como cliente", action: runAddSelfAsClientFlow},
		{key: "7", label: "Configurar Hermes", action: func(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
			// Lazy resolution: a bad bind or an unresolvable home is returned as
			// a semantic error, which runAdminMenu renders as "Error: ..." and
			// reopens the menu instead of aborting the session.
			hermes, err := resolveHermes()
			if err != nil {
				return err
			}
			return runConfigureHermesFlow(ctx, identity, input, stdout, hermes)
		}},
		{key: "8", label: "Ver audit log", action: runAuditTrailFlow},
		{key: "9", label: "Purgar cuentas desactivadas", action: runPurgeInactiveFlow},
	}
}

// runAuditTrailFlow prints the read-only audit page: the same masked rows the
// Bubble Tea view renders, flat lines for the pipe/winery operator (owner
// decisions OQ-6: últimas 200, read-only, sin paginación). Nothing about the
// flow can write: the core takes a reader-only port and the loop only prints.
func runAuditTrailFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	views, err := admin.ListAuditTrail(ctx, identity.audit, admin.AuditTrailViewLimit)
	if err != nil {
		return err
	}
	if len(views) == 0 {
		return writeConsole(stdout, "El trail de auditoría está vacío: aún no hay mutaciones de cuentas.\n")
	}

	if err := writeConsole(stdout, fmt.Sprintf("Audit log (últimos %d events, read-only):\n", len(views))); err != nil {
		return err
	}
	for _, v := range views {
		if err := writeConsole(stdout, "  "+admin.AuditLine(v)+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// runPurgeInactiveFlow is the console mirror of the TUI purge: list, two
// confirmations (the ordinary one and the permanent one), batch core call and
// per-outcome report. Exactly the same core the TUI uses: only the prompts
// differ.
func runPurgeInactiveFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	views, err := admin.InactiveAccounts(ctx, identity.accounts)
	if err != nil {
		return err
	}
	if len(views) == 0 {
		return writeConsole(stdout, "No hay cuentas desactivadas para purgar.\n")
	}

	if err := writeConsole(stdout, fmt.Sprintf("Cuentas desactivadas (%d):\n", len(views))); err != nil {
		return err
	}
	for _, v := range views {
		if err := writeConsole(stdout, "  "+admin.AccountLabel(v)+"\n"); err != nil {
			return err
		}
	}
	ids := make([]string, 0, len(views))
	for _, v := range views {
		ids = append(ids, v.ID)
	}

	first, err := promptConfirm(input, stdout,
		fmt.Sprintf("Esta acción ELIMINA PERMANENTEMENTE %d cuenta(s) desactivada(s): %s. ¿Confirmar?", len(views), admin.PurgeQuestion(views)))
	if err != nil {
		return err
	}
	if !first {
		return writeConsole(stdout, "Operación cancelada: no se purgó ninguna cuenta.\n")
	}
	second, err := promptConfirm(input, stdout,
		"ÚLTIMA CONFIRMACIÓN: la eliminación es permanente y sin vuelta atrás. ¿Confirmar?")
	if err != nil {
		return err
	}
	if !second {
		return writeConsole(stdout, "Operación cancelada: no se purgó ninguna cuenta.\n")
	}

	outcomes, err := admin.PurgeInactiveAccounts(ctx, identity.accounts, ids)
	// Mid-batch failures keep the completed outcomes visible: the operator
	// must read WHICH accounts were already permanently deleted, then the error.
	// The per-outcome report comes first (masked ids), THEN the sanitized
	// error: raw causes stay server-side, mirroring the TUI flow.
	return writeConsole(stdout, purgeConsoleReport(outcomes, len(ids), err))
}

// purgeConsoleReport composes the console report text: the per-outcome rows
// (masked ids) first, then the sanitized hint on a failed batch. It is split
// out so the wording is unit-testable — identityDeps carries the concrete
// repository, so the flow itself cannot be driven into the error branch.
func purgeConsoleReport(outcomes []admin.PurgeOutcome, requested int, err error) string {
	// B8 f9b (GGA round 2): the count line claims completeness ONLY when the
	// batch finished; a mid-batch abort is a PARTIAL purge (the rows above stay
	// the completed ones, the hint below points at the service log).
	state := "completa"
	if err != nil {
		state = "parcial"
	}
	summary := fmt.Sprintf("Purga %s: %s.\n", state, purgeReportLine(outcomes, requested))
	if err != nil {
		summary += "Resultado parcial: la purga no se completó; revisá el log del servicio.\n"
	}
	return summary
}

// purgeReportLine is the count + per-account phrasing shared with the TUI
// report; the view of every outcome id is already masked at persistence.
func purgeReportLine(outcomes []admin.PurgeOutcome, requested int) string {
	purged := 0
	rows := make([]string, 0, len(outcomes))
	for _, o := range outcomes {
		state := "no purgada"
		if o.Purged {
			purged++
			state = "eliminada permanente"
		}
		rows = append(rows, fmt.Sprintf("  %s: %s", o.Account.ID, state))
	}
	report := fmt.Sprintf("%d cuenta(s) eliminada(s) permanentemente de %d pedidas", purged, requested)
	if len(rows) > 0 {
		report += "\n" + strings.Join(rows, "\n")
	}
	return report
}

// runAdminMenu renders the numbered menu and dispatches the operator's choice
// until they quit or the stream ends. End of input (Ctrl+D or a
// non-interactive invocation) is a clean exit, not an error: an operator who
// only needed the seed must not get a failure for not answering the menu.
func runAdminMenu(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer, resolveHermes tui.HermesResolver) error {
	options := adminMenuOptions(resolveHermes)
	for {
		if err := writeMenu(stdout, options); err != nil {
			return err
		}

		choice, answered, err := readMenuChoice(input, stdout)
		if err != nil {
			return err
		}
		if !answered || choice == menuExitKey {
			return nil
		}

		option, found := findMenuOption(options, choice)
		if !found {
			if err := writeConsole(stdout,
				fmt.Sprintf("Error: opción desconocida %q\n", choice)); err != nil {
				return err
			}
			continue
		}

		if err := option.action(ctx, identity, input, stdout); err != nil {
			// Business errors reopen the menu; only a console that can no
			// longer echo would fail here, and that failure is propagated.
			// The rendered copy is sanitized (issue #157): a semantic error's
			// message passes through, any other error collapses to the generic
			// console line instead of exposing filesystem or driver detail.
			if werr := writeConsole(stdout, fmt.Sprintf("Error: %s\n", consoleErrorText(err))); werr != nil {
				return werr
			}
		}
	}
}

// consoleErrorText is the operator-facing rendering of one console error
// (issue #157): a *domain.SemanticError renders its Spanish message verbatim —
// every business error copy of the admin flows keeps printing exactly as before
// — and any other error collapses to a single generic line, so a filesystem
// path, a driver message or any other internal detail can never reach the
// console. The raw error itself stays at the caller for anything it already
// logs server-side.
func consoleErrorText(err error) string {
	var semErr *domain.SemanticError
	if errors.As(err, &semErr) {
		return semErr.Message
	}
	return "ocurrió un error inesperado: revisá el archivo de configuración del grupo y volvé a intentar"
}

// writeMenu renders the numbered capability list plus the quit option.
func writeMenu(stdout io.Writer, options []adminMenuOption) error {
	if err := writeConsole(stdout, "\n¿Qué querés hacer?\n"); err != nil {
		return err
	}
	for _, option := range options {
		if err := writeConsole(stdout, fmt.Sprintf("  [%s] %s\n", option.key, option.label)); err != nil {
			return err
		}
	}
	return writeConsole(stdout, fmt.Sprintf("  [%s] Salir\n", menuExitKey))
}

// findMenuOption resolves a menu key against the dispatch table.
func findMenuOption(options []adminMenuOption, choice string) (adminMenuOption, bool) {
	for _, option := range options {
		if option.key == choice {
			return option, true
		}
	}
	return adminMenuOption{}, false
}

// readMenuChoice prompts for one menu key. answered is false when the stream
// ended (clean quit) and true when the operator typed something.
func readMenuChoice(input *consoleInput, stdout io.Writer) (choice string, answered bool, err error) {
	if err := writeConsole(stdout, "Seleccioná una opción: "); err != nil {
		return "", false, err
	}
	// GGA round 17/21: EOF/abort stays a clean quit (answered=false, err=nil)
	// exactly as the former Scanner branch did — but a REAL read failure is
	// propagated instead of masquerading as a successful quit.
	line, lerr := input.Line()
	if lerr != nil {
		if errors.Is(lerr, errInputAborted) {
			return "", false, nil
		}
		return "", false, lerr
	}
	return line, true, nil
}

// runAddStaffFlow drives the Add Staff capability (ADR-0016 Decision 1): list
// the active professionals, let the operator pick one by number, prefill the
// phone from professionals.phone (fact 8), ask for the display name and create
// the staff account through admin.AddStaff.
//
// With no active professional there is nothing to link the account to, so the
// flow reports it semantically and returns to the menu instead of failing.
func runAddStaffFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	candidates, err := admin.ListPickableProfessionals(ctx, identity.professionals)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return writeConsole(stdout,
			"No hay profesionales activos: no se puede crear una cuenta de staff.\n")
	}

	if err := writeConsole(stdout, "Profesionales activos:\n"); err != nil {
		return err
	}
	for i, candidate := range candidates {
		if err := writeConsole(stdout, fmt.Sprintf("  [%d] %s\n", i+1, candidate.Name)); err != nil {
			return err
		}
	}

	selected, err := promptIndex(input, stdout, "Elegí un profesional", len(candidates))
	if err != nil {
		return err
	}
	professional := candidates[selected-1]

	phone, err := promptValidated(input, stdout, "Teléfono del staff", professional.Phone, admin.ValidatePhone)
	if err != nil {
		return err
	}

	displayName, err := promptValidated(input, stdout, "Nombre para mostrar", "", admin.ValidateDisplayName)
	if err != nil {
		return err
	}

	if err := admin.AddStaff(ctx, identity.professionals, identity.accounts, admin.StaffInput{
		ProfessionalID: professional.ID,
		Phone:          phone,
		DisplayName:    displayName,
	}); err != nil {
		return err
	}

	return writeConsole(stdout, fmt.Sprintf("Cuenta de staff creada: %s (%s) para el profesional %s\n",
		displayName, phone, professional.Name))
}

// runDeactivateAccountFlow drives the Deactivate capability (ADR-0016
// Decision 1): list the ACTIVE accounts, let the operator pick one by number,
// require an explicit confirmation and soft-delete it through
// admin.DeactivateAccount.
//
// The picker shows role and phone on purpose: the operator must recognise which
// row they are about to deactivate before confirming. The single-owner
// invariant is enforced by the core, so a last-owner attempt returns to the
// menu as a semantic error without writing anything.
//
// The picker snapshot is display-only: admin.DeactivateAccount re-reads the
// accounts before writing, so the guard always runs against the current state
// and not against the list the operator saw.
func runDeactivateAccountFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	views, err := admin.ListAccounts(ctx, identity.accounts, admin.ListFilter{})
	if err != nil {
		return err
	}
	if len(views) == 0 {
		return writeConsole(stdout, "No hay cuentas activas para desactivar.\n")
	}

	if err := writeConsole(stdout, "Cuentas activas:\n"); err != nil {
		return err
	}
	for i, view := range views {
		if err := writeConsole(stdout,
			fmt.Sprintf("  [%d] %s\n", i+1, admin.AccountLabel(view))); err != nil {
			return err
		}
	}

	index, err := promptIndex(input, stdout, "Elegí la cuenta a desactivar", len(views))
	if err != nil {
		return err
	}
	selected := views[index-1]

	confirmed, err := promptConfirm(input, stdout,
		fmt.Sprintf("Esta acción desactiva la cuenta %s. ¿Confirmar?", admin.AccountLabel(selected)))
	if err != nil {
		return err
	}
	if !confirmed {
		return writeConsole(stdout, "Operación cancelada: no se desactivó ninguna cuenta.\n")
	}

	outcome, err := admin.DeactivateAccount(ctx, identity.accounts, selected.ID)
	if err != nil {
		return err
	}
	if outcome.AlreadyInactive {
		return writeConsole(stdout,
			fmt.Sprintf("La cuenta ya estaba inactiva: %s\n", admin.AccountLabel(outcome.Account)))
	}
	return writeConsole(stdout,
		fmt.Sprintf("Cuenta desactivada: %s\n", admin.AccountLabel(outcome.Account)))
}

// runListAllAccountsFlow renders the "all accounts" view (ADR-0016
// Decision 1), asking first whether the soft-deleted rows must show up.
func runListAllAccountsFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	includeInactive, err := promptConfirm(input, stdout, "¿Incluir las cuentas inactivas?")
	if err != nil {
		return err
	}
	return renderAccountList(ctx, identity, stdout, admin.ListFilter{IncludeInactive: includeInactive})
}

// runListByRoleFlow renders the role-filtered view: the operator picks a role
// from the frozen sub-menu and then decides whether inactive rows show up.
func runListByRoleFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	roles := admin.ListableRoles()
	if err := writeConsole(stdout, "Roles disponibles:\n"); err != nil {
		return err
	}
	for i, role := range roles {
		if err := writeConsole(stdout, fmt.Sprintf("  [%d] %s\n", i+1, role)); err != nil {
			return err
		}
	}

	index, err := promptIndex(input, stdout, "Elegí el rol", len(roles))
	if err != nil {
		return err
	}
	role := roles[index-1]

	includeInactive, err := promptConfirm(input, stdout, "¿Incluir las cuentas inactivas?")
	if err != nil {
		return err
	}
	return renderAccountList(ctx, identity, stdout, admin.ListFilter{Role: role, IncludeInactive: includeInactive})
}

// renderAccountList reads the selected view through the core and prints it as a
// read-only table. An empty result is a normal outcome, not an error: a role
// with no account is a legitimate answer to the operator's question.
func renderAccountList(ctx context.Context, identity identityDeps, stdout io.Writer, filter admin.ListFilter) error {
	views, err := admin.ListAccounts(ctx, identity.accounts, filter)
	if err != nil {
		return err
	}
	if len(views) == 0 {
		return writeConsole(stdout, "No hay cuentas para mostrar.\n")
	}
	return writeAccountTable(stdout, views)
}

// writeAccountTable renders the list views as a fixed-width table so the
// operator can scan roles and state without parsing prose. Column widths are
// computed from the rows by rune count (display names carry accents) and every
// cell is written through writeConsole, so the console keeps a single write
// path.
func writeAccountTable(stdout io.Writer, views []admin.AccountView) error {
	headers := [5]string{"TELÉFONO", "ROL", "NOMBRE", "PROFESIONAL", "ESTADO"}
	var widths [5]int
	for i, header := range headers {
		widths[i] = utf8.RuneCountInString(header)
	}

	rows := make([][5]string, 0, len(views))
	for _, view := range views {
		row := [5]string{view.ID, string(view.Role), view.DisplayName, professionalColumn(view), stateColumn(view)}
		for i, cell := range row {
			if width := utf8.RuneCountInString(cell); width > widths[i] {
				widths[i] = width
			}
		}
		rows = append(rows, row)
	}

	if err := writeConsole(stdout,
		fmt.Sprintf("%s\n", formatAccountRow(headers, widths))); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeConsole(stdout, fmt.Sprintf("%s\n", formatAccountRow(row, widths))); err != nil {
			return err
		}
	}
	return nil
}

// formatAccountRow pads every column to its width; a cell longer than its
// column is never truncated, because hiding operator data to keep a table tidy
// would be a silent lie.
func formatAccountRow(cells [5]string, widths [5]int) string {
	padded := make([]string, 0, len(cells))
	for i, cell := range cells {
		padded = append(padded, padCell(cell, widths[i]))
	}
	return strings.Join(padded, "  ")
}

// padCell right-pads a cell with spaces up to width, measured in runes.
func padCell(cell string, width int) string {
	if shortfall := width - utf8.RuneCountInString(cell); shortfall > 0 {
		return cell + strings.Repeat(" ", shortfall)
	}
	return cell
}

// professionalColumn renders the account's professional reference, or "-" when
// the account has none (owner and admin accounts).
func professionalColumn(view admin.AccountView) string {
	if view.ProfessionalID == "" {
		return "-"
	}
	return view.ProfessionalID
}

// stateColumn renders the soft-delete (is_active) state in the operator's
// language instead of the raw 0/1 of the column.
func stateColumn(view admin.AccountView) string {
	if view.Active {
		return "activa"
	}
	return "inactiva"
}

// promptIndex reads a 1-based index into a numbered list and re-prompts while
// the answer is not a number inside the range. label names the list, so the
// same helper serves the professional, account and role pickers.
func promptIndex(input *consoleInput, stdout io.Writer, label string, count int) (int, error) {
	for {
		if err := writeConsole(stdout, fmt.Sprintf("%s [1-%d]: ", label, count)); err != nil {
			return 0, err
		}
		line, lerr := input.Line()
		if lerr != nil {
			return 0, lerr
		}
		choice, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || choice < 1 || choice > count {
			if err := writeConsole(stdout,
				fmt.Sprintf("Error: opción inválida: elegí un número entre 1 y %d\n", count)); err != nil {
				return 0, err
			}
			continue
		}
		return choice, nil
	}
}

// writeConsole renders one operator-facing chunk to stdout. Discarding the
// write error would let a broken console masquerade as a successful run, so
// the failure is propagated as a semantic error: an operator flow that cannot
// echo its prompts has no usable environment.
//
// There is no dynamically-typed boundary left: callers format at the call site
// and hand this function a finished string, so a plain string carries the whole
// contract. The format step stays go vet checked, because fmt.Sprintf is the
// first thing the printf checker validates at each call site.
func writeConsole(stdout io.Writer, text string) error {
	if _, err := io.WriteString(stdout, text); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo escribir en la consola del operador",
			Cause:   err,
		}
	}
	return nil
}

// promptConfirm renders a yes/no question and returns true only for an explicit
// affirmative answer. Anything that is neither a yes nor a no is re-prompted: a
// typo must never be read as consent for a destructive action. A "no" is a
// decision, not an error — the caller decides what the cancellation prints.
//
// An interrupted stream aborts the flow with the same semantic error as the
// other prompts: nothing is written on a half-answered confirmation.
func promptConfirm(input *consoleInput, stdout io.Writer, question string) (bool, error) {
	for {
		if err := writeConsole(stdout, fmt.Sprintf("%s (s/n): ", question)); err != nil {
			return false, err
		}

		line, lerr := input.Line()
		if lerr != nil {
			return false, lerr
		}

		switch strings.ToLower(strings.TrimSpace(line)) {
		case "s", "si", "sí":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			if err := writeConsole(stdout, "Error: respondé \"s\" o \"n\"\n"); err != nil {
				return false, err
			}
		}
	}
}

// promptSeedInput drives the minimal line-based seed questionnaire. The
// professional picker belongs to the Add Staff flow.
func promptSeedInput(input *consoleInput, stdout io.Writer) (admin.SeedInput, error) {
	phone, err := promptValidated(input, stdout, "Teléfono del owner (ej. +5491100000000)", "", admin.ValidatePhone)
	if err != nil {
		return admin.SeedInput{}, err
	}

	displayName, err := promptValidated(input, stdout, "Nombre para mostrar", "", admin.ValidateDisplayName)
	if err != nil {
		return admin.SeedInput{}, err
	}

	return admin.SeedInput{Phone: phone, DisplayName: displayName}, nil
}

// promptValidated prints label, reads one line and re-prompts while validate
// rejects the answer. When defaultValue is not empty it is rendered between
// brackets and accepted by pressing Enter; the value handed to validate is
// already defaulted, so the single validation point stays authoritative.
// Rejections are rendered as "Error: <detalle semántico>" so the operator reads
// the business message, never a parser dump.
//
// A stream that ends before a valid answer aborts the flow with a semantic
// error: an interrupted questionnaire must not create a half-built account.
// promptSecret reads one secret value without terminal echo (GGA round 14):
// on a real terminal the read goes through input.Secret()'s bounded no-echo
// reader (echo disabled, newline consumed, oversized paste rejected and
// drained); with piped stdin the consoleSource path is
// echo-free by construction. B8 f6 (owner: limpieza): the separate
// secretLineReader/stdinSecretLine hook never ran — input.Secret() already owns
// the tty/piped split, so the dead plumbing is removed and the single code path
// stands. Injectability comes from the consoleSource/reader split: tests pipe
// stdin, so no special hook is needed — a terminal exercise is a manual smoke.
// Together with the Bubble Tea form's sensitive field this keeps both
// presentations' secret handling symmetric.
// promptSecret collects a credential (the Telegram bot token). allowEmpty marks
// the answer as optional: an empty line returns "" without validation, which the
// caller reads as "skip this optional field group" (S11). Any other answer must
// pass validate before it is accepted.
func promptSecret(input *consoleInput, stdout io.Writer, label string, validate func(string) error, allowEmpty bool) (string, error) {
	for {
		if err := writeConsole(stdout, label+": "); err != nil {
			return "", err
		}
		line, serr := input.Secret()
		if serr != nil {
			return "", serr
		}

		value := strings.TrimSpace(line)
		if value == "" && allowEmpty {
			return "", nil
		}
		if err := validate(line); err != nil {
			if err := writeConsole(stdout, fmt.Sprintf("Error: %v\n", err)); err != nil {
				return "", err
			}
			continue
		}
		return line, nil
	}
}

func promptValidated(input *consoleInput, stdout io.Writer, label, defaultValue string, validate func(string) error) (string, error) {
	for {
		if defaultValue != "" {
			if err := writeConsole(stdout, fmt.Sprintf("%s [%s]: ", label, defaultValue)); err != nil {
				return "", err
			}
		} else if err := writeConsole(stdout, fmt.Sprintf("%s: ", label)); err != nil {
			return "", err
		}

		line, lerr := input.Line()
		if lerr != nil {
			return "", lerr
		}

		value := strings.TrimSpace(line)
		if value == "" && defaultValue != "" {
			value = defaultValue
		}
		if err := validate(value); err != nil {
			if err := writeConsole(stdout, fmt.Sprintf("Error: %v\n", err)); err != nil {
				return "", err
			}
			continue
		}
		return value, nil
	}
}

// errInputAborted reports an interrupted operator questionnaire (Ctrl+D or a
// non-interactive invocation). It is never an interactive re-prompt: there is
// no more input to read.
// consoleInput is the single input abstraction of the console flows (GGA
// round 17): it owns BOTH ordinary lines and no-echo secret reads, so a
// secret prompt can never bypass input another prompt already has buffered.
//
// Which reader owns the stream depends on the mode (B14):
//
//   - piped stdin has exactly one consumer, so the buffered *bufio.Reader is
//     safe and a secret is an ordinary Line (echo-free by construction).
//   - a real terminal has two consumers — the text prompt and
//     term.ReadPassword — draining ONE kernel queue through the same fd. Any
//     read-ahead in the text path would swallow bytes the secret prompt must
//     read (and vice versa: a secret can never leave stale buffered state
//     behind), so the terminal path never buffers: it reads the fd byte by
//     byte and the no-echo read takes the same fd raw. Text → secret → text
//     therefore behaves exactly like text → text.
//
// All prompts share the same *consoleInput, so the ownership decision is made
// once per flow and cannot drift between prompt kinds.
type consoleInput struct {
	src  *bufio.Reader
	term *consoleTerm // nil unless stdin is the process terminal
}

// newConsoleInput wraps the console/pipe input source. Only a real process
// stdin that is a terminal marks the terminal mode; tests and scripts keep the
// plain reader path.
func newConsoleInput(stdin io.Reader) *consoleInput {
	c := &consoleInput{src: bufio.NewReader(stdin)}
	if f, ok := stdin.(*os.File); ok && f == os.Stdin && term.IsTerminal(f.Fd()) {
		c.term = newConsoleTerm(f)
	}
	return c
}

// consoleTerm is the terminal-side read path: the shared, non-prefetching
// owner of the terminal fd. read and password are fields (not method bodies)
// because a real termios read cannot be faked in a unit test without a pty; the
// production wiring below is the only place that touches the kernel, and the
// B14 regression test injects the same fd contract over a shared queue.
type consoleTerm struct {
	read     func([]byte) (int, error)
	password func() ([]byte, error)
}

// newConsoleTerm binds the terminal read path to the process terminal file.
func newConsoleTerm(f *os.File) *consoleTerm {
	return &consoleTerm{
		read:     f.Read,
		password: func() ([]byte, error) { return readTerminalSecret(f, consoleInputLineMax) },
	}
}

// consoleInputLineMax bounds one operator line: every prompt value the
// console collects fits far below it, and a hostile or pasted stream cannot
// make ReadString allocate unbounded memory (GGA round 21).
const consoleInputLineMax = 4096

// errConsoleLineTooLong is the semantic rejection of an over-long operator
// line; it never echoes the rejected bytes.
func errConsoleLineTooLong() *domain.SemanticError {
	return &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: "la entrada del operador excede la longitud máxima permitida",
	}
}

// Line reads one bounded operator line incrementally (GGA round 23): no
// single unbounded read happens, an oversized line is rejected with its
// remainder drained (so it cannot become the next prompt's answer), and EOF
// without a newline still returns the carried text. The shared input-abort
// contract covers EOF and reader failures.
//
// On a terminal the read is byte-wise (B14): the terminal fd is the shared
// queue of the text and the secret prompts, so the text path must never hold
// bytes the secret path has to read.
func (c *consoleInput) Line() (string, error) {
	if c.term != nil {
		return c.term.Line()
	}
	return collectConsoleLine(func() ([]byte, error) {
		return c.src.ReadSlice('\n')
	})
}

// Line reads one bounded operator line from the terminal, one byte at a time.
// The extra syscalls are bounded by consoleInputLineMax per line and only
// happen on the interactive path, which is the price of never owning bytes the
// no-echo read still needs (B14).
func (t *consoleTerm) Line() (string, error) {
	return collectConsoleLine(func() ([]byte, error) {
		var one [1]byte
		n, rerr := t.read(one[:])
		return one[:n], rerr
	})
}

// collectConsoleLine accumulates one bounded line from successive pieces. The
// piece source decides the prefetch behaviour: bufio chunks for the single-
// consumer pipe path, single bytes for the shared terminal fd (B14).
func collectConsoleLine(next func() ([]byte, error)) (string, error) {
	var collected []byte
	oversized := false
	for {
		piece, ferr := next()
		// A read that yields neither bytes nor EOF is a no-progress read
		// (GGA round 1 of the #157 console round, e.g. Ctrl+D at an empty
		// line): silently skipping it would re-read forever on a source that
		// keeps returning nothing, so the prompt fails immediately — the same
		// fail-secure answer the secret path's consoleByteSource applies.
		if len(piece) == 0 && ferr == nil {
			return "", fmt.Errorf("leer la entrada del operador: %w", io.ErrNoProgress)
		}
		// A line ends when the piece carries the delimiter; the byte-wise
		// terminal path therefore signals the end of a line with the same
		// predicate the buffered path gets from ReadSlice.
		newline := len(piece) > 0 && piece[len(piece)-1] == '\n'
		if len(piece) > 0 && !oversized {
			if len(collected)+len(piece) > consoleInputLineMax {
				oversized = true
			} else {
				collected = append(collected, piece...)
			}
		}
		if ferr != nil {
			if errors.Is(ferr, io.EOF) {
				if oversized {
					return "", errConsoleLineTooLong()
				}
				// A carried final line without newline is still content.
				if len(collected) > 0 {
					return string(collected), nil
				}
				return "", errInputAborted
			}
			if errors.Is(ferr, bufio.ErrBufferFull) {
				continue
			}
			return "", fmt.Errorf("leer la entrada del operador: %w", ferr)
		}
		if !newline {
			continue
		}
		if oversized {
			return "", errConsoleLineTooLong()
		}
		return strings.TrimSpace(string(collected)), nil
	}
}

// Secret reads one secret value. On a terminal it is the no-echo path on the
// same fd the text path reads (no buffering in between, so a secret can never
// be starved by a previous text prompt nor leave stale bytes behind); on piped
// stdin it is an ordinary Line (echo-free by construction, and no buffered
// line can be bypassed because both kinds share this reader).
func (c *consoleInput) Secret() (string, error) {
	if c.term == nil {
		return c.Line()
	}
	raw, err := c.term.password()
	if err != nil {
		var semErr *domain.SemanticError
		if errors.As(err, &semErr) {
			return "", err
		}
		return "", fmt.Errorf("leer la entrada secreta: %w", err)
	}
	// The no-echo read is bounded while reading on both platforms (issue #154
	// closed the Windows gap); this re-check stays as the second layer of
	// defense in depth, so a future reader that collects the line itself cannot
	// hand an oversized value to the caller (GGA round 1).
	if len(raw) > consoleInputLineMax {
		return "", errConsoleLineTooLong()
	}
	return strings.TrimSpace(string(raw)), nil
}

// readTerminalSecret reads one no-echo secret line under a hard byte limit
// (GGA round 1: a secret must be bounded while reading, not after the fact, so
// an oversized paste can never allocate unbounded memory).
//
// Both platforms run the same bounded accumulator (readSecretBounded) inside
// the same orchestration (readSecretWithNoEcho); only the "no echo" primitive
// is platform-specific and lives behind readSecretNoEcho in a build-tagged
// sibling file (admin_console_secret_windows.go / admin_console_secret_other.go).
//
// The no-echo mode drops the line discipline, so the accumulator emulates the
// erase key and maps Ctrl+C/Ctrl+D to the shared cancellation error instead of
// a signal — where the platform delivers those bytes as input (see the Windows
// hook for the one place it does not, issue #154).
func readTerminalSecret(f *os.File, limit int) ([]byte, error) {
	return readSecretNoEcho(f, limit)
}

// readSecretWithNoEcho is the one secret read both platforms execute: it
// engages the platform no-echo mode before the first byte is read (a secret is
// never collected with echo on), consumes the fd byte by byte through the
// bounded accumulator, and restores the console mode on every exit path, error
// included, through the deferred restore.
func readSecretWithNoEcho(limit int, enter func() (func(), error), read func([]byte) (int, error)) ([]byte, error) {
	restore, err := enter()
	if err != nil {
		// Without the no-echo mode the secret would be echoed: fail, never read.
		return nil, err
	}
	defer restore()
	return readSecretBounded(limit, consoleByteSource(read))
}

// consoleByteSource turns an fd read into the single-byte piece source the
// bounded accumulator consumes. A read that reports neither bytes nor an error
// becomes io.ErrNoProgress instead of an empty piece: the accumulator would
// otherwise spin forever on a console that keeps returning nothing, and a
// no-progress read is a failure of the secret read, not an empty secret.
func consoleByteSource(read func([]byte) (int, error)) func() ([]byte, error) {
	return func() ([]byte, error) {
		var one [1]byte
		n, rerr := read(one[:])
		if n == 0 && rerr == nil {
			return nil, io.ErrNoProgress
		}
		return one[:n], rerr
	}
}

// readSecretBounded accumulates one no-echo secret line from a raw byte source:
// it stops at limit bytes, drains the remainder of an oversized line (so the
// leftover can never become the next prompt's answer, the same contract Line()
// keeps), emulates the erase key raw mode removes, and maps the abort keys to
// the shared cancellation error.
func readSecretBounded(limit int, next func() ([]byte, error)) ([]byte, error) {
	var collected []byte
	oversized := false
	aborted := false
	for {
		piece, ferr := next()
		for _, b := range piece {
			switch {
			case b == '\r' || b == '\n':
				// Raw mode delivers the Enter key as a bare CR on POSIX and
				// Windows. The abort drain ends at the aborted line's OWN
				// terminator: bytes after it belonged to the NEXT prompt all
				// along (GGA round 1 of issue #154 — a queued suffix can never
				// become a later prompt's answer, the same isolation the
				// oversized drain keeps).
				if aborted {
					return nil, errInputAborted
				}
				if oversized {
					return nil, errConsoleLineTooLong()
				}
				return collected, nil
			case b == 0x03 || b == 0x04: // Ctrl+C / Ctrl+D: no signal in raw mode
				// Aborting the line drops the rest of it too: until its own
				// terminator, everything queued after the abort byte belongs to
				// the aborted prompt and is drained, never stored or forwarded.
				aborted = true
				collected = nil // never keep a partial aborted secret
			case b == 0x7f || b == 0x08: // erase key of the dropped line discipline
				if len(collected) > 0 {
					collected = collected[:len(collected)-1]
				}
			case oversized || aborted:
				// Draining the rejected/aborted line: its bytes are dropped,
				// never stored.
			default:
				if len(collected)+1 > limit {
					oversized = true
					collected = nil // never keep a partial oversized secret
					continue
				}
				collected = append(collected, b)
			}
		}
		if ferr != nil {
			if errors.Is(ferr, io.EOF) {
				if aborted {
					return nil, errInputAborted
				}
				if oversized {
					return nil, errConsoleLineTooLong()
				}
				// A carried final line without newline is still content.
				if len(collected) > 0 {
					return collected, nil
				}
				return nil, errInputAborted
			}
			return nil, ferr
		}
	}
}

var errInputAborted = &domain.SemanticError{
	Code:    domain.ErrCodeInvalidInput,
	Message: "se canceló la operación: no se recibió la entrada necesaria",
}

// transferSuccessorOption is one numbered successor candidate of the transfer
// picker: an existing account row or the "new phone" escape hatch. The extra
// fields only carry operator input for the new-phone case.
type transferSuccessorOption struct {
	kind        admin.SuccessorKind
	id          string
	phone       string
	displayName string
	label       string
}

// runTransferOwnershipFlow drives the Transfer Ownership capability (ADR-0016
// Decision 1) as the explicit two-step flow that respects the single-owner
// invariant of ADR-0009:
//
//	Step 1 of 2 — pick the successor: an existing INACTIVE owner row, an active
//	staff account (promoted to owner), or a brand-new phone (a fresh owner row
//	is created inactive).
//	Step 2 of 2 — confirm and swap: admin.PrepareSuccessor leaves a role=owner,
//	is_active=0 row behind and admin.TransferOwnership deactivates the current
//	owner plus activates the successor inside one transaction.
//
// The caller-id file is rewritten to the new owner before returning: the old
// owner's account is now inactive and the resolver rejects inactive accounts
// ("tu cuenta está deshabilitada"), so keeping the stale id would break every
// authenticated MCP call (ADR-0012). This closes the ADR-0016 chicken-and-egg
// note: the TUI is the only legitimate owner-management path.
func runTransferOwnershipFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	owner, err := admin.ActiveOwner(ctx, identity.accounts)
	if err != nil {
		return err
	}

	if err := writeConsole(stdout, "Paso 1 de 2: elegir el sucesor.\n"); err != nil {
		return err
	}
	if err := writeConsole(stdout,
		fmt.Sprintf("Owner actual: %s\n", admin.AccountLabel(owner))); err != nil {
		return err
	}

	options, err := transferSuccessorOptions(ctx, identity)
	if err != nil {
		return err
	}
	for i, option := range options {
		if err := writeConsole(stdout, fmt.Sprintf("  [%d] %s\n", i+1, option.label)); err != nil {
			return err
		}
	}

	index, err := promptIndex(input, stdout, "Elegí el sucesor", len(options))
	if err != nil {
		return err
	}
	chosen := options[index-1]

	if chosen.kind == admin.SuccessorNewPhone {
		phone, err := promptValidated(input, stdout, "Teléfono del nuevo owner", "", admin.ValidatePhone)
		if err != nil {
			return err
		}
		displayName, err := promptValidated(input, stdout, "Nombre para mostrar", "", admin.ValidateDisplayName)
		if err != nil {
			return err
		}
		chosen.phone = phone
		chosen.displayName = displayName
		chosen.label = fmt.Sprintf("%s (%s)", displayName, phone)
	}

	if chosen.kind == admin.SuccessorStaff {
		// Promote is a role switch, not a copy: the operator must know the
		// account stops being a staff account before confirming.
		if err := writeConsole(stdout,
			"Al promover una cuenta de staff, la cuenta pierde su vínculo con el profesional.\n"); err != nil {
			return err
		}
	}

	if err := writeConsole(stdout, "Paso 2 de 2: confirmar la transferencia.\n"); err != nil {
		return err
	}
	confirmed, err := promptConfirm(input, stdout, "El owner actual quedará desactivado. ¿Confirmar?")
	if err != nil {
		return err
	}
	if !confirmed {
		return writeConsole(stdout, "Operación cancelada: no se transfirió la propiedad.\n")
	}

	successor, err := admin.PrepareSuccessor(ctx, identity.accounts, admin.TransferSuccessor{
		Kind:        chosen.kind,
		ID:          chosen.id,
		Phone:       chosen.phone,
		DisplayName: chosen.displayName,
	})
	if err != nil {
		return err
	}

	outcome, err := admin.TransferOwnership(ctx, identity.accounts, owner.ID, successor.ID)
	if err != nil {
		return err
	}

	path, err := admin.WriteCallerID(outcome.To.ID)
	if err != nil {
		return err
	}

	if err := writeConsole(stdout, fmt.Sprintf("Ownership transferido: %s → %s\n",
		admin.AccountLabel(outcome.From), admin.AccountLabel(outcome.To))); err != nil {
		return err
	}
	return writeConsole(stdout, fmt.Sprintf("caller-id actualizado en %s\n", path))
}

// transferSuccessorOptions maps the shared core successor list (internal/admin)
// onto the console's own option struct. The console keeps its struct because it
// also carries the operator input of the new-phone case; the ordering, the
// eligibility and the labels are single-sourced in the core.
func transferSuccessorOptions(ctx context.Context, identity identityDeps) ([]transferSuccessorOption, error) {
	candidates, err := admin.SuccessorCandidates(ctx, identity.accounts)
	if err != nil {
		return nil, err
	}

	options := make([]transferSuccessorOption, 0, len(candidates))
	for _, candidate := range candidates {
		options = append(options, transferSuccessorOption{
			kind:  candidate.Kind,
			id:    candidate.ID,
			label: candidate.Label,
		})
	}
	return options, nil
}

// runAddSelfAsClientFlow drives the "Agregarme como cliente" capability
// (ADR-0011, ADR-0016 Decision 1): it registers the operator's own account as a
// client of the business, so the caller id Hermes sends resolves into a Caller
// with BOTH the owner/admin/staff role and a ClientID (ADR-0011 double role).
//
// The account id is read from the accounts port — the ACTIVE owner of this
// installation, not a typed value — and shown to the operator before the flow
// asks for the display name; admin.AddSelfAsClient re-validates it against the
// same port, so the console cannot invent an id. Registering an
// already-registered phone is an outcome, not an error: the operator reads the
// honest state and returns to the menu with the session intact.
func runAddSelfAsClientFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer) error {
	owner, err := admin.ActiveOwner(ctx, identity.accounts)
	if err != nil {
		return err
	}

	if err := writeConsole(stdout, fmt.Sprintf(
		"Se va a registrar tu cuenta como cliente del negocio.\n"+
			"Tu teléfono de cuenta es: %s\n", owner.ID)); err != nil {
		return err
	}

	displayName, err := promptValidated(input, stdout, "Nombre para mostrar del cliente", "", admin.ValidateDisplayName)
	if err != nil {
		return err
	}

	outcome, err := admin.AddSelfAsClient(ctx, identity.accounts, identity.clients, owner.ID, displayName)
	if err != nil {
		return err
	}
	if outcome.AlreadyRegistered {
		return writeConsole(stdout, "Ya estabas registrado como cliente con este teléfono.\n")
	}
	return writeConsole(stdout, "Ya podés operar como cliente con este teléfono.\n")
}

// runConfigureHermesFlow drives the "Configurar Hermes" capability (ADR-0017,
// ADR-0018 Decision 3b): it prefills the server phone from the ACTIVE owner,
// collects the OPTIONAL Telegram owner-bot field group (bot token, allowlist chat
// id and owner phone) with the same admin validators the Bubble Tea wizard uses,
// shows the bootstrap facts with the token masked, asks for an explicit
// confirmation and then merges and writes the mcp_servers.mcp-appointments entry.
// The server entry and the Telegram env block are merged in one atomic write
// through the shared tui.ApplyHermesConfigWithTelegram chain. It mirrors the
// Bubble Tea wizard 1:1; the resolved Hermes facts arrive through the menu
// closure, so the console never re-derives the path or the endpoint.
//
// Skip and removal semantics (S11): an empty bot token disables the Telegram
// group, so the flow asks nothing else about the token/chat pair and leaves any
// pre-existing MCP_TELEGRAM_* block untouched, reporting the neutral skip
// summary "Telegram: omitido (sin cambios en el bloque de Telegram)" (B15 C3).
// The closed group is then resolved explicitly: the flow
// asks "¿Retirar el canal Telegram del bot?", and a yes DELETES a pre-existing
// block ("Telegram: canal deshabilitado (bloque MCP_TELEGRAM eliminado)") while a
// no preserves it. A token present keeps the chat id and the owner phone
// mandatory.
//
// Without an ACTIVE owner the flow has nothing to prefill and no owner id Hermes
// could send, so it reports the semantic failure and the menu reopens.
//
// A failed write chain is never a dead end: the exact YAML snippet the writer
// would have produced is printed with the target path and the semantic error, so
// the operator can finish the bootstrap by hand (ADR-0017 Decision 1). The chain
// itself lives in tui.ApplyHermesConfigWithTelegram, the same helper the Bubble
// Tea wizard runs, so the console never re-derives the path or the endpoint.
func runConfigureHermesFlow(ctx context.Context, identity identityDeps, input *consoleInput, stdout io.Writer, hermes tui.HermesConfig) error {
	owner, err := admin.ActiveOwner(ctx, identity.accounts)
	if err != nil {
		return err
	}

	phone, err := promptValidated(input, stdout, "Teléfono del owner (X-Caller-Id)", owner.ID, admin.ValidatePhone)
	if err != nil {
		return err
	}

	token, err := promptSecret(input, stdout,
		"Token del bot de Telegram (formato <id>:<hash>) - no se verá al escribir (Enter para omitir: canal deshabilitado)",
		admin.ValidateTelegramBotToken, true)
	if err != nil {
		return err
	}

	// S11 (owner 2026-10-19 "skip + deshabilitar"): the Telegram owner bot is an
	// optional channel, so an empty token closes the whole group — the chat id and
	// the Telegram owner phone are not asked and the entry is written without the
	// MCP_TELEGRAM_* block. A token that is present keeps today's required pair:
	// the chat id and the owner phone stay mandatory and are validated as before.
	//
	// A closed group still has TWO shapes, and the flow asks which one the
	// operator means (S11 unit B): skipping preserves a block the config already
	// has (SetHermesServer carries it over), while the explicit removal deletes
	// it. The question is asked for every closed group because the answer is the
	// operator's intent — the removal is idempotent, so answering "s" on a host
	// that never configured the bot changes nothing and the summary stays
	// accurate as a post-condition.
	telegram := tui.TelegramFields{}
	if strings.TrimSpace(token) != "" {
		chatID, err := promptValidated(input, stdout, "Chat id de la allowlist de Telegram (ej. 987654321)", "", admin.ValidateTelegramChatID)
		if err != nil {
			return err
		}
		telegramPhone, err := promptValidated(input, stdout, "Teléfono del owner para Telegram (X-Caller-Id)", owner.ID, admin.ValidatePhone)
		if err != nil {
			return err
		}
		telegram = tui.NewTelegramFields(token, chatID, telegramPhone)
	} else {
		remove, err := promptConfirm(input, stdout, "¿Retirar el canal Telegram del bot? (sí borra el bloque MCP_TELEGRAM_* del config; no conserva el que ya existía)")
		if err != nil {
			return err
		}
		if remove {
			telegram = tui.TelegramRemoval()
		}
	}

	if err := writeHermesFacts(stdout, hermes, phone, telegram); err != nil {
		return err
	}

	confirmed, err := promptConfirm(input, stdout,
		"Se escribirá la entrada mcp-appointments en el config de Hermes. ¿Confirmar?")
	if err != nil {
		return err
	}
	if !confirmed {
		return writeConsole(stdout, "Operación cancelada: no se cambió la configuración de Hermes.\n")
	}

	snippet, err := tui.ApplyHermesConfigWithTelegram(hermes, phone, telegram)
	if err == nil {
		if err := writeConsole(stdout, "Configuración de Hermes actualizada.\n"); err != nil {
			return err
		}
		if err := writeHermesFacts(stdout, hermes, phone, telegram); err != nil {
			return err
		}
		// ADR-0020 §4 cascade: the skill regeneration runs as part of option 7,
		// with the same outcomes the installer prints. A modified skill asks
		// before replacing; a failure surfaces but never breaks the config flow
		// (the write already succeeded).
		outcome, skillErr := tui.RegenerateSkillForConsole(ctx, hermes, identity.businessProfiles, func() (bool, error) {
			return promptConfirm(input, stdout,
				"La skill recepcionista instalada difiere del template del sistema. ¿Reemplazarla? Se descartan los cambios manuales. Puede tratarse de una modificación de Hermes (procedural memory) o manual.")
		})
		if skillErr == nil {
			return writeConsole(stdout, "Skill recepcionista: "+string(outcome)+"\nHermes recarga el índice de skills en su próximo arranque.\n")
		}
		// Sanitized like every console print (issue #157): the regeneration
		// failure may be a plain filesystem error with a config path on it.
		if werr := writeConsole(stdout, fmt.Sprintf("Error: %s\n", consoleErrorText(skillErr))); werr != nil {
			return werr
		}
		return nil
	}
	return writeHermesSnippetFallback(stdout, hermes, snippet, err)
}

// writeHermesFacts renders the facts of the bootstrap — endpoint, server phone,
// the masked Telegram owner-bot summary and target file — in the same order and
// wording as the Bubble Tea confirmation and success screens, so the two
// presentations report the same contract. The token is masked through the
// shared tui.TelegramFields.Summary, so it is never echoed.
func writeHermesFacts(stdout io.Writer, hermes tui.HermesConfig, phone string, telegram tui.TelegramFields) error {
	if err := writeConsole(stdout, fmt.Sprintf("Endpoint: %s\n", hermes.EndpointURL)); err != nil {
		return err
	}
	if err := writeConsole(stdout, fmt.Sprintf("Teléfono (X-Caller-Id): %s\n", phone)); err != nil {
		return err
	}
	if err := writeConsole(stdout, fmt.Sprintf("Telegram: %s\n", telegram.Summary())); err != nil {
		return err
	}
	return writeConsole(stdout, fmt.Sprintf("Archivo: %s\n", hermes.Path))
}

// writeHermesSnippetFallback renders the manual path of ADR-0017 Decision 1 when
// the write chain failed: the exact YAML block the shared chain (R2-01) already
// rendered, the file it belongs in, and the semantic error that forced the
// fallback. The error is returned so runAdminMenu renders it as "Error:
// <detalle>" and reopens the menu.
func writeHermesSnippetFallback(stdout io.Writer, hermes tui.HermesConfig, snippet string, writeErr error) error {
	if err := writeConsole(stdout, "Configuración manual de Hermes\n"); err != nil {
		return err
	}
	if err := writeConsole(stdout,
		"No se pudo escribir el archivo automáticamente. Copiá este bloque en el archivo indicado. El bloque de Telegram quedó sin aplicar: re-corre la opción 7 para completar esos tres campos.\n"); err != nil {
		return err
	}
	if err := writeConsole(stdout, fmt.Sprintf("%s\n", hermes.Path)); err != nil {
		return err
	}
	if snippet != "" {
		if err := writeConsole(stdout, snippet); err != nil {
			return err
		}
	}
	return writeErr
}
