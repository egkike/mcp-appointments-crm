package usecase

import (
	"testing"
	"time"
)

// rateLimitBase is a fixed instant so the injected clock is fully
// deterministic (design §1.5: injected func() time.Time).
var rateLimitBase = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func TestRegistrationRateLimiter(t *testing.T) {
	const (
		phoneA = "+5491100999999"
		phoneB = "+5491100888888"
	)

	t.Run("limit reached rejects the attempt past the budget", func(t *testing.T) {
		now := rateLimitBase
		limiter := newRegistrationRateLimiter(3, func() time.Time { return now })

		for i := 1; i <= 3; i++ {
			if !limiter.allow(phoneA) {
				t.Fatalf("attempt %d must be allowed (limit 3)", i)
			}
		}
		if limiter.allow(phoneA) {
			t.Fatal("attempt 4 must be rejected: the limit was reached")
		}
	})

	t.Run("a rejected phone stays rejected for the rest of the window", func(t *testing.T) {
		now := rateLimitBase
		limiter := newRegistrationRateLimiter(1, func() time.Time { return now })

		if !limiter.allow(phoneA) {
			t.Fatal("first attempt must be allowed")
		}
		if limiter.allow(phoneA) {
			t.Fatal("second attempt must be rejected")
		}
		// Still inside the same window: refreshing the attempt must not reset
		// the counter (design §1.5: rejections are counted).
		now = now.Add(30 * time.Minute)
		if limiter.allow(phoneA) {
			t.Fatal("attempt later in the same window must still be rejected")
		}
	})

	t.Run("per-phone isolation", func(t *testing.T) {
		now := rateLimitBase
		limiter := newRegistrationRateLimiter(1, func() time.Time { return now })

		if !limiter.allow(phoneA) {
			t.Fatal("phone A first attempt must be allowed")
		}
		if limiter.allow(phoneA) {
			t.Fatal("phone A second attempt must be rejected")
		}
		if !limiter.allow(phoneB) {
			t.Fatal("phone B must not be affected by phone A exhausting its budget")
		}
	})

	t.Run("window rollover resets the counter", func(t *testing.T) {
		now := rateLimitBase
		limiter := newRegistrationRateLimiter(1, func() time.Time { return now })

		if !limiter.allow(phoneA) {
			t.Fatal("first attempt must be allowed")
		}
		if limiter.allow(phoneA) {
			t.Fatal("same-window second attempt must be rejected")
		}

		now = rateLimitBase.Add(registrationWindow)
		if !limiter.allow(phoneA) {
			t.Fatal("a new window must reset the per-phone counter")
		}
	})

	t.Run("limit zero disables registration entirely (fail closed)", func(t *testing.T) {
		now := rateLimitBase
		limiter := newRegistrationRateLimiter(0, func() time.Time { return now })

		if limiter.allow(phoneA) {
			t.Fatal("limit 0 must reject every attempt")
		}
		if limiter.allow(phoneB) {
			t.Fatal("limit 0 must reject every phone, not just one")
		}
	})
}
