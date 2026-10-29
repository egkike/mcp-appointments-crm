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

// fakeConsoleMode models the platform no-echo primitive of readSecretWithNoEcho:
// entering the mode is observable (echoOff), the restore step is counted, and a
// read attempted while echo is still on is recorded as a violation. It keeps the
// ordering and restore-on-every-exit-path contract testable on any GOOS without
// a real console.
type fakeConsoleMode struct {
	echoOff   bool
	enters    int
	restores  int
	reads     int
	echoReads int
	pending   []byte
}

func newFakeConsoleMode(script string) *fakeConsoleMode {
	return &fakeConsoleMode{pending: []byte(script)}
}

// enter is the platform no-echo step: it reports the mode as engaged and returns
// the restore the orchestration must defer.
func (m *fakeConsoleMode) enter() (func(), error) {
	m.enters++
	m.echoOff = true
	return func() {
		m.echoOff = false
		m.restores++
	}, nil
}

// read is the fd source: a console read drains what is already typed ahead.
func (m *fakeConsoleMode) read(buf []byte) (int, error) {
	m.reads++
	if !m.echoOff {
		m.echoReads++
	}
	if len(m.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(buf, m.pending)
	m.pending = m.pending[n:]
	return n, nil
}

// assertNoEchoLifecycle pins the two invariants of the shared orchestration: the
// secret is never collected with echo on, and the console mode is restored
// exactly once (leaving it in no-echo mode would swallow the next prompts).
func assertNoEchoLifecycle(t *testing.T, mode *fakeConsoleMode) {
	t.Helper()
	if mode.echoReads != 0 {
		t.Fatalf("reads with echo still on = %d, want 0 (the secret must never be collected with echo on)", mode.echoReads)
	}
	if mode.enters != 1 {
		t.Fatalf("no-echo mode entries = %d, want 1", mode.enters)
	}
	if mode.restores != 1 {
		t.Fatalf("console mode restores = %d, want 1", mode.restores)
	}
	if mode.echoOff {
		t.Fatal("console left in no-echo mode after the read, want restored")
	}
}

// TestReadSecretWithNoEchoDisablesEchoBeforeFirstByteAndRestores pins the shared
// orchestration both platforms run (admin_console_secret_windows.go and
// admin_console_secret_other.go only supply the mode dance): no byte is read
// before the no-echo mode is engaged, and the mode is restored on every exit
// path — accepted secret, oversized rejection, abort and read failure alike.
func TestReadSecretWithNoEchoDisablesEchoBeforeFirstByteAndRestores(t *testing.T) {
	t.Run("accepted secret", func(t *testing.T) {
		mode := newFakeConsoleMode("clave\nresto\n")

		secret, err := readSecretWithNoEcho(consoleInputLineMax, mode.enter, mode.read)
		if err != nil {
			t.Fatalf("readSecretWithNoEcho() error = %v, want nil", err)
		}
		if string(secret) != "clave" {
			t.Fatalf("secret = %q, want %q", string(secret), "clave")
		}
		assertNoEchoLifecycle(t, mode)
		if got, want := string(mode.pending), "resto\n"; got != want {
			t.Fatalf("queue after the secret = %q, want %q", got, want)
		}
	})

	t.Run("oversized secret is rejected and drained with the mode restored", func(t *testing.T) {
		mode := newFakeConsoleMode(strings.Repeat("x", consoleInputLineMax+1) + "\nresto\n")

		_, err := readSecretWithNoEcho(consoleInputLineMax, mode.enter, mode.read)
		assertSecretSemanticError(t, err, tooLongMessage)
		assertNoEchoLifecycle(t, mode)
		if got, want := string(mode.pending), "resto\n"; got != want {
			t.Fatalf("queue after the drained secret = %q, want %q", got, want)
		}
	})

	t.Run("aborted secret restores the mode", func(t *testing.T) {
		mode := newFakeConsoleMode("ab\x03\n")

		_, err := readSecretWithNoEcho(consoleInputLineMax, mode.enter, mode.read)
		assertSecretSemanticError(t, err, abortedMessage)
		assertNoEchoLifecycle(t, mode)
	})

	t.Run("read failure restores the mode", func(t *testing.T) {
		mode := newFakeConsoleMode("clave\n")
		readFailure := errors.New("la consola falló")
		reads := 0

		_, err := readSecretWithNoEcho(consoleInputLineMax, mode.enter, func([]byte) (int, error) {
			reads++
			return 0, readFailure
		})
		if !errors.Is(err, readFailure) {
			t.Fatalf("readSecretWithNoEcho() error = %v, want the read failure", err)
		}
		if reads != 1 {
			t.Fatalf("reads = %d, want 1", reads)
		}
		assertNoEchoLifecycle(t, mode)
	})

	t.Run("no-echo mode failure never reads", func(t *testing.T) {
		mode := newFakeConsoleMode("clave\n")
		enterFailure := errors.New("modo de consola no disponible")

		_, err := readSecretWithNoEcho(consoleInputLineMax, func() (func(), error) {
			return nil, enterFailure
		}, mode.read)
		if !errors.Is(err, enterFailure) {
			t.Fatalf("readSecretWithNoEcho() error = %v, want the mode failure", err)
		}
		if mode.reads != 0 {
			t.Fatalf("reads with echo on = %d, want 0 (fail before reading)", mode.reads)
		}
	})
}

// TestConsoleByteSourceReportsNoProgressReadAsFailure pins the read-loop guard:
// a console read that returns neither bytes nor an error must fail the secret
// read instead of spinning the accumulator forever.
func TestConsoleByteSourceReportsNoProgressReadAsFailure(t *testing.T) {
	t.Run("byte pieces are forwarded", func(t *testing.T) {
		mode := newFakeConsoleMode("ab\n")

		secret, err := readSecretBounded(consoleInputLineMax, consoleByteSource(mode.read))
		if err != nil {
			t.Fatalf("readSecretBounded() error = %v, want nil", err)
		}
		if got, want := string(secret), "ab"; got != want {
			t.Fatalf("secret = %q, want %q", got, want)
		}
	})

	t.Run("silent empty read fails instead of spinning", func(t *testing.T) {
		reads := 0

		_, err := readSecretBounded(consoleInputLineMax, consoleByteSource(func([]byte) (int, error) {
			reads++
			return 0, nil
		}))
		if !errors.Is(err, io.ErrNoProgress) {
			t.Fatalf("readSecretBounded() error = %v, want io.ErrNoProgress", err)
		}
		if reads != 1 {
			t.Fatalf("reads = %d, want 1 (a no-progress read must not loop)", reads)
		}
	})
}

// TestReadSecretNoEchoPlatformHookFailsSecureWithoutConsole pins the fail-secure
// contract of the platform hook itself on any GOOS: an fd that cannot be
// switched to a no-echo mode (a disk file here; a redirected stdin in
// production) makes the read fail before a single byte is consumed, so a secret
// is never collected with echo on. Asserting the untouched file offset is what
// proves "no byte was read" without a real terminal.
func TestReadSecretNoEchoPlatformHookFailsSecureWithoutConsole(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "no-es-un-console")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if _, err := f.WriteString("secreto-crudo\n"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}

	if _, err := readSecretNoEcho(f, consoleInputLineMax); err == nil {
		t.Fatal("readSecretNoEcho() error = nil, want the no-echo failure")
	}
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	if offset != 0 {
		t.Fatalf("file offset after the failed read = %d, want 0 (a byte was read with echo on)", offset)
	}
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

// TestConsoleInputSecretAbortDrainsTheRemainder pins the abort drain contract
// (GGA round 1 of #154): pressing Ctrl+C aborts THE LINE, and the remainder of
// it (the suffix bytes the queue still holds after the abort byte) must be
// drained exactly like the oversized remainder - it can never become the
// answer of a later prompt.
func TestConsoleInputSecretAbortDrainsTheRemainder(t *testing.T) {
	queue := newTerminalQueue("\x03rogue\nplus\n")
	input := boundedTerminalConsoleInput(queue, consoleInputLineMax)

	_, err := input.Secret()
	if err == nil {
		t.Fatal("Secret() err = nil, want the abort semantic error")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) || semErr != errInputAborted {
		t.Fatalf("Secret() err = %v, want errInputAborted", err)
	}
	got, lerr := input.Line()
	if lerr != nil {
		t.Fatalf("next Line() error = %v, want nil", lerr)
	}
	if got != "plus" {
		t.Fatalf("next Line() = %q, want %q (the aborted line's suffix \"rogue\" must be drained)", got, "plus")
	}
	if r := queue.remaining(); r != "" {
		t.Fatalf("queue remaining = %q, want empty", r)
	}
}
