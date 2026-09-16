package admin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Exact query text mirrors internal/repository/accounts.go; the mock uses
// QueryMatcherEqual so a repository query change fails this suite loudly.
const (
	selectAllAccountsQuery  = `SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts ORDER BY created_at ASC`
	selectAccountStateQuery = `SELECT is_active, role FROM accounts WHERE id = ?`
	updateDeactivateQuery   = `UPDATE accounts SET is_active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`
)

// accountRow describes one accounts row for the mock result sets.
type accountRow struct {
	id             string
	role           entity.AccountRole
	displayName    string
	professionalID *string
	isActive       int
}

// accountsRows builds the accounts result set in the given order, using the
// column set AccountsRepo scans.
func accountsRows(rows ...accountRow) *sqlmock.Rows {
	result := sqlmock.NewRows(accountColumns())
	for _, row := range rows {
		result.AddRow(row.id, string(row.role), row.displayName, row.professionalID, row.isActive,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
	}
	return result
}

// accountFixture builds the entity row a fake port returns.
func accountFixture(id string, role entity.AccountRole, active bool) *entity.Account {
	return &entity.Account{ID: id, Role: role, DisplayName: "Cuenta " + id, Active: active}
}

// fakeAccountsAdmin records every port call and returns scripted rows, so the
// core's pre-checks can be tested without a database.
type fakeAccountsAdmin struct {
	listCalls  int
	listRows   []*entity.Account
	listErr    error
	listCaller auth.Caller

	roleCalls int
	roleArg   entity.AccountRole
	roleRows  []*entity.Account
	roleErr   error

	deactivateCalls  int
	deactivatedID    string
	deactivateErr    error
	deactivateCaller auth.Caller
	deactivateHas    bool
}

func (f *fakeAccountsAdmin) List(ctx context.Context) ([]*entity.Account, error) {
	f.listCalls++
	f.listCaller, _ = auth.FromContext(ctx)
	return f.listRows, f.listErr
}

func (f *fakeAccountsAdmin) GetByRole(ctx context.Context, role entity.AccountRole) ([]*entity.Account, error) {
	f.roleCalls++
	f.roleArg = role
	return f.roleRows, f.roleErr
}

func (f *fakeAccountsAdmin) Deactivate(ctx context.Context, id string) error {
	f.deactivateCalls++
	f.deactivatedID = id
	f.deactivateCaller, f.deactivateHas = auth.FromContext(ctx)
	return f.deactivateErr
}

func TestMapAccountViews(t *testing.T) {
	t.Run("an empty result maps to an empty slice, never nil", func(t *testing.T) {
		got := mapAccountViews(nil, true)
		if got == nil {
			t.Error("mapAccountViews(nil, true) = nil, want an empty slice")
		}
		if len(got) != 0 {
			t.Errorf("mapAccountViews(nil, true) length = %d, want 0", len(got))
		}
	})

	t.Run("rows keep the repository order and project every column", func(t *testing.T) {
		rows := []*entity.Account{
			{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Dueño", Active: true},
			{ID: "+5491100000001", Role: entity.RoleStaff, DisplayName: "Ana Staff", ProfessionalID: strPtr("prof-1"), Active: true},
			{ID: "+5491100000002", Role: entity.RoleAdmin, DisplayName: "Admin", Active: true},
		}

		got := mapAccountViews(rows, false)
		if len(got) != 3 {
			t.Fatalf("mapAccountViews() length = %d, want 3", len(got))
		}
		for i, want := range []string{"+5491100000000", "+5491100000001", "+5491100000002"} {
			if got[i].ID != want {
				t.Errorf("view[%d].ID = %q, want %q", i, got[i].ID, want)
			}
		}
		if got[0].Role != entity.RoleOwner || got[2].Role != entity.RoleAdmin {
			t.Errorf("roles = %q/%q, want owner/admin", got[0].Role, got[2].Role)
		}
		if got[1].ProfessionalID != "prof-1" {
			t.Errorf("view[1].ProfessionalID = %q, want prof-1", got[1].ProfessionalID)
		}
		if got[0].ProfessionalID != "" {
			t.Errorf("view[0].ProfessionalID = %q, want an empty reference for an owner", got[0].ProfessionalID)
		}
		if !got[0].Active {
			t.Error("view[0].Active = false, want the active state preserved")
		}
	})

	t.Run("inactive rows are hidden by default and shown on request", func(t *testing.T) {
		rows := []*entity.Account{
			accountFixture("+5491100000000", entity.RoleOwner, true),
			accountFixture("+5491100000001", entity.RoleStaff, false),
		}

		activeOnly := mapAccountViews(rows, false)
		if len(activeOnly) != 1 {
			t.Fatalf("mapAccountViews(rows, false) length = %d, want 1 active row", len(activeOnly))
		}
		if activeOnly[0].ID != "+5491100000000" {
			t.Errorf("active row = %q, want the owner", activeOnly[0].ID)
		}

		withInactive := mapAccountViews(rows, true)
		if len(withInactive) != 2 {
			t.Fatalf("mapAccountViews(rows, true) length = %d, want both rows", len(withInactive))
		}
		if withInactive[1].Active {
			t.Error("withInactive[1].Active = true, want the inactive row reported as inactive")
		}
	})
}

func TestListAccounts_UsesTheFullListWithoutAFilter(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
		accountRow{id: "+5491100000001", role: entity.RoleStaff, displayName: "Ana Staff", professionalID: strPtr("prof-1"), isActive: 1},
		accountRow{id: "+5491100000002", role: entity.RoleAdmin, displayName: "Ex Admin", isActive: 0},
	))

	got, err := ListAccounts(context.Background(), accounts, ListFilter{})
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAccounts() length = %d, want 2 active rows", len(got))
	}
	if got[0].ID != "+5491100000000" || got[1].ID != "+5491100000001" {
		t.Errorf("ListAccounts() ids = %q/%q, want the repository order", got[0].ID, got[1].ID)
	}
}

