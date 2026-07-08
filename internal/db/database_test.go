package db

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB creates an in-memory SQLite DB with pragmas matching production
// and the full schema applied (idempotent).
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			t.Fatalf("pragma %q: %v", p, err)
		}
	}

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}
	return db
}

// skipIfNoFTS5 skips the test when the driver lacks FTS5 or JSON1 support.
func skipIfNoFTS5(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	var hasFTS5 int
	if err := db.QueryRowContext(ctx, "SELECT sqlite_compileoption_used('ENABLE_FTS5')").Scan(&hasFTS5); err != nil || hasFTS5 == 0 {
		t.Skipf("driver does not support FTS5 (compile option ENABLE_FTS5)")
	}
	var result string
	if err := db.QueryRowContext(ctx, `SELECT json_extract('{"a":1}', '$.a')`).Scan(&result); err != nil {
		t.Skipf("driver does not support JSON1: %v", err)
	}
}

func TestInitSchema_CreatesAllTables(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	expectedTables := []string{
		"business_profile",
		"business_hours_exception",
		"professionals",
		"schedules",
		"services",
		"clients",
		"bookings",
		"pending_alerts",
		"schema_version",
	}
	for _, table := range expectedTables {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	// FTS virtual tables (sqlite_master reports them as type='table')
	for _, fts := range []string{"clients_fts", "services_fts"} {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE name=? AND sql LIKE 'CREATE VIRTUAL TABLE%'", fts,
		).Scan(&name)
		if err != nil {
			t.Errorf("virtual table %q not found: %v", fts, err)
		}
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("first initSchema: %v", err)
	}
	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("second initSchema (should be idempotent): %v", err)
	}
	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("third initSchema (should be idempotent): %v", err)
	}
}

func TestSchemaVersion_RowInserted(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	var version int
	var description string
	var appliedAt string
	err := db.QueryRowContext(ctx, "SELECT version, applied_at, description FROM schema_version WHERE version=1").
		Scan(&version, &appliedAt, &description)
	if err != nil {
		t.Fatalf("schema_version row not found: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d; want 1", version)
	}
	wantDesc := "initial schema: 9 domain tables per PRD §3.7 + accounts (auth, PRD §3.8.2) + schema_version + 6 FTS sync triggers + 4 secondary indexes"
	if description != wantDesc {
		t.Errorf("description = %q; want %q", description, wantDesc)
	}
	if appliedAt == "" {
		t.Error("applied_at is empty; want ISO 8601 UTC timestamp")
	}
}

func TestSchemaVersion_NoDuplicateOnReInit(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	_ = initSchema(ctx, db)
	_ = initSchema(ctx, db)

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM schema_version").Scan(&count); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_version row count = %d; want 1", count)
	}
}

func TestFTS_ClientsSync(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	// INSERT → FTS row appears
	_, err := db.ExecContext(ctx,
		`INSERT INTO clients (id, name, phone) VALUES ('c-001', 'María García', '+5491112345678')`)
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	var count int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'María'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query: %v", err)
	}
	if count != 1 {
		t.Errorf("after INSERT: fts count = %d; want 1", count)
	}

	// UPDATE → FTS row updated
	_, err = db.ExecContext(ctx,
		`UPDATE clients SET name='María López', preferences='alérgica a penicilina' WHERE id='c-001'`)
	if err != nil {
		t.Fatalf("update client: %v", err)
	}

	err = db.QueryRowContext(ctx, `SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'López'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query after update: %v", err)
	}
	if count != 1 {
		t.Errorf("after UPDATE: fts count for 'López' = %d; want 1", count)
	}

	// Old name should be gone
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'García'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query for old name: %v", err)
	}
	if count != 0 {
		t.Errorf("after UPDATE: fts count for 'García' = %d; want 0", count)
	}

	// DELETE → FTS row removed
	_, err = db.ExecContext(ctx, `DELETE FROM clients WHERE id='c-001'`)
	if err != nil {
		t.Fatalf("delete client: %v", err)
	}

	err = db.QueryRowContext(ctx, `SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'López'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("after DELETE: fts count for 'López' = %d; want 0", count)
	}
}

