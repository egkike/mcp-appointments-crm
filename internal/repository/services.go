package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// Compile-time interface conformance check.
var _ domainrepo.ServicesRepo = (*ServicesRepo)(nil)

// ServicesRepo provides CRUD and FTS5 search for the services table.
type ServicesRepo struct {
	db *sql.DB
}

// NewServicesRepo creates a new ServicesRepo.
func NewServicesRepo(db *sql.DB) *ServicesRepo {
	return &ServicesRepo{db: db}
}

// Save inserts a new service. Returns domain.ErrInvalidInput if duration_minutes <= 0,
// name is empty, or price is zero or negative.
func (r *ServicesRepo) Save(ctx context.Context, s *entity.Service) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("crear servicio: %w", err)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO services (id, name, description, duration_minutes, price, is_active)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Description, s.DurationMinutes, s.Price, s.Active,
	)
	if err != nil {
		return fmt.Errorf("crear servicio: %w", err)
	}
	return nil
}

// FindByID returns a service by ID. Returns domain.ErrNotFound if not found.
func (r *ServicesRepo) FindByID(ctx context.Context, id string) (*entity.Service, error) {
	s := &entity.Service{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, duration_minutes, price, is_active, created_at, updated_at
		 FROM services WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Description, &s.DurationMinutes, &s.Price,
		&s.Active, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener servicio %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("obtener servicio %s: %w", id, err)
	}
	return s, nil
}

// FindActive returns all services with is_active=1, ordered by name.
func (r *ServicesRepo) FindActive(ctx context.Context) ([]*entity.Service, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, duration_minutes, price, is_active, created_at, updated_at
		 FROM services WHERE is_active = 1 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listar servicios activos: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	var services []*entity.Service
	for rows.Next() {
		s := &entity.Service{}
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.DurationMinutes,
			&s.Price, &s.Active, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("listar servicios activos: escaneo: %w", err)
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar servicios activos: iteración: %w", err)
	}
	return services, nil
}

// Update updates an existing service. Returns domain.ErrInvalidInput for invalid
// fields, domain.ErrNotFound if no row matches.
func (r *ServicesRepo) Update(ctx context.Context, s *entity.Service) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("actualizar servicio: %w", err)
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE services SET name=?, description=?, duration_minutes=?, price=?,
		 is_active=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id=?`,
		s.Name, s.Description, s.DurationMinutes, s.Price, s.Active, s.ID,
	)
	if err != nil {
		return fmt.Errorf("actualizar servicio: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar servicio: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("actualizar servicio: %w", domain.ErrNotFound)
	}
	return nil
}

// Delete removes a service by ID. Returns domain.ErrNotFound if no row matches.
func (r *ServicesRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM services WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("eliminar servicio: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("eliminar servicio: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("eliminar servicio: %w", domain.ErrNotFound)
	}
	return nil
}

// SearchFTS performs a full-text search on services using FTS5 MATCH.
// Results are ordered by FTS5 rank (most relevant first).
// Returns domain.ErrInvalidInput if the query contains FTS5 operator characters.
func (r *ServicesRepo) SearchFTS(ctx context.Context, query string) ([]*model.Service, error) {
	if err := validateFTSQuery(query); err != nil {
		return nil, fmt.Errorf("buscar servicios: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT s.id, s.name, s.description, s.duration_minutes, s.price,
			s.is_active, s.created_at, s.updated_at
		 FROM services s
		 JOIN services_fts f ON s.rowid = f.rowid
		 WHERE f MATCH ?
		 ORDER BY bm25(f)`,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf("buscar servicios: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	var services []*model.Service
	for rows.Next() {
		s := &model.Service{}
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.DurationMinutes,
			&s.Price, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("buscar servicios: escaneo: %w", err)
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buscar servicios: iteración: %w", err)
	}
	return services, nil
}
