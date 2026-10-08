package repository

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// newDiscardLogger returns a logger that swallows output so repository tests
// do not print audit noise. The accounts adapter only needs a non-nil logger.
func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newRegistrationLookupTestDB seeds nothing and returns the composed adapter
// over a real SQLite database (WAL + busy_timeout), plus the raw connection for
// seeding rows. BEGIN IMMEDIATE semantics and the role gates cannot be mocked.
func newRegistrationLookupTestDB(t *testing.T) (*RegistrationLookup, *AccountsRepo) {
	t.Helper()
	conn := newPhoneTestDB(t)
	accounts := NewAccountsRepo(conn, newDiscardLogger())
	return NewRegistrationLookup(accounts, NewClientsRepo(conn)), accounts
}

func TestRegistrationLookupAccountExistsByID(t *testing.T) {
	ctx := context.Background()
	const accountID = "+5491100000000"

	t.Run("reports an existing account without a caller", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		for _, active := range []int{1, 0} {
			if _, err := conn.ExecContext(ctx,
				`INSERT INTO accounts (id, role, display_name, is_active) VALUES (?, 'admin', 'Admin', ?)`,
				accountID, active); err != nil {
				t.Fatalf("seed account (active=%d): %v", active, err)
			}
			lookup := NewRegistrationLookup(NewAccountsRepo(conn, newDiscardLogger()), NewClientsRepo(conn))

			exists, err := lookup.AccountExistsByID(ctx, accountID)
			if err != nil {
				t.Fatalf("active=%d: AccountExistsByID: %v", active, err)
			}
			if !exists {
				t.Errorf("active=%d: exists = false, want true (collision check covers inactive accounts too)", active)
			}
			if _, err := conn.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, accountID); err != nil {
				t.Fatalf("cleanup account: %v", err)
			}
		}
	})

	t.Run("reports absence without a caller", func(t *testing.T) {
		lookup, _ := newRegistrationLookupTestDB(t)

		exists, err := lookup.AccountExistsByID(ctx, accountID)
		if err != nil {
			t.Fatalf("AccountExistsByID: %v", err)
		}
		if exists {
			t.Error("exists = true, want false for an absent account")
		}
	})

	t.Run("contrast: AccountsRepo.FindByID requires a caller", func(t *testing.T) {
		_, accounts := newRegistrationLookupTestDB(t)

		_, err := accounts.FindByID(ctx, accountID)
		if err == nil {
			t.Fatal("AccountsRepo.FindByID with a caller-less context must fail")
		}
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("err = %v, want domain.ErrUnauthenticated (pins the auth-free port difference)", err)
		}
	})
}

func TestRegistrationLookupFindClientByPhoneAny(t *testing.T) {
	ctx := context.Background()
	const (
		phone  = "+5491100999999"
		legacy = "550e8400-e29b-41d4-a716-446655440000"
	)

	t.Run("returns the row whatever its id, without a caller", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO clients (id, name, phone) VALUES (?, 'Legacy', ?)`, legacy, phone); err != nil {
			t.Fatalf("seed legacy client: %v", err)
		}
		lookup := NewRegistrationLookup(NewAccountsRepo(conn, newDiscardLogger()), NewClientsRepo(conn))

		got, err := lookup.FindClientByPhoneAny(ctx, phone)
		if err != nil {
			t.Fatalf("FindClientByPhoneAny: %v", err)
		}
		if got.ID != legacy || got.Phone != phone {
			t.Errorf("got (id, phone) = (%q, %q), want (%q, %q)", got.ID, got.Phone, legacy, phone)
		}
	})

	t.Run("missing phone maps to domain.ErrNotFound", func(t *testing.T) {
		lookup, _ := newRegistrationLookupTestDB(t)

		if _, err := lookup.FindClientByPhoneAny(ctx, phone); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("err = %v, want domain.ErrNotFound", err)
		}
	})

	t.Run("contrast: ClientsRepo.FindByPhone enforces the admin/owner role", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		clients := NewClientsRepo(conn)

		_, err := clients.FindByPhone(staffCtx("p1"), phone)
		if err == nil {
			t.Fatal("ClientsRepo.FindByPhone must fail for a non-admin/owner caller")
		}
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("err = %v, want domain.ErrForbidden (role gate before any query)", err)
		}
	})
}
