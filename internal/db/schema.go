package db

func domainTableDDL() []string {
	return []string{
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

		`CREATE TABLE IF NOT EXISTS schedules (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			professional_id     TEXT NOT NULL REFERENCES professionals(id) ON DELETE CASCADE,
			day_of_week         INTEGER NOT NULL,
			start_time          TEXT NOT NULL,
			end_time            TEXT NOT NULL,
			UNIQUE(professional_id, day_of_week),
			CHECK (day_of_week BETWEEN 0 AND 6),
			CHECK (start_time < end_time)
		)`,

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

		`CREATE TABLE IF NOT EXISTS clients (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			phone        TEXT NOT NULL UNIQUE,
			email        TEXT,
			preferences  TEXT,
			created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,

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
			CHECK (status IN ('pending', 'confirmed', 'cancelled')),
			CHECK (start_datetime < end_datetime)
		)`,

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

		`CREATE TABLE IF NOT EXISTS schema_version (
			version      INTEGER PRIMARY KEY,
			applied_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			description  TEXT
		)`,
	}
}

func ftsTableDDL() []string {
	return []string{
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
	}
}

func ftsTriggerDDL() []string {
	return []string{
		`CREATE TRIGGER IF NOT EXISTS clients_fts_ai AFTER INSERT ON clients BEGIN
			INSERT INTO clients_fts(rowid, name, preferences)
			VALUES (new.rowid, new.name, new.preferences);
		END`,

		`CREATE TRIGGER IF NOT EXISTS clients_fts_ad AFTER DELETE ON clients BEGIN
			INSERT INTO clients_fts(clients_fts, rowid, name, preferences)
			VALUES ('delete', old.rowid, old.name, old.preferences);
		END`,

		`CREATE TRIGGER IF NOT EXISTS clients_fts_au AFTER UPDATE ON clients BEGIN
			INSERT INTO clients_fts(clients_fts, rowid, name, preferences)
			VALUES ('delete', old.rowid, old.name, old.preferences);
			INSERT INTO clients_fts(rowid, name, preferences)
			VALUES (new.rowid, new.name, new.preferences);
		END`,

		`CREATE TRIGGER IF NOT EXISTS services_fts_ai AFTER INSERT ON services BEGIN
			INSERT INTO services_fts(rowid, name, description)
			VALUES (new.rowid, new.name, new.description);
		END`,

		`CREATE TRIGGER IF NOT EXISTS services_fts_ad AFTER DELETE ON services BEGIN
			INSERT INTO services_fts(services_fts, rowid, name, description)
			VALUES ('delete', old.rowid, old.name, old.description);
		END`,

		`CREATE TRIGGER IF NOT EXISTS services_fts_au AFTER UPDATE ON services BEGIN
			INSERT INTO services_fts(services_fts, rowid, name, description)
			VALUES ('delete', old.rowid, old.name, old.description);
			INSERT INTO services_fts(rowid, name, description)
			VALUES (new.rowid, new.name, new.description);
		END`,
	}
}

func secondaryIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_bookings_overlap
			ON bookings(professional_id, start_datetime, end_datetime)`,

		`CREATE INDEX IF NOT EXISTS idx_pending_alerts_scheduled_status
			ON pending_alerts(scheduled_datetime, status)`,
	}
}

func seedDDL() []string {
	return []string{
		`INSERT OR IGNORE INTO schema_version (version, description) VALUES
			(1, 'initial schema: 8 domain tables per PRD §3.7 + schema_version + 6 FTS sync triggers + 4 secondary indexes')`,
	}
}
