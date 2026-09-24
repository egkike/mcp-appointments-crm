// Package loopback classifies bind addresses as literal loopback IPs.
//
// It holds the single accept/reject predicate shared by the MCP server startup
// validation (internal/mcp) and the Hermes endpoint URL builder (internal/admin),
// so the two policies cannot drift. It imports stdlib only and never depends on
// either consumer, which is what keeps the admin -> mcp import at zero.
package loopback

import "net"

// Reason is the classification of a bind string.
type Reason int

const (
	// OK means the bind is a literal loopback IP (127.0.0.0/8 or ::1).
	OK Reason = iota
	// NotAnIP means the bind is not a parseable IP literal (typically a hostname).
	NotAnIP
	// Unspecified means the bind is the unspecified address (0.0.0.0 or ::).
	Unspecified
	// NotLoopback means the bind is a valid IP that is not a loopback address.
	NotLoopback
)

// Classify parses bind as an IP literal and reports whether it is a valid
// loopback address. The parsed IP is never exposed: no caller consumes it, and
// returning it would only widen the surface without adding safety.
//
// The order is load-bearing: an unspecified address is reported before the
// loopback check so 0.0.0.0 and :: map to a stable reason instead of falling
// into the generic non-loopback bucket. The caller owns any trimming or
// empty-input handling; Classify treats bind verbatim.
func Classify(bind string) Reason {
	ip := net.ParseIP(bind)
	if ip == nil {
		return NotAnIP
	}
	if ip.IsUnspecified() {
		return Unspecified
	}
	if !ip.IsLoopback() {
		return NotLoopback
	}
	return OK
}
