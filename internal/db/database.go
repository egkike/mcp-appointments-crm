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

// initSchema creates all tables, FTS virtual tables, triggers, and indexes.
// It is idempotent: calling it N times produces the same state as calling it
// once. All DDL uses IF NOT EXISTS for safety on partial-failure retries.
//
// Schema version tracking: after all DDL succeeds, a row with version=1 is
// inserted into schema_version (INSERT OR IGNORE for idempotency).
func initSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		// ── 1. business_profile (singleton) ──────────────────────────
		`CREATE TABLE IF NOT EXISTS business_profile (
			id                          TEXT PRIMARY KEY DEFAULT 'singleton',
			name                        TEXT NOT NULL,
			industry                    TEXT,
			country                     TEXT,
			address                     TEXT,
			latitude                    REAL,
			longitude                   REAL,
			cover_photo_url             TEXT,
			public_phone                TEXT,
			messenger_platform          TEXT,
			messenger_id                TEXT,
			contact_email               TEXT,
			website_url                 TEXT,
			general_description         TEXT,
			currency_code               TEXT NOT NULL DEFAULT 'ARS',
			currency_symbol             TEXT NOT NULL DEFAULT '$',
			accepted_payment_methods    TEXT,
			timezone                    TEXT NOT NULL DEFAULT 'UTC',
			slot_interval_minutes       INTEGER NOT NULL DEFAULT 30,
			business_hours              TEXT NOT NULL DEFAULT '{}',
			created_at                  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at                  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			CHECK (id = 'singleton')
		)`,

		// ── 2. business_hours_exception ──────────────────────────────
		`CREATE TABLE IF NOT EXISTS business_hours_exception (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			exception_date  TEXT NOT NULL UNIQUE,
			is_closed       BOOLEAN NOT NULL DEFAULT 1,
			open_time       TEXT,
			close_time      TEXT,
			reason          TEXT,
			created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			CHECK (is_closed = 1 OR (open_time IS NOT NULL AND close_time IS NOT NULL AND open_time < close_time))
		)`,

		// ── 3. professionals ─────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS professionals (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL,
			role_specialty  TEXT,
			status          TEXT NOT NULL DEFAULT 'active',
			email           TEXT,
			phone           TEXT,
			specialties     TEXT,
			created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			CHECK (status IN ('active', 'inactive'))
		)`,

		// ── 4. schedules ─────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS schedules (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			professional_id     TEXT NOT NULL REFERENCES professionals(id) ON DELETE CASCADE,
			day_of_week         INTEGER NOT NULL,
			start_time          TEXT NOT NULL,
			end_time            TEXT NOT NULL,
			UNIQUE(professional_id, day_of_week)
		)`,

		// ── 5. services ──────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS services (
			id               TEXT PRIMARY KEY,
			name             TEXT NOT NULL,
			description      TEXT,
			duration_minutes INTEGER NOT NULL,
			price            REAL NOT NULL,
			is_active        BOOLEAN NOT NULL DEFAULT 1,
			created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			CHECK (duration_minutes > 0),
			CHECK (is_active IN (0, 1))
		)`,

		// ── 6. clients ───────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS clients (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			phone        TEXT NOT NULL UNIQUE,
			email        TEXT,
			preferences  TEXT,
			created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,

		// ── 7. bookings ──────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS bookings (
			id               TEXT PRIMARY KEY,
			client_id        TEXT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
			professional_id  TEXT NOT NULL REFERENCES professionals(id) ON DELETE RESTRICT,
			service_id       TEXT NOT NULL REFERENCES services(id) ON DELETE RESTRICT,
			start_datetime   TEXT NOT NULL,
			end_datetime     TEXT NOT NULL,
			status           TEXT NOT NULL DEFAULT 'pending',
			notes            TEXT,
			payment_method   TEXT,
			created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			CHECK (status IN ('pending', 'confirmed', 'cancelled'))
		)`,

		// ── 8. pending_alerts ────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS pending_alerts (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			type                TEXT NOT NULL,
			message             TEXT NOT NULL,
			scheduled_datetime  TEXT NOT NULL,
			status              TEXT NOT NULL DEFAULT 'pending',
			related_booking_id  TEXT REFERENCES bookings(id),
			created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			CHECK (status IN ('pending', 'sent', 'cancelled'))
		)`,

		// ── 9. schema_version ────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS schema_version (
			version      INTEGER PRIMARY KEY,
			applied_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			description  TEXT
		)`,

		// ── FTS5 virtual tables ──────────────────────────────────────
		`CREATE VIRTUAL TABLE IF NOT EXISTS clients_fts USING fts5(
			name,
			preferences,
			content='clients',
			content_rowid='rowid'
		)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS services_fts USING fts5(
			name,
			description,
			content='services',
			content_rowid='rowid'
		)`,

		// ── 6 FTS sync triggers ──────────────────────────────────────
		// clients_fts: AFTER INSERT
		`CREATE TRIGGER IF NOT EXISTS clients_fts_ai AFTER INSERT ON clients BEGIN
			INSERT INTO clients_fts(rowid, name, preferences)
			VALUES (new.rowid, new.name, new.preferences);
		END`,

		// clients_fts: AFTER DELETE
		`CREATE TRIGGER IF NOT EXISTS clients_fts_ad AFTER DELETE ON clients BEGIN
			INSERT INTO clients_fts(clients_fts, rowid, name, preferences)
			VALUES ('delete', old.rowid, old.name, old.preferences);
		END`,

		// clients_fts: AFTER UPDATE
		`CREATE TRIGGER IF NOT EXISTS clients_fts_au AFTER UPDATE ON clients BEGIN
			INSERT INTO clients_fts(clients_fts, rowid, name, preferences)
			VALUES ('delete', old.rowid, old.name, old.preferences);
			INSERT INTO clients_fts(rowid, name, preferences)
			VALUES (new.rowid, new.name, new.preferences);
		END`,

		// services_fts: AFTER INSERT
		`CREATE TRIGGER IF NOT EXISTS services_fts_ai AFTER INSERT ON services BEGIN
			INSERT INTO services_fts(rowid, name, description)
			VALUES (new.rowid, new.name, new.description);
		END`,

		// services_fts: AFTER DELETE
		`CREATE TRIGGER IF NOT EXISTS services_fts_ad AFTER DELETE ON services BEGIN
			INSERT INTO services_fts(services_fts, rowid, name, description)
			VALUES ('delete', old.rowid, old.name, old.description);
		END`,

		// services_fts: AFTER UPDATE
		`CREATE TRIGGER IF NOT EXISTS services_fts_au AFTER UPDATE ON services BEGIN
			INSERT INTO services_fts(services_fts, rowid, name, description)
			VALUES ('delete', old.rowid, old.name, old.description);
			INSERT INTO services_fts(rowid, name, description)
			VALUES (new.rowid, new.name, new.description);
		END`,

		// ── 2 secondary indexes ──────────────────────────────────────
		// Note: business_hours_exception.exception_date and
		// schedules(professional_id, day_of_week) have UNIQUE table
		// constraints that create implicit indexes — no need for
		// redundant explicit UNIQUE indexes.
		`CREATE INDEX IF NOT EXISTS idx_bookings_overlap
			ON bookings(professional_id, start_datetime, end_datetime)`,

		`CREATE INDEX IF NOT EXISTS idx_pending_alerts_scheduled_status
			ON pending_alerts(scheduled_datetime, status)`,

		// ── Seed business_profile singleton (idempotent) ─────────────
		// name has no DEFAULT (per PRD §3.7.1); supply placeholder for seed.
		`INSERT OR IGNORE INTO business_profile (id, name) VALUES
			('singleton', 'Mi Negocio')`,

		// ── Seed schema_version v1 (idempotent) ──────────────────────
		`INSERT OR IGNORE INTO schema_version (version, description) VALUES
			(1, 'initial schema: 10 domain tables per PRD §3.7 + schema_version + 6 FTS sync triggers + 2 secondary indexes')`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec schema statement: %w", err)
		}
	}

	return nil
}
