package db

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB creates an in-memory SQLite DB with pragmas matching production.
// Pragmas are set via the DSN (see buildDSN) so every connection in the pool
// inherits them. WAL is silently ignored for in-memory databases.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := buildDSN(":memory:")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	wantDesc := "initial schema: 8 domain tables per PRD §3.7 + schema_version + 6 FTS sync triggers + 4 secondary indexes"
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

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("first initSchema: %v", err)
	}
	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("second initSchema: %v", err)
	}

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

func TestBusinessProfile_MessengerPlatformCheck(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	// Invalid value: 'discord' must fail the CHECK constraint.
	_, err := db.ExecContext(ctx,
		`INSERT INTO business_profile (id, name, messenger_platform) VALUES ('singleton', 'Test', 'discord')`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for messenger_platform='discord'; got nil")
	}
	if !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("expected CHECK constraint error, got: %v", err)
	}
}

func TestSecondaryIndexes_Exist(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	// 4 secondary indexes: 2 explicit CREATE INDEX + 2 auto-indexes from
	// UNIQUE constraints (business_hours_exception.exception_date and
	// schedules(professional_id, day_of_week)).
	expectedIndexes := []struct {
		table string
		index string
	}{
		{"bookings", "idx_bookings_overlap"},
		{"pending_alerts", "idx_pending_alerts_scheduled_status"},
		{"business_hours_exception", "sqlite_autoindex_business_hours_exception_1"},
		{"schedules", "sqlite_autoindex_schedules_1"},
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

// TestInitSchema_Concurrent verifies that 8 goroutines calling initSchema
// concurrently on a shared-cache in-memory database all succeed without
// errors, produce exactly one schema_version row, and leave the FTS indexes
// functional. This locks in the DSN-based pragma contract (busy_timeout,
// foreign_keys) that makes concurrent initialization safe.
func TestInitSchema_Concurrent(t *testing.T) {
	dsn := buildSharedCacheDSN(t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open shared in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	skipIfNoFTS5(t, db)

	ctx := context.Background()
	const goroutines = 8

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = initSchema(ctx, db)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d initSchema: %v", i, err)
		}
	}

	// Exactly one schema_version row (singleton).
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM schema_version").Scan(&count); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_version row count = %d; want 1", count)
	}

	// Insert a test client and verify FTS MATCH works after concurrent init.
	_, err = db.ExecContext(ctx,
		`INSERT INTO clients (id, name, phone) VALUES ('c-concurrent', 'Concurrent Client', '+5491100000000')`)
	if err != nil {
		t.Fatalf("insert test client: %v", err)
	}

	var ftsCount int
	err = db.QueryRowContext(ctx,
		`SELECT count(*) FROM clients_fts WHERE clients_fts MATCH 'Concurrent'`).Scan(&ftsCount)
	if err != nil {
		t.Fatalf("fts query: %v", err)
	}
	if ftsCount != 1 {
		t.Errorf("fts count for 'Concurrent' = %d; want 1", ftsCount)
	}

	// Open a second handle to verify DSN pragmas propagate to new connections.
	db2, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })

	var fk int
	if err := db2.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d; want 1", fk)
	}

	var timeout int
	if err := db2.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != busyTimeoutMillis {
		t.Errorf("busy_timeout = %d; want %d", timeout, busyTimeoutMillis)
	}
}

// installLegacySchema creates the pre-release 4-table schema with column
// types that are INCOMPATIBLE with the new schema (e.g., business_profile
// has slots_config NOT NULL and duration_mins instead of duration_minutes).
// This is used to test the destructive-replace migration path.
func installLegacySchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	legacyDDL := []string{
		`CREATE TABLE business_profile (
			id            TEXT PRIMARY KEY DEFAULT 'singleton',
			name          TEXT NOT NULL,
			slots_config  TEXT NOT NULL,
			duration_mins INTEGER NOT NULL
		)`,
		`CREATE TABLE clients (
			id    TEXT PRIMARY KEY,
			name  TEXT NOT NULL,
			phone TEXT NOT NULL
		)`,
		`CREATE TABLE services (
			id       TEXT PRIMARY KEY,
			name     TEXT NOT NULL,
			duration INTEGER NOT NULL
		)`,
		`CREATE TABLE appointments (
			id        TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			start_at  TEXT NOT NULL
		)`,
	}
	for _, stmt := range legacyDDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("install legacy schema: %v", err)
		}
	}
}

// TestNewDatabase_OnLegacySchema verifies that NewDatabase transparently
// migrates from the legacy 4-table schema to the new schema by dropping
// legacy tables and creating the new ones.
func TestNewDatabase_OnLegacySchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	// Step 1: Open the file directly and install the legacy schema.
	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	installLegacySchema(t, legacyDB)

	// Verify legacy tables exist.
	ctx := context.Background()
	var legacyCount int
	if err := legacyDB.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('business_profile','clients','services','appointments')",
	).Scan(&legacyCount); err != nil {
		t.Fatalf("count legacy tables: %v", err)
	}
	if legacyCount != 4 {
		t.Fatalf("legacy table count = %d; want 4", legacyCount)
	}
	_ = legacyDB.Close()

	// Step 2: Call NewDatabase on the same file — should migrate.
	db, err := NewDatabase(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewDatabase on legacy DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Step 3: Verify new schema is in place.
	newTables := []string{"schema_version", "professionals", "bookings", "pending_alerts"}
	for _, table := range newTables {
		var name string
		if err := db.Conn.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name); err != nil {
			t.Errorf("new table %q not found: %v", table, err)
		}
	}

	// Step 4: Verify legacy-only table "appointments" is gone.
	var apCount int
	if err := db.Conn.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='appointments'",
	).Scan(&apCount); err != nil {
		t.Fatalf("check appointments: %v", err)
	}
	if apCount != 0 {
		t.Error("legacy table 'appointments' still exists after migration")
	}

	// Step 5: Verify schema_version row exists.
	var version int
	if err := db.Conn.QueryRowContext(ctx,
		"SELECT version FROM schema_version WHERE version=1",
	).Scan(&version); err != nil {
		t.Errorf("schema_version row not found: %v", err)
	}
}

