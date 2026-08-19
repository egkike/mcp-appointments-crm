// Package buildinfo exposes build-time metadata injected via -ldflags.
package buildinfo

// Version is the application version, set at build time with
//
//	-ldflags "-X github.com/egkike/mcp-appointments-crm/internal/buildinfo.Version=$(git describe)"
//
// It defaults to "dev" when the binary is built without ldflags.
var Version = "dev"
