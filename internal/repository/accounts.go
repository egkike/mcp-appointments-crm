package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.AccountsRepo = (*AccountsRepo)(nil)

// AccountsRepo provides CRUD operations for the accounts table.
// All mutations emit structured audit logs via the injected *slog.Logger and
// persist the same outcome into the append-only `accounts_audit` table via
// the embedded AccountsAuditRepo (feat-tui-ops-residuals S1: the DB row is the
// durable consultable source, slog stays the operational channel — same
// emission sites, one row per outcome, never PII).
type AccountsRepo struct {
	db     *sql.DB
	logger *slog.Logger
	audit  *AccountsAuditRepo
	// digester produces the actor reference the durable audit row stores
	// (ADR-0021). It defaults to auth.HashCallerID — the keyed digest the
	// composition root configures at boot — and a nil digester means "not
	// configured": the row is persisted WITHOUT actor_ref (the empty shape the
	// sink already accepts) instead of falling back to the retired unkeyed
	// digest.
	digester auth.CallerIDDigester
}

// NewAccountsRepo creates a repo with an already-open *sql.DB and configured *slog.Logger.
// It does NOT open connections or run migrations. The audit emitter uses the
// boot-configured keyed caller digest (auth.HashCallerID); tests in this package
// override the digester field to pin a fixed key or to exercise the
// unconfigured fail-closed seam.
func NewAccountsRepo(db *sql.DB, logger *slog.Logger) *AccountsRepo {
	return &AccountsRepo{db: db, logger: logger, audit: NewAccountsAuditRepo(db), digester: auth.HashCallerID}
}

// persistAuditEvent writes the durable audit row for a mutation outcome. The
// slog emission stays authoritative-operational: an audit write failure is
// logged and swallowed, never rolled back (spec: same criterion as the
// current slog trail — the mutation MUST NOT fail because the audit sink
// failed, otherwise the audit sink would become a write blocker on the
// critical path).
func (r *AccountsRepo) persistAuditEvent(ctx context.Context, e *entity.AccountAuditEvent) {
	if err := r.audit.Append(ctx, e); err != nil {
		r.logger.Warn("audit row write failed", "audit_event", e.Event, "error", err)
	}
}

// newAccountAuditEvent assembles the masked, non-PII event every emission
// site persists: the actor is digested with the injected keyed digest
// (HMAC-SHA-256, ADR-0021 — the same reference decision the middleware made for
// its records), the target phone is always MaskPhone-ed, and detail JSON is
// expected pre-masked by the caller (the transfer's previous_owner_id) so no
// column ever carries a raw phone.
//
// Fail closed on an unconfigured digest: actor_ref is omitted and the
// degradation is logged. The row is still written — a missing reference is an
// audit-quality problem, while dropping the row would be an audit-completeness
// problem — and the reference never degrades to the retired unkeyed SHA-256.
func (r *AccountsRepo) newAccountAuditEvent(actorID, eventID, action, targetID string, targetRole entity.AccountRole, detail *entity.AccountAuditDetail) *entity.AccountAuditEvent {
	role := ""
	if targetRole != "" {
		role = string(targetRole)
	}
	actorRef := ""
	if actorID != "" {
		var ref string
		if r.digester != nil {
			ref = r.digester(actorID)
		}
		// An empty reference means "not configured": either no digester was
		// injected, or the process digest was never installed and HashCallerID
		// answered "". Both are wiring bugs, both must be visible, and neither
		// may degrade to the retired unkeyed digest.
		if ref == "" {
			r.logger.Error("audit digest unconfigured, actor_ref omitted",
				"scheme", auth.AuditDigestScheme,
				"event", eventID,
				"action", action,
			)
		} else {
			actorRef = ref
		}
	}
	return &entity.AccountAuditEvent{
		Ts:       time.Now().UTC().Format(time.RFC3339Nano),
		Event:    eventID,
		ActorRef: actorRef,
		// The account id IS the phone: the persistence rule "no raw PII in any
		// column" binds this field too, so the target ships masked here as well
		// (phone_masked makes the rule explicit for every reader of the row).
		TargetID:    entity.MaskPhone(targetID),
		TargetRole:  role,
		Action:      action,
		PhoneMasked: entity.MaskPhone(targetID),
		Detail:      detail,
	}
}

