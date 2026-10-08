package config

import "testing"

func TestRegistrationRateLimit(t *testing.T) {
	getenv := func(vals map[string]string) func(string) string {
		return func(key string) string { return vals[key] }
	}

	t.Run("unset falls back to the 10/hour default", func(t *testing.T) {
		if got := registrationRateLimitFrom(getenv(nil)); got != DefaultRegistrationRateLimit {
			t.Errorf("limit = %d; want default %d", got, DefaultRegistrationRateLimit)
		}
	})

	t.Run("positive override wins", func(t *testing.T) {
		vals := map[string]string{RegistrationRateLimitEnv: "25"}
		if got := registrationRateLimitFrom(getenv(vals)); got != 25 {
			t.Errorf("limit = %d; want 25", got)
		}
	})

	t.Run("zero stays zero (auto-registration disabled, fail closed)", func(t *testing.T) {
		vals := map[string]string{RegistrationRateLimitEnv: "0"}
		if got := registrationRateLimitFrom(getenv(vals)); got != 0 {
			t.Errorf("limit = %d; want 0 (disabled)", got)
		}
	})

	t.Run("malformed falls back to the default", func(t *testing.T) {
		for _, raw := range []string{"ten", "10.5", " ", "1e3"} {
			vals := map[string]string{RegistrationRateLimitEnv: raw}
			if got := registrationRateLimitFrom(getenv(vals)); got != DefaultRegistrationRateLimit {
				t.Errorf("limit for %q = %d; want default %d", raw, got, DefaultRegistrationRateLimit)
			}
		}
	})

	t.Run("negative falls back to the default", func(t *testing.T) {
		vals := map[string]string{RegistrationRateLimitEnv: "-1"}
		if got := registrationRateLimitFrom(getenv(vals)); got != DefaultRegistrationRateLimit {
			t.Errorf("limit = %d; want default %d", got, DefaultRegistrationRateLimit)
		}
	})
}
