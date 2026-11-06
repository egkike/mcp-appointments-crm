// Package db manages the SQLite database connection and schema lifecycle.
//
// The schema consists of 8 domain tables (per docs/PRD.md §3.7) plus the
// `accounts` authorization whitelist (PRD §3.8.2 / ADR-0009), 2 FTS5
// virtual tables, 6 FTS sync triggers, 2 secondary indexes, 2 single-owner
// triggers, and a schema_version metadata table. All DDL is executed by
// initSchema, which is idempotent (safe to call multiple times).
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// busyTimeoutMillis is the SQLite busy_timeout value in milliseconds.
// It is embedded in the DSN so every connection in the pool inherits it,
// fixing the per-connection pragma leakage that occurred when pragmas were
// set via ExecContext on a pooled *sql.DB.
const busyTimeoutMillis = 5000

// DB wraps a *sql.DB connection to the SQLite database.
type DB struct {
	Conn *sql.DB
}

// buildDSN appends modernc.org/sqlite _pragma query parameters to dbPath so
// that per-connection pragmas (foreign_keys, busy_timeout, journal_mode,
// synchronous) are applied to every connection in the pool.
//
// Issue #175: the DSN must be a PROPER file: URI (url.URL with Path set),
// never a bare path glued to a query string. url.Parse().String() on a plain
// filesystem path silently re-encodes ordinary characters — spaces in
// ~/Library/Application Support/… (the documented macOS install path) become
// literal "%20" in the filename and the driver then seeks a directory named
// "Application%20Support" that does not exist (SQLITE_CANTOPEN, error 14).
// A file:-prefixed URI keeps the percent-encoding inside the URI component,
// which the driver decodes back before the syscall. ":memory:" keeps its
// special-case (it is not a filesystem path).
func buildDSN(dbPath string) string {
	// modernc solo procesa query params en URIs con scheme file:: un raw
	// ":memory:?_pragma=…" deja journal_mode en "memory" (verifyPragmas
	// fallaba) — la forma canónica documentada del driver es file::memory:.
	if dbPath == ":memory:" {
		return "file::memory:" + "?" + pragmaQuery()
	}
	if strings.HasPrefix(dbPath, "file:") {
		return dbPath + "?" + pragmaQuery()
	}
	pathURI := url.URL{Scheme: "file", Path: dbPath}
	return pathURI.String() + "?" + pragmaQuery()
}

// pragmaParams returns the 4 production pragma values applied to every connection.
func pragmaParams() []string {
	return []string{
		"foreign_keys(1)",
		fmt.Sprintf("busy_timeout(%d)", busyTimeoutMillis),
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
	}
}

// pragmaQuery returns the pragma parameters as a URL query string (without leading '?').
func pragmaQuery() string {
	params := pragmaParams()
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = "_pragma=" + p
	}
	return strings.Join(parts, "&")
}

