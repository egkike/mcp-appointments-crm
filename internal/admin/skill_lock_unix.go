//go:build !windows

package admin

import (
	"os"
	"runtime"
	"syscall"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// lockFile takes an exclusive advisory lock on the skill, non-blocking: a
// concurrent writer makes the consent write fail closed (GGA round 18).
// POSIX-only build tag: syscall.Flock does not exist on Windows (round 20).
func lockFile(f *os.File) *domain.SemanticError {
	_ = runtime.GOOS
	if lerr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); lerr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "la skill está en uso por otro proceso; no se reemplazó",
			Cause:   lerr,
		}
	}
	return nil
}

// unlockFile releases the advisory lock; a release failure cannot undo the
// write and carries no retry path, so it is swallowed on purpose.
func unlockFile(f *os.File) {
	// #nosec G104 -- no meaningful recovery exists for the release itself.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
