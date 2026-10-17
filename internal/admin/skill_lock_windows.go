//go:build windows

package admin

import (
	"os"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// lockFile is a no-op on Windows: cross-file advisory locks are not portable
// and the consent write's byte comparison is its guard there. The skill has a
// single writer (this flow) by design; note kept for parity (GGA round 20).
func lockFile(*os.File) *domain.SemanticError { return nil }

// unlockFile is the Windows counterpart no-op of the POSIX advisory unlock.
func unlockFile(*os.File) {}
