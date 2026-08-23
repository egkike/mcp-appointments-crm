# Apply Progress — feat-repository-auth-integration

## Mode
Strict TDD (`go test -v -race`).

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| T-01 | `internal/repository/applyClientsAuthFilter_test.go` | Unit | baseline green | Written | Passed | 9 cases | Clean |
| T-02..T-05 | `internal/repository/clients_role_test.go`, `clients_scope_test.go`, `getorcreate_roles_test.go`, `clients_test.go` | Unit | baseline green | Written | Passed | role matrix + scope + oracle | Clean |
| T-06..T-07 | `internal/repository/business_hours_exception_roles_test.go`, `business_hours_exception_test.go` | Unit | baseline green | Written | Passed | role matrix + hot path | Clean |
| T-08..T-09 | `internal/repository/*_test.go` | Unit | all green | Migrated | Passed | new role cases | Clean |
| T-10 | whole repo | Integration | all green | N/A | N/A | N/A | Clean |

## Completed Tasks
- [x] T-01: `applyClientsAuthFilter` helper
- [x] T-02..T-05: Clients wiring (Save/Create/Update/Delete, FindByPhone, FindByID, SearchFTS, GetOrCreate)
- [x] T-06..T-07: BHE wiring (Create/Delete `RequireRole`; Get/List `RequireCaller`)
- [x] T-08..T-09: Test migration and role matrix tests
- [x] T-10: Final verification

## Verification Results
- `go fmt ./...` pass
- `go vet ./...` pass
- `golangci-lint run ./...` pass (0 issues)
- `go build -o /dev/null ./...` pass
- `go test -v -race ./...` pass (all packages)
- Coverage `internal/repository`: 88.4%

## Files Changed
- `internal/repository/clients.go` — auth wiring + `applyClientsAuthFilter`
- `internal/repository/business_hours_exception.go` — auth wiring
- `internal/repository/*_test.go` — role matrix, scope, DoS tests
- `openspec/changes/feat-repository-auth-integration/*.md` — SDD artifacts

## Commits
- `0e4633d` feat(repository): T-01 add applyClientsAuthFilter helper for client self-scope
- `17b7ddf` feat(repository): T-02..T-07 wire auth into clients and business_hours_exception repos
- `660f1d9` docs(sdd): add proposal, specs, design and tasks for feat-repository-auth-integration
- `188e92f` test(repository): T-08..T-09 add role matrix and DoS-planting tests for auth wiring
