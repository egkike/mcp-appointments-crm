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
		Accounts:      identity.accounts,
		Professionals: identity.professionals,
		Clients:       identity.clients,
		Hermes:        tui.NewHermesBootstrap(resolveHermes),
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
// One scanner is shared by the questionnaire and the menu on purpose: two
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
	scanner := bufio.NewScanner(stdin)

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
		return runAdminMenu(flowCtx, identity, scanner, stdout, resolveHermes)
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
		if err := runSeedDeadendRecovery(flowCtx, identity, scanner, stdout, inactiveOwners); err != nil {
			return err
		}
		return runAdminMenu(flowCtx, identity, scanner, stdout, resolveHermes)
	}

	if err := writeConsole(stdout,
		"No hay ningún owner activo: vamos a crear el primero.\n"+
			"El administrador del sistema operativo es el gatekeeper de este paso (ADR-0010).\n"); err != nil {
		return err
	}

	input, err := promptSeedInput(scanner, stdout)
	if err != nil {
		return err
	}

	if err := admin.Seed(flowCtx, identity.accounts, input); err != nil {
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

	path, err := admin.WriteCallerID(input.Phone)
	if err != nil {
		return err
	}

	if err := writeConsole(stdout,
		fmt.Sprintf("Owner creado: %s (%s)\n", input.DisplayName, input.Phone)); err != nil {
		return err
	}
	if err := writeConsole(stdout, fmt.Sprintf("caller-id escrito en %s\n", path)); err != nil {
		return err
	}
	if err := writeConsole(stdout, "Ya podés usar `mcp-server hermes chat` con ese caller id.\n"); err != nil {
		return err
	}

	return runAdminMenu(flowCtx, identity, scanner, stdout, resolveHermes)
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
func runSeedDeadendRecovery(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer, inactiveOwners []admin.AccountView) error {
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

		index, err := promptIndex(scanner, stdout, "Elegí una opción", createKey)
		if err != nil {
			return err
		}

		if index != createKey {
			done, err := runReactivateOwnerChoice(ctx, identity, scanner, stdout, inactiveOwners[index-1])
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}

		done, err := runNewOwnerSeed(ctx, identity, scanner, stdout, inactiveOwners)
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
func runReactivateOwnerChoice(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer, candidate admin.AccountView) (done bool, err error) {
	confirmed, err := promptConfirm(scanner, stdout,
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
func runNewOwnerSeed(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer, inactiveOwners []admin.AccountView) (done bool, err error) {
	input, err := promptSeedInput(scanner, stdout)
	if err != nil {
		return false, err
	}

	if err := admin.Seed(ctx, identity.accounts, input); err != nil {
		if errors.Is(err, admin.ErrOwnerAlreadyExists) {
			if err := writeConsole(stdout, fmt.Sprintf("No se creó ninguna cuenta: %v\n", err)); err != nil {
				return false, err
			}
			if hint := admin.ReactivationHint(inactiveOwners, input.Phone); hint != "" {
				if err := writeConsole(stdout, fmt.Sprintf("%s\n", hint)); err != nil {
					return false, err
				}
			}
			return false, nil
		}
		return false, err
	}

	path, err := admin.WriteCallerID(input.Phone)
	if err != nil {
		return false, err
	}
	if err := writeConsole(stdout,
		fmt.Sprintf("Owner creado: %s (%s)\n", input.DisplayName, input.Phone)); err != nil {
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
type adminMenuAction func(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error

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
		{key: "7", label: "Configurar Hermes", action: func(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
			// Lazy resolution: a bad bind or an unresolvable home is returned as
			// a semantic error, which runAdminMenu renders as "Error: ..." and
			// reopens the menu instead of aborting the session.
			hermes, err := resolveHermes()
			if err != nil {
				return err
			}
			return runConfigureHermesFlow(ctx, identity, scanner, stdout, hermes)
		}},
	}
}

// runAdminMenu renders the numbered menu and dispatches the operator's choice
// until they quit or the stream ends. End of input (Ctrl+D or a
// non-interactive invocation) is a clean exit, not an error: an operator who
// only needed the seed must not get a failure for not answering the menu.
func runAdminMenu(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer, resolveHermes tui.HermesResolver) error {
	options := adminMenuOptions(resolveHermes)
	for {
		if err := writeMenu(stdout, options); err != nil {
			return err
		}

		choice, answered, err := readMenuChoice(scanner, stdout)
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

		if err := option.action(ctx, identity, scanner, stdout); err != nil {
			// Business errors reopen the menu; only a console that can no
			// longer echo would fail here, and that failure is propagated.
			if werr := writeConsole(stdout, fmt.Sprintf("Error: %v\n", err)); werr != nil {
				return werr
			}
		}
	}
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
func readMenuChoice(scanner *bufio.Scanner, stdout io.Writer) (choice string, answered bool, err error) {
	if err := writeConsole(stdout, "Seleccioná una opción: "); err != nil {
		return "", false, err
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", false, fmt.Errorf("leer la entrada del operador: %w", err)
		}
		return "", false, nil
	}
	return strings.TrimSpace(scanner.Text()), true, nil
}

// runAddStaffFlow drives the Add Staff capability (ADR-0016 Decision 1): list
// the active professionals, let the operator pick one by number, prefill the
// phone from professionals.phone (fact 8), ask for the display name and create
// the staff account through admin.AddStaff.
//
// With no active professional there is nothing to link the account to, so the
// flow reports it semantically and returns to the menu instead of failing.
func runAddStaffFlow(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
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

	selected, err := promptIndex(scanner, stdout, "Elegí un profesional", len(candidates))
	if err != nil {
		return err
	}
	professional := candidates[selected-1]

	phone, err := promptValidated(scanner, stdout, "Teléfono del staff", professional.Phone, admin.ValidatePhone)
	if err != nil {
		return err
	}

	displayName, err := promptValidated(scanner, stdout, "Nombre para mostrar", "", admin.ValidateDisplayName)
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
func runDeactivateAccountFlow(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
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

	index, err := promptIndex(scanner, stdout, "Elegí la cuenta a desactivar", len(views))
	if err != nil {
		return err
	}
	selected := views[index-1]

	confirmed, err := promptConfirm(scanner, stdout,
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
func runListAllAccountsFlow(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
	includeInactive, err := promptConfirm(scanner, stdout, "¿Incluir las cuentas inactivas?")
	if err != nil {
		return err
	}
	return renderAccountList(ctx, identity, stdout, admin.ListFilter{IncludeInactive: includeInactive})
}

// runListByRoleFlow renders the role-filtered view: the operator picks a role
// from the frozen sub-menu and then decides whether inactive rows show up.
func runListByRoleFlow(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
	roles := admin.ListableRoles()
	if err := writeConsole(stdout, "Roles disponibles:\n"); err != nil {
		return err
	}
	for i, role := range roles {
		if err := writeConsole(stdout, fmt.Sprintf("  [%d] %s\n", i+1, role)); err != nil {
			return err
		}
	}

	index, err := promptIndex(scanner, stdout, "Elegí el rol", len(roles))
	if err != nil {
		return err
	}
	role := roles[index-1]

	includeInactive, err := promptConfirm(scanner, stdout, "¿Incluir las cuentas inactivas?")
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
func promptIndex(scanner *bufio.Scanner, stdout io.Writer, label string, count int) (int, error) {
	for {
		if err := writeConsole(stdout, fmt.Sprintf("%s [1-%d]: ", label, count)); err != nil {
			return 0, err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return 0, fmt.Errorf("leer la entrada del operador: %w", err)
			}
			return 0, errInputAborted
		}
		choice, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
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
func promptConfirm(scanner *bufio.Scanner, stdout io.Writer, question string) (bool, error) {
	for {
		if err := writeConsole(stdout, fmt.Sprintf("%s (s/n): ", question)); err != nil {
			return false, err
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return false, fmt.Errorf("leer la entrada del operador: %w", err)
			}
			return false, errInputAborted
		}

		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
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
func promptSeedInput(scanner *bufio.Scanner, stdout io.Writer) (admin.SeedInput, error) {
	phone, err := promptValidated(scanner, stdout, "Teléfono del owner (ej. +5491100000000)", "", admin.ValidatePhone)
	if err != nil {
		return admin.SeedInput{}, err
	}

	displayName, err := promptValidated(scanner, stdout, "Nombre para mostrar", "", admin.ValidateDisplayName)
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
func promptValidated(scanner *bufio.Scanner, stdout io.Writer, label, defaultValue string, validate func(string) error) (string, error) {
	for {
		if defaultValue != "" {
			if err := writeConsole(stdout, fmt.Sprintf("%s [%s]: ", label, defaultValue)); err != nil {
				return "", err
			}
		} else if err := writeConsole(stdout, fmt.Sprintf("%s: ", label)); err != nil {
			return "", err
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("leer la entrada del operador: %w", err)
			}
			return "", errInputAborted
		}

		value := strings.TrimSpace(scanner.Text())
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
func runTransferOwnershipFlow(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
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

	index, err := promptIndex(scanner, stdout, "Elegí el sucesor", len(options))
	if err != nil {
		return err
	}
	chosen := options[index-1]

	if chosen.kind == admin.SuccessorNewPhone {
		phone, err := promptValidated(scanner, stdout, "Teléfono del nuevo owner", "", admin.ValidatePhone)
		if err != nil {
			return err
		}
		displayName, err := promptValidated(scanner, stdout, "Nombre para mostrar", "", admin.ValidateDisplayName)
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
	confirmed, err := promptConfirm(scanner, stdout, "El owner actual quedará desactivado. ¿Confirmar?")
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
func runAddSelfAsClientFlow(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
	owner, err := admin.ActiveOwner(ctx, identity.accounts)
	if err != nil {
		return err
	}

	if err := writeConsole(stdout, fmt.Sprintf(
		"Se va a registrar tu cuenta como cliente del negocio.\n"+
			"Tu teléfono de cuenta es: %s\n", owner.ID)); err != nil {
		return err
	}

	displayName, err := promptValidated(scanner, stdout, "Nombre para mostrar del cliente", "", admin.ValidateDisplayName)
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

// runConfigureHermesFlow drives the "Configurar Hermes" capability (ADR-0017):
// it prefills the phone from the ACTIVE owner, validates it through the single
// phone validator, shows the three facts of the bootstrap (endpoint, phone and
// target file), asks for an explicit confirmation and then merges and writes the
// mcp_servers.mcp-appointments entry. It mirrors the Bubble Tea wizard 1:1; the
// resolved Hermes facts arrive through the menu closure, so the console never
// re-derives the path or the endpoint.
//
// Without an ACTIVE owner the flow has nothing to prefill and no owner id Hermes
// could send, so it reports the semantic failure and the menu reopens.
//
// A failed write chain is never a dead end: the exact YAML snippet the writer
// would have produced is printed with the target path and the semantic error, so
// the operator can finish the bootstrap by hand (ADR-0017 Decision 1). The chain
// itself lives in admin.ApplyHermesConfig, the same helper the Bubble Tea
// wizard runs (R2-01), so the console never re-derives the path or the endpoint.
func runConfigureHermesFlow(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer, hermes tui.HermesConfig) error {
	owner, err := admin.ActiveOwner(ctx, identity.accounts)
	if err != nil {
		return err
	}

	phone, err := promptValidated(scanner, stdout, "Teléfono del owner (X-Caller-Id)", owner.ID, admin.ValidatePhone)
	if err != nil {
		return err
	}

	if err := writeHermesFacts(stdout, hermes, phone); err != nil {
		return err
	}

	confirmed, err := promptConfirm(scanner, stdout,
		"Se escribirá la entrada mcp-appointments en el config de Hermes. ¿Confirmar?")
	if err != nil {
		return err
	}
	if !confirmed {
		return writeConsole(stdout, "Operación cancelada: no se cambió la configuración de Hermes.\n")
	}

	snippet, err := admin.ApplyHermesConfig(hermes.Path, hermes.EndpointURL, phone)
	if err == nil {
		if err := writeConsole(stdout, "Configuración de Hermes actualizada.\n"); err != nil {
			return err
		}
		return writeHermesFacts(stdout, hermes, phone)
	}
	return writeHermesSnippetFallback(stdout, hermes, snippet, err)
}

// writeHermesFacts renders the three facts of the bootstrap — endpoint, phone
// and target file — in the same order and wording as the Bubble Tea confirmation
// and success screens, so the two presentations report the same contract.
func writeHermesFacts(stdout io.Writer, hermes tui.HermesConfig, phone string) error {
	if err := writeConsole(stdout, fmt.Sprintf("Endpoint: %s\n", hermes.EndpointURL)); err != nil {
		return err
	}
	if err := writeConsole(stdout, fmt.Sprintf("Teléfono (X-Caller-Id): %s\n", phone)); err != nil {
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
		"No se pudo escribir el archivo automáticamente. Copiá este bloque en el archivo indicado:\n"); err != nil {
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
