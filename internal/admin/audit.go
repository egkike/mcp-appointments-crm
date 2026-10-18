// Package admin — audit.go is the core of the read-only audit trail view
// (change feat-tui-ops-residuals S1): the narrow port + operator projection
// shared by the Bubble Tea TUI and the console, mirroring ListAccounts.
package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// AuditTrailReader is the read side of the persisted accounts audit. The
// concrete adapter is repository.AccountsAuditRepo; the write side has no
// port on purpose (rows are emitted inside the repository layer at the exact
// slog emission sites, design §1.4).
type AuditTrailReader interface {
	LastAccountsAudit(ctx context.Context, limit int) ([]*entity.AccountAuditEvent, error)
}

// AuditEventView is the operator-facing projection of one audit row: the
// columns the read-only view renders (El owner's round decision OQ-6: las
// últimas 200, sin paginación interactiva). Every field is already masked at
// the persistence layer, so the projection adds nothing sensitive.
type AuditEventView struct {
	Ts          string
	Event       string
	ActorRef    string
	TargetID    string
	TargetRole  string
	Action      string
	PhoneMasked string
	Detail      string
}

// AuditTrailViewLimit is the page the audit view asks for (OQ-6).
const AuditTrailViewLimit = 200

// ListAuditTrail returns the latest audit events projected onto views, newest
// first. The limit is clamped by the reader (repository), so a tooling bug
// cannot turn the read into a full-table scan; the view renders whatever the
// reader returned, nothing more.
func ListAuditTrail(ctx context.Context, reader AuditTrailReader, limit int) ([]AuditEventView, error) {
	rows, err := reader.LastAccountsAudit(ctx, limit)
	if err != nil {
		return nil, err
	}

	views := make([]AuditEventView, 0, len(rows))
	for _, row := range rows {
		views = append(views, AuditEventView{
			Ts:          row.Ts,
			Event:       row.Event,
			ActorRef:    row.ActorRef,
			TargetID:    row.TargetID,
			TargetRole:  row.TargetRole,
			Action:      row.Action,
			PhoneMasked: row.PhoneMasked,
			Detail:      auditDetailText(row.Detail),
		})
	}
	return views, nil
}

// AuditLine renders one operator line of the audit view. Detail is
// appended only when present so empty rows stay terse.
func AuditLine(v AuditEventView) string {
	line := fmt.Sprintf("%s  %s  %s  %s  %s", v.Ts, v.Event, v.Action, v.TargetID, v.PhoneMasked)
	if v.Detail != "" {
		line += "  " + v.Detail
	}
	return line
}

// auditDetailText renders the typed audit payload as display JSON (masked by
// construction); nil → "" keeps empty rows terse.
func auditDetailText(d *entity.AccountAuditDetail) string {
	if d == nil {
		return ""
	}
	b, err := json.Marshal(d)
	if err != nil {
		return ""
	}
	return string(b)
}
