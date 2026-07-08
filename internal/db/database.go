package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB Wrapper para centralizar las operaciones de la base de datos
type DB struct {
	Conn *sql.DB
}

// NewDatabase inicializa la conexión, activa WAL y ejecuta las migraciones base
func NewDatabase(ctx context.Context, dbPath string) (*DB, error) {
	// Asegurar que el directorio de la BD exista
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("error al crear directorio de BD: %w", err)
	}

	// Abrir conexión usando el driver puro de Go (modernc)
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error al abrir sqlite: %w", err)
	}

	// Configurar optimizaciones críticas (Pragmas)
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",    // Forzar integridad referencial
		"PRAGMA journal_mode = WAL;",   // Modo WAL para alta concurrencia
		"PRAGMA synchronous = NORMAL;", // Balance óptimo entre seguridad y velocidad
		"PRAGMA busy_timeout = 5000;",  // Esperar hasta 5s si la BD está bloqueada
	}

	for _, pragma := range pragmas {
		if _, err := conn.ExecContext(ctx, pragma); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("error aplicando pragma (%s): %w", pragma, err)
		}
	}

	db := &DB{Conn: conn}

	// Ejecutar la creación de tablas
	if err := db.initSchema(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return db, nil
}

// Close cierra la conexión de manera segura
func (db *DB) Close() error {
	if db.Conn != nil {
		return db.Conn.Close()
	}
	return nil
}

// initSchema define las tablas del sistema e inicializa FTS5
func (db *DB) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS business_profile (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		slots_config TEXT NOT NULL, -- Guardado como JSON (ej: días, horas, duración)
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		phone TEXT UNIQUE NOT NULL,
		email TEXT,
		messenger_platform TEXT, -- 'whatsapp', 'telegram', etc.
		messenger_id TEXT,       -- ID único de la plataforma de mensajería
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		duration_mins INTEGER NOT NULL,
		price REAL NOT NULL,
		is_active INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS appointments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id INTEGER NOT NULL,
		service_id INTEGER NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME NOT NULL,
		status TEXT DEFAULT 'pending', -- 'pending', 'confirmed', 'cancelled'
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
		FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE RESTRICT
	);

	-- Tablas Virtuales FTS5 para búsquedas instantáneas
	CREATE VIRTUAL TABLE IF NOT EXISTS clients_fts USING fts5(
		name,
		phone,
		content='clients',
		content_rowid='id'
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS services_fts USING fts5(
		name,
		content='services',
		content_rowid='id'
	);

	-- Authorization: accounts whitelist (owner/admin/staff) per PRD §3.8.2
	CREATE TABLE IF NOT EXISTS accounts (
		id              TEXT PRIMARY KEY,
		role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'staff')),
		display_name    TEXT,
		professional_id TEXT,
		is_active       INTEGER NOT NULL DEFAULT 1,
		created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		CHECK ((role = 'staff' AND professional_id IS NOT NULL) OR (role IN ('admin', 'owner')))
	);

	-- Single-owner invariant: reject INSERT of second active owner
	CREATE TRIGGER IF NOT EXISTS accounts_single_owner_insert BEFORE INSERT ON accounts
	WHEN NEW.role = 'owner' AND NEW.is_active = 1
	 AND (SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1) >= 1
	BEGIN SELECT RAISE(ABORT, 'single-owner invariant: only one active owner allowed'); END;

	-- Single-owner invariant: reject UPDATE that would create second active owner
	-- Covers: (a) activating an inactive owner, (b) changing role to owner on active row.
	-- id != NEW.id prevents self-rejection when updating an existing owner without changing status.
	CREATE TRIGGER IF NOT EXISTS accounts_single_owner_update BEFORE UPDATE ON accounts
	WHEN NEW.role = 'owner' AND NEW.is_active = 1
	 AND (OLD.role != 'owner' OR OLD.is_active = 0)
	 AND (SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1 AND id != NEW.id) >= 1
	BEGIN SELECT RAISE(ABORT, 'single-owner invariant: only one active owner allowed'); END;
	`

	if _, err := db.Conn.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("error al inicializar el esquema de tablas: %w", err)
	}

	return nil
}