func TestFTS_ServicesSync(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	// INSERT → FTS row appears
	_, err := db.ExecContext(ctx,
		`INSERT INTO services (id, name, description, duration_minutes, price)
		 VALUES ('s-001', 'Consulta Veterinaria', 'Revisión general de mascotas', 30, 5000.00)`)
	if err != nil {
		t.Fatalf("insert service: %v", err)
	}

	var count int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM services_fts WHERE services_fts MATCH 'Veterinaria'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query: %v", err)
	}
	if count != 1 {
		t.Errorf("after INSERT: fts count = %d; want 1", count)
	}

	// Also searchable by description
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM services_fts WHERE services_fts MATCH 'mascotas'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query description: %v", err)
	}
	if count != 1 {
		t.Errorf("after INSERT: fts count for description = %d; want 1", count)
	}

	// UPDATE → FTS row updated
	_, err = db.ExecContext(ctx,
		`UPDATE services SET name='Consulta General', description='Revisión perros y gatos' WHERE id='s-001'`)
	if err != nil {
		t.Fatalf("update service: %v", err)
	}

	err = db.QueryRowContext(ctx, `SELECT count(*) FROM services_fts WHERE services_fts MATCH 'General'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query after update: %v", err)
	}
	if count != 1 {
		t.Errorf("after UPDATE: fts count for 'General' = %d; want 1", count)
	}

	// Old name should be gone
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM services_fts WHERE services_fts MATCH 'Veterinaria'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query for old name: %v", err)
	}
	if count != 0 {
		t.Errorf("after UPDATE: fts count for 'Veterinaria' = %d; want 0", count)
	}

	// DELETE → FTS row removed
	_, err = db.ExecContext(ctx, `DELETE FROM services WHERE id='s-001'`)
	if err != nil {
		t.Fatalf("delete service: %v", err)
	}

	err = db.QueryRowContext(ctx, `SELECT count(*) FROM services_fts WHERE services_fts MATCH 'General'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("after DELETE: fts count for 'General' = %d; want 0", count)
	}
}

func TestBusinessProfile_SingletonConstraint(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	// Inserting with id != 'singleton' must fail due to CHECK constraint
	_, err := db.ExecContext(ctx,
		`INSERT INTO business_profile (id, name) VALUES ('not-singleton', 'Test')`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for id != 'singleton'; got nil")
	}
}

func TestSecondaryIndexes_Exist(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	expectedIndexes := []struct {
		table string
		index string
	}{
		{"business_hours_exception", "idx_business_hours_exception_date"},
		{"schedules", "idx_schedules_professional_day"},
		{"bookings", "idx_bookings_overlap"},
		{"pending_alerts", "idx_pending_alerts_scheduled_status"},
	}

	for _, ei := range expectedIndexes {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", ei.index,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %q on table %q not found: %v", ei.index, ei.table, err)
		}
	}
}

// ─── feat-authorization: accounts table (per PRD §3.8.2) ──────────────
// The following 12 tests verify the accounts table added by feat-authorization:
// schema, CHECK constraints, default is_active, ISO 8601 timestamps, and the
// single-owner invariant (INSERT and UPDATE triggers). The tests were
// introduced in PR #5 (feat-authorization) and retroactively added to
// feat-db-layer PR 1 to reconcile the schema.

func TestAccountsTable_Exists(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(context.Background(), "PRAGMA table_info(accounts)")
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

	_, err = db.ExecContext(ctx, `UPDATE accounts SET is_active = 0 WHERE id = '+5491100000000'`)
	if err != nil {
		t.Fatalf("deactivate first owner: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'owner')`)
	if err != nil {
		t.Fatalf("second owner after deactivation should succeed, got: %v", err)
	}
}

// TestAccountsTable_SingleOwnerReactivateRejected verifies the UPDATE trigger
// rejects reactivating a deactivated owner when another active owner exists.
func TestAccountsTable_SingleOwnerReactivateRejected(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100000000', 'owner')`)
	if err != nil {
		t.Fatalf("first owner: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO accounts (id, role, is_active) VALUES ('+5491100001111', 'owner', 0)`)
	if err != nil {
		t.Fatalf("inactive owner: %v", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE accounts SET is_active = 1 WHERE id = '+5491100001111'`)
	if err == nil {
		t.Fatal("expected trigger to reject reactivation of owner B while A is active, got nil")
	}
	if !strings.Contains(err.Error(), "single-owner invariant") {
		t.Errorf("expected single-owner trigger error, got: %v", err)
	}
}

// TestAccountsTable_SingleOwnerRoleChangeRejected verifies the UPDATE trigger
// rejects changing an active non-owner row's role to 'owner' when another
// active owner exists.
func TestAccountsTable_SingleOwnerRoleChangeRejected(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100000000', 'owner')`)
	if err != nil {
		t.Fatalf("first owner: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'admin')`)
	if err != nil {
		t.Fatalf("admin B: %v", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE accounts SET role = 'owner' WHERE id = '+5491100001111'`)
	if err == nil {
		t.Fatal("expected trigger to reject role change to owner, got nil")
	}
	if !strings.Contains(err.Error(), "single-owner invariant") {
		t.Errorf("expected single-owner trigger error, got: %v", err)
	}
}
