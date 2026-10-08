package usecase

import (
	"log/slog"
	"time"
)

// Registration audit trail (change feat-whatsapp-bot, design §1.3 step 6).
//
// Exactly one structured record is emitted per outcome — created, no-op,
// account rejection, legacy-id rejection and rate-limit rejection — at the
// use-case layer, mirroring
// the maintenance_audit.go pattern: an RFC3339Nano "ts" plus the operation
// identifiers. The phone is ALWAYS masked: a full number is PII and the spec
// forbids logging it.
const registrationAuditMessage = "client registration"

// Registration audit event identifiers (spec client-registration).
const (
	registrationEventRegistered      = "client_registered"
	registrationEventNoop            = "client_registration_noop"
	registrationEventRejectedAccount = "registration_rejected_account"
	registrationEventRejectedLegacy  = "registration_rejected_legacy_id"
	registrationEventRateLimited     = "registration_rate_limited"
)

// registrationLogger normalizes the optional logger dependency. A nil logger
// falls back to slog.Default() (same fail-safe as maintenanceLogger) so a
// wiring mistake degrades to the default sink instead of nil-dereferencing on
// the registration path.
func registrationLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

// maskPhone keeps only the last 4 digits of phone and replaces the rest with
// "***" (spec: the audit value MUST be "masked to its last 4 digits"). Values
// short enough that the last-4 rule would expose the whole number collapse to
// "***", so the full phone can never reach the log.
func maskPhone(phone string) string {
	if len(phone) <= 4 {
		return "***"
	}
	return "***" + phone[len(phone)-4:]
}

// logRegistrationEvent emits the single structured audit record of one
// registration outcome. Attribute naming mirrors the maintenance audit record.
func logRegistrationEvent(logger *slog.Logger, event, phone string) {
	logger.Info(registrationAuditMessage,
		"event", event,
		"phone", maskPhone(phone),
		"ts", time.Now().UTC().Format(time.RFC3339Nano),
	)
}
