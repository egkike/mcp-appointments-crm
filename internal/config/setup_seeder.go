package config

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/idgen"
)

// SeedOnBoot seeds the database from the wizard setup JSONs on first boot.
// It returns nil (and logs an info message) when the database is already
// configured, so subsequent boots are no-ops. When the database is fresh and
// any setup file is missing or malformed, it returns a semantic Spanish error
// that aborts startup.
func SeedOnBoot(ctx context.Context, conn *sql.DB, logger *slog.Logger) error {
	fresh, err := isFreshDB(ctx, conn)
	if err != nil {
		return err
	}
	if !fresh {
		logger.Info("importación de setup omitida: la base de datos ya está configurada")
		return nil
	}

	dir, err := ResolveSetupDir()
	if err != nil {
		return err
	}

	data, err := LoadSetup(dir)
	if err != nil {
		return err
	}

	if err := validateForSeed(data); err != nil {
		return err
	}

	if err := seed(ctx, conn, data); err != nil {
		return err
	}

	logger.Info("base de datos sembrada desde la configuración inicial",
		"profesionales", len(data.Staff),
		"servicios", len(data.Services))
	return nil
}

// isFreshDB mirrors the repository's lazy-init: it guarantees the singleton
// business_profile row exists and reports whether its name is still the empty
// placeholder. A name other than "" means the database has already been seeded.
func isFreshDB(ctx context.Context, conn *sql.DB) (bool, error) {
	if _, err := conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO business_profile (id, name) VALUES (?, ?)`,
		"singleton", ""); err != nil {
		return false, fmt.Errorf("verificar perfil del negocio: %w", err)
	}

	var name string
	if err := conn.QueryRowContext(ctx,
		`SELECT name FROM business_profile WHERE id = ?`, "singleton").Scan(&name); err != nil {
		return false, fmt.Errorf("verificar perfil del negocio: %w", err)
	}
	return name == "", nil
}

// seed writes the setup data into the database inside a single transaction.
// Any failure rolls back all writes and leaves the database in its pre-seed
// state. All SQL uses parameterized placeholders.
func seed(ctx context.Context, conn *sql.DB, data *SetupData) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar transacción de setup: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	mappedHours, err := mapBusinessHours(data.Business.BusinessHours)
	if err != nil {
		return err
	}

	paymentMethodsJSON, err := stringSliceToJSON(data.Business.AcceptedPaymentMethods)
	if err != nil {
		return fmt.Errorf("serializar métodos de pago: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE business_profile SET
			name = ?,
			industry = ?,
			country = ?,
			address = ?,
			latitude = ?,
			longitude = ?,
			cover_photo_url = ?,
			public_phone = ?,
			messenger_platform = ?,
			messenger_id = ?,
			contact_email = ?,
			website_url = ?,
			general_description = ?,
			accepted_payment_methods = ?,
			currency_code = ?,
			currency_symbol = ?,
			timezone = ?,
			slot_interval_minutes = ?,
			business_hours = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		data.Business.Name,
		data.Business.Industry,
		data.Business.Country,
		data.Business.Address,
		data.Business.Latitude,
		data.Business.Longitude,
		data.Business.CoverPhotoURL,
		data.Business.PublicPhone,
		data.Business.MessengerPlatform,
		data.Business.MessengerID,
		data.Business.ContactEmail,
		data.Business.WebsiteURL,
		data.Business.GeneralDescription,
		paymentMethodsJSON,
		data.Business.CurrencyCode,
		data.Business.CurrencySymbol,
		data.Business.Timezone,
		data.Business.SlotIntervalMinutes,
		mappedHours,
		"singleton")
	if err != nil {
		return fmt.Errorf("actualizar perfil del negocio: %w", translateConstraint(err))
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar perfil del negocio: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("actualizar perfil del negocio: no se encontró la fila singleton")
	}

	for _, m := range data.Staff {
		profID := idgen.NewUUID()
		specialtiesJSON, err := stringSliceToJSON(m.Specialties)
		if err != nil {
			return fmt.Errorf("serializar especialidades de %q: %w", m.Name, err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO professionals
				(id, name, role_specialty, status, email, phone, specialties)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			profID, m.Name, m.RoleSpecialty, statusOr(m.Status), m.Email, m.Phone, specialtiesJSON); err != nil {
			return fmt.Errorf("insertar profesional %q: %w", m.Name, translateConstraint(err))
		}

		for _, s := range m.Schedule {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO schedules
					(professional_id, day_of_week, start_time, end_time)
				VALUES (?, ?, ?, ?)`,
				profID, s.DayOfWeek, s.StartTime, s.EndTime); err != nil {
				return fmt.Errorf("insertar horario del profesional %q (día %d): %w",
					m.Name, s.DayOfWeek, translateConstraint(err))
			}
		}
	}

	for _, svc := range data.Services {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO services
				(id, name, description, duration_minutes, price, is_active)
			VALUES (?, ?, ?, ?, ?, ?)`,
			idgen.NewUUID(), svc.Name, svc.Description,
			svc.DurationMinutes, svc.Price, svc.IsActive); err != nil {
			return fmt.Errorf("insertar servicio %q: %w", svc.Name, translateConstraint(err))
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmar transacción de setup: %w", err)
	}
	committed = true
	return nil
}

// translateConstraint maps SQLite constraint violations to semantic Spanish
// messages. Unrecognized errors are returned unchanged so higher levels can
// wrap them with their own context.
func translateConstraint(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed: schedules.professional_id, schedules.day_of_week") {
		return fmt.Errorf("ya existe un horario para ese día: %w", err)
	}
	return err
}