// validRole checks if the role is one of the three allowed values.
func validRole(role entity.AccountRole) bool {
	return role == entity.RoleOwner || role == entity.RoleAdmin || role == entity.RoleStaff
}

// actorFromContext extracts the actor ID from the context's auth.Caller.
// Returns empty string when no Caller is present (omitted from audit log).
func actorFromContext(ctx context.Context) string {
	if caller, ok := auth.FromContext(ctx); ok {
		return caller.ID
	}
	return ""
}

// auditAttrs builds the common audit log attributes, omitting actor_id when empty.
// The journal mirrors the persisted row's masking rule (ADR-0021 decision 6,
// GGA round 1 of the #155 commit): actor_id and target_id carry MaskPhone-ed
// identifiers - journal readers are not bound by the ephemeral trust of a TUI
// session, and the slog lines are as PII-free as the columns they mirror.
//
// The attrs are TYPED (GGA round 3): the builder answers []slog.Attr, so the
// compiler owns the key/value shape instead of a positional convention.
func auditAttrs(actorID, targetID, targetRole string) []slog.Attr {
	attrs := make([]slog.Attr, 0, 4)
	if actorID != "" {
		attrs = append(attrs, slog.String("actor_id", entity.MaskPhone(actorID)))
	}
	attrs = append(attrs,
		slog.String("target_id", entity.MaskPhone(targetID)),
		slog.String("target_role", targetRole),
		slog.String("ts", time.Now().UTC().Format(time.RFC3339Nano)),
	)
	return attrs
}

// asAnyLifts promotes typed attrs to the slog method variadic form.
func asAnyLifts(attrs []slog.Attr) []any {
	anyAttrs := make([]any, 0, len(attrs))
	for _, a := range attrs {
		anyAttrs = append(anyAttrs, a)
	}
	return anyAttrs
}

// Create inserts a new account. Validates before touching the DB.
// For role "owner", performs a single-owner pre-check (defense-in-depth with the SQLite trigger).
// Requires admin or owner role.
func (r *AccountsRepo) Create(ctx context.Context, a *entity.Account) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("crear cuenta: %w", err)
	}

	if err := a.Validate(); err != nil {
		return fmt.Errorf("crear cuenta: %w", err)
	}

	// Single-owner pre-check (defense-in-depth with SQLite trigger)
	if a.Role == entity.RoleOwner && a.Active {
		var count int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1").Scan(&count); err != nil {
			return fmt.Errorf("verificar owner activo: %w", err)
		}
		if count > 0 {
			// Emit security audit log for the rejection attempt (slog + durable
			// audit row: the rejection leaves a trace and nothing else).
			actorID := actorFromContext(ctx)
			attrs := auditAttrs(actorID, a.ID, string(a.Role))
			attrs = append(attrs, slog.String("result", "rejected"))
			r.logger.Warn("second active owner rejected", asAnyLifts(attrs)...)
			r.persistAuditEvent(ctx, r.newAccountAuditEvent(actorID, entity.AccountAuditEventRejected, entity.AccountAuditActionReject, a.ID, a.Role, nil))
			return fmt.Errorf("crear cuenta: %w: ya existe un owner activo; desactívalo antes de crear otro", domain.ErrConflict)
		}
	}

	isActive := 0
	if a.Active {
		isActive = 1
	}

	var profID *string
	if a.ProfessionalID != nil && *a.ProfessionalID != "" {
		profID = a.ProfessionalID
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
		a.ID, string(a.Role), a.DisplayName, profID, isActive,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("crear cuenta: %w: ya existe una cuenta con id %q", domain.ErrConflict, a.ID)
		}
		if isSingleOwnerViolation(err) {
			return fmt.Errorf("crear cuenta: %w: ya existe un owner activo; desactívalo antes de crear otro", domain.ErrConflict)
		}
		return fmt.Errorf("crear cuenta: %w", err)
	}

	actorID := actorFromContext(ctx)
	r.logger.Info("account created", asAnyLifts(auditAttrs(actorID, a.ID, string(a.Role)))...)
	r.persistAuditEvent(ctx, r.newAccountAuditEvent(actorID, entity.AccountAuditEventCreated, entity.AccountAuditActionCreate, a.ID, a.Role, nil))
	return nil
}

