package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// newPhoneTestDB creates a real SQLite database (WAL + busy_timeout, as in
// production): BEGIN IMMEDIATE and cross-connection locking cannot be mocked.
func newPhoneTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), filepath.Join(t.TempDir(), "phone.db"))
	if err != nil {
		t.Fatalf("create phone test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database.Conn
}

// countClientRowsByID returns the number of clients rows with the given id.
func countClientRowsByID(t *testing.T, conn *sql.DB, id string) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM clients WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count clients: %v", err)
	}
	return n
}

func TestClientsRepoGetOrCreateByPhone(t *testing.T) {
	const phone = "+5491100999999"
	ctx := context.Background()

	t.Run("absent phone creates row with id equal to phone", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		repo := NewClientsRepo(conn)

		got, created, err := repo.GetOrCreateByPhone(ctx, phone, "Cliente "+phone)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !created {
			t.Error("created = false, want true")
		}
		if got.ID != phone || got.Phone != phone {
			t.Errorf("id=%q phone=%q, want both %q", got.ID, got.Phone, phone)
		}
		var name, stored string
		if err := conn.QueryRowContext(ctx,
			`SELECT name, phone FROM clients WHERE id = ?`, phone).Scan(&name, &stored); err != nil {
			t.Fatalf("read stored row: %v", err)
		}
		if stored != phone || name != "Cliente "+phone {
			t.Errorf("stored (name, phone) = (%q, %q), want (%q, %q)", name, stored, "Cliente "+phone, phone)
		}
	})

	t.Run("existing row returned unchanged without duplicating", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		repo := NewClientsRepo(conn)
		if _, _, err := repo.GetOrCreateByPhone(ctx, phone, "Ana"); err != nil {
			t.Fatalf("first call: %v", err)
		}

		got, created, err := repo.GetOrCreateByPhone(ctx, phone, "Otro Nombre")
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if created {
			t.Error("created = true, want false")
		}
		if got.ID != phone || got.Name != "Ana" {
			t.Errorf("got (id, name) = (%q, %q), want (%q, Ana)", got.ID, got.Name, phone)
		}
		if n := countClientRowsByID(t, conn, phone); n != 1 {
			t.Errorf("row count = %d, want 1", n)
		}
		var storedName string
		if err := conn.QueryRowContext(ctx, `SELECT name FROM clients WHERE id = ?`, phone).Scan(&storedName); err != nil {
			t.Fatalf("read stored name after commit: %v", err)
		}
		if storedName != "Ana" {
			t.Errorf("stored name = %q, want %q (second call must not overwrite)", storedName, "Ana")
		}
	})

	t.Run("concurrent calls yield one row and exactly one created", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		repo := NewClientsRepo(conn)
		var wg sync.WaitGroup
		created := make([]bool, 2)
		errs := make([]error, 2)
		for i := range created {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, created[i], errs[i] = repo.GetOrCreateByPhone(ctx, phone, "Cliente "+phone)
			}(i)
		}
		wg.Wait()

		createdCount := 0
		for i, err := range errs {
			if err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
			if created[i] {
				createdCount++
			}
		}
		if createdCount != 1 {
			t.Errorf("created==true count = %d, want 1", createdCount)
		}
		if n := countClientRowsByID(t, conn, phone); n != 1 {
			t.Errorf("row count = %d, want 1", n)
		}
	})

	t.Run("rejects empty phone or display name", func(t *testing.T) {
		conn := newPhoneTestDB(t)
		repo := NewClientsRepo(conn)

		if _, _, err := repo.GetOrCreateByPhone(ctx, "", "Ana"); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("empty phone: err = %v, want ErrInvalidInput", err)
		}
		if _, _, err := repo.GetOrCreateByPhone(ctx, phone, "  "); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("blank name: err = %v, want ErrInvalidInput", err)
		}
		if n := countClientRowsByID(t, conn, ""); n != 0 {
			t.Errorf("empty-phone row count = %d, want 0", n)
		}
	})
}
