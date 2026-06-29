package db

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB creates an in-memory SQLite DB with pragmas matching production.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
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
		if _, err := db.ExecContext(context.Background(), p); err != nil {
			t.Fatalf("pragma %q: %v", p, err)
		}
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
	err := db.QueryRowContext(ctx,"SELECT version, applied_at, description FROM schema_version WHERE version=1").
		Scan(&version, &appliedAt, &description)
	if err != nil {
		t.Fatalf("schema_version row not found: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d; want 1", version)
	}
	wantDesc := "initial schema: 10 domain tables per PRD §3.7 + schema_version + 6 FTS sync triggers + 4 secondary indexes"
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
	if err := db.QueryRowContext(ctx,"SELECT count(*) FROM schema_version").Scan(&count); err != nil {
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
	err = db.QueryRowContext(ctx,`SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'María'`).Scan(&count)
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

	err = db.QueryRowContext(ctx,`SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'López'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query after update: %v", err)
	}
	if count != 1 {
		t.Errorf("after UPDATE: fts count for 'López' = %d; want 1", count)
	}

	// Old name should be gone
	err = db.QueryRowContext(ctx,`SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'García'`).Scan(&count)
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

	err = db.QueryRowContext(ctx,`SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'López'`).Scan(&count)
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
	err = db.QueryRowContext(ctx,`SELECT count(*) FROM services_fts WHERE services_fts MATCH 'Veterinaria'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query: %v", err)
	}
	if count != 1 {
		t.Errorf("after INSERT: fts count = %d; want 1", count)
	}

	// Also searchable by description
	err = db.QueryRowContext(ctx,`SELECT count(*) FROM services_fts WHERE services_fts MATCH 'mascotas'`).Scan(&count)
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

	err = db.QueryRowContext(ctx,`SELECT count(*) FROM services_fts WHERE services_fts MATCH 'General'`).Scan(&count)
	if err != nil {
		t.Fatalf("fts query after update: %v", err)
	}
	if count != 1 {
		t.Errorf("after UPDATE: fts count for 'General' = %d; want 1", count)
	}

	// Old name should be gone
	err = db.QueryRowContext(ctx,`SELECT count(*) FROM services_fts WHERE services_fts MATCH 'Veterinaria'`).Scan(&count)
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

	err = db.QueryRowContext(ctx,`SELECT count(*) FROM services_fts WHERE services_fts MATCH 'General'`).Scan(&count)
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
