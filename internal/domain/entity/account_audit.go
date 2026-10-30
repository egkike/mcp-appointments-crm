// Package entity — account_audit.go types the persisted account audit trail
// (change feat-tui-ops-residuals, S1). The struct is the wire-free domain
// carrier: no JSON tags (audit rows never leave the server through the MCP
// transport) and no behavior beyond the masked-identity rule shared by every
// emitter.
package entity

import (
	"fmt"
	"strings"
)

// Account audit event identifiers (design §1.3 catalog). Exactly one row per
// mutation outcome; reads are never audited.
const (
	AccountAuditEventCreated  = "account_created"
	AccountAuditEventUpdated  = "account_updated"
	AccountAuditEventDeactive = "account_deactivated"
	AccountAuditEventTransfer = "ownership_transferred"
	AccountAuditEventRejected = "second_owner_rejected"

	// AccountAuditEventPurged is reserved for Slice S3 (purge-inactive): the
	// catalog ships complete from day one so the DDL CHECK can pin all legal
	// values without a later additive schema touch.
	AccountAuditEventPurged = "inactive_purged"
)

// Account audit action values (design §1.3, `action` column).
const (
	AccountAuditActionCreate     = "create"
	AccountAuditActionUpdate     = "update"
	AccountAuditActionDeactivate = "deactivate"
	AccountAuditActionTransfer   = "transfer"
	AccountAuditActionReject     = "reject"
	// AccountAuditActionPurge maps to the reserved S3 event.
	AccountAuditActionPurge = "purge"
)

// AccountAuditDetail is the wire-free typed carrier of optional audit payload
// fields: every field is a mask-by-construction value produced via MaskPhone,
// so no emitter can sneak a free-form string and no column ever carries an
// unmasked value. The persistence JSON shape lives in the sink adapter's DTO
// (internal/repository/accounts_audit.go), not here — entity types carry no
// wire tags (GGA round 3).
type AccountAuditDetail struct {
	// PreviousOwnerMasked carries the MaskPhone of the ownership transfer's
	// source account (ownership_transferred). Empty omits the key on the wire.
	PreviousOwnerMasked string
}

// AccountAuditEvent is one persisted audit row of the accounts trail. Fields
// mirror the `accounts_audit` DDL (internal/db/schema.go): masked identities
// only, no PII in any column.
type AccountAuditEvent struct {
	ID          int64               // AUTOINCREMENT id; 0 until the row is committed.
	Ts          string              // RFC3339Nano UTC, same shape as the current audit attrs.
	Event       string              // catalog constant above.
	ActorRef    string              // opaque keyed actor reference (auth.HashCallerID: HMAC-SHA-256, 64 hex; histórico unkeyed aceptado por la ventana mixed de ADR-0021); "" cuando el flow no tiene Caller y también como falla cerrada sin clave configurada.
	TargetID    string              // account id, ALWAYS via MaskPhone: the id is the phone, so the row stores the mask (spec: no raw PII in any column).
	TargetRole  string              // owner | admin | staff; "" when the row exists without role.
	Action      string              // action catalog above.
	PhoneMasked string              // MaskPhone(target); empty when the event carries no phone.
	Detail      *AccountAuditDetail // optional typed payload; the sink marshals it.
}

// ValidateAuditTargetRole accepts the three account roles ("" for catalog
// events that carry no role). Any other value is a wiring bug, not operator
// input: the caller surfaces it as the semantic wiring error it is.
func ValidateAuditTargetRole(role string) error {
	switch role {
	case "", string(RoleOwner), string(RoleAdmin), string(RoleStaff):
		return nil
	default:
		return fmt.Errorf("rol de auditoría %q inválido: solo owner, admin o staff", role)
	}
}

// MaskPhone keeps only the last 4 digits of phone and replaces the rest with
// "***" so a full number can never reach a persisted audit column (spec
// "no PII"). Values of 4 runes or fewer collapse to "***" because keeping them
// whole would expose the entire identifier.
//
// This is the single source of the masking rule: the registration audit keeps
// its 5-line rule by delegation (usecase.maskPhone → MaskPhone) so both
// emitters cannot drift, and the persistence layer never re-implements it.
func MaskPhone(phone string) string {
	trimmed := strings.TrimSpace(phone)
	runes := []rune(trimmed)
	if len(runes) <= 4 {
		return "***"
	}
	// The suffix MUST be four ASCII digits (the persisted-mask shape the audit
	// sink validates) or the input is not an identifier-shaped value: it gets
	// buried entirely instead of leaking an unknown suffix behind "***".
	suffix := runes[len(runes)-4:]
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return "***"
		}
	}
	return "***" + string(suffix)
}
