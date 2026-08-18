package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// shutdownDeadline is the grace period granted to in-flight requests once a
// termination signal arrives. Connections still active after the deadline are
// force-closed.
const shutdownDeadline = 10 * time.Second

// ShutdownResult reports the outcome of a graceful shutdown.
type ShutdownResult struct {
	// Drained counts requests whose in-flight processing completed during
	// the grace period (each completion on a keep-alive connection counts
	// once).
	Drained int
	// ForceClosed counts in-flight connections the server killed when the
	// drain ended — either because the grace period expired or because a
	// second signal demanded an immediate exit.
	ForceClosed int
}

// Run listens on srv.Addr and serves requests until a termination signal
// (SIGTERM or SIGINT) arrives or ctx is cancelled. Both triggers drain
// in-flight requests with a bounded grace period: srv.Shutdown is given
// shutdownDeadline, and connections still active afterwards are force-closed
// via srv.Close. A second signal during the drain forces an immediate close.
//
// Bind-address safety (loopback-only) is enforced at configuration time by
// ValidateLoopback, not here: Run is a generic HTTP server runner and must
// not duplicate policy from the composition root.
func Run(ctx context.Context, srv *http.Server, logger *slog.Logger) (ShutdownResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return ShutdownResult{}, fmt.Errorf("listen on %s: %w", srv.Addr, err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	return run(ctx, srv, ln, logger, shutdownDeadline, sigCh)
}

// run is the testable core of Run: the listener, the signal source and the
// drain deadline are injected so tests drive the shutdown deterministically.
func run(ctx context.Context, srv *http.Server, ln net.Listener, logger *slog.Logger, deadline time.Duration, sigCh <-chan os.Signal) (ShutdownResult, error) {
	var (
		mu       sync.Mutex
		active   = make(map[net.Conn]struct{})
		draining bool
		result   ShutdownResult
	)

	// Track connections so the drain outcome is measurable. While Shutdown
	// is waiting, the server never kills an active connection: a connection
	// that leaves the active set during the grace period finished its
	// response on its own (clean drain). Only connections still active when
	// the drain ends are force-closed. The hook is installed before Serve
	// so no connection can be handled without the tracker; a hook installed
	// by the caller is composed, not replaced.
	prevConnState := srv.ConnState
	srv.ConnState = func(c net.Conn, state http.ConnState) {
		if prevConnState != nil {
			prevConnState(c, state)
		}

		mu.Lock()
		defer mu.Unlock()

		switch state {
		case http.StateActive:
			active[c] = struct{}{}
			return
		default: // StateNew, StateIdle, StateClosed, StateHijacked
		}

		if _, wasActive := active[c]; !wasActive {
			return
		}
		delete(active, c)
		if draining {
			result.Drained++
		}
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	// Wait for the first termination trigger. A Serve failure is one too:
	// the process must not sit alive while serving nothing.
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return result, nil
		}
		return result, fmt.Errorf("serve: %w", err)
	case sig := <-sigCh:
		logger.Info("shutdown: drain requested", "signal", sig.String())
	case <-ctx.Done():
		logger.Info("shutdown: drain requested via context cancellation")
	}

	mu.Lock()
	draining = true
	mu.Unlock()

	drainCtx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- srv.Shutdown(drainCtx)
	}()

	// The grace period runs until Shutdown returns (clean drain or expired
	// deadline). A second signal during the drain short-circuits it.
	forced := false
	select {
	case <-shutdownDone:
		forced = drainCtx.Err() != nil
	case <-sigCh:
		logger.Info("shutdown: second signal received, forcing immediate close")
		forced = true
		_ = srv.Close()
		<-shutdownDone
	}

	if forced {
		logger.Info("shutdown: force-closing remaining active connections", "elapsed", time.Since(start))
		_ = srv.Close()

		// Close() tears down the raw connections immediately, but the
		// StateClosed transition only fires when each connection's serve
		// goroutine unwinds — a handler stuck in user code may never reach
		// it. Count the kill here, when it actually happens.
		mu.Lock()
		for c := range active {
			delete(active, c)
			result.ForceClosed++
		}
		mu.Unlock()
	} else {
		logger.Info("shutdown: drained cleanly", "elapsed", time.Since(start))
	}

	mu.Lock()
	defer mu.Unlock()
	return result, nil
}
