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

	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
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
//     each capability (Add Staff first) is a self-contained action.
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
// T4-T6 appends an entry here instead of adding branches to runAdminTUIFlow, so
// the menu stays trivial to grow.
func adminMenuOptions() []adminMenuOption {
	return []adminMenuOption{
		{key: "1", label: "Add Staff", action: runAddStaffFlow},
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

	selected, err := promptSelection(scanner, stdout, len(candidates))
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

// promptSelection reads a 1-based index into the picker list and re-prompts
// while the answer is not a number inside the range.
func promptSelection(scanner *bufio.Scanner, stdout io.Writer, count int) (int, error) {
	for {
		if err := writeConsole(stdout, "Elegí un profesional [1-%d]: ", count); err != nil {
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
