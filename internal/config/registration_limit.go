package config

import (
	"os"
	"strconv"
)

// RegistrationRateLimitEnv overrides the per-phone registration rate limit.
// It follows the existing MCP_* environment convention (MCP_DB_PATH,
// MCP_SETUP_DIR, MCP_CONFIG_DIR, ...).
const RegistrationRateLimitEnv = "MCP_REGISTRATION_RATE_LIMIT"

// DefaultRegistrationRateLimit is the conservative per-phone cap per hour
// (design §1.5: "default 10/hour").
const DefaultRegistrationRateLimit = 10

// RegistrationRateLimit resolves the per-phone fixed-window registration limit
// from the environment (design §1.5):
//
//   - unset/empty → DefaultRegistrationRateLimit (10/hour);
//   - "0" → 0, which disables auto-registration entirely (fail closed);
//   - a positive integer → that value;
//   - malformed or negative → DefaultRegistrationRateLimit, so a typo can
//     never silently remove the limit.
func RegistrationRateLimit() int {
	return registrationRateLimitFrom(os.Getenv)
}

// registrationRateLimitFrom is the injectable core of RegistrationRateLimit.
func registrationRateLimitFrom(getenv func(string) string) int {
	raw := getenv(RegistrationRateLimitEnv)
	if raw == "" {
		return DefaultRegistrationRateLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return DefaultRegistrationRateLimit
	}
	return n
}
