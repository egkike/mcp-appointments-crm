package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

const (
	tooLongMessage = "la entrada del operador excede la longitud máxima permitida"
	abortedMessage = "se canceló la operación: no se recibió la entrada necesaria"
)

// assertSecretSemanticError asserts err is the semantic rejection carrying want
// as its operator-facing message.
func assertSecretSemanticError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want the semantic rejection %q", want)
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("error = %T (%v), want *domain.SemanticError", err, err)
	}
	if semErr.Message != want {
		t.Fatalf("message = %q, want %q", semErr.Message, want)
	}
}

// bytewiseTerminalSource drives the bounded no-echo accumulator from the shared
// terminal queue one byte at a time, which is exactly how the production reader
// consumes a raw fd. It keeps the whole test pty-free.
func bytewiseTerminalSource(read func([]byte) (int, error)) func() ([]byte, error) {
	return func() ([]byte, error) {
		var one [1]byte
		n, rerr := read(one[:])
		return one[:n], rerr
	}
}

// oneShotTerminalSource hands the whole script over in one piece, the way a
// pasted burst can arrive, and then reports EOF.
func oneShotTerminalSource(script string) func() ([]byte, error) {
	done := false
	return func() ([]byte, error) {
		if done {
			return nil, io.EOF
		}
		done = true
		return []byte(script), nil
	}
}

// boundedTerminalConsoleInput binds the injected consoleTerm field to the
// production bounded no-echo accumulator over the shared queue, so an oversized
// paste is simulated without a real terminal.
func boundedTerminalConsoleInput(queue *terminalQueue, limit int) *consoleInput {
	return &consoleInput{term: &consoleTerm{
		read: queue.Read,
		password: func() ([]byte, error) {
			return readSecretBounded(limit, bytewiseTerminalSource(queue.Read))
		},
	}}
}

// TestConsoleInputSecretTerminalOversizeIsRejectedAndDrained is the GGA
// regression test for the no-echo path: an oversized paste must be rejected
// with the semantic length error, and its remainder must be drained so it can
// never become the next prompt's answer.
//
// The fake line reader below returns the whole line at once — the shape of the
// Windows console read and of any injected reader that collects the line itself
// — so this pins the post-read rejection in Secret() plus the drain contract,
// which are the testable halves of the finding. The bounded while-reading loop
// is pinned by the following tests; the echo itself is structural (raw mode is
// engaged before the first byte is read) and cannot be observed without a pty.
func TestConsoleInputSecretTerminalOversizeIsRejectedAndDrained(t *testing.T) {
	oversized := strings.Repeat("x", consoleInputLineMax+1)
	queue := newTerminalQueue(oversized + "\nsiguiente\n")
	input := newTerminalConsoleInput(queue)

	_, err := input.Secret()
	assertSecretSemanticError(t, err, tooLongMessage)

	if got, want := queue.remaining(), "siguiente\n"; got != want {
		t.Fatalf("queue after the rejected secret = %q, want %q (drain failed)", got, want)
	}

	next, err := input.Line()
	if err != nil {
		t.Fatalf("Line() after the rejection error = %v, want nil", err)
	}
	if next != "siguiente" {
		t.Fatalf("Line() after the rejection = %q, want %q", next, "siguiente")
	}
}

// TestConsoleInputSecretBoundedReaderDrainsOversizedPaste exercises the real
// bounded accumulator behind the injected terminal field: the limit is enforced
// while reading and the leftover of the oversized line is consumed.
func TestConsoleInputSecretBoundedReaderDrainsOversizedPaste(t *testing.T) {
	const limit = 8
	queue := newTerminalQueue("0123456789\nclave\n")
	input := boundedTerminalConsoleInput(queue, limit)

	_, err := input.Secret()
	assertSecretSemanticError(t, err, tooLongMessage)

	if got, want := queue.remaining(), "clave\n"; got != want {
		t.Fatalf("queue after the drained secret = %q, want %q", got, want)
	}

	next, err := input.Line()
	if err != nil {
		t.Fatalf("Line() error = %v, want nil", err)
	}
	if next != "clave" {
		t.Fatalf("Line() = %q, want %q", next, "clave")
	}
}