func TestListAccounts_IncludesInactiveRowsOnlyOnRequest(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
		accountRow{id: "+5491100000001", role: entity.RoleStaff, displayName: "Ana Staff", isActive: 0},
	))

	got, err := ListAccounts(context.Background(), accounts, ListFilter{IncludeInactive: true})
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAccounts() length = %d, want both rows", len(got))
	}
	if got[1].ID != "+5491100000001" || got[1].Active {
		t.Errorf("inactive row = %+v, want the deactivated staff account", got[1])
	}
}

func TestListAccounts_ByRoleUsesTheRoleQuery(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectOwnersQuery).WithArgs("owner").WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
	))

	got, err := ListAccounts(context.Background(), accounts, ListFilter{Role: entity.RoleOwner})
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(got) != 1 || got[0].Role != entity.RoleOwner {
		t.Fatalf("ListAccounts() = %+v, want the single owner", got)
	}
}

func TestListAccounts_KeepsThePortOrderAndTheRequestedRole(t *testing.T) {
	fake := &fakeAccountsAdmin{roleRows: []*entity.Account{
		accountFixture("+5491100000001", entity.RoleStaff, true),
		accountFixture("+5491100000002", entity.RoleStaff, true),
	}}

	got, err := ListAccounts(context.Background(), fake, ListFilter{Role: entity.RoleStaff})
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if fake.listCalls != 0 {
		t.Errorf("ListAccounts() called List %d times for a role filter, want 0", fake.listCalls)
	}
	if fake.roleCalls != 1 || fake.roleArg != entity.RoleStaff {
		t.Errorf("GetByRole calls = %d with role %q, want 1 with %q", fake.roleCalls, fake.roleArg, entity.RoleStaff)
	}
	if len(got) != 2 || got[0].ID != "+5491100000001" || got[1].ID != "+5491100000002" {
		t.Errorf("ListAccounts() = %+v, want the port order preserved", got)
	}
}

