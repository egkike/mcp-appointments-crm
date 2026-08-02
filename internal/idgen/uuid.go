// Package idgen provides shared identifier generation helpers for the
// application layer. Currently it generates UUID v4 strings suitable for
// new entity IDs (bookings, alerts, etc.).
package idgen

import (
	"crypto/rand"
	"fmt"
)

// New returns a UUID v4 string using crypto/rand for cryptographic strength.
// The format is xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx, where the version
// (4) and variant (8/9/a/b) bits are set per RFC 4122.
//
// Each call reads 16 bytes of randomness and mutates the version + variant
// bits in place. Returns an error only if crypto/rand fails (extremely
// rare; usually indicates a broken kernel RNG).
func New() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("idgen: read entropy: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
