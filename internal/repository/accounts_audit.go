package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// auditActorRefRe is the documented shape of the hashed actor reference: 64
// lowercase hex chars. "" is handled before the regex; any other value is a
// wiring bug this sink refuses to persist.
//
// MIXED ACCEPTANCE WINDOW (ADR-0021): the shape is scheme-agnostic on purpose.
// Rows written before the keyed digest landed carry an unkeyed SHA-256 digest;
// rows written after carry an HMAC-SHA-256 digest keyed with the installation
// key. Both are byte-for-byte the same 64-lowercase-hex shape, so this table,
// its validation and every reader stay untouched — there is no schema change, no
// backfill and no migration: the plaintext behind historical digests is gone, so
// a re-hash is impossible by construction. What changes is verifiability: a
// digest is only comparable to ids hashed with the SAME key, which is why
// rotating the key starts a new digest namespace (ADR-0021).
var auditActorRefRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// maxAuditListLimit bounds the read the audit view may ask for. The OQ-6
// decision is "last 200, no pagination"; 500 keeps headroom for a future dial
// without changing the default the operator sees.
const (
	auditDefaultLimit = 200
	auditMaxLimit     = 500
)

// AuditTrailDefaultLimit is the default page the TUI audit view shows
// (owner decision OQ-6).
const AuditTrailDefaultLimit = auditDefaultLimit

// AccountsAuditRepo owns the append-only persistence of the account audit
// trail. Reads are never audited: the audit trail records mutations only
// (design §1.3, spec " Reads (list) MUST NOT generate audit rows").
type AccountsAuditRepo struct {
	db *sql.DB
}

// NewAccountsAuditRepo builds the adapter over an already-open *sql.DB. It
// opens no connections and runs no migrations: initSchema owns the DDL.
func NewAccountsAuditRepo(db *sql.DB) *AccountsAuditRepo {
	return &AccountsAuditRepo{db: db}
}

// Append persists exactly one audit row. The caller owns event construction
// (masked identities, hashed actor reference, RFC3339Nano ts): the adapter is
// a dumb sink so the emitters cannot drift from the shared rules, and the
// single SQL statement is the prepared `?` shape AGENTS.md mandates.
//
// An error bubbles up to the emission site, where the mutation caller decides
// the policy: the spec keeps audit failures non-fatal for the mutation itself
// (same criterion as the current slog trail), so Append is deliberately NOT
// best-effort inside this file — fail-open lives at the call sites.
func (r *AccountsAuditRepo) Append(ctx context.Context, e *entity.AccountAuditEvent) error {
	if ctx == nil {
		return fmt.Errorf("append audit: contexto obligatorio")
	}
	if e.Ts == "" {
		e.Ts = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := entity.ValidateAuditTargetRole(e.TargetRole); err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	if e.Event == "" || e.Action == "" {
		return fmt.Errorf("append audit: event y action son obligatorios")
	}
	detailJSON, err := auditDetailJSON(e)
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}

	// Sink-side contract enforcement (defense-in-depth with the
	// construction-time masking in newAccountAuditEvent): the account id IS
	// the phone, so the persisted target and phone columns are ALWAYS the
	// mask (caller-supplied mask strings are discarded, never trusted), the
	// actor reference must be the documented hash shape (64 lowercase hex,
	// either scheme — see auditActorRefRe) or absent, and detail is the typed
	// payload the entity defines — no free-form JSON.
	masked := entity.MaskPhone(e.TargetID)
	e.TargetID, e.PhoneMasked = masked, masked
	if e.ActorRef != "" && !auditActorRefRe.MatchString(e.ActorRef) {
		return fmt.Errorf("append audit: actor_ref inválido: solo 64 hexes minúsculas (ADR-0021) o vacío")
	}

	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts_audit (ts, event, actor_ref, target_id, target_role, action, phone_masked, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Ts, e.Event, nilIfEmpty(e.ActorRef), nilIfEmpty(e.TargetID), nilIfEmpty(e.TargetRole), e.Action,
		nilIfEmpty(e.PhoneMasked), detailJSON,
	); err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}

// Last persists a read of the latest audit events, newest first, capped by
// limit. The limit is a hard clamp (1..auditMaxLimit, default
// auditDefaultLimit): an operator tooling bug cannot turn the read into a
// full-table scan.
func (r *AccountsAuditRepo) LastAccountsAudit(ctx context.Context, limit int) ([]*entity.AccountAuditEvent, error) {
	if limit <= 0 {
		limit = auditDefaultLimit
	}
	if limit > auditMaxLimit {
		limit = auditMaxLimit
	}

	// Order by the AUTOINCREMENT id (monotonic insertion order), not the ts
	// string: RFC3339Nano has variable-width fractions, so the lexically
	// sortable-looking ts is display-only (ORDER BY ts DESC could interleave
	// same-second rows written by concurrent flows).
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, ts, event, actor_ref, target_id, target_role, action, phone_masked, detail
		 FROM accounts_audit
		 ORDER BY id DESC
		 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := make([]*entity.AccountAuditEvent, 0, limit)
	for rows.Next() {
		var (
			e                                             entity.AccountAuditEvent
			actorRef, targetID, targetRole, phone, detail sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.Ts, &e.Event, &actorRef, &targetID, &targetRole, &e.Action, &phone, &detail); err != nil {
			return nil, fmt.Errorf("list audit: %w", err)
		}
		e.ActorRef, e.TargetID, e.TargetRole, e.PhoneMasked =
			actorRef.String, targetID.String, targetRole.String, phone.String
		if detail.Valid {
			var wire auditDetailWire
			if err := json.Unmarshal([]byte(detail.String), &wire); err != nil {
				return nil, fmt.Errorf("list audit: detail: %w", err)
			}
			// Map the wire detail back onto the wire-free domain carrier: the
			// adapter is the only place where the two shapes meet.
			e.Detail = &entity.AccountAuditDetail{PreviousOwnerMasked: wire.PreviousOwnerMasked}
		}
		events = append(events, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	return events, nil
}

// nilIfEmpty maps Go empty strings to SQL NULL: an absent identity stays
// absent in the row instead of an empty-string sentinel.
func auditDetailJSON(e *entity.AccountAuditEvent) (sql.NullString, error) {
	if e.Detail == nil {
		return sql.NullString{}, nil
	}
	dto := entity.AccountAuditDetail{PreviousOwnerMasked: e.Detail.PreviousOwnerMasked}
	if dto.PreviousOwnerMasked != "" && !maskShapeRe.MatchString(dto.PreviousOwnerMasked) {
		return sql.NullString{}, fmt.Errorf("append audit: detail.previous_owner_masked no cumple el shape de mask (***/***+4 dígitos)")
	}
	b, err := json.Marshal(auditDetailWire{PreviousOwnerMasked: dto.PreviousOwnerMasked})
	if err != nil {
		return sql.NullString{}, fmt.Errorf("append audit: marshal detail: %w", err)
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}

// auditDetailWire is the pinned persistence shape of the `detail` column
// (GGA round 3): the domain entity stays wire-free, so the sink adapter owns
// the json tags here. The bytes are byte-identical to the contract the B8
// review pinned originally.
type auditDetailWire struct {
	PreviousOwnerMasked string `json:"previous_owner_masked,omitempty"`
}

// maskShapeRe is the complete legal shape of a persisted mask value: exactly
// "***" or "***" followed by EXACTLY four runes, so a caller cannot smuggle a
// suffix of the raw number behind the prefix ("***+549…" rejected here).
var maskShapeRe = regexp.MustCompile(`^\*\*\*(?:[0-9]{4})?$`)

func nilIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
