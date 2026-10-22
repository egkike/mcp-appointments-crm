package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// terminalQueue models the kernel-side terminal input queue: every read drains
// the shared bytes, exactly like stdin does for a real TTY, so whichever input
// path reads first owns the bytes and no later path can recover them. This is
// the property B14 is about: the text prompt and the no-echo secret prompt of a
// real terminal read the SAME queue through the same fd.
type terminalQueue struct {
	mu      sync.Mutex
	pending []byte
}

func newTerminalQueue(script string) *terminalQueue {
	return &terminalQueue{pending: []byte(script)}
}

// Read is the plain-fd read: a real terminal hands over everything the operator
// already typed ahead, so the fake does the same.
func (q *terminalQueue) Read(buf []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(buf, q.pending)
	q.pending = q.pending[n:]
	return n, nil
}

// readSecretLine models term.ReadPassword's raw-fd read: it consumes exactly one
// line (newline included) from the shared queue and returns it without the
// newline. It never reads ahead, which is why the text path must not either.
func (q *terminalQueue) readSecretLine() ([]byte, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil, io.EOF
	}
	if idx := bytes.IndexByte(q.pending, '\n'); idx >= 0 {
		line := string(q.pending[:idx])
		q.pending = q.pending[idx+1:]
		return []byte(line), nil
	}
	line := string(q.pending)
	q.pending = nil
	return []byte(line), nil
}

func (q *terminalQueue) remaining() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return string(q.pending)
}

// newTerminalConsoleInput builds a consoleInput in terminal mode over the fake
// queue: the two production read paths (byte-wise fd read for prompts,
// raw-fd no-echo read for secrets) are bound to the one shared queue.
func newTerminalConsoleInput(queue *terminalQueue) *consoleInput {
	return &consoleInput{term: &consoleTerm{
		read:     queue.Read,
		password: queue.readSecretLine,
	}}
}

// TestConsoleInputMixedPromptsShareTerminalQueue is the B14 regression test: a
// text prompt, then a secret prompt, then another text prompt in one flow. Each
// prompt must read exactly its own line, leaving the rest of the typed-ahead
// queue untouched. Before the fix the text prompt used a 4 KiB prefetching
// buffered reader, so the secret prompt found an empty kernel queue and the
// operator's credential was silently lost.
func TestConsoleInputMixedPromptsShareTerminalQueue(t *testing.T) {
	queue := newTerminalQueue("operador\nclave-secreta\nConfirmar\n")
	input := newTerminalConsoleInput(queue)

	first, err := input.Line()
	if err != nil {
		t.Fatalf("Line() #1 error = %v, want nil", err)
	}
	if first != "operador" {
		t.Fatalf("Line() #1 = %q, want %q", first, "operador")
	}
	if got, want := queue.remaining(), "clave-secreta\nConfirmar\n"; got != want {
		t.Fatalf("queue after the text prompt = %q, want %q (the text path consumed bytes it did not own)", got, want)
	}

	secret, err := input.Secret()
	if err != nil {
		t.Fatalf("Secret() error = %v, want nil", err)
	}
	if secret != "clave-secreta" {
		t.Fatalf("Secret() = %q, want %q", secret, "clave-secreta")
	}
	if got, want := queue.remaining(), "Confirmar\n"; got != want {
		t.Fatalf("queue after the secret prompt = %q, want %q", got, want)
	}

	third, err := input.Line()
	if err != nil {
		t.Fatalf("Line() #2 error = %v, want nil", err)
	}
	if third != "Confirmar" {
		t.Fatalf("Line() #2 = %q, want %q", third, "Confirmar")
	}
	if got := queue.remaining(); got != "" {
		t.Fatalf("queue after the flow = %q, want empty", got)
	}
}

// TestConsoleInputSecretThenTextOnTerminal pins the reverse order: a no-echo
// read must not leave stale buffered state that a later text prompt would
// re-deliver or skip (the other half of the shared-queue contract).
func TestConsoleInputSecretThenTextOnTerminal(t *testing.T) {
	queue := newTerminalQueue("clave-secreta\nDueño\n")
	input := newTerminalConsoleInput(queue)

	secret, err := input.Secret()
	if err != nil {
		t.Fatalf("Secret() error = %v, want nil", err)
	}
	if secret != "clave-secreta" {
		t.Fatalf("Secret() = %q, want %q", secret, "clave-secreta")
	}

	name, err := input.Line()
	if err != nil {
		t.Fatalf("Line() error = %v, want nil", err)
	}
	if name != "Dueño" {
		t.Fatalf("Line() = %q, want %q", name, "Dueño")
	}
	if got := queue.remaining(); got != "" {
		t.Fatalf("queue after the flow = %q, want empty", got)
	}
}