// TestInitSchema_RepeatedCallsAreIdempotent verifies that initSchema can be
// called multiple times on the same database without errors, building on the
// guarantees of TestInitSchema_Idempotent and TestSchemaVersion_NoDuplicateOnReInit.
func TestInitSchema_RepeatedCallsAreIdempotent(t *testing.T) {
	db := newTestDB(t)
	skipIfNoFTS5(t, db)
	ctx := context.Background()

	// First call: full initialization.
	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("first initSchema: %v", err)
	}

	// Subsequent calls: must succeed without error (idempotent retry).
	for i := 0; i < 5; i++ {
		if err := initSchema(ctx, db); err != nil {
			t.Fatalf("initSchema retry %d: %v", i+1, err)
		}
	}

	// Verify schema is still correct after retries.
	// Count only the tables we explicitly created (not FTS5 shadow tables
	// like clients_fts_data, clients_fts_idx, etc.).
	expectedTables := []string{
		"business_profile", "business_hours_exception", "professionals",
		"schedules", "services", "clients", "bookings", "pending_alerts",
		"schema_version", "clients_fts", "services_fts",
	}
	for _, table := range expectedTables {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE (type='table' OR type='virtual table') AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after retry: %v", table, err)
		}
	}

	// Exactly one schema_version row.
	var versionCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM schema_version").Scan(&versionCount); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}
	if versionCount != 1 {
		t.Errorf("schema_version row count = %d; want 1", versionCount)
	}
}

// TestNewDatabase_RecoversFromBadDSN verifies that NewDatabase can open a
// database previously initialized with a malformed DSN, because buildDSN
// supplies the correct pragmas on the new connection.
func TestNewDatabase_RecoversFromBadDSN(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/bad_dsn.db"

	// Open the DB directly with a DSN that omits busy_timeout.
	// This simulates a misconfigured connection.
	badDSN := dbPath + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	badDB, err := sql.Open("sqlite", badDSN)
	if err != nil {
		t.Fatalf("open bad DSN: %v", err)
	}
	// Set up the schema manually (skip NewDatabase which would fail verifyPragmas).
	ctx := context.Background()
	if err := initSchema(ctx, badDB); err != nil {
		t.Fatalf("initSchema on bad DB: %v", err)
	}
	_ = badDB.Close()

	// Now call NewDatabase on the same file. It should succeed because
	// buildDSN adds all required pragmas. This verifies that NewDatabase
	// with a proper DSN works even on a DB that was previously opened
	// with a bad DSN.
	db, err := NewDatabase(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewDatabase on fixed DSN: %v", err)
	}
	_ = db.Close()

	// The verifyPragmas guard is exercised implicitly by every test that
	// calls NewDatabase: a connection missing the required pragmas would
	// fail here. Directly mocking a wrong-pragma connection is not
	// feasible with a real SQLite driver, so this test confirms recovery
	// from a bad-DSN initialization rather than pragma validation itself.
}

// TestBuildSharedCacheDSN verifies that buildSharedCacheDSN returns a DSN
// with the expected format: file:<name>?mode=memory&cache=shared plus the
// 4 production pragma parameters.
func TestBuildSharedCacheDSN(t *testing.T) {
	dsn := buildSharedCacheDSN("test_db")

	// Must contain cache=shared for shared-cache mode.
	if !strings.Contains(dsn, "cache=shared") {
		t.Errorf("DSN %q does not contain 'cache=shared'", dsn)
	}

	// Must contain mode=memory.
	if !strings.Contains(dsn, "mode=memory") {
		t.Errorf("DSN %q does not contain 'mode=memory'", dsn)
	}

	// Must contain the database name.
	if !strings.Contains(dsn, "file:test_db") {
		t.Errorf("DSN %q does not contain 'file:test_db'", dsn)
	}

	// Must contain all 4 pragma parameters.
	expectedPragmas := []string{
		"_pragma=foreign_keys",
		"_pragma=busy_timeout",
		"_pragma=journal_mode",
		"_pragma=synchronous",
	}
	for _, p := range expectedPragmas {
		if !strings.Contains(dsn, p) {
			t.Errorf("DSN %q does not contain %q", dsn, p)
		}
	}
}

// TestBuildDSN verifies that buildDSN returns a DSN with the expected format:
// <dbPath> plus the 4 production pragma parameters.
func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name             string
		dbPath           string
		wantPathContains string // substring expected in the DSN path portion
	}{
		{"file path", "/tmp/test.db", "/tmp/test.db"},
		{"memory", ":memory:", ":memory:"},
		{"path with space", "/tmp/my db.db", "/tmp/my%20db.db"},
		{"windows-style path", "C:/path/to/db.db", "c:/path/to/db.db"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := buildDSN(tt.dbPath)

			// Must contain the expected path portion (may be URL-encoded).
			if !strings.Contains(dsn, tt.wantPathContains) {
				t.Errorf("DSN %q does not contain expected path %q", dsn, tt.wantPathContains)
			}

			// Must contain all 4 pragma parameters.
			expectedPragmas := []string{
				"_pragma=foreign_keys",
				"_pragma=busy_timeout",
				"_pragma=journal_mode",
				"_pragma=synchronous",
			}
			for _, p := range expectedPragmas {
				if !strings.Contains(dsn, p) {
					t.Errorf("DSN %q does not contain %q", dsn, p)
				}
			}
		})
	}
}
