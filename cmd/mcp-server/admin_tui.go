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
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// runAdminTUI is the entry point of `mcp-server admin tui`, the operator-facing
// identity and accounts TUI (ADR-0016 §5, ADR-0010). It binds the process
// streams to the testable flow.
func runAdminTUI() error {
	return runAdminTUIFlow(os.Stdin, os.Stdout)
}

// runAdminTUIFlow implements the T2 owner seed gateway plus the T3 operator
// menu (ADR-0016 Decision 1).
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
// keeps the TUI independent from it (shared *sql.DB, logger and repos only).
// No UI framework is used here: T7 assembles the Bubble Tea screens on top.
func runAdminTUIFlow(stdin io.Reader, stdout io.Writer) error {
	deps, err := openCommandDependencies()
	if err != nil {
		return err
	}
	defer deps.close()

	identity := newIdentityDeps(deps.database, deps.logger)
	ctx := context.Background()
	scanner := bufio.NewScanner(stdin)

	needsSeed, err := admin.NeedsSeed(ctx, identity.accounts)
	if err != nil {
		return err
	}

	if !needsSeed {
		// R4-partial-seed-wedge: a previous run may have created the owner and
		// then failed to write the caller-id file. Repair it before reporting a
		// clean exit, or Hermes keeps failing to resolve a caller id.
		repaired, err := admin.EnsureCallerID(ctx, identity.accounts)
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
		return runAdminMenu(ctx, identity, scanner, stdout)
	}

	// R4-deactivated-owner-seed-deadend: "no ACTIVE owner" does not mean "no
	// owner row". When deactivated owner rows exist, the seed questionnaire
	// stalls on the accounts.id PRIMARY KEY the moment the operator retypes that
	// phone, so the reactivation path is offered first.
	inactiveOwners, err := admin.InactiveOwners(ctx, identity.accounts)
	if err != nil {
		return err
	}
	if len(inactiveOwners) > 0 {
		if err := runSeedDeadendRecovery(ctx, identity, scanner, stdout, inactiveOwners); err != nil {
			return err
		}
		return runAdminMenu(ctx, identity, scanner, stdout)
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

	if err := admin.Seed(ctx, identity.accounts, input); err != nil {
		if errors.Is(err, admin.ErrOwnerAlreadyExists) {
			// Idempotent outcome: an owner appeared between the decision and
			// the write. Report it semantically and exit clean — never
			// duplicate, never surface a driver error.
			if err := writeConsole(stdout, "No se creó ninguna cuenta: %v\n", err); err != nil {
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

	if err := writeConsole(stdout, "Owner creado: %s (%s)\n", input.DisplayName, input.Phone); err != nil {
		return err
	}
	if err := writeConsole(stdout, "caller-id escrito en %s\n", path); err != nil {
		return err
	}
	if err := writeConsole(stdout, "Ya podés usar `mcp-server hermes chat` con ese caller id.\n"); err != nil {
		return err
	}

	return runAdminMenu(ctx, identity, scanner, stdout)
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
		if err := writeConsole(stdout,
			"No hay ningún owner activo, pero hay %d cuenta(s) de owner desactivada(s).\n"+
				"Podés reactivar una de ellas o crear un owner nuevo con otro teléfono.\n",
			len(inactiveOwners)); err != nil {
			return err
		}
		for i, view := range inactiveOwners {
			if err := writeConsole(stdout, "  [%d] Reactivar %s\n", i+1, accountLabel(view)); err != nil {
				return err
			}
		}
		if err := writeConsole(stdout, "  [%d] Crear un owner nuevo con otro teléfono\n", createKey); err != nil {
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
		fmt.Sprintf("Se reactivará %s como owner activo. ¿Confirmar?", accountLabel(candidate)))
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
	if err := writeConsole(stdout, "Owner reactivado: %s (%s)\n", reactivated.DisplayName, reactivated.ID); err != nil {
		return false, err
	}
	if err := writeConsole(stdout, "caller-id escrito en %s\n", path); err != nil {
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
			if err := writeConsole(stdout, "No se creó ninguna cuenta: %v\n", err); err != nil {
				return false, err
			}
			if hint := reactivationHint(inactiveOwners, input.Phone); hint != "" {
				if err := writeConsole(stdout, "%s\n", hint); err != nil {
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
	if err := writeConsole(stdout, "Owner creado: %s (%s)\n", input.DisplayName, input.Phone); err != nil {
		return false, err
	}
	if err := writeConsole(stdout, "caller-id escrito en %s\n", path); err != nil {
		return false, err
	}
	return true, writeConsole(stdout, "Ya podés usar `mcp-server hermes chat` con ese caller id.\n")
}

// reactivationHint names the deactivated owner whose phone the operator just
// tried to reuse, so the semantic conflict carries the exact next step. It
// returns an empty string when the phone belongs to something else.
func reactivationHint(inactiveOwners []admin.AccountView, phone string) string {
	for _, view := range inactiveOwners {
		if view.ID == phone {
			return fmt.Sprintf(
				"Sugerencia: el teléfono %s pertenece a la cuenta de owner desactivada %q; reactivala con la opción de reactivación en lugar de crear una cuenta nueva.",
				phone, view.DisplayName)
		}
	}
	return ""
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
// T5-T6 appends an entry here instead of adding branches to runAdminTUIFlow, so
// the menu stays trivial to grow.
func adminMenuOptions() []adminMenuOption {
	return []adminMenuOption{
		{key: "1", label: "Add Staff", action: runAddStaffFlow},
		{key: "2", label: "Desactivar cuenta", action: runDeactivateAccountFlow},
		{key: "3", label: "Listar cuentas", action: runListAllAccountsFlow},
		{key: "4", label: "Listar por rol", action: runListByRoleFlow},
		{key: "5", label: "Transferir ownership", action: runTransferOwnershipFlow},
		{key: "6", label: "Agregarme como cliente", action: runAddSelfAsClientFlow},
	}
}

// runAdminMenu renders the numbered menu and dispatches the operator's choice
// until they quit or the stream ends. End of input (Ctrl+D or a
// non-interactive invocation) is a clean exit, not an error: an operator who
// only needed the seed must not get a failure for not answering the menu.
func runAdminMenu(ctx context.Context, identity identityDeps, scanner *bufio.Scanner, stdout io.Writer) error {
	options := adminMenuOptions()
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
			if err := writeConsole(stdout, "Error: opción desconocida %q\n", choice); err != nil {
				return err
			}
			continue
		}

		if err := option.action(ctx, identity, scanner, stdout); err != nil {
			// Business errors reopen the menu; only a console that can no
			// longer echo would fail here, and that failure is propagated.
			if werr := writeConsole(stdout, "Error: %v\n", err); werr != nil {
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
		if err := writeConsole(stdout, "  [%s] %s\n", option.key, option.label); err != nil {
			return err
		}
	}
	return writeConsole(stdout, "  [%s] Salir\n", menuExitKey)
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
		if err := writeConsole(stdout, "  [%d] %s\n", i+1, candidate.Name); err != nil {
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

	return writeConsole(stdout, "Cuenta de staff creada: %s (%s) para el profesional %s\n",
		displayName, phone, professional.Name)
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
		if err := writeConsole(stdout, "  [%d] %s\n", i+1, accountLabel(view)); err != nil {
			return err
		}
	}

	index, err := promptIndex(scanner, stdout, "Elegí la cuenta a desactivar", len(views))
	if err != nil {
		return err
	}
	selected := views[index-1]

	confirmed, err := promptConfirm(scanner, stdout,
		fmt.Sprintf("Esta acción desactiva la cuenta %s. ¿Confirmar?", accountLabel(selected)))
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
		return writeConsole(stdout, "La cuenta ya estaba inactiva: %s\n", accountLabel(outcome.Account))
	}
	return writeConsole(stdout, "Cuenta desactivada: %s\n", accountLabel(outcome.Account))
}

// accountLabel renders one account for the operator: the display name the human
// recognises, plus the role and the phone, the two fields that cannot be
// guessed from the name.
func accountLabel(view admin.AccountView) string {
	return fmt.Sprintf("%s (%s, %s)", view.DisplayName, view.Role, view.ID)
}

// listableRoles is the role sub-menu of the "Listar por rol" flow. It mirrors
// the domain enumeration (ADR-0009) in menu order; the core validates the
// selected role again before any port call.
var listableRoles = [...]entity.AccountRole{entity.RoleOwner, entity.RoleAdmin, entity.RoleStaff}

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
	if err := writeConsole(stdout, "Roles disponibles:\n"); err != nil {
		return err
	}
	for i, role := range listableRoles {
		if err := writeConsole(stdout, "  [%d] %s\n", i+1, role); err != nil {
			return err
		}
	}

	index, err := promptIndex(scanner, stdout, "Elegí el rol", len(listableRoles))
	if err != nil {
		return err
	}
	role := listableRoles[index-1]

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

	if err := writeConsole(stdout, "%s\n", formatAccountRow(headers, widths)); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeConsole(stdout, "%s\n", formatAccountRow(row, widths)); err != nil {
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
		if err := writeConsole(stdout, "%s [1-%d]: ", label, count); err != nil {
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
			if err := writeConsole(stdout, "Error: opción inválida: elegí un número entre 1 y %d\n", count); err != nil {
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
func writeConsole(stdout io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(stdout, format, args...); err != nil {
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
		if err := writeConsole(stdout, "%s (s/n): ", question); err != nil {
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
			if err := writeConsole(stdout, "%s [%s]: ", label, defaultValue); err != nil {
				return "", err
			}
		} else if err := writeConsole(stdout, "%s: ", label); err != nil {
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
			if err := writeConsole(stdout, "Error: %v\n", err); err != nil {
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
	if err := writeConsole(stdout, "Owner actual: %s\n", accountLabel(owner)); err != nil {
		return err
	}

	options, err := transferSuccessorOptions(ctx, identity)
	if err != nil {
		return err
	}
	for i, option := range options {
		if err := writeConsole(stdout, "  [%d] %s\n", i+1, option.label); err != nil {
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

	if err := writeConsole(stdout, "Ownership transferido: %s → %s\n",
		accountLabel(outcome.From), accountLabel(outcome.To)); err != nil {
		return err
	}
	return writeConsole(stdout, "caller-id actualizado en %s\n", path)
}

// transferSuccessorOptions builds the numbered successor list: every inactive
// owner row, then every active staff account, then the "new phone" escape
// hatch. The active owner is never offered — the transfer deactivates it.
// Labels carry name, role and phone so the operator recognises the row they are
// about to promote.
func transferSuccessorOptions(ctx context.Context, identity identityDeps) ([]transferSuccessorOption, error) {
	owners, err := admin.ListAccounts(ctx, identity.accounts, admin.ListFilter{
		Role:            entity.RoleOwner,
		IncludeInactive: true,
	})
	if err != nil {
		return nil, err
	}

	options := make([]transferSuccessorOption, 0, len(owners)+1)
	for _, view := range owners {
		if view.Active {
			continue
		}
		options = append(options, transferSuccessorOption{
			kind:  admin.SuccessorInactiveOwner,
			id:    view.ID,
			label: accountLabel(view),
		})
	}

	staff, err := admin.ListAccounts(ctx, identity.accounts, admin.ListFilter{Role: entity.RoleStaff})
	if err != nil {
		return nil, err
	}
	for _, view := range staff {
		options = append(options, transferSuccessorOption{
			kind:  admin.SuccessorStaff,
			id:    view.ID,
			label: accountLabel(view) + " — promover a owner",
		})
	}

	return append(options, transferSuccessorOption{
		kind:  admin.SuccessorNewPhone,
		label: "Otro teléfono (crear una cuenta de owner nueva)",
	}), nil
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

	if err := writeConsole(stdout,
		"Se va a registrar tu cuenta como cliente del negocio.\n"+
			"Tu teléfono de cuenta es: %s\n", owner.ID); err != nil {
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
