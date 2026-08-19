package mcp

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// newTestHTTPServer binds a real http.Server on an ephemeral loopback port so
// that ConnState tracking (active connection accounting) sees real
// connections, not the httptest.Server wrapper. Serving is owned by run.
func newTestHTTPServer(t *testing.T, handler http.Handler) (*http.Server, net.Listener, string) {
	t.Helper()

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback: %v", err)
	}

	t.Cleanup(func() {
		_ = ln.Close()
		_ = srv.Close()
	})

	return srv, ln, ln.Addr().String()
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRunDrainsActiveConnectionOnSignal: a request is in flight when the
// shutdown signal arrives; the drain window (10s) outlives the handler, so the
// connection completes cleanly and is counted as drained.
func TestRunDrainsActiveConnectionOnSignal(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	srv, ln, addr := newTestHTTPServer(t, handler)

	sigCh := make(chan os.Signal, 1)
	resultCh := make(chan ShutdownResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := run(context.Background(), srv, ln, discardLogger(), 10*time.Second, sigCh)
		resultCh <- res
		errCh <- err
	}()

	// The signal arrives ~50ms in, while the handler is still sleeping.
	go func() {
		time.Sleep(50 * time.Millisecond)
		sigCh <- syscall.SIGTERM
	}()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr, nil)
	if err != nil {
		t.Fatalf("build GET request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET in flight: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Drained != 1 {
		t.Errorf("Drained = %d, want 1 (in-flight request completed during drain)", result.Drained)
	}
	if result.ForceClosed != 0 {
		t.Errorf("ForceClosed = %d, want 0 (drain window outlived the handler)", result.ForceClosed)
	}
}

// TestRunForceClosesAfterDeadline: a handler that never returns forces the
// drain to hit its deadline; the server then closes the active connection and
// the result counts it as force-closed.
func TestRunForceClosesAfterDeadline(t *testing.T) {
	block := make(chan struct{})
	handlerDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer close(handlerDone)
		<-block
		w.WriteHeader(http.StatusOK)
	})
	srv, ln, addr := newTestHTTPServer(t, handler)

	sigCh := make(chan os.Signal, 1)
	resultCh := make(chan ShutdownResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := run(context.Background(), srv, ln, discardLogger(), 150*time.Millisecond, sigCh)
		resultCh <- res
		errCh <- err
	}()

	go func() {
		time.Sleep(50 * time.Millisecond)
		sigCh <- syscall.SIGINT
	}()

	// The request blocks in the handler; the response is never delivered.
	go func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr, nil)
		if err != nil {
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.ForceClosed != 1 {
		t.Errorf("ForceClosed = %d, want 1 (stuck handler killed by deadline)", result.ForceClosed)
	}
	if result.Drained != 0 {
		t.Errorf("Drained = %d, want 0 (no request completed during drain)", result.Drained)
	}

	// Let the stuck handler finish so no goroutine leaks past the test.
	close(block)
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Error("handler did not return after being unblocked")
	}
}

// TestRunCatchesSIGTERM verifies the public Run wrapper wires the real
// SIGTERM/SIGINT signals into the drain logic. A safety channel registered
// before the kill prevents process termination if Run's signal.Notify has not
// fired yet.
// TestRunCatchesSIGTERM verifies the public Run wrapper wires the real
// SIGTERM/SIGINT signals into the drain logic: the process self-kills and Run
// must return instead of blocking forever. No request is in flight — Run owns
// its own listener (srv.Addr), so the drain result is zero; the point is the
// signal wiring. A safety channel registered before the kill prevents process
// termination if Run's signal.Notify has not fired yet.
func TestRunCatchesSIGTERM(t *testing.T) {
	srv := &http.Server{Handler: http.NotFoundHandler(), Addr: "127.0.0.1:0", ReadHeaderTimeout: 5 * time.Second}
	t.Cleanup(func() { _ = srv.Close() })

	safety := make(chan os.Signal, 1)
	signal.Notify(safety, syscall.SIGTERM)
	defer signal.Stop(safety)

	resultCh := make(chan ShutdownResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := Run(context.Background(), srv, discardLogger())
		resultCh <- res
		errCh <- err
	}()

	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("self SIGTERM: %v", err)
	}

	select {
	case result := <-resultCh:
		if err := <-errCh; err != nil {
			t.Fatalf("Run: %v", err)
		}
		if result.Drained != 0 || result.ForceClosed != 0 {
			t.Errorf("result = %+v, want zero result (no in-flight requests)", result)
		}
	case <-time.After(5 * time.Second):
		t.Error("Run did not return after SIGTERM")
	}
}

