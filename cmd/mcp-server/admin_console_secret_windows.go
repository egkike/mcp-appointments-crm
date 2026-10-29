//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// readSecretNoEcho is the Windows end of the platform hook: it clears the
// console's echo and line-input flags and hands the very same fd to the shared
// bounded secret read (readSecretWithNoEcho → readSecretBounded).
//
// This replaces term.ReadPassword, whose readPasswordLine (charmbracelet/x/term
// v0.2.2, util.go) accumulates every typed byte with no size cap: a pasted
// secret larger than the console input buffer was allocated in full before
// Secret() could reject it (issue #154). Clearing ENABLE_LINE_INPUT alone is not
// enough — the bound has to be enforced while reading, which is what the shared
// byte-wise accumulator does.
func readSecretNoEcho(f *os.File, limit int) ([]byte, error) {
	return readSecretWithNoEcho(limit, func() (func(), error) {
		return enterWindowsNoEcho(windows.Handle(f.Fd()))
	}, f.Read)
}

// enterWindowsNoEcho disables console echo and returns the step that restores
// the previous mode. The returned restore is never nil when err is nil, so the
// shared orchestration can defer it unconditionally.
//
// The mask is the same one charmbracelet/x/term sets (term_windows.go:64-76):
// ENABLE_ECHO_INPUT and ENABLE_LINE_INPUT cleared, ENABLE_PROCESSED_OUTPUT and
// ENABLE_PROCESSED_INPUT set.
//
//   - ENABLE_ECHO_INPUT cleared is the whole no-echo mechanism on this platform:
//     the console stops writing typed characters back to the screen buffer, so
//     nothing the operator types reaches the terminal. There is no second echo
//     to suppress afterwards, and no redraw to undo.
//   - ENABLE_LINE_INPUT cleared is what makes the read byte-wise: without it the
//     console would withhold input until Enter, which would defeat the bound
//     (the console input buffer would grow with the paste) and the erase/abort
//     handling of the accumulator.
//   - ENABLE_PROCESSED_OUTPUT stays set as the library leaves it, and so does
//     ENABLE_PROCESSED_INPUT: Ctrl+C is therefore handled by the console as a
//     CTRL_C_EVENT and never reaches the reader as a 0x03 byte, so the
//     accumulator's Ctrl+C mapping only fires for a pasted 0x03. That is the
//     platform difference the shared loop documents; the erase key (0x08) does
//     reach the reader, because the line discipline that used to consume it is
//     off.
//
// A non-console fd (a redirected stdin, or a test pipe) makes GetConsoleMode
// fail, and then nothing is read at all: an echoed secret is never collected.
func enterWindowsNoEcho(handle windows.Handle) (func(), error) {
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return nil, fmt.Errorf("no se pudo desactivar el eco de la terminal: %w", err)
	}
	noEcho := mode &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT)
	noEcho |= windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_PROCESSED_INPUT
	if err := windows.SetConsoleMode(handle, noEcho); err != nil {
		return nil, fmt.Errorf("no se pudo desactivar el eco de la terminal: %w", err)
	}
	// Restore on every exit path, error included: a console left in char mode
	// with echo off would silently swallow the operator's next prompts.
	return func() { _ = windows.SetConsoleMode(handle, mode) }, nil
}