// FindByID retrieves a single account by ID. Returns domain.ErrNotFound if the row does not exist.
// Requires an authenticated caller.
func (r *AccountsRepo) FindByID(ctx context.Context, id string) (*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("obtener cuenta %s: %w", id, err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`, id,
	)
	return scanAccount(row)
}

// AccountExistsByID reports whether an accounts row with the given id exists.
//
// Auth-free by design: it backs RegistrationLookup for the anonymous client
// registration path, which runs before any Caller can be resolved. It answers
// existence only — role and is_active are irrelevant because a phone owned by
// ANY account (active or inactive) must be rejected by registration. Unlike
// FindByID it never runs a role/caller check and a missing row is (false, nil),
// not domain.ErrNotFound.
//
// The composition root wires this method ONLY into the registration use case
// (through the RegistrationLookup adapter); it is deliberately absent from the
// AccountsRepo interface, so no authenticated flow can reach it by accident.
func (r *AccountsRepo) AccountExistsByID(ctx context.Context, id string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verificar existencia de cuenta: %w", err)
	}
	return true, nil
}

// GetByRole returns all accounts matching the given role, ordered by created_at ASC.
// Returns domain.ErrInvalidInput for unrecognized roles. Returns empty slice (not nil) when no rows match.
// Requires an authenticated caller.
func (r *AccountsRepo) GetByRole(ctx context.Context, role entity.AccountRole) ([]*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("buscar cuentas por role: %w", err)
	}

	if !validRole(role) {
		return nil, fmt.Errorf("buscar por role: %w: role %q no válido", domain.ErrInvalidInput, role)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = ? ORDER BY created_at ASC`, string(role),
	)
	if err != nil {
		return nil, fmt.Errorf("buscar cuentas por role: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	accounts := make([]*entity.Account, 0)
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, fmt.Errorf("buscar cuentas por role: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buscar cuentas por role: %w", err)
	}
	return accounts, nil
}

// List returns all accounts ordered by created_at ASC. Returns empty slice (not nil) when no rows exist.
// Requires an authenticated caller.
// PurgeInactive hard-deletes ONE account row by id, refusing anything that
// is not an INACTIVE account (feat-tui-ops-residuals S3; owner-only per
// ADR-0016 governance). The `is_active = 0` re-check inside the DELETE keeps
// the confirm-to-execute window closed without a transaction: an account
// re-activated between the operator's confirmation and the write is not
// deleted (0 rows affected → purged=false). The caller re-reads the current
// state for display, never trusting a stale pick (mirrors the deactivate
// snapshot rule).
//
// FK-safe by construction: no table references accounts (verified, design
// §3.3), so the DELETE cannot cascade or violate a RESTRICT.
// Both the slog record and the durable audit row fire only when the row was
// really deleted; the audit write stays fail-open (S1 policy).
func (r *AccountsRepo) PurgeInactive(ctx context.Context, id string) (bool, error) {
	if _, err := auth.RequireRole(ctx, auth.RoleOwner); err != nil {
		return false, fmt.Errorf("purgar cuenta: %w", err)
	}
	if id == "" {
		return false, fmt.Errorf("purgar cuenta: %w: el id no puede estar vacío", domain.ErrInvalidInput)
	}

	// The row dies in this call, so the REAL role must be captured ATOMICALLY
	// with the delete (read-then-delete opens a stale-role window if a
	// concurrent update lands between the two statements): one transaction
	// guards both, and the is_active=0 re-check runs INSIDE it.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("purgar cuenta: iniciar la transacción: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var role string
	if err := tx.QueryRowContext(ctx,
		`SELECT role FROM accounts WHERE id = ? AND is_active = 0`, id,
	).Scan(&role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("purgar cuenta: %w", err)
	}

	result, err := tx.ExecContext(ctx,
		`DELETE FROM accounts WHERE id = ? AND is_active = 0`, id,
	)
	if err != nil {
		return false, fmt.Errorf("purgar cuenta: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("purgar cuenta: %w", err)
	}
	if affected == 0 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("purgar cuenta: confirmar la transacción: %w", err)
	}
	committed = true

	actorID := actorFromContext(ctx)
	r.logger.Info("account purged", asAnyLifts(auditAttrs(actorID, id, role))...)
	r.persistAuditEvent(ctx, r.newAccountAuditEvent(
		actorID, entity.AccountAuditEventPurged, entity.AccountAuditActionPurge,
		id, entity.AccountRole(role), nil))
	return true, nil
}

// LastAccountsAudit delegates the read-only audit page to the embedded audit
// adapter, so the single AccountsRepo handle satisfies reader ports too
// (admin.AuditTrailReader) without the composition root keeping two handles
// alive per database.
func (r *AccountsRepo) LastAccountsAudit(ctx context.Context, limit int) ([]*entity.AccountAuditEvent, error) {
	return r.audit.LastAccountsAudit(ctx, limit)
}

// List returns every account row, active or not, oldest first — the raw base
// of the list views and of the purge preview.
func (r *AccountsRepo) List(ctx context.Context) ([]*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("listar cuentas: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listar cuentas: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	accounts := make([]*entity.Account, 0)
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, fmt.Errorf("listar cuentas: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar cuentas: %w", err)
	}
	return accounts, nil
}

// Update modifies an existing account. Returns domain.ErrNotFound if the row does not exist.
// Regenerates updated_at with SQLite strftime.
// Requires admin or owner role.
func (r *AccountsRepo) Update(ctx context.Context, a *entity.Account) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("actualizar cuenta: %w", err)
	}

	if err := a.Validate(); err != nil {
		return fmt.Errorf("actualizar cuenta: %w", err)
	}

	// Verify the row exists before UPDATE so RowsAffected() == 0 unambiguously means
	// "not found" (not "no-op update with same values").
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id = ?`, a.ID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("actualizar cuenta: %w: cuenta con id %q no encontrada", domain.ErrNotFound, a.ID)
		}
		return fmt.Errorf("actualizar cuenta: verificar existencia: %w", err)
	}

	isActive := 0
	if a.Active {
		isActive = 1
	}

	var profID *string
	if a.ProfessionalID != nil && *a.ProfessionalID != "" {
		profID = a.ProfessionalID
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE accounts SET role = ?, display_name = ?, professional_id = ?, is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		string(a.Role), a.DisplayName, profID, isActive, a.ID,
	)
	if err != nil {
		if isSingleOwnerViolation(err) {
			return fmt.Errorf("actualizar cuenta: %w: ya existe un owner activo; desactívalo antes de crear otro", domain.ErrConflict)
		}
		return fmt.Errorf("actualizar cuenta: %w", err)
	}

	// GGA round 1 of the #155 commit (TOCTOU): RowsAffected == 0 here does NOT
	// prove a no-op update - a concurrent purge can delete the row between the
	// existence pre-check and the UPDATE (fetchAccounts/precheck are separate
	// statements, not one atomic step). Fail closed: answer NotFound and emit
	// NOTHING (no success slog, no audit row) instead of manufacturing history.
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar cuenta: medir la escritura: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("actualizar cuenta: %w: la fila desapareció durante la operación (purga concurrente)", domain.ErrNotFound)
	}

	actorID := actorFromContext(ctx)
	r.logger.Info("account updated", asAnyLifts(auditAttrs(actorID, a.ID, string(a.Role)))...)
	r.persistAuditEvent(ctx, r.newAccountAuditEvent(actorID, entity.AccountAuditEventUpdated, entity.AccountAuditActionUpdate, a.ID, a.Role, nil))
	return nil
}

// Deactivate soft-deletes an account by setting is_active=0. Idempotent: second call is no-op.
// Returns domain.ErrNotFound if the account does not exist. Returns nil if already deactivated.
// Requires admin or owner role.
func (r *AccountsRepo) Deactivate(ctx context.Context, id string) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("desactivar cuenta: %w", err)
	}

	if id == "" {
		return fmt.Errorf("desactivar cuenta: %w: el id no puede estar vacío", domain.ErrInvalidInput)
	}

	// Check current state to get role for audit log and handle idempotency
	var isActive int
	var role string
	err := r.db.QueryRowContext(ctx,
		`SELECT is_active, role FROM accounts WHERE id = ?`, id,
	).Scan(&isActive, &role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("desactivar cuenta: %w: cuenta con id %q no encontrada", domain.ErrNotFound, id)
		}
		return fmt.Errorf("desactivar cuenta: %w", err)
	}

	// Idempotent: already deactivated → no-op, no audit log
	if isActive == 0 {
		return nil
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE accounts SET is_active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("desactivar cuenta: %w", err)
	}
	// Same TOCTOU answer as Update (GGA round 1 of the #155 commit): the
	// pre-check is not atomic with the write, so RowsAffected == 0 means the
	// row was purged concurrently - NotFound, no success slog, no audit row.
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("desactivar cuenta: medir la escritura: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("desactivar cuenta: %w: la fila desapareció durante la operación (purga concurrente)", domain.ErrNotFound)
	}

	actorID := actorFromContext(ctx)
	r.logger.Info("account deactivated", asAnyLifts(auditAttrs(actorID, id, role))...)
	r.persistAuditEvent(ctx, r.newAccountAuditEvent(actorID, entity.AccountAuditEventDeactive, entity.AccountAuditActionDeactivate, id, entity.AccountRole(role), nil))
	return nil
}

// TransferOwnership atomically moves ownership from the current active owner
// (fromID) to an existing INACTIVE owner-role account (toID).
//
// Both writes run inside ONE transaction, in this order:
//
//  1. deactivate fromID;
//  2. activate toID.
//
// The order is load-bearing, not cosmetic: the accounts_single_owner_update
// trigger (internal/db/schema.go:225-247) fires per statement and aborts when
// the table would hold two active owners. Activating toID while fromID is still
// active would therefore abort; with the deactivate first, no statement ever
// observes two active owners. Because both UPDATEs share the transaction, a
// failure rolls the pair back and the original owner stays active — the system
// is never left without an owner.
//
// toID's row must already exist with role=owner and is_active=0: preparing it
// (a fresh owner row, or a promoted staff row) is the caller's responsibility
// and deliberately stays outside this transaction (ADR-0016 Decision 1).
//
// Requires the owner role. Semantic errors carry domain codes: ErrNotFound for
// a missing fromID/toID, ErrInvalidInput for the wrong role or a self-transfer,
// ErrConflict for a fromID that is not active or a toID that already is.
func (r *AccountsRepo) TransferOwnership(ctx context.Context, fromID, toID string) error {
	if _, err := auth.RequireRole(ctx, auth.RoleOwner); err != nil {
		return fmt.Errorf("transferir ownership: %w", err)
	}

	if fromID == "" || toID == "" {
		return fmt.Errorf("transferir ownership: %w: el owner actual y el sucesor son obligatorios", domain.ErrInvalidInput)
	}
	if fromID == toID {
		return fmt.Errorf("transferir ownership: %w: el owner actual y el sucesor no pueden ser la misma cuenta", domain.ErrInvalidInput)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("transferir ownership: iniciar la transacción: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			// Rollback on every early return: the deferred rollback is what makes
			// the pair of UPDATEs all-or-nothing.
			_ = tx.Rollback()
		}
	}()

	if err := assertActiveOwnerTx(ctx, tx, fromID); err != nil {
		return err
	}
	if err := assertInactiveOwnerTx(ctx, tx, toID); err != nil {
		return err
	}

	if err := updateAccountActiveTx(ctx, tx, fromID, 0); err != nil {
		return fmt.Errorf("transferir ownership: desactivar al owner actual: %w", err)
	}
	if err := updateAccountActiveTx(ctx, tx, toID, 1); err != nil {
		if isSingleOwnerViolation(err) {
			return fmt.Errorf("transferir ownership: %w: ya existe un owner activo; la transferencia no se aplicó", domain.ErrConflict)
		}
		return fmt.Errorf("transferir ownership: activar al nuevo owner: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("transferir ownership: confirmar la transacción: %w", err)
	}
	committed = true

	actorID := actorFromContext(ctx)
	// Detail is the typed audit payload: the sink marshals it, so no
	// free-form JSON can bypass the mask rule (previous owner ships masked).
	detail := &entity.AccountAuditDetail{PreviousOwnerMasked: entity.MaskPhone(fromID)}
	// The journal mirrors the row's mask (ADR-0021 decision 6): the previous
	// owner ships masked here too, instead of keeping the historical raw shape.
	attrs := auditAttrs(actorID, toID, string(entity.RoleOwner))
	attrs = append(attrs, slog.String("previous_owner_id", entity.MaskPhone(fromID)))
	r.logger.Info("ownership transferred", asAnyLifts(attrs)...)
	r.persistAuditEvent(ctx, r.newAccountAuditEvent(actorID, entity.AccountAuditEventTransfer, entity.AccountAuditActionTransfer, toID, entity.RoleOwner, detail))
	return nil
}

// assertActiveOwnerTx verifies inside tx that id is an ACTIVE owner-role
// account, the only valid starting point of a transfer.
func assertActiveOwnerTx(ctx context.Context, tx *sql.Tx, id string) error {
	role, active, found, err := accountStateTx(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("transferir ownership: leer la cuenta del owner actual: %w", err)
	}
	if !found {
		return fmt.Errorf("transferir ownership: %w: la cuenta del owner actual %q no existe", domain.ErrNotFound, id)
	}
	if role != string(entity.RoleOwner) {
		return fmt.Errorf("transferir ownership: %w: la cuenta %q no tiene rol owner", domain.ErrInvalidInput, id)
	}
	if active != 1 {
		return fmt.Errorf("transferir ownership: %w: la cuenta %q no es un owner activo", domain.ErrConflict, id)
	}
	return nil
}

// assertInactiveOwnerTx verifies inside tx that id already exists as an
// INACTIVE owner-role account, the only valid transfer target.
func assertInactiveOwnerTx(ctx context.Context, tx *sql.Tx, id string) error {
	role, active, found, err := accountStateTx(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("transferir ownership: leer la cuenta del sucesor: %w", err)
	}
	if !found {
		return fmt.Errorf("transferir ownership: %w: la cuenta del sucesor %q no existe", domain.ErrNotFound, id)
	}
	if role != string(entity.RoleOwner) {
		return fmt.Errorf("transferir ownership: %w: la cuenta %q no tiene rol owner; preparala antes de transferir", domain.ErrInvalidInput, id)
	}
	if active != 0 {
		return fmt.Errorf("transferir ownership: %w: la cuenta %q ya está activa", domain.ErrConflict, id)
	}
	return nil
}

// accountStateTx reads the (role, is_active) pair of one account inside tx.
// found is false when the row does not exist.
func accountStateTx(ctx context.Context, tx *sql.Tx, id string) (role string, active int, found bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT role, is_active FROM accounts WHERE id = ?`, id).Scan(&role, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return role, active, true, nil
}

