package admin

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// rowFailingPurger is a minimal AccountsPurger stub: only PurgeInactive can
// actually be driven by PurgeInactiveAccounts, so the ember methods are
// programmers-never-reach stubs.
type rowFailingPurger struct{ err error }

func (p *rowFailingPurger) List(ctx context.Context) ([]*entity.Account, error) {
	return nil, nil
}
func (p *rowFailingPurger) GetByRole(ctx context.Context, role entity.AccountRole) ([]*entity.Account, error) {
	return nil, nil
}
func (p *rowFailingPurger) Deactivate(ctx context.Context, id string) error { return nil }
func (p *rowFailingPurger) PurgeInactive(ctx context.Context, id string) (bool, error) {
	return false, p.err
}

// captureDefaultLogger redirects slog.Default() into a buffer for the test and
// restores the previous logger on cleanup.
func captureDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// TestAdminPurgeInactiveAccounts_LogsRowFailureServerSide pins B8 f9 (GGA
// round 1 of the feat-b8-hygiene-triage commit): the operator-facing partial
// message says "revisá el log del servicio", so the raw per-row cause MUST
// actually reach the service log (masked id, raw error) on the error branch.
// The masked id keeps the f5 plain-id disposition scoped — this path never
// had a raw-id emission and must not grow one.
func TestAdminPurgeInactiveAccounts_LogsRowFailureServerSide(t *testing.T) {
	buf := captureDefaultLogger(t)
	const phone = "+5491122334455"
	_, err := PurgeInactiveAccounts(context.Background(), &rowFailingPurger{err: errors.New("sqlite: disk I/O error")}, []string{phone})
	if err == nil {
		t.Fatal("PurgeInactiveAccounts error = nil, want the wrapped row failure")
	}
	if !strings.Contains(err.Error(), "purgar cuenta") {
		t.Errorf("wrapped error = %v, want the \"purgar cuenta\" context", err)
	}
	out := buf.String()
	if !strings.Contains(out, "purge-inactive row failed") {
		t.Errorf("slog output = %q; want the server-side row-failure event (the operator is told to read this log)", out)
	}
	if !strings.Contains(out, "sqlite: disk I/O error") {
		t.Errorf("slog output = %q; want the raw cause server-side", out)
	}
	if !strings.Contains(out, "account="+entity.MaskPhone(phone)) {
		t.Errorf("slog output = %q; want the masked id %s in the account field", out, "account="+entity.MaskPhone(phone))
	}
	if strings.Contains(out, phone) {
		t.Errorf("slog output = %q; must never carry the raw phone", out)
	}
}
