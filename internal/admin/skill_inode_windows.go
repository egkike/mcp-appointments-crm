//go:build windows

package admin

import (
	"os"
)

// guardSkillSwap is a no-op on Windows: the syscall.Stat_t inode comparison is
// POSIX-shaped, and there the consent write's byte comparison under the advisory
// lock is the documented guard (mirrors the skill_lock split rationale).
func guardSkillSwap(f *os.File, path string) error {
	_ = f
	_ = path
	return nil
}
