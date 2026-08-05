# Tasks: fix-repo-perf-and-auth-gating

## Review Workload Forecast
- **Decision needed before apply**: No
- **Chained PRs recommended**: No
- **400-line budget risk**: Low

## Scope
Single PR addressing two pre-existing issues in the repository layer.

---

## Phase 1: Implementation

### Task 1: Fix N+1 query in ProfessionalsRepo specialty validation (#40)
- [x] 1.1 Collapse per-specialty `SELECT COUNT(*)` loop into single `SELECT id FROM services WHERE id IN (?, ?, ...)` query in `Save`
- [x] 1.2 Apply same fix to `Update` method (identical pattern)
- [x] 1.3 Verify existing tests continue to pass and new N+1-specific tests pass

### Task 2: Add auth gating to write methods (#41)
- [x] 2.1 Add `auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner)` to `ServicesRepo.Save`
- [x] 2.2 Add `auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner)` to `ServicesRepo.Update`
- [x] 2.3 Add `auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner)` to `ServicesRepo.Delete`
- [x] 2.4 Add `auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner)` to `BusinessProfileRepo.Update`
- [x] 2.5 Verify all auth tests pass: admin/owner succeed, unauthenticated/staff/client rejected
