package usecase

import (
	"sync"
	"time"
)

// registrationWindow is the fixed window width of the per-phone registration
// limit (design §1.5: a "fixed 1-hour window counter").
const registrationWindow = time.Hour

// registrationRateLimiter is the in-memory, single-process, per-phone
// fixed-window counter guarding anonymous self-registration (design §1.5).
//
// It lives inside the use-case package so every present and future transport
// inherits the same limit. State is intentionally not persisted: a reset on
// restart is accepted for a loopback server with operator-initiated restarts
// (R4). The clock is injected so the window boundary is deterministic in
// tests.
type registrationRateLimiter struct {
	limit int
	now   func() time.Time

	mu          sync.Mutex
	counts      map[string]int
	windowStart time.Time
}

// NewRegistrationRateLimiter builds the limiter the composition root wires into
// the registration use case, using the real clock. Tests inside this package
// construct it through newRegistrationRateLimiter to inject a deterministic
// clock.
func NewRegistrationRateLimiter(limit int) *registrationRateLimiter {
	return newRegistrationRateLimiter(limit, time.Now)
}

// newRegistrationRateLimiter is the injectable core of the constructor. A nil
// clock falls back to time.Now so a wiring mistake cannot nil-panic on the
// registration path.
func newRegistrationRateLimiter(limit int, now func() time.Time) *registrationRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &registrationRateLimiter{
		limit:  limit,
		now:    now,
		counts: make(map[string]int),
	}
}

// allow records one attempt for phone and reports whether it is within budget.
//
// A limit of 0 (or negative) disables auto-registration entirely and always
// reports false, so the caller answers the rate-limit semantic error: this is
// the fail-closed posture required by design §1.5. Once a phone reaches the
// limit it stays rejected for the rest of the window — rejected attempts do
// not consume extra budget and refreshing the attempt cannot reset it.
func (l *registrationRateLimiter) allow(phone string) bool {
	if l.limit <= 0 {
		return false
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.windowStart.IsZero() || now.Sub(l.windowStart) >= registrationWindow {
		l.windowStart = now
		l.counts = make(map[string]int)
	}
	if l.counts[phone] >= l.limit {
		return false
	}
	l.counts[phone]++
	return true
}