// NewDatabase opens the SQLite database at dbPath, verifies production
// pragmas (WAL), and runs initSchema. Per-connection pragmas (foreign_keys,
// busy_timeout) are set via the DSN (see buildDSN) so they apply to every
// connection in the pool.
func NewDatabase(ctx context.Context, dbPath string) (*DB, error) {
	// Fail-closed (issue #177): un '?' en el path compondría una DSN
	// malformada (doble '?') y el driver ignoraría los _pragma en silencio —
	// foreign_keys OFF = integridad degradada sin crash ni log. Rechazo
	// semántico antes de tocar el driver; '#' es legal (url.URL lo escapea
	// dentro del component Path) y tiene test guard.
	if strings.ContainsRune(dbPath, '?') {
		return nil, fmt.Errorf("el path de la base de datos es inválido: contiene '?': usá un path de filesystem sin query string (el '?' rompe la composición del DSN y los pragmas quedarían en silent-ignore)")
	}
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := buildDSN(dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := verifyPragmas(ctx, conn, dbPath == ":memory:"); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := initSchema(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return &DB{Conn: conn}, nil
}

// Close releases the underlying database connection.
func (db *DB) Close() error {
	if db.Conn != nil {
		return db.Conn.Close()
	}
	return nil
}

// verifyPragmas asserts that critical SQLite pragmas are active on the
// connection: synchronous NORMAL, foreign_keys enabled, busy_timeout set, and
// WAL journal mode for file-backed databases. isMemory relaxes only the WAL
// assertion: an in-memory database structurally cannot use
// journal_mode=WAL (sqlite reports "memory"), so relaxing it is what makes
// NewDatabase(":memory:") possible while foreign_keys et al. stay verified
// below (issue #177).
func verifyPragmas(ctx context.Context, conn *sql.DB, isMemory bool) error {
	var mode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("query journal_mode: %w", err)
	}
	if mode != "wal" && !isMemory {
		return fmt.Errorf("expected WAL journal mode, got %q", mode)
	}

	// synchronous: 0=OFF, 1=NORMAL, 2=FULL, 3=EXTRA.
	// PRD §3.1 requires NORMAL for performance with acceptable durability.
	var sync int
	if err := conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&sync); err != nil {
		return fmt.Errorf("query synchronous: %w", err)
	}
	if sync != 1 {
		return fmt.Errorf("expected synchronous=NORMAL (1), got %d", sync)
	}

	var fk int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return fmt.Errorf("query foreign_keys: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("expected foreign_keys=1, got %d", fk)
	}

	var timeout int
	if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		return fmt.Errorf("query busy_timeout: %w", err)
	}
	if timeout != busyTimeoutMillis {
		return fmt.Errorf("expected busy_timeout=%d, got %d", busyTimeoutMillis, timeout)
	}

	return nil
}

// firstLine returns the first line of s, trimmed of leading/trailing whitespace.
// Used in error messages to identify which DDL statement failed.
func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

// initSchema creates all tables, FTS virtual tables, triggers, and indexes.
//
// Legacy schema migration: if the legacy "appointments" table is found in
// sqlite_master, the database contains a pre-release schema with incompatible
// column types. All legacy tables (business_profile, clients, services,
// appointments, clients_fts, services_fts) are dropped before the new DDL
// runs. This is safe because no production data existed before the new schema,
// and the schema-version spec explicitly permits destructive replace when
// legacy tables are detected.
//
// It is idempotent: calling it N times produces the same state as calling it
// once. All DDL uses IF NOT EXISTS for safety on partial-failure retries.
// Schema version tracking: after all DDL succeeds, a row with version=1 is
// inserted into schema_version (INSERT OR IGNORE for idempotency).
func initSchema(ctx context.Context, db *sql.DB) error {
	// If the legacy "appointments" table exists, this is a pre-release DB
	// with an incompatible schema. Drop all legacy tables (including FTS
	// virtual tables) so the new DDL can create tables with the correct
	// column types. The "appointments" table is used as a marker because it
	// exists ONLY in the legacy schema — it is never created by the new DDL.
	// This check is safe under concurrency: on a fresh DB, "appointments"
	// does not exist so no drops run; on a legacy DB, all goroutines drop
	// the same tables with IF EXISTS (idempotent).
	var hasLegacy int
	err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='appointments'",
	).Scan(&hasLegacy)
	if err != nil {
		return fmt.Errorf("initSchema: check legacy tables: %w", err)
	}
	if hasLegacy > 0 {
		// Drop base tables first (auto-drops associated triggers),
		// then FTS virtual tables.
		legacyTables := []string{
			"business_profile", "clients", "services", "appointments",
			"clients_fts", "services_fts",
		}
		for _, table := range legacyTables {
			stmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", table)
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("initSchema: drop legacy table %s: %w", table, err)
			}
		}
	}

	ddl := make([]string, 0, 100)
	ddl = append(ddl, domainTableDDL()...)
	ddl = append(ddl, ftsTableDDL()...)
	ddl = append(ddl, ftsTriggerDDL()...)
	ddl = append(ddl, secondaryIndexDDL()...)
	ddl = append(ddl, accountTriggerDDL()...)
	ddl = append(ddl, accountAuditDDL()...)
	ddl = append(ddl, accountAuditImmutabilityDDL()...)
	ddl = append(ddl, seedDDL()...)

	for _, stmt := range ddl {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initSchema: failed to execute %q: %w", firstLine(stmt), err)
		}
	}
	return nil
}
