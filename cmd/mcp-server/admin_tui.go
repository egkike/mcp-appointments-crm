package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// runAdminTUIFlow implements the T2 owner seed gateway (ADR-0016 Decision 1).
//
// It:
//
//  1. validates the dependencies serve mode also validates at startup
//     (configuration + SQLite), so a broken install fails here instead of
//     half-opening a TUI;
//  2. constructs the identity repositories through the shared construction
//     site (newIdentityDeps), so the flows inherit production wiring;
//  3. decides whether the first owner must be created — if an active owner
//     already exists it prints a semantic message and returns nil, never
//     duplicating the account (ownership transfer is T5);
//  4. drives a line-based questionnaire (bufio.Scanner) that re-prompts on
//     invalid input, creates the owner through admin.Seed and writes
//     <config-dir>/caller-id.
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
		return nil
	}

	if err := writeConsole(stdout,
		"No hay ningún owner activo: vamos a crear el primero.\n"+
			"El administrador del sistema operativo es el gatekeeper de este paso (ADR-0010).\n"); err != nil {
		return err
	}

	input, err := promptSeedInput(stdin, stdout)
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

	return nil
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
// professional picker is T3 and is intentionally absent here.
func promptSeedInput(stdin io.Reader, stdout io.Writer) (admin.SeedInput, error) {
	scanner := bufio.NewScanner(stdin)

	phone, err := promptValidated(scanner, stdout, "Teléfono del owner (ej. +5491100000000): ", admin.ValidatePhone)
	if err != nil {
		return admin.SeedInput{}, err
	}

	displayName, err := promptValidated(scanner, stdout, "Nombre para mostrar: ", admin.ValidateDisplayName)
	if err != nil {
		return admin.SeedInput{}, err
	}

	return admin.SeedInput{Phone: phone, DisplayName: displayName}, nil
}

// promptValidated prints label, reads one line and re-prompts while validate
// rejects the answer. Rejections are rendered as "Error: <detalle semántico>"
// so the operator reads the business message, never a parser dump.
//
// A stream that ends before a valid answer aborts the flow with a semantic
// error: an interrupted seed must not create a half-built owner.
func promptValidated(scanner *bufio.Scanner, stdout io.Writer, label string, validate func(string) error) (string, error) {
	for {
		if err := writeConsole(stdout, "%s", label); err != nil {
			return "", err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("leer la entrada del operador: %w", err)
			}
			return "", errSeedAborted
		}
		value := strings.TrimSpace(scanner.Text())
		if err := validate(value); err != nil {
			if err := writeConsole(stdout, "Error: %v\n", err); err != nil {
				return "", err
			}
			continue
		}
		return value, nil
	}
}

// errSeedAborted reports an interrupted seed questionnaire (Ctrl+D or a
// non-interactive invocation). It is never an interactive re-prompt: there is
// no more input to read.
var errSeedAborted = &domain.SemanticError{
	Code:    domain.ErrCodeInvalidInput,
	Message: "se canceló la creación del owner: no se recibió la entrada necesaria",
}
