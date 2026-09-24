package admin

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestAccountLabelRendersNameRoleAndPhone(t *testing.T) {
	view := AccountView{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Dueño"}

	if got, want := AccountLabel(view), "Dueño (owner, +5491100000000)"; got != want {
		t.Errorf("AccountLabel() = %q, want %q", got, want)
	}
}

func TestListableRolesIsTheDomainMenuOrder(t *testing.T) {
	got := ListableRoles()
	want := []entity.AccountRole{entity.RoleOwner, entity.RoleAdmin, entity.RoleStaff}

	if len(got) != len(want) {
		t.Fatalf("ListableRoles() length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListableRoles()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReactivationHintNamesTheDeactivatedOwner(t *testing.T) {
	inactive := []AccountView{
		{ID: "+5491100000001", Role: entity.RoleOwner, DisplayName: "Ana"},
		{ID: "+5491100000002", Role: entity.RoleOwner, DisplayName: "Beto"},
	}

	t.Run("a matching phone yields the recovery step", func(t *testing.T) {
		got := ReactivationHint(inactive, "+5491100000002")
		if !strings.Contains(got, `"Beto"`) {
			t.Errorf("ReactivationHint() = %q, want it to name the deactivated owner", got)
		}
		if !strings.Contains(got, "+5491100000002") {
			t.Errorf("ReactivationHint() = %q, want it to echo the phone", got)
		}
		if !strings.Contains(got, "reactivala") {
			t.Errorf("ReactivationHint() = %q, want the reactivation step", got)
		}
	})

	t.Run("an unknown phone yields no hint", func(t *testing.T) {
		if got := ReactivationHint(inactive, "+5491199999999"); got != "" {
			t.Errorf("ReactivationHint() = %q, want an empty hint", got)
		}
	})

	t.Run("no inactive owners yields no hint", func(t *testing.T) {
		if got := ReactivationHint(nil, "+5491100000001"); got != "" {
			t.Errorf("ReactivationHint() = %q, want an empty hint", got)
		}
	})
}

// successorAccounts is a role-aware AccountsAdmin fake: SuccessorCandidates
// queries the owner role and then the staff role, and each query must answer its
// own rows, which the single-scripted fakeAccountsAdmin cannot express.
type successorAccounts struct {
	byRole  map[entity.AccountRole][]*entity.Account
	roleErr error
}

func (f *successorAccounts) List(context.Context) ([]*entity.Account, error) { return nil, nil }

func (f *successorAccounts) GetByRole(_ context.Context, role entity.AccountRole) ([]*entity.Account, error) {
	if f.roleErr != nil {
		return nil, f.roleErr
	}
	return f.byRole[role], nil
}

func (f *successorAccounts) Deactivate(context.Context, string) error { return nil }

func TestSuccessorCandidatesOrdersInactiveOwnersThenActiveStaffThenNewPhone(t *testing.T) {
	accounts := &successorAccounts{byRole: map[entity.AccountRole][]*entity.Account{
		entity.RoleOwner: {
			{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Owner Activo", Active: true},
			{ID: "+5491100000001", Role: entity.RoleOwner, DisplayName: "Owner Viejo", Active: false},
		},
		entity.RoleStaff: {
			{ID: "+5491100000002", Role: entity.RoleStaff, DisplayName: "Ana Staff", Active: true},
		},
	}}

	got, err := SuccessorCandidates(context.Background(), accounts)
	if err != nil {
		t.Fatalf("SuccessorCandidates() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("SuccessorCandidates() length = %d, want 3", len(got))
	}

	if got[0].Kind != SuccessorInactiveOwner || got[0].ID != "+5491100000001" {
		t.Errorf("candidate[0] = %+v, want the inactive owner", got[0])
	}
	if want := "Owner Viejo (owner, +5491100000001)"; got[0].Label != want {
		t.Errorf("candidate[0].Label = %q, want %q", got[0].Label, want)
	}

	if got[1].Kind != SuccessorStaff || got[1].ID != "+5491100000002" {
		t.Errorf("candidate[1] = %+v, want the active staff account", got[1])
	}
	if want := "Ana Staff (staff, +5491100000002) — promover a owner"; got[1].Label != want {
		t.Errorf("candidate[1].Label = %q, want %q", got[1].Label, want)
	}

	if got[2].Kind != SuccessorNewPhone || got[2].ID != "" {
		t.Errorf("candidate[2] = %+v, want the new-phone escape hatch", got[2])
	}
	if want := "Otro teléfono (crear una cuenta de owner nueva)"; got[2].Label != want {
		t.Errorf("candidate[2].Label = %q, want %q", got[2].Label, want)
	}
}

func TestSuccessorCandidatesActiveOwnerIsNeverOffered(t *testing.T) {
	accounts := &successorAccounts{byRole: map[entity.AccountRole][]*entity.Account{
		entity.RoleOwner: {
			{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Owner Activo", Active: true},
		},
	}}

	got, err := SuccessorCandidates(context.Background(), accounts)
	if err != nil {
		t.Fatalf("SuccessorCandidates() error = %v", err)
	}
	if len(got) != 1 || got[0].Kind != SuccessorNewPhone {
		t.Errorf("SuccessorCandidates() = %+v, want only the new-phone escape hatch", got)
	}
}

func TestSuccessorCandidatesPropagatesTheQueryError(t *testing.T) {
	accounts := &successorAccounts{roleErr: errors.New("database is down")}

	got, err := SuccessorCandidates(context.Background(), accounts)
	if err == nil {
		t.Fatal("SuccessorCandidates() error = nil, want the query failure")
	}
	if got != nil {
		t.Errorf("SuccessorCandidates() = %+v, want nil on failure", got)
	}
}

// TestApplyHermesConfigWritesTheMergedEntry pins the relocated chain (R2-01,
// now admin.ApplyHermesConfig): a success returns no fallback snippet and the
// written file carries the merged section. The message-level fallback behavior
// stays covered by the Bubble Tea and console flow tests.
func TestApplyHermesConfigWritesTheMergedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), HermesConfigFileName)

	snippet, err := ApplyHermesConfig(path, testHermesURL, testHermesPhone)
	if err != nil {
		t.Fatalf("ApplyHermesConfig() error = %v", err)
	}
	if snippet != "" {
		t.Errorf("ApplyHermesConfig() snippet = %q, want empty on success", snippet)
	}

	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("LoadHermesConfig() after write error = %v", err)
	}
	if _, ok := doc.storage()[hermesServersSection]; !ok {
		t.Error("ApplyHermesConfig() did not write the mcp_servers section")
	}
}
