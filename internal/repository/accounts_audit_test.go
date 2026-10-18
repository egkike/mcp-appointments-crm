package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Real tmp-file SQLite against the production schema (same helper the
// duplicate-violation test uses): the audit assertions must run against the
// actual DDL (CHECKs + immutability triggers), never against mocks.
func newAuditTestRepo(t *testing.T) (*AccountsRepo, *AccountsAuditRepo) {
	t.Helper()
	db, cleanup := newAggregateTestDB(t)
	t.Cleanup(cleanup)
	logger, _ := newTestLogger()
	repo := NewAccountsRepo(db, logger)
	return repo, repo.audit
}

// seedAuditFixture creates two accounts (the second inactive) the mutation
// scenarios operate on, and returns the current handler for querying data.
func seedAuditFixture(t *testing.T, repo *AccountsRepo) {
	t.Helper()
	// The FIRST owner activation must happen after any owner pre-exists, so the
	// fixture starts with an admin, then a second admin that will be deactivated.
	owner := &entity.Account{ID: "+5491100000001", Role: entity.RoleAdmin, DisplayName: "Admin One", Active: true}
	profID := "prof-audit-1" // accounts.professional_id has no FK: a typed id validates locally only
	staff := &entity.Account{ID: "+5491100000002", Role: entity.RoleStaff, DisplayName: "Staff", Active: true, ProfessionalID: &profID}
	if err := repo.Create(adminCtx(), owner); err != nil {
		t.Fatalf("fixture owner Create: %v", err)
	}
	if err := repo.Create(adminCtx(), staff); err != nil {
		t.Fatalf("fixture staff Create: %v", err)
	}
}

// ---- RED targets ----

// TestAccountsAudit_PersistedPerMutationType pins spec S1 "Cada tipo de
// mutación deja su fila": create / update / deactivate / transfer leave ONE
// row each, with the catalog event and action.
func TestAccountsAudit_PersistedPerMutationType(t *testing.T) {
	repo, audit := newAuditTestRepo(t)
	seedAuditFixture(t, repo)

	if err := repo.Deactivate(adminCtx(), "+5491100000002"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	events, err := audit.LastAccountsAudit(context.Background(), 50)
	if err != nil {
		t.Fatalf("LastAccountsAudit: %v", err)
	}

	if len(events) < 3 {
		t.Fatalf("audit rows = %d, want 3 (create staff, deactivate staff, create fixture second)", len(events))
	}
	actions := map[string]string{} // event -> action
	for _, e := range events {
		actions[e.Event] = e.Action
	}
	if actions[entity.AccountAuditEventCreated] != entity.AccountAuditActionCreate {
		t.Errorf("created event action = %q", actions[entity.AccountAuditEventCreated])
	}
	if actions[entity.AccountAuditEventDeactive] != entity.AccountAuditActionDeactivate {
		t.Errorf("deactivated event action = %q", actions[entity.AccountAuditEventDeactive])
	}
}

// TestAccountsAudit_MaskedNoPII pins spec "El teléfono nunca se persiste
// completo": every row's phone_masked is "***"-prefixed and the raw target id
// never appears anywhere in the row text.
func TestAccountsAudit_MaskedNoPII(t *testing.T) {
	repo, audit := newAuditTestRepo(t)
	seedAuditFixture(t, repo)
	rawPhone := "+5491100000002"
	if err := repo.Deactivate(adminCtx(), rawPhone); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	events, err := audit.LastAccountsAudit(context.Background(), 20)
	if err != nil {
		t.Fatalf("LastAccountsAudit: %v", err)
	}
	for _, e := range events {
		if e.PhoneMasked != "" && !strings.HasPrefix(e.PhoneMasked, "***") {
			t.Errorf("row %s: phone_masked = %q, want a ***-prefixed mask", e.Event, e.PhoneMasked)
		}
		// The fixture phone is known: NEITHER column nor detail may carry it raw.
		rowText := e.Ts + e.Event + e.ActorRef + e.TargetID + e.TargetRole + e.Action + e.PhoneMasked
		if strings.Contains(rowText, rawPhone) {
			t.Errorf("row %s: the raw phone appears in the persisted row", e.Event)
		}
	}
}

func TestAccountsAudit_SinkEnforcesMasking(t *testing.T) {
	_, audit := newAuditTestRepo(t)

	// A wiring bug that hands the RAW phone through the sink lands masked.
	eRaw := &entity.AccountAuditEvent{
		Event: entity.AccountAuditEventCreated, Action: entity.AccountAuditActionCreate,
		TargetID: "+5491100000888", TargetRole: string(entity.RoleStaff),
	}
	if err := audit.Append(context.Background(), eRaw); err != nil {
		t.Fatalf("Append: %v", err)
	}
	rows, err := audit.LastAccountsAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("LastAccountsAudit: %v", err)
	}
	if got := rows[0].TargetID; got == "+5491100000888" || !strings.HasPrefix(got, "***") {
		t.Errorf("persisted target_id = %q, want the enforced mask", got)
	}

	// A caller that sneaks the raw id into detail is rejected: the row must
	// not land, so the column contract cannot be bypassed through detail.
	eBadActor := &entity.AccountAuditEvent{
		Event: entity.AccountAuditEventCreated, Action: entity.AccountAuditActionCreate,
		TargetID: "+5491100000888", TargetRole: string(entity.RoleStaff),
		ActorRef: "not-a-hash",
	}
	appErr := audit.Append(context.Background(), eBadActor)
	if appErr == nil || !strings.Contains(appErr.Error(), "actor_ref") {
		t.Fatalf("Append with an invalid actor ref: err = %v, want the actor_ref rejection", appErr)
	}
}

func TestAccountsAudit_TriggersRejectMutation(t *testing.T) {
	_, audit := newAuditTestRepo(t)
	seed := &entity.AccountAuditEvent{
		Event: entity.AccountAuditEventCreated, Action: entity.AccountAuditActionCreate,
		TargetID: "+5491100000099", TargetRole: string(entity.RoleStaff), PhoneMasked: "***0099",
	}
	if err := audit.Append(context.Background(), seed); err != nil {
		t.Fatalf("Append: %v", err)
	}

	updateErr := probeRawExec(t, audit, "UPDATE accounts_audit SET detail = 'lie'")
	if updateErr == nil || !strings.Contains(updateErr.Error(), "inmutable") {
		t.Fatalf("UPDATE on accounts_audit: err = %v, want the immutability abort", updateErr)
	}
	deleteErr := probeRawExec(t, audit, "DELETE FROM accounts_audit")
	if deleteErr == nil || !strings.Contains(deleteErr.Error(), "inmutable") {
		t.Fatalf("DELETE on accounts_audit: err = %v, want the immutability abort", deleteErr)
	}
}

// probeRawExec runs raw SQL through the audit repo's handle to prove the
// trigger rejects, not the adapter (the adapter only ever inserts, so the
// mutation SQL must not be reachable through it).
func probeRawExec(t *testing.T, audit *AccountsAuditRepo, sql string) error {
	t.Helper()
	_, err := audit.db.ExecContext(context.Background(), sql)
	return err
}

// TestAccountsAudit_AuditFailureDoesNotRollBackMutation pins the dual-write
// policy (design §1.4): the row is best-effort — an audit sink error is logged
// and swallowed, and the mutation still succeeds (the audit sink must never
// become a write blocker on the critical path).
func TestAccountsAudit_AuditFailureDoesNotRollBackMutation(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectExec(`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`).WithArgs(
		"+5491100000777", "admin", "Admin AuditFail", nil, 1,
	).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO accounts_audit`).
		WillReturnError(errors.New("disk full"))

	err := repo.Create(adminCtx(), &entity.Account{
		ID: "+5491100000777", Role: entity.RoleAdmin, DisplayName: "Admin AuditFail", Active: true,
	})
	if err != nil {
		t.Fatalf("Create must succeed when only the audit sink fails: %v", err)
	}
}

// TestAccountsAudit_TransferRowCarriesMaskedDetail pins the remaining S1 row:
// ownership_transferred stores its successor target and its previous_owner_id
// only masked in the detail JSON (no raw PII in any column).
func TestAccountsAudit_TransferRowCarriesMaskedDetail(t *testing.T) {
	repo, audit := newAuditTestRepo(t)

	owner := &entity.Account{ID: "+5491100000101", Role: entity.RoleOwner, DisplayName: "Owner", Active: true}
	successor := &entity.Account{ID: "+5491100000102", Role: entity.RoleOwner, DisplayName: "Next Owner", Active: false}
	if err := repo.Create(ownerCtx(), owner); err != nil {
		t.Fatalf("owner Create: %v", err)
	}
	if err := repo.Create(ownerCtx(), successor); err != nil {
		t.Fatalf("successor Create: %v", err)
	}
	if err := repo.TransferOwnership(ownerCtx(), "+5491100000101", "+5491100000102"); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}

	events, err := audit.LastAccountsAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("LastAccountsAudit: %v", err)
	}
	var transfer *entity.AccountAuditEvent
	for _, e := range events {
		if e.Event == entity.AccountAuditEventTransfer {
			transfer = e
		}
	}
	if transfer == nil {
		t.Fatalf("no ownership_transferred row; events = %+v", events)
	}
	if transfer.Detail == nil || transfer.Detail.PreviousOwnerMasked != "***0101" {
		t.Errorf("transfer detail = %+v, want the masked previous owner", transfer.Detail)
	}
}