// TestConsoleInputTerminalOversizeLineIsRejectedAndDrained keeps the GGA round
// 23 bound in the byte-wise path: an over-long line is rejected semantically and
// its remainder is drained, so it can never become the next prompt's answer.
func TestConsoleInputTerminalOversizeLineIsRejectedAndDrained(t *testing.T) {
	oversized := strings.Repeat("x", consoleInputLineMax+1)
	queue := newTerminalQueue(oversized + "\nsiguiente\n")
	input := newTerminalConsoleInput(queue)

	_, err := input.Line()
	if err == nil {
		t.Fatal("Line() error = nil, want the semantic length rejection")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("error = %T (%v), want *domain.SemanticError", err, err)
	}
	if want := "la entrada del operador excede la longitud máxima permitida"; semErr.Message != want {
		t.Fatalf("message = %q, want %q", semErr.Message, want)
	}

	next, err := input.Line()
	if err != nil {
		t.Fatalf("Line() after the rejection error = %v, want nil", err)
	}
	if next != "siguiente" {
		t.Fatalf("Line() after the rejection = %q, want %q (drain failed)", next, "siguiente")
	}
}

// TestConsoleInputTerminalEOF pins the abort contract of the byte-wise path: a
// terminal that ends mid-flow aborts with the shared semantic error, while a
// carried final line without newline is still an answer.
func TestConsoleInputTerminalEOF(t *testing.T) {
	t.Run("empty terminal aborts", func(t *testing.T) {
		input := newTerminalConsoleInput(newTerminalQueue(""))

		_, err := input.Line()
		if err == nil {
			t.Fatal("Line() error = nil, want the abort")
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Fatalf("error = %T (%v), want *domain.SemanticError", err, err)
		}
		if want := "se canceló la operación: no se recibió la entrada necesaria"; semErr.Message != want {
			t.Fatalf("message = %q, want %q", semErr.Message, want)
		}
	})

	t.Run("final line without newline is content", func(t *testing.T) {
		input := newTerminalConsoleInput(newTerminalQueue("sin-salto"))

		line, err := input.Line()
		if err != nil {
			t.Fatalf("Line() error = %v, want nil", err)
		}
		if line != "sin-salto" {
			t.Fatalf("Line() = %q, want %q", line, "sin-salto")
		}
	})
}

// TestConsoleInputTerminalSecretFailure pins the wrap of a failed no-echo read:
// the operator sees the semantic read failure, never a bare syscall error.
func TestConsoleInputTerminalSecretFailure(t *testing.T) {
	input := &consoleInput{term: &consoleTerm{
		password: func() ([]byte, error) { return nil, errors.New("terminal sin termios") },
	}}

	_, err := input.Secret()
	if err == nil {
		t.Fatal("Secret() error = nil, want the read failure")
	}
	if !strings.HasPrefix(err.Error(), "leer la entrada secreta: ") {
		t.Fatalf("error = %q, want the secret-read prefix", err.Error())
	}
}

// TestConsoleInputPipedModeKeepsBufferedBehaviour locks the unchanged piped
// contract: a single consumer owns the stream, so a secret is an ordinary
// buffered Line and no bytes are lost across prompt kinds.
func TestConsoleInputPipedModeKeepsBufferedBehaviour(t *testing.T) {
	input := newConsoleInput(bufio.NewReader(strings.NewReader("operador\nclave-secreta\n")))
	if input.term != nil {
		t.Fatal("piped stdin must not enter terminal mode")
	}

	line, err := input.Line()
	if err != nil {
		t.Fatalf("Line() error = %v, want nil", err)
	}
	if line != "operador" {
		t.Fatalf("Line() = %q, want %q", line, "operador")
	}

	secret, err := input.Secret()
	if err != nil {
		t.Fatalf("Secret() error = %v, want nil", err)
	}
	if secret != "clave-secreta" {
		t.Fatalf("Secret() = %q, want %q", secret, "clave-secreta")
	}
}
