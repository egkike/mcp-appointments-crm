//go:build !windows

package admin

import (
	"os"
	"syscall"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// guardSkillSwap compares the held fd's inode against the current PATH inode
// (feat-skill-inode-guard, B9): divergence means an external rename-replace
// landed after the open, so the flow aborts with the same "re-run
// regeneration" contract the changed-after-consent path uses, and the external
// edit survives untouched. Filesystems without inode semantics (both inos
// zero) skip the guard instead of failing closed on an opaque stat.
func guardSkillSwap(f *os.File, path string) error {
	fdInfo, err := f.Stat()
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo inspeccionar la skill instalada abierta",
			Cause:   err,
		}
	}
	pInfo, perr := os.Stat(path)
	if perr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "la skill instalada desapareció durante la regeneración",
			Cause:   perr,
		}
	}
	fdStat, fdOK := fdInfo.Sys().(*syscall.Stat_t)
	pStat, pOK := pInfo.Sys().(*syscall.Stat_t)
	if !fdOK || !pOK {
		// Opaque sys handles: no inode contract available — no guard, same as
		// the zero-inode filesystems.
		return nil
	}
	if fdStat.Ino != 0 && pStat.Ino != 0 && fdStat.Ino != pStat.Ino {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "la skill fue reemplazada por otra writer después del consentimiento; corré la regeneración de nuevo",
		}
	}
	return nil
}
