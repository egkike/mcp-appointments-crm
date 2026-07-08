package db

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB creates an in-memory SQLite database with all pragmas and schema applied.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite in-memory: %v", err)
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			t.Fatalf("aplicar pragma %q: %v", p, err)
		}
	}

	d := &DB{Conn: db}
	if err := d.initSchema(ctx); err != nil {
		t.Fatalf("initSchema: %v", err)
	}
	return db
}

func TestAccountsTable_Exists(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, "PRAGMA table_info(accounts)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()

	expected := map[string]string{
		"id":              "TEXT",
		"role":            "TEXT",
		"display_name":    "TEXT",
		"professional_id": "TEXT",
		"is_active":       "INTEGER",
		"created_at":      "TEXT",
		"updated_at":      "TEXT",
	}

	found := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		found[name] = colType
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	for col, expectedType := range expected {
		actualType, ok := found[col]
		if !ok {
			t.Errorf("column %q missing from accounts table", col)
			continue
		}
		if actualType != expectedType {
			t.Errorf("column %q: expected type %q, got %q", col, expectedType, actualType)
		}
	}
}

func TestAccountsTable_DefaultIsActive(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100000000', 'admin')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var isActive int
	var createdAt, updatedAt string
	err = db.QueryRowContext(ctx, `SELECT is_active, created_at, updated_at FROM accounts WHERE id = '+5491100000000'`).
		Scan(&isActive, &createdAt, &updatedAt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if isActive != 1 {
		t.Errorf("expected is_active=1, got %d", isActive)
	}

	iso8601ms := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)
	if !iso8601ms.MatchString(createdAt) {
		t.Errorf("created_at %q does not match ISO 8601 UTC with milliseconds", createdAt)
	}
	if !iso8601ms.MatchString(updatedAt) {
		t.Errorf("updated_at %q does not match ISO 8601 UTC with milliseconds", updatedAt)
	}
}

func TestAccountsTable_RoleInvalidRejected(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'manager')`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for role='manager', got nil")
	}
}

func TestAccountsTable_ClientRoleRejected(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'client')`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for role='client', got nil")
	}
}

func TestAccountsTable_StaffRequiresProfessionalID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'staff')`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for staff without professional_id, got nil")
	}
}

func TestAccountsTable_OwnerAcceptsNullProfessionalID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100000000', 'owner')`)
	if err != nil {
		t.Fatalf("expected owner with NULL professional_id to be accepted, got: %v", err)
	}
}

func TestAccountsTable_StaffWithProfessionalIDAccepted(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role, professional_id) VALUES ('+5491100002222', 'staff', 'p-001')`)
	if err != nil {
		t.Fatalf("expected staff with professional_id to be accepted, got: %v", err)
	}
}

func TestAccountsTable_SingleOwnerInsertOK(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role, display_name) VALUES ('+5491100000000', 'owner', 'Dueño')`)
	if err != nil {
		t.Fatalf("first owner insert should succeed, got: %v", err)
	}
}

func TestAccountsTable_SingleOwnerSecondOwnerRejected(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100000000', 'owner')`)
	if err != nil {
		t.Fatalf("first owner: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'owner')`)
	if err == nil {
		t.Fatal("expected trigger to reject second active owner, got nil")
	}
}

func TestAccountsTable_SingleOwnerAfterDeactivationOK(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100000000', 'owner')`)
	if err != nil {
		t.Fatalf("first owner: %v", err)
	}

	// Deactivate the first owner
	_, err = db.ExecContext(ctx, `UPDATE accounts SET is_active = 0 WHERE id = '+5491100000000'`)
	if err != nil {
		t.Fatalf("deactivate first owner: %v", err)
	}

	// Second owner should now succeed
	_, err = db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'owner')`)
	if err != nil {
		t.Fatalf("second owner after deactivation should succeed, got: %v", err)
	}
}
