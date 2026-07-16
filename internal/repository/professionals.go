package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// ProfessionalsRepo provides CRUD operations for the professionals table.
// No hard-delete method is exposed; soft-delete via status='inactive' is the
// only sanctioned way to remove a professional from active use.
type ProfessionalsRepo struct {
	db *sql.DB
}

// NewProfessionalsRepo creates a new ProfessionalsRepo.
func NewProfessionalsRepo(db *sql.DB) *ProfessionalsRepo {
	return &ProfessionalsRepo{db: db}
}

// validateProfessional checks business-rule invariants for a professional.
func validateProfessional(p *model.Professional) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("el nombre no puede estar vacío: %w", ErrInvalidInput)
	}
	if p.Status != "active" && p.Status != "inactive" {
		return fmt.Errorf("el estado %q no es válido (debe ser 'active' o 'inactive'): %w", p.Status, ErrInvalidInput)
	}
	return nil
}

// Create inserts a new professional. The ID is auto-assigned as a UUID v4.
// If Status is empty, it defaults to "active".
// Returns ErrInvalidInput if name is empty or status is invalid.
// Returns ErrNotFound if a specialty references a non-existent service.
// Requires admin or owner role.
func (r *ProfessionalsRepo) Create(ctx context.Context, p *model.Professional) error {
	if _, err := requireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("crear profesional: %w", err)
	}

	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("crear profesional: el nombre no puede estar vacío: %w", ErrInvalidInput)
	}

	// Default status to "active" if not specified (per spec)
	if p.Status == "" {
		p.Status = "active"
	}

	if p.Status != "active" && p.Status != "inactive" {
		return fmt.Errorf("crear profesional: el estado %q no es válido (debe ser 'active' o 'inactive'): %w", p.Status, ErrInvalidInput)
	}

	// Validate specialties: each service_id must exist in the services table
	if p.Specialties != nil && *p.Specialties != "" {
		var serviceIDs []string
		if err := json.Unmarshal([]byte(*p.Specialties), &serviceIDs); err != nil {
			return fmt.Errorf("crear profesional: specialties debe ser un array JSON válido: %w", ErrInvalidInput)
		}
		for _, svcID := range serviceIDs {
			var count int
			err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM services WHERE id = ?`, svcID).Scan(&count)
			if err != nil {
				return fmt.Errorf("crear profesional: verificar servicio %s: %w", svcID, err)
			}
			if count == 0 {
				return fmt.Errorf("crear profesional: el servicio %s no existe: %w", svcID, ErrNotFound)
			}
		}
	}

	p.ID = model.NewUUID()

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO professionals (id, name, role_specialty, status, email, phone, specialties)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.RoleSpecialty, p.Status, p.Email, p.Phone, p.Specialties,
	)
	if err != nil {
		return fmt.Errorf("crear profesional: %w", err)
	}
	return nil
}

// Get returns a professional by ID. Returns ErrNotFound if not found.
// Staff callers can only retrieve their own professional record.
func (r *ProfessionalsRepo) Get(ctx context.Context, id string) (*model.Professional, error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtener profesional %s: %w", id, err)
	}
	// Staff restricted to their own row
	if caller.Role == auth.RoleStaff {
		if caller.ProfessionalID == nil || *caller.ProfessionalID != id {
			return nil, &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "no tiene permiso para ver este profesional",
				Cause:   ErrUnauthenticated,
			}
		}
	}

	p := &model.Professional{}
	err = r.db.QueryRowContext(ctx,
		`SELECT id, name, role_specialty, status, email, phone, specialties, created_at, updated_at
		 FROM professionals WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.RoleSpecialty, &p.Status, &p.Email, &p.Phone,
		&p.Specialties, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener profesional %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("obtener profesional %s: %w", id, err)
	}
	return p, nil
}

// GetActive returns all professionals with status='active', ordered by name.
// Staff callers see only their own professional record.
func (r *ProfessionalsRepo) GetActive(ctx context.Context) ([]*model.Professional, error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar profesionales activos: %w", err)
	}

	var rows *sql.Rows
	if caller.Role == auth.RoleStaff && caller.ProfessionalID != nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT id, name, role_specialty, status, email, phone, specialties, created_at, updated_at
			 FROM professionals WHERE status = 'active' AND id = ? ORDER BY name`, *caller.ProfessionalID)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT id, name, role_specialty, status, email, phone, specialties, created_at, updated_at
			 FROM professionals WHERE status = 'active' ORDER BY name`)
	}
	if err != nil {
		return nil, fmt.Errorf("listar profesionales activos: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	var professionals []*model.Professional
	for rows.Next() {
		p := &model.Professional{}
		if err := rows.Scan(&p.ID, &p.Name, &p.RoleSpecialty, &p.Status, &p.Email,
			&p.Phone, &p.Specialties, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("listar profesionales activos: escaneo: %w", err)
		}
		professionals = append(professionals, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar profesionales activos: iteración: %w", err)
	}
	return professionals, nil
}

// Update updates an existing professional. Returns ErrInvalidInput for invalid
// fields, ErrNotFound if no row matches or if a specialty references a non-existent service.
// Requires admin or owner role.
func (r *ProfessionalsRepo) Update(ctx context.Context, p *model.Professional) error {
	if _, err := requireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("actualizar profesional: %w", err)
	}

	if err := validateProfessional(p); err != nil {
		return fmt.Errorf("actualizar profesional: %w", err)
	}

	// Validate specialties: each service_id must exist in the services table
	if p.Specialties != nil && *p.Specialties != "" {
		var serviceIDs []string
		if err := json.Unmarshal([]byte(*p.Specialties), &serviceIDs); err != nil {
			return fmt.Errorf("actualizar profesional: specialties debe ser un array JSON válido: %w", ErrInvalidInput)
		}
		for _, svcID := range serviceIDs {
			var count int
			err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM services WHERE id = ?`, svcID).Scan(&count)
			if err != nil {
				return fmt.Errorf("actualizar profesional: verificar servicio %s: %w", svcID, err)
			}
			if count == 0 {
				return fmt.Errorf("actualizar profesional: el servicio %s no existe: %w", svcID, ErrNotFound)
			}
		}
	}

	// strftime format must match storageTimeLayout in datetime.go.
	result, err := r.db.ExecContext(ctx,
		`UPDATE professionals SET name=?, role_specialty=?, status=?, email=?, phone=?,
		 specialties=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id=?`,
		p.Name, p.RoleSpecialty, p.Status, p.Email, p.Phone, p.Specialties, p.ID,
	)
	if err != nil {
		return fmt.Errorf("actualizar profesional: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar profesional: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("actualizar profesional: %w", ErrNotFound)
	}
	return nil
}
