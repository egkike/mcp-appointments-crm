package mcp

import (
	"fmt"
	"net"
)

// LoopbackError reports a rejected MCP_BIND value at startup (ADR-0007 §D4).
type LoopbackError struct {
	Message string
}

func (e *LoopbackError) Error() string { return e.Message }

// ValidateLoopback verifies that bind is a literal loopback IP (127.0.0.0/8 or
// ::1) per REQ-MT-001 / ADR-0007 §D4. Wildcard binds, hostnames and
// non-loopback addresses fail fast with a Spanish error before any socket is
// opened. The server never binds to a non-loopback interface.
func ValidateLoopback(bind string) error {
	if bind == "0.0.0.0" {
		return &LoopbackError{
			Message: "Error: MCP_BIND=0.0.0.0 expone el server en TODAS las interfaces. Use solo direcciones loopback (127.0.0.0/8 o ::1).",
		}
	}
	ip := net.ParseIP(bind)
	if ip == nil {
		return &LoopbackError{
			Message: fmt.Sprintf("Error: MCP_BIND=%s es un hostname, no una IP. Use la IP literal (127.0.0.1 o ::1).", bind),
		}
	}
	if !ip.IsLoopback() {
		return &LoopbackError{
			Message: fmt.Sprintf("Error: MCP_BIND=%s no es una dirección loopback. Use 127.0.0.1 (IPv4) o ::1 (IPv6).", bind),
		}
	}
	return nil
}