// updateAccountActiveTx flips one account's is_active flag inside tx. A single
// parameterized statement serves both halves of the transfer.
func updateAccountActiveTx(ctx context.Context, tx *sql.Tx, id string, active int) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE accounts SET is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		active, id,
	)
	return err
}

// IsActive checks if an account is active. Returns (false, nil) for missing rows — NOT domain.ErrNotFound.
// Requires an authenticated caller.
func (r *AccountsRepo) IsActive(ctx context.Context, id string) (bool, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return false, fmt.Errorf("verificar estado activo: %w", err)
	}

	var isActive int
	err := r.db.QueryRowContext(ctx,
		`SELECT is_active FROM accounts WHERE id = ?`, id,
	).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("verificar estado activo: %w", err)
	}
	return isActive == 1, nil
}

// ListByProfessional returns staff accounts matching the given professional ID, ordered by display_name ASC.
// Requires an authenticated caller.
func (r *AccountsRepo) ListByProfessional(ctx context.Context, professionalID string) ([]*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
	}

	if professionalID == "" {
		return nil, fmt.Errorf("buscar por profesional: %w: professional_id no puede estar vacío", domain.ErrInvalidInput)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = 'staff' AND professional_id = ? ORDER BY display_name ASC`,
		professionalID,
	)
	if err != nil {
		return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	accounts := make([]*entity.Account, 0)
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
	}
	return accounts, nil
}

// scanAccount scans a *sql.Row into an *entity.Account. Wraps sql.ErrNoRows as domain.ErrNotFound.
func scanAccount(row *sql.Row) (*entity.Account, error) {
	var a entity.Account
	var isActive int
	var profID *string
	var roleStr string

	err := row.Scan(&a.ID, &roleStr, &a.DisplayName, &profID, &isActive, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("cuenta no encontrada: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("leer cuenta: %w", err)
	}

	a.Role = entity.AccountRole(roleStr)
	a.ProfessionalID = profID
	a.Active = isActive == 1
	return &a, nil
}

// scanAccountRow scans a *sql.Rows (from Query) into an *entity.Account.
func scanAccountRow(rows *sql.Rows) (*entity.Account, error) {
	var a entity.Account
	var isActive int
	var profID *string
	var roleStr string

	if err := rows.Scan(&a.ID, &roleStr, &a.DisplayName, &profID, &isActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, fmt.Errorf("leer cuenta: %w", err)
	}

	a.Role = entity.AccountRole(roleStr)
	a.ProfessionalID = profID
	a.Active = isActive == 1
	return &a, nil
}
