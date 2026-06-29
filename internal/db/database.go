// Package db manages the SQLite database connection and schema lifecycle.
//
// The schema consists of 10 domain tables (per docs/PRD.md §3.7), 2 FTS5
// virtual tables, 6 FTS sync triggers, 2 secondary indexes, and a
// schema_version metadata table. All DDL is executed by initSchema, which
// is idempotent (safe to call multiple times).
package db

import (
	"context"
	"database/sql"
	"fmt"
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
// that per-connection pragmas (foreign_keys, busy_timeout) are applied to
// every connection the pool creates — not just the first one.
func buildDSN(dbPath string) string {
	return fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)",
		dbPath, busyTimeoutMillis)
}

// NewDatabase opens the SQLite database at dbPath, verifies production
// pragmas (WAL), and runs initSchema. Per-connection pragmas (foreign_keys,
// busy_timeout) are set via the DSN (see buildDSN) so they apply to every
// connection in the pool.
func NewDatabase(ctx context.Context, dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := buildDSN(dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := verifyPragmas(ctx, conn); err != nil {
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

// verifyPragmas asserts that WAL journal mode is active on the connection.
// Per-connection pragmas (foreign_keys, busy_timeout) are set via the DSN;
// this function only verifies the result for WAL since it is critical for
// concurrent read/write performance.
func verifyPragmas(ctx context.Context, conn *sql.DB) error {
	var mode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("query journal_mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("expected WAL journal mode, got %q", mode)
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
	ddl = append(ddl, seedDDL()...)

	for _, stmt := range ddl {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initSchema: failed to execute %q: %w", firstLine(stmt), err)
		}
	}
	return nil
}