// TestRunDrainsOnContextCancellation: a cancelled context is a teardown
// trigger like a signal; with no requests in flight the drain completes
// immediately and reports a zero result.
func TestRunDrainsOnContextCancellation(t *testing.T) {
	srv, ln, _ := newTestHTTPServer(t, http.NotFoundHandler())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resultCh := make(chan ShutdownResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := run(ctx, srv, ln, discardLogger(), 10*time.Second, make(chan os.Signal))
		resultCh <- res
		errCh <- err
	}()

	select {
	case result := <-resultCh:
		if err := <-errCh; err != nil {
			t.Fatalf("run: %v", err)
		}
		if result.Drained != 0 || result.ForceClosed != 0 {
			t.Errorf("result = %+v, want zero result on cancelled context", result)
		}
	case <-time.After(2 * time.Second):
		t.Error("run did not return after context cancellation")
	}
}

// TestRunReturnsServeError: when the listener dies before any signal, run
// must report the failure instead of waiting forever — a supervisor needs the
// process to exit non-zero rather than sit alive while serving nothing.
func TestRunReturnsServeError(t *testing.T) {
	srv := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: 5 * time.Second}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	resultCh := make(chan ShutdownResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := run(context.Background(), srv, ln, discardLogger(), 10*time.Second, make(chan os.Signal))
		resultCh <- res
		errCh <- err
	}()

	select {
	case result := <-resultCh:
		if result.Drained != 0 || result.ForceClosed != 0 {
			t.Errorf("result = %+v, want zero result on serve failure", result)
		}
	case <-time.After(2 * time.Second):
		t.Error("run did not return after serve failure")
	}

	if err := <-errCh; err == nil {
		t.Error("run returned nil error on serve failure, want non-nil")
	}
}

// TestRunSecondSignalForcesImmediateClose verifies the second-signal path
// (REQ-MT-010): a request is stuck in the handler when the first signal
// starts the drain; a SECOND signal during the drain must force-close the
// connection immediately instead of waiting out the (10s) grace deadline.
func TestRunSecondSignalForcesImmediateClose(t *testing.T) {
	started := make(chan struct{})
	handlerDone := make(chan struct{})
	block := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		defer close(handlerDone)
		<-block
		w.WriteHeader(http.StatusOK)
	})
	srv, ln, addr := newTestHTTPServer(t, handler)

	sigCh := make(chan os.Signal, 2)
	resultCh := make(chan ShutdownResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := run(context.Background(), srv, ln, discardLogger(), 10*time.Second, sigCh)
		resultCh <- res
		errCh <- err
	}()

	go func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr, nil)
		if err != nil {
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	// Deterministic ordering: the handler is inside user code when the first
	// signal arrives; the second signal lands during the drain, long before
	// the 10s deadline.
	<-started
	sigCh <- syscall.SIGINT
	sigCh <- syscall.SIGTERM

	select {
	case result := <-resultCh:
		if result.ForceClosed != 1 {
			t.Errorf("ForceClosed = %d; want 1 (second signal killed the stuck connection)", result.ForceClosed)
		}
		if result.Drained != 0 {
			t.Errorf("Drained = %d; want 0 (no request completed during drain)", result.Drained)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after the second signal (force close)")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("run: %v", err)
	}

	// Unblock the handler so no goroutine leaks past the test.
	close(block)
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Error("handler did not return after being unblocked")
	}
}