func TestListAccounts_InvalidRoleIsRejectedBeforeThePort(t *testing.T) {
	fake := &fakeAccountsAdmin{}

	got, err := ListAccounts(context.Background(), fake, ListFilter{Role: entity.AccountRole("root")})
	if err == nil {
		t.Fatal("ListAccounts() error = nil, want the invalid-role rejection")
	}
	if got != nil {
		t.Errorf("ListAccounts() = %+v, want no rows for an invalid role", got)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("ListAccounts() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("ListAccounts() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
	if !strings.Contains(semErr.Message, "root") {
		t.Errorf("ListAccounts() message = %q, want it to name the rejected role", semErr.Message)
	}
	if fake.listCalls != 0 || fake.roleCalls != 0 {
		t.Errorf("ListAccounts() reached the port (list=%d, role=%d) for an invalid role, want 0",
			fake.listCalls, fake.roleCalls)
	}
}

func TestListAccounts_RepoErrorIsWrapped(t *testing.T) {
	t.Run("full list failure", func(t *testing.T) {
		fake := &fakeAccountsAdmin{listErr: errors.New("database is down")}

		_, err := ListAccounts(context.Background(), fake, ListFilter{})
		if err == nil {
			t.Fatal("ListAccounts() error = nil, want the port failure")
		}
		if !strings.Contains(err.Error(), "listar cuentas") {
			t.Errorf("ListAccounts() error = %v, want it to wrap the listing", err)
		}
	})

	t.Run("role list failure", func(t *testing.T) {
		fake := &fakeAccountsAdmin{roleErr: errors.New("database is down")}

		_, err := ListAccounts(context.Background(), fake, ListFilter{Role: entity.RoleOwner})
		if err == nil {
			t.Fatal("ListAccounts() error = nil, want the port failure")
		}
		if !strings.Contains(err.Error(), "listar cuentas por rol") {
			t.Errorf("ListAccounts() error = %v, want it to wrap the role listing", err)
		}
	})
}

func TestDeactivateAccount_DeactivatesAnActiveAccount(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
		accountRow{id: "+5491100000001", role: entity.RoleStaff, displayName: "Ana Staff", professionalID: strPtr("prof-1"), isActive: 1},
	))
	mock.ExpectQuery(selectAccountStateQuery).WithArgs("+5491100000001").
		WillReturnRows(sqlmock.NewRows([]string{"is_active", "role"}).AddRow(1, "staff"))
	mock.ExpectExec(updateDeactivateQuery).WithArgs("+5491100000001").
		WillReturnResult(sqlmock.NewResult(0, 1))

	outcome, err := DeactivateAccount(context.Background(), accounts, "+5491100000001")
	if err != nil {
		t.Fatalf("DeactivateAccount() error = %v", err)
	}
	if outcome.AlreadyInactive {
		t.Error("outcome.AlreadyInactive = true, want a real deactivation")
	}
	if outcome.Account.ID != "+5491100000001" {
		t.Errorf("outcome.Account.ID = %q, want the deactivated account", outcome.Account.ID)
	}
	if outcome.Account.DisplayName != "Ana Staff" {
		t.Errorf("outcome.Account.DisplayName = %q, want the operator-facing name", outcome.Account.DisplayName)
	}
	if outcome.Account.Role != entity.RoleStaff {
		t.Errorf("outcome.Account.Role = %q, want %q", outcome.Account.Role, entity.RoleStaff)
	}
	if outcome.Account.ProfessionalID != "prof-1" {
		t.Errorf("outcome.Account.ProfessionalID = %q, want prof-1", outcome.Account.ProfessionalID)
	}
}

// TestDeactivateAccount_AlreadyInactiveIsAnOutcomeNotAWrite locks the
// idempotency contract: the operator asked for a state the account is already
// in, so the flow reports the outcome and does NOT issue another write. The
// unmet-expectation check in newMockAccountsRepo proves the UPDATE never ran.
func TestDeactivateAccount_AlreadyInactiveIsAnOutcomeNotAWrite(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
		accountRow{id: "+5491100000001", role: entity.RoleStaff, displayName: "Ana Staff", isActive: 0},
	))

	outcome, err := DeactivateAccount(context.Background(), accounts, "+5491100000001")
	if err != nil {
		t.Fatalf("DeactivateAccount() error = %v, want a semantic outcome", err)
	}
	if !outcome.AlreadyInactive {
		t.Error("outcome.AlreadyInactive = false, want true for an already deactivated account")
	}
	if outcome.Account.Active {
		t.Error("outcome.Account.Active = true, want the inactive state reported")
	}
	if outcome.Account.ID != "+5491100000001" {
		t.Errorf("outcome.Account.ID = %q, want the staff account", outcome.Account.ID)
	}
}

