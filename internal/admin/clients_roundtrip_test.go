package admin

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

// newIntegrationDB opens a real tmp-file SQLite database (WAL journal mode,
// production schema) for the acceptance proofs of this package. A tmp file is
// required: the WAL pragma rejects :memory: (verified fact 10).
func newIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), filepath.Join(t.TempDir(), "admin-tui.db"))
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database.Conn
}

// countClientRows counts the clients rows of the integration database. Raw SQL
// here is test-only scaffolding: every production path goes through the repos.
func countClientRows(t *testing.T, conn *sql.DB) int {
	t.Helper()
	var count int
	if err := conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM clients`).Scan(&count); err != nil {
		t.Fatalf("count clients rows: %v", err)
	}
	return count
}

// TestAddSelfAsClient_ResolverRoundTrip_RealSQLite is the acceptance proof of
// T6: on a real database, the operator registers itself as a client through the
// admin core and the REAL auth.CallerResolver then resolves the same caller id
// into a Caller that is BOTH the owner of the installation and a client of the
// business (ADR-0011 double role). The ClientID is non-nil only because the row
// carries id = phone, which is exactly what resolver step 2 matches on.
func TestAddSelfAsClient_ResolverRoundTrip_RealSQLite(t *testing.T) {
	ctx := context.Background()
	conn := newIntegrationDB(t)
	accounts := repository.NewAccountsRepo(conn, slog.New(slog.DiscardHandler))
	clients := repository.NewClientsRepo(conn)

	if err := Seed(ctx, accounts, SeedInput{Phone: selfClientPhone, DisplayName: "Dueño"}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	outcome, err := AddSelfAsClient(ctx, accounts, clients, selfClientPhone, "Dueño Cliente")
	if err != nil {
		t.Fatalf("AddSelfAsClient() error = %v", err)
	}
	if outcome.AlreadyRegistered {
		t.Error("AddSelfAsClient() AlreadyRegistered = true, want a fresh registration")
	}
	if outcome.ClientID != selfClientPhone {
		t.Errorf("AddSelfAsClient() ClientID = %q, want %q", outcome.ClientID, selfClientPhone)
	}

	// The stored row must use the phone as id AND as phone.
	stored, err := clients.FindByID(TUIContext(ctx), selfClientPhone)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.ID != selfClientPhone {
		t.Errorf("stored client id = %q, want the phone %q", stored.ID, selfClientPhone)
	}
	if stored.Phone != selfClientPhone {
		t.Errorf("stored client phone = %q, want %q", stored.Phone, selfClientPhone)
	}
	if stored.Name != "Dueño Cliente" {
		t.Errorf("stored client name = %q, want %q", stored.Name, "Dueño Cliente")
	}

	// The real resolver over the same database: owner role AND discovered client.
	resolver := auth.NewCallerResolver(conn)
	caller, err := resolver.Resolve(ctx, selfClientPhone)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if caller.ID != selfClientPhone {
		t.Errorf("caller.ID = %q, want %q", caller.ID, selfClientPhone)
	}
	if caller.Role != auth.RoleOwner {
		t.Errorf("caller.Role = %q, want %q", caller.Role, auth.RoleOwner)
	}
	if caller.ClientID == nil {
		t.Fatal("caller.ClientID = nil, want the resolver to discover the client row")
	}
	if *caller.ClientID != selfClientPhone {
		t.Errorf("caller.ClientID = %q, want %q", *caller.ClientID, selfClientPhone)
	}

	// Idempotent re-run: the state is reported, not rewritten.
	again, err := AddSelfAsClient(ctx, accounts, clients, selfClientPhone, "Otro Nombre")
	if err != nil {
		t.Fatalf("AddSelfAsClient() second run error = %v", err)
	}
	if !again.AlreadyRegistered {
		t.Error("second AddSelfAsClient() AlreadyRegistered = false, want the idempotent outcome")
	}
	if got := countClientRows(t, conn); got != 1 {
		t.Errorf("clients row count = %d, want exactly 1", got)
	}
}

// TestAddSelfAsClient_UUIDRowForThePhoneIsASemanticConflict_RealSQLite covers
// the operator-facing duplicate path end to end: clients.phone is UNIQUE, so a
// pre-existing clients row for the same phone with a DIFFERENT id (the UUID
// bootstrap) rejects the insert with SQLITE_CONSTRAINT_UNIQUE (2067) and the
// core reports it as a business conflict, never as a driver dump.
func TestAddSelfAsClient_UUIDRowForThePhoneIsASemanticConflict_RealSQLite(t *testing.T) {
	ctx := context.Background()
	conn := newIntegrationDB(t)
	accounts := repository.NewAccountsRepo(conn, slog.New(slog.DiscardHandler))
	clients := repository.NewClientsRepo(conn)

	if err := Seed(ctx, accounts, SeedInput{Phone: selfClientPhone, DisplayName: "Dueño"}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	bootstrapped, err := clients.GetOrCreate(TUIContext(ctx), selfClientPhone, "Otra Ficha")
	if err != nil {
		t.Fatalf("GetOrCreate() fixture error = %v", err)
	}
	if bootstrapped.ID == selfClientPhone {
		t.Fatalf("fixture id = %q, want a generated UUID", bootstrapped.ID)
	}

	_, err = AddSelfAsClient(ctx, accounts, clients, selfClientPhone, "Dueño Cliente")
	if err == nil {
		t.Fatal("AddSelfAsClient() error = nil, want the duplicate-phone conflict")
	}
	if !errors.Is(err, ErrClientPhoneTaken) {
		t.Errorf("AddSelfAsClient() error = %v, want errors.Is(ErrClientPhoneTaken)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("AddSelfAsClient() error = %v, want errors.Is(domain.ErrConflict)", err)
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("AddSelfAsClient() error = %T, want *domain.SemanticError", err)
	}
	if got := countClientRows(t, conn); got != 1 {
		t.Errorf("clients row count = %d, want the pre-existing row only", got)
	}
}

// TestGetOrCreateUUIDRowIsInvisibleToTheResolver_RealSQLite locks verified fact
// 6 and the reason the add-self flow does not reuse GetOrCreate: a clients row
// whose id is a generated UUID never matches `SELECT id FROM clients WHERE id =
// <caller id>`, so the resolver keeps ClientID nil and the operator never gains
// the client role.
func TestGetOrCreateUUIDRowIsInvisibleToTheResolver_RealSQLite(t *testing.T) {
	ctx := context.Background()
	conn := newIntegrationDB(t)
	accounts := repository.NewAccountsRepo(conn, slog.New(slog.DiscardHandler))
	clients := repository.NewClientsRepo(conn)

	if err := Seed(ctx, accounts, SeedInput{Phone: selfClientPhone, DisplayName: "Dueño"}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	created, err := clients.GetOrCreate(TUIContext(ctx), selfClientPhone, "Dueño Cliente")
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if created.ID == selfClientPhone {
		t.Fatalf("GetOrCreate() id = %q, want a generated UUID", created.ID)
	}

	caller, err := auth.NewCallerResolver(conn).Resolve(ctx, selfClientPhone)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if caller.Role != auth.RoleOwner {
		t.Errorf("caller.Role = %q, want %q", caller.Role, auth.RoleOwner)
	}
	if caller.ClientID != nil {
		t.Errorf("caller.ClientID = %q, want nil for a UUID client id", *caller.ClientID)
	}
}
