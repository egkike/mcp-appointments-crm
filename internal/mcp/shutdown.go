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

	return run(ctx, srv, ln, logger, shutdownDeadline, sigCh, true)
}

// run is the testable core of Run: the listener, the signal source and the
// drain deadline are injected so tests drive the shutdown deterministically.
// dedupeSignalTrigger toggles the production-only dedupe of a duplicate copy
// of the FIRST trigger signal: mcp.Run verifies a real runtime subscription so
// the same SIGTERM/SIGINT delivery is registered BOTH by the caller's
// NotifyContext and by our sigCh, and a queued copy left on sigCh would be
// misread as a second operator request (GGA round 22). Direct run() tests use
// a private signal channel where each write is a deliberate operator
// delivery, hence no dedupe.
func run(ctx context.Context, srv *http.Server, ln net.Listener, logger *slog.Logger, deadline time.Duration, sigCh <-chan os.Signal, dedupeSignalTrigger bool) (ShutdownResult, error) {
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
	// The drain completion signal carries the Shutdown error back: a drain can
	// end with an error (GGA round 8: drainCtx.Err()==nil does NOT prove a clean
	// drain), so the receivers force-close on it instead of trusting only the
	// context state.
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- srv.Shutdown(drainCtx)
	}()

	// The grace period runs until Shutdown returns (clean drain or expired
	// deadline). A second signal during the drain short-circuits it (GGA
	// round 22: the drain's own subscription starts AFTER the trigger — the
	// FIRST signal fired the trigger through BOTH the caller's NotifyContext
	// and OUR sigCh, and a copy still queued in sigCh must never be misread
	// as a second signal; the queued copy is drained first, and the fresh
	// channel receives only new deliveries).
	// GGA round 22: in production, the FIRST trigger signal is registered by
	// BOTH the caller's NotifyContext and our sigCh; the duplicate copy left
	// queued in sigCh would be misread as a second operator request during the
	// drain. Drain it once, only when the production subscription is in play.
	if dedupeSignalTrigger {
		drainQueued(sigCh)
	}

	forced := false
	select {
	case drainErr := <-shutdownDone:
		// The drain can end with an error even when the context deadline did not
		// fire (GGA round 8); any drain error forces the close, same as a deadline
		// expiry — the forced path below already logs the elapsed time.
		forced = drainCtx.Err() != nil || drainErr != nil
		if drainErr != nil {
			logger.Info("shutdown: drain ended with error, forcing close", "elapsed", time.Since(start))
		}
	case <-sigCh:
		// Anything still queued NOW (> nothing) is a genuine second signal:
		// the trigger copy was drained at the start of the drain (GGA 22).
		logger.Info("shutdown: second signal received, forcing immediate close")
		forced = true
		// Classify BEFORE the raw Close: the ConnState callback for a killed
		// connection can unwind at any moment, and a callback that runs after
		// the snapshot sees an already-removed entry (classified exactly once
		// by whoever deletes it under the lock; GGA round 9).
		mu.Lock()
		result.ForceClosed += len(active)
		active = make(map[net.Conn]struct{})
		mu.Unlock()
		_ = srv.Close()
		// Await the drain so the goroutine never outlives the flow; its outcome
		// is already handled by the classification above.
		if drainErr := <-shutdownDone; drainErr != nil {
			logger.Info("shutdown: forced drain also returned with error")
		}
	}

	if forced {
		logger.Info("shutdown: force-closing remaining active connections", "elapsed", time.Since(start))
		// Snapshot and classify BEFORE the raw Close (GGA round 9): the
		// StateClosed callback of a just-killed connection can fire before we
		// reach the count, and that callback runs as Drained because draining is
		// already set — snapshotting first pins every still-active connection as
		// classified exactly once (by this deletion, or by its own callback if
		// it finishes in the tiny window: whichever removes it under the lock).
		mu.Lock()
		result.ForceClosed += len(active)
		active = make(map[net.Conn]struct{})
		mu.Unlock()
		_ = srv.Close()
		// Close() tears down the raw connections immediately; the serve
		// goroutines unwind against an already-classified set. A handler stuck
		// in user code never reaches StateClosed — the count above is the kill
		// itself.
	} else {
		logger.Info("shutdown: drained cleanly", "elapsed", time.Since(start))
	}

	mu.Lock()
	defer mu.Unlock()
	return result, nil
}

// drainQueued consumes every signal already queued on sigCh NON-blocking, so
// the copy of the FIRST trigger signal can never be re-read as a second
// operator request (GGA round 22). second-signal deliveries after this point
// land on drainSignals, which was subscribed only after the trigger fired.
func drainQueued(sigCh <-chan os.Signal) {
	for {
		select {
		case <-sigCh:
			// TRIGGER copy: discarted on purpose.
			continue
		default:
			return
		}
	}
}