func TestDeactivateAccount_RefusesTheLastActiveOwner(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
	))

	outcome, err := DeactivateAccount(context.Background(), accounts, "+5491100000000")
	if err == nil {
		t.Fatal("DeactivateAccount() error = nil, want the single-owner refusal")
	}
	if outcome != (DeactivateOutcome{}) {
		t.Errorf("DeactivateAccount() outcome = %+v, want the zero value", outcome)
	}
	if !errors.Is(err, ErrLastActiveOwner) {
		t.Errorf("DeactivateAccount() error = %v, want errors.Is(ErrLastActiveOwner)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("DeactivateAccount() error = %v, want errors.Is(domain.ErrConflict)", err)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("DeactivateAccount() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeConflict {
		t.Errorf("DeactivateAccount() code = %q, want %q", semErr.Code, domain.ErrCodeConflict)
	}
	if !strings.Contains(semErr.Message, "único owner activo") {
		t.Errorf("DeactivateAccount() message = %q, want the single-owner detail", semErr.Message)
	}
}

// TestDeactivateAccount_AllowsAnOwnerWhenAnotherOwnerIsActive pins the exact
// boundary of the guard. The SQLite trigger forbids two active owners in a real
// database, so this uses the fake port: the pre-check must allow the write as
// soon as another active owner exists, instead of refusing every owner.
func TestDeactivateAccount_AllowsAnOwnerWhenAnotherOwnerIsActive(t *testing.T) {
	fake := &fakeAccountsAdmin{listRows: []*entity.Account{
		accountFixture("+5491100000000", entity.RoleOwner, true),
		accountFixture("+5491100000009", entity.RoleOwner, true),
	}}

	outcome, err := DeactivateAccount(context.Background(), fake, "+5491100000000")
	if err != nil {
		t.Fatalf("DeactivateAccount() error = %v, want the write to proceed", err)
	}
	if outcome.AlreadyInactive {
		t.Error("outcome.AlreadyInactive = true, want a real deactivation")
	}
	if fake.deactivateCalls != 1 || fake.deactivatedID != "+5491100000000" {
		t.Errorf("Deactivate calls = %d with id %q, want 1 with the selected owner",
			fake.deactivateCalls, fake.deactivatedID)
	}
}

func TestDeactivateAccount_UnknownAccountIsSemanticNotFound(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
	))

	outcome, err := DeactivateAccount(context.Background(), accounts, "+5491199999999")
	if err == nil {
		t.Fatal("DeactivateAccount() error = nil, want a not-found rejection")
	}
	if outcome != (DeactivateOutcome{}) {
		t.Errorf("DeactivateAccount() outcome = %+v, want the zero value", outcome)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("DeactivateAccount() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeNotFound {
		t.Errorf("DeactivateAccount() code = %q, want %q", semErr.Code, domain.ErrCodeNotFound)
	}
}

func TestDeactivateAccount_BlankIDIsRejectedBeforeThePort(t *testing.T) {
	fake := &fakeAccountsAdmin{}

	_, err := DeactivateAccount(context.Background(), fake, "   ")
	if err == nil {
		t.Fatal("DeactivateAccount() error = nil, want the blank-id rejection")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("DeactivateAccount() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("DeactivateAccount() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
	if fake.listCalls != 0 || fake.deactivateCalls != 0 {
		t.Errorf("DeactivateAccount() reached the port (list=%d, deactivate=%d) for a blank id, want 0",
			fake.listCalls, fake.deactivateCalls)
	}
}

func TestDeactivateAccount_RepoErrorIsWrapped(t *testing.T) {
	t.Run("list failure", func(t *testing.T) {
		fake := &fakeAccountsAdmin{listErr: errors.New("database is down")}

		_, err := DeactivateAccount(context.Background(), fake, "+5491100000001")
		if err == nil {
			t.Fatal("DeactivateAccount() error = nil, want the port failure")
		}
		if !strings.Contains(err.Error(), "listar cuentas") {
			t.Errorf("DeactivateAccount() error = %v, want it to wrap the listing", err)
		}
		if fake.deactivateCalls != 0 {
			t.Errorf("DeactivateAccount() wrote %d times after a read failure, want 0", fake.deactivateCalls)
		}
	})

	t.Run("deactivate failure", func(t *testing.T) {
		fake := &fakeAccountsAdmin{
			listRows:      []*entity.Account{accountFixture("+5491100000001", entity.RoleStaff, true)},
			deactivateErr: errors.New("disk I/O error"),
		}

		_, err := DeactivateAccount(context.Background(), fake, "+5491100000001")
		if err == nil {
			t.Fatal("DeactivateAccount() error = nil, want the write failure")
		}
		if !strings.Contains(err.Error(), "desactivar la cuenta") {
			t.Errorf("DeactivateAccount() error = %v, want it to wrap the deactivation", err)
		}
		if errors.Is(err, ErrLastActiveOwner) {
			t.Errorf("DeactivateAccount() error = %v, want no single-owner classification for an I/O failure", err)
		}
	})
}

func TestDeactivateAccount_FabricatesOwnerCallerForThePort(t *testing.T) {
	fake := &fakeAccountsAdmin{
		listRows: []*entity.Account{
			accountFixture("+5491100000000", entity.RoleOwner, true),
			accountFixture("+5491100000001", entity.RoleStaff, true),
		},
	}

	if _, err := DeactivateAccount(context.Background(), fake, "+5491100000001"); err != nil {
		t.Fatalf("DeactivateAccount() error = %v", err)
	}

	want := auth.Caller{ID: TUICallerID, Role: auth.RoleOwner}
	if fake.listCaller != want {
		t.Errorf("List caller = %+v, want %+v", fake.listCaller, want)
	}
	if !fake.deactivateHas || fake.deactivateCaller != want {
		t.Errorf("Deactivate caller = %+v (attached=%v), want %+v", fake.deactivateCaller, fake.deactivateHas, want)
	}
}
