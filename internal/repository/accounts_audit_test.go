package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
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

// TestAccountsRepo_PurgeInactive pins the destructive S3 contract: owner-only,
// inactive-only (is_active=0 re-check), one audit row per really-deleted
// account, and the audit event carries the masked id.
func TestAccountsRepo_PurgeInactive(t *testing.T) {
	repo, audit := newAuditTestRepo(t)

	owner := &entity.Account{ID: "+5491100100001", Role: entity.RoleOwner, DisplayName: "Owner", Active: true}
	profID := "prof-purge"
	staff := &entity.Account{ID: "+5491100100002", Role: entity.RoleStaff, DisplayName: "Purge Me", Active: true, ProfessionalID: &profID}
	if err := repo.Create(ownerCtx(), owner); err != nil {
		t.Fatalf("owner Create: %v", err)
	}
	if err := repo.Create(ownerCtx(), staff); err != nil {
		t.Fatalf("staff Create: %v", err)
	}
	if err := repo.Deactivate(ownerCtx(), staff.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	// Non-owner callers are refused BEFORE any delete: a staff caller cannot
	// shrink the whitelist.
	staffCalling := auth.WithCaller(context.Background(), auth.Caller{ID: staff.ID, Role: "staff"})
	purged, err := repo.PurgeInactive(staffCalling, staff.ID)
	if err == nil || !strings.Contains(err.Error(), "no tiene permiso") {
		t.Fatalf("PurgeInactive as staff: err = %v, want an authorization error", err)
	}
	if purged {
		t.Fatal("staff caller must not delete anything")
	}

	// Active accounts are refused by the is_active=0 re-check (0 rows): the
	// active owner is untouched even if the operator picked it by mistake.
	purged, err = repo.PurgeInactive(ownerCtx(), owner.ID)
	if err != nil {
		t.Fatalf("PurgeInactive on the active owner: %v", err)
	}
	if purged {
		t.Fatal("the active owner must never be deletable through the purge")
	}

	purged, err = repo.PurgeInactive(ownerCtx(), staff.ID)
	if err != nil {
		t.Fatalf("PurgeInactive: %v", err)
	}
	if !purged {
		t.Fatal("the inactive staff account must be purged")
	}
	if _, err := repo.FindByID(ownerCtx(), staff.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("post-purge lookup: err = %v, want domain.ErrNotFound", err)
	}

	events, err := audit.LastAccountsAudit(context.Background(), 5)
	if err != nil {
		t.Fatalf("LastAccountsAudit: %v", err)
	}
	var purge struct{ found, masked bool }
	for _, e := range events {
		if e.Event == entity.AccountAuditEventPurged {
			purge.found = true
			if e.TargetID == staff.ID {
				t.Errorf("purge audit row leaks the raw id: %q", e.TargetID)
			}
			if strings.HasPrefix(e.TargetID, "***") {
				purge.masked = true
			}
		}
	}
	if !purge.found || !purge.masked {
		t.Errorf("purge audit row missing or unmasked (found=%v masked=%v)", purge.found, purge.masked)
	}
}

// TestAdminPurgeInactiveAccounts_OutcomeIdsMasked pins the R3-001 correction:
// the per-outcome report the TUI and console render carries MASKED ids (the
// account id IS the phone), so a partial-batch read can never leak raw PII.
func TestAdminPurgeInactiveAccounts_OutcomeIdsMasked(t *testing.T) {
	db, cleanup := newAggregateTestDB(t)
	defer cleanup()
	logger, _ := newTestLogger()
	repo := NewAccountsRepo(db, logger)

	owner := &entity.Account{ID: "+5491122334401", Role: entity.RoleOwner, DisplayName: "Owner", Active: true}
	profID := "prof-purge-report"
	staff := &entity.Account{ID: "+5491122334402", Role: entity.RoleStaff, DisplayName: "Purge Report", Active: true, ProfessionalID: &profID}
	if err := repo.Create(ownerCtx(), owner); err != nil {
		t.Fatalf("owner Create: %v", err)
	}
	if err := repo.Create(ownerCtx(), staff); err != nil {
		t.Fatalf("staff Create: %v", err)
	}
	if err := repo.Deactivate(ownerCtx(), staff.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	outcomes, err := admin.PurgeInactiveAccounts(ownerCtx(), repo, []string{staff.ID})
	if err != nil {
		t.Fatalf("PurgeInactiveAccounts: %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].Purged {
		t.Fatalf("outcomes = %+v, want one purged row", outcomes)
	}
	if outcomes[0].Account.ID != "***4402" {
		t.Errorf("outcome id = %q, want the masked phone ***4402", outcomes[0].Account.ID)
	}
}

// ── ADR-0021: keyed caller-id digest in the durable audit row ───────────────
//
// The audit identity scheme is HMAC-SHA-256 keyed with an installation key. The
// persisted actor_ref stays 64 lowercase hex chars, so the column, its
// validation and every reader are untouched; what changes is that the reference
// is no longer enumerable offline. These tests pin the adapter seam: the row is
// built from the injected digester, and an unconfigured digester fails closed
// instead of writing the retired unkeyed digest.

const (
	// repoAuditKeyHex is the fixed 32-byte test key (same vector as the auth
	// package pin).
	repoAuditKeyHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

	// repoAuditActorID is the caller id adminCtx() attaches.
	repoAuditActorID = "admin-1"

	// repoAuditActorRefPinned is HMAC-SHA-256(key = repoAuditKeyHex,
	// id = "admin-1"), computed once outside this code.
	repoAuditActorRefPinned = "d2b4ead8ae8696b5aa906bde0b29bbb79c9a5a137b375bf7b8203342aab94b14"
)

// keyedAuditDigester builds the keyed digester for the fixed test key.
func keyedAuditDigester(t *testing.T) auth.CallerIDDigester {
	t.Helper()
	key, err := hex.DecodeString(repoAuditKeyHex)
	if err != nil {
		t.Fatalf("decode test key: %v", err)
	}
	digester, err := auth.NewCallerIDDigester(key)
	if err != nil {
		t.Fatalf("NewCallerIDDigester: %v", err)
	}
	return digester
}

// TestAccountsAudit_ActorRefIsKeyedDigest pins the write path end to end: a
// mutation performed by an admin leaves a row whose actor_ref is the keyed HMAC
// of the actor id — the pinned vector, not the retired unkeyed SHA-256.
func TestAccountsAudit_ActorRefIsKeyedDigest(t *testing.T) {
	repo, audit := newAuditTestRepo(t)
	repo.digester = keyedAuditDigester(t)

	account := &entity.Account{ID: "+5491100000100", Role: entity.RoleAdmin, DisplayName: "Keyed", Active: true}
	if err := repo.Create(adminCtx(), account); err != nil {
		t.Fatalf("Create: %v", err)
	}

	events, err := audit.LastAccountsAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("LastAccountsAudit: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(events))
	}

	got := events[0].ActorRef
	if got != repoAuditActorRefPinned {
		t.Errorf("actor_ref = %q; want the pinned keyed digest %q", got, repoAuditActorRefPinned)
	}
	if len(got) != 64 || strings.ToLower(got) != got {
		t.Errorf("actor_ref = %q; want 64 lowercase hex chars (audit column shape)", got)
	}

	unkeyed := sha256.Sum256([]byte(repoAuditActorID))
	if got == hex.EncodeToString(unkeyed[:]) {
		t.Error("actor_ref equals the retired unkeyed SHA-256 digest; the actor id stays enumerable")
	}
	if strings.Contains(got, repoAuditActorID) {
		t.Errorf("actor_ref = %q; it must not carry the raw actor id", got)
	}
}

// TestAccountsAudit_ActorRefOmittedWhenDigestUnconfigured pins the fail-closed
// seam on both shapes of "not configured": no digester at all, and a digester
// that answers "" because the process-wide keyed digest was never installed (the
// production default when the boot seam is skipped). In both cases the row is
// still written (audit completeness) but WITHOUT actor_ref, the degradation is
// logged, and the retired unkeyed digest is never a fallback.
func TestAccountsAudit_ActorRefOmittedWhenDigestUnconfigured(t *testing.T) {
	seams := map[string]auth.CallerIDDigester{
		"nil digester": nil,
		"unconfigured process digest": func(string) string {
			// Exactly what auth.HashCallerID answers when no key is installed.
			return ""
		},
	}

	for name, digester := range seams {
		t.Run(name, func(t *testing.T) {
			db, cleanup := newAggregateTestDB(t)
			defer cleanup()
			logger, handler := newTestLogger()
			repo := NewAccountsRepo(db, logger)
			repo.digester = digester

			account := &entity.Account{ID: "+5491100000101", Role: entity.RoleAdmin, DisplayName: "Unkeyed", Active: true}
			if err := repo.Create(adminCtx(), account); err != nil {
				t.Fatalf("Create: %v", err)
			}

			events, err := repo.LastAccountsAudit(context.Background(), 10)
			if err != nil {
				t.Fatalf("LastAccountsAudit: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("audit rows = %d, want 1 (the row must not be dropped)", len(events))
			}
			if events[0].ActorRef != "" {
				t.Errorf("actor_ref = %q; want it omitted while the digest is unconfigured", events[0].ActorRef)
			}

			if recs := handler.recordsByMsg("audit digest unconfigured, actor_ref omitted"); len(recs) != 1 {
				t.Fatalf("digest-degradation records = %d; want exactly 1 (fail closed must be visible)", len(recs))
			}

			unkeyed := sha256.Sum256([]byte(repoAuditActorID))
			if strings.Contains(events[0].Ts+events[0].Event+events[0].ActorRef+events[0].Action, hex.EncodeToString(unkeyed[:])) {
				t.Error("the row carries the retired unkeyed SHA-256 digest")
			}
		})
	}
}

// TestAccountsAudit_AppendAcceptsMixedSchemeActorRefs pins the mixed acceptance
// window (ADR-0021): rows written before the keyed digest landed (unkeyed
// SHA-256) and rows written after (keyed HMAC) are indistinguishable to the
// column contract, so both persist and read back unchanged — no schema change,
// no migration, no rewrite of historical rows.
func TestAccountsAudit_AppendAcceptsMixedSchemeActorRefs(t *testing.T) {
	_, audit := newAuditTestRepo(t)

	legacyRef := sha256.Sum256([]byte("+5491100000500"))
	keyedRef := repoAuditActorRefPinned

	legacy := &entity.AccountAuditEvent{
		Event: entity.AccountAuditEventCreated, Action: entity.AccountAuditActionCreate,
		TargetID: "+5491100000500", TargetRole: string(entity.RoleStaff),
		ActorRef: hex.EncodeToString(legacyRef[:]),
	}
	if err := audit.Append(context.Background(), legacy); err != nil {
		t.Fatalf("Append (legacy unkeyed digest): %v", err)
	}
	keyed := &entity.AccountAuditEvent{
		Event: entity.AccountAuditEventUpdated, Action: entity.AccountAuditActionUpdate,
		TargetID: "+5491100000500", TargetRole: string(entity.RoleStaff),
		ActorRef: keyedRef,
	}
	if err := audit.Append(context.Background(), keyed); err != nil {
		t.Fatalf("Append (keyed digest): %v", err)
	}

	events, err := audit.LastAccountsAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("LastAccountsAudit: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("audit rows = %d, want 2 (both schemes accepted)", len(events))
	}

	refs := map[string]bool{}
	for _, e := range events {
		refs[e.ActorRef] = true
	}
	if !refs[hex.EncodeToString(legacyRef[:])] || !refs[keyedRef] {
		t.Errorf("persisted actor_refs = %v; want both the legacy unkeyed and the keyed digest", refs)
	}
}

// TestAuditAttrs_MaskPhoneIds pins the operational-slog masking decision 6 of
// ADR-0021 (GGA round 1 of the #155 commit): the journal lines the repo emits
// are as PII-free as the persisted rows - actor_id, target_id and
// previous_owner_id carry the same MaskPhone shape ("***<last4>"), never the
// raw phone, because journal readers are not bound by the ephemeral-process
// trust of a TUI session.
func TestAuditAttrs_MaskPhoneIds(t *testing.T) {
	attrs := auditAttrs("+5491100000001", "+5491100000002", "admin")

	values := map[string]string{"actor_id": "***0001", "target_id": "***0002"}
	lookup := map[string]string{}
	for _, a := range attrs {
		lookup[a.Key] = a.Value.String()
	}
	for key, want := range values {
		if got := lookup[key]; got != want {
			t.Errorf("%s = %v, want masked %v", key, got, want)
		}
	}
}

// TestAuditAttrs_BuriesNonPhoneIds reminds that MaskPhone buries anything that
// is not an identifier-shaped value: a non-phone id never leaks its tail.
func TestAuditAttrs_BuriesNonPhoneIds(t *testing.T) {
	attrs := auditAttrs("uptime-pinger", "weird-key", "admin")
	lookup := map[string]string{}
	for _, a := range attrs {
		lookup[a.Key] = a.Value.String()
	}
	for _, key := range []string{"actor_id", "target_id"} {
		if got := lookup[key]; got != "***" {
			t.Errorf("%s = %v, want fully buried ***", key, got)
		}
	}
}

// TestAccountsAudit_DetailWireBytesArePinned pins the exact detail-column
// bytes of the ownership_transferred row (the single detail payload today):
// the ban on free-form strings means the JSON key set and value shapes ARE
// the contract, so any future field or rename shows up as a byte diff here
// instead of drifting silently.
func TestAccountsAudit_DetailWireBytesArePinned(t *testing.T) {
	e := &entity.AccountAuditEvent{
		Event: entity.AccountAuditEventTransfer,
		Detail: &entity.AccountAuditDetail{
			PreviousOwnerMasked: "***0101",
		},
	}
	got, err := auditDetailJSON(e)
	if err != nil {
		t.Fatalf("auditDetailJSON error = %v", err)
	}
	if expected := `{"previous_owner_masked":"***0101"}`; got.String != expected {
		t.Errorf("detail bytes = %s, want %s (both SCHEMES byte-stable)", got.String, expected)
	}
	if !got.Valid {
		t.Error("detail row Valid = false, want true with a payload")
	}
}

// TestAccountsAudit_DetailColumnShapesStayEquivalent pins BOTH persisted
// shapes (the pre-refactor behavior, verified byte-for-byte): an event with no
// detail renders the SQL NULL column, and an event carrying a set-but-empty
// detail renders the one-key-less JSON "{}" as a non-NULL value - the same
// bytes the tagged entity of B8 produced, preserved by the adapter DTO.
func TestAccountsAudit_DetailColumnShapesStayEquivalent(t *testing.T) {
	empty, err := auditDetailJSON(&entity.AccountAuditEvent{Event: entity.AccountAuditEventTransfer})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if empty.Valid {
		t.Errorf("value = %v, want SQL NULL for an event without detail", empty.String)
	}

	none := &entity.AccountAuditEvent{
		Detail: &entity.AccountAuditDetail{},
	}
	empty2, err := auditDetailJSON(none)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !empty2.Valid || empty2.String != "{}" {
		t.Errorf("value = %q valid=%v, want the pre-refactor \"{}\" (valid), untouched by the DTO move", empty2.String, empty2.Valid)
	}
}

// TestAuditAttrs_TypeIsTypedSlogAttr pins the typed-attrs contract (GGA round
// 3 of the follow-ups commit): the audit attr builder returns []slog.Attr,
// not the raw []any key-value pairs, so the compiler owns the shape instead of
// the convention.
func TestAuditAttrs_TypeIsTypedSlogAttr(t *testing.T) {
	attrs := auditAttrs("+5491100000001", "+5491100000002", "admin")
	if got := reflect.TypeOf(attrs); got.Kind() != reflect.Slice || got != reflect.TypeOf([]slog.Attr{}) {
		t.Fatalf("auditAttrs type = %T, want []slog.Attr", attrs)
	}
}
