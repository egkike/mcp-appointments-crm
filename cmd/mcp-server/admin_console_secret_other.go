//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
)

// readSecretNoEcho is the POSIX end of the platform hook: it switches the
// terminal to raw mode and hands the fd to the shared bounded secret read
// (readSecretWithNoEcho → readSecretBounded).
//
// Raw mode is the no-echo mechanism here: tcsetattr drops ECHO and ICANON in
// one call, so no typed byte is echoed back and no read-ahead happens. The
// restore returned by MakeRaw runs on every exit path of the shared
// orchestration, so an aborted or oversized secret still leaves the terminal
// usable.
func readSecretNoEcho(f *os.File, limit int) ([]byte, error) {
	return readSecretWithNoEcho(limit, func() (func(), error) {
		state, err := term.MakeRaw(f.Fd())
		if err != nil {
			return nil, fmt.Errorf("no se pudo desactivar el eco de la terminal: %w", err)
		}
		return func() { _ = term.Restore(f.Fd(), state) }, nil
	}, f.Read)
}
