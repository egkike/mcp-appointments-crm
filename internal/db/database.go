// Package db manages the SQLite database connection and schema lifecycle.
//
// The schema consists of 10 domain tables (per docs/PRD.md §3.7), 2 FTS5
// virtual tables, 6 FTS sync triggers, 4 secondary indexes, and a
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

// DB wraps a *sql.DB connection to the SQLite database.
type DB struct {
	Conn *sql.DB
}

// NewDatabase opens the SQLite database at dbPath, configures production
// pragmas (WAL, foreign_keys, busy_timeout), and runs initSchema.
func NewDatabase(ctx context.Context, dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := configurePragmas(ctx, conn); err != nil {
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

// configurePragmas sets the production pragmas on the connection.
func configurePragmas(ctx context.Context, conn *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := conn.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("apply pragma %q: %w", p, err)
		}
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
			name                        TEXT NOT NULL DEFAULT '',
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
			created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
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
			updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
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
			updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
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
			updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,

		// ── 8. pending_alerts ────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS pending_alerts (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			type                TEXT NOT NULL,
			message             TEXT NOT NULL,
			scheduled_datetime  TEXT NOT NULL,
			status              TEXT NOT NULL DEFAULT 'pending',
			related_booking_id  TEXT REFERENCES bookings(id),
			created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
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

		// ── 4 secondary indexes ──────────────────────────────────────
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_business_hours_exception_date
			ON business_hours_exception(exception_date)`,

		`CREATE UNIQUE INDEX IF NOT EXISTS idx_schedules_professional_day
			ON schedules(professional_id, day_of_week)`,

		`CREATE INDEX IF NOT EXISTS idx_bookings_overlap
			ON bookings(professional_id, start_datetime, end_datetime)`,

		`CREATE INDEX IF NOT EXISTS idx_pending_alerts_scheduled_status
			ON pending_alerts(scheduled_datetime, status)`,

		// ── Seed schema_version v1 (idempotent) ──────────────────────
		`INSERT OR IGNORE INTO schema_version (version, description) VALUES
			(1, 'initial schema: 10 domain tables per PRD §3.7 + schema_version + 6 FTS sync triggers + 4 secondary indexes')`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec schema statement: %w", err)
		}
	}

	return nil
}