// TestConsoleInputSecretBoundedReaderBoundary pins the boundary at
// consoleInputLineMax for both arrival shapes (whole paste in one piece, and the
// byte-wise terminal read): exactly the limit is accepted, one byte more is
// rejected.
func TestConsoleInputSecretBoundedReaderBoundary(t *testing.T) {
	atLimit := strings.Repeat("a", consoleInputLineMax)

	t.Run("one piece at the limit is accepted", func(t *testing.T) {
		input := &consoleInput{term: &consoleTerm{
			password: func() ([]byte, error) {
				return readSecretBounded(consoleInputLineMax, oneShotTerminalSource(atLimit+"\n"))
			},
		}}

		secret, err := input.Secret()
		if err != nil {
			t.Fatalf("Secret() error = %v, want nil", err)
		}
		if len(secret) != consoleInputLineMax {
			t.Fatalf("len(Secret()) = %d, want %d", len(secret), consoleInputLineMax)
		}
	})

	t.Run("byte-wise one over the limit is rejected and drained", func(t *testing.T) {
		// The oversized line is atLimit+"z"; the drain must stop at its own
		// terminator, so only the following line survives.
		queue := newTerminalQueue(atLimit + "z\notra\n")
		input := boundedTerminalConsoleInput(queue, consoleInputLineMax)

		_, err := input.Secret()
		assertSecretSemanticError(t, err, tooLongMessage)

		if got, want := queue.remaining(), "otra\n"; got != want {
			t.Fatalf("queue after the drained secret = %q, want %q", got, want)
		}
	})
}

// TestConsoleInputSecretBoundedReaderKeepsLineEditing pins the raw-mode
// behaviour the terminal no longer provides once the line discipline is off: the
// erase key drops the previous byte instead of landing inside the secret, and
// Ctrl+C/Ctrl+D abort with the shared semantic error instead of a signal.
func TestConsoleInputSecretBoundedReaderKeepsLineEditing(t *testing.T) {
	t.Run("erase key removes the previous byte", func(t *testing.T) {
		queue := newTerminalQueue("abc\x7fd\nresto\n")
		input := boundedTerminalConsoleInput(queue, consoleInputLineMax)

		secret, err := input.Secret()
		if err != nil {
			t.Fatalf("Secret() error = %v, want nil", err)
		}
		if secret != "abd" {
			t.Fatalf("Secret() = %q, want %q", secret, "abd")
		}
		if got, want := queue.remaining(), "resto\n"; got != want {
			t.Fatalf("queue after the secret = %q, want %q", got, want)
		}
	})

	t.Run("ctrl+d aborts", func(t *testing.T) {
		input := boundedTerminalConsoleInput(newTerminalQueue("ab\x04\n"), consoleInputLineMax)

		_, err := input.Secret()
		assertSecretSemanticError(t, err, abortedMessage)
	})

	t.Run("ctrl+c aborts", func(t *testing.T) {
		input := boundedTerminalConsoleInput(newTerminalQueue("ab\x03\n"), consoleInputLineMax)

		_, err := input.Secret()
		assertSecretSemanticError(t, err, abortedMessage)
	})
}

// TestReadTerminalSecretFailsSecureWithoutTerminal pins the fail-secure
// contract: when the fd cannot be switched to a no-echo mode the read fails
// instead of collecting the secret with echo on.
func TestReadTerminalSecretFailsSecureWithoutTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "no-es-una-terminal")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if _, err := readTerminalSecret(f, consoleInputLineMax); err == nil {
		t.Fatal("readTerminalSecret() error = nil, want the no-echo failure")
	}
}
