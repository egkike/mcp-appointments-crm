package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// ServicesRepo provides CRUD and FTS5 search for the services table.
type ServicesRepo struct {
	db *sql.DB
}

// NewServicesRepo creates a new ServicesRepo.
func NewServicesRepo(db *sql.DB) *ServicesRepo {
	return &ServicesRepo{db: db}
}

// Create inserts a new service. Returns ErrInvalidInput if duration_minutes <= 0.
func (r *ServicesRepo) Create(ctx context.Context, s *model.Service) error {
	if s.DurationMinutes <= 0 {
		return fmt.Errorf("create service: duration_minutes must be positive: %w", ErrInvalidInput)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO services (id, name, description, duration_minutes, price, is_active)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Description, s.DurationMinutes, s.Price, s.IsActive,
	)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	return nil
}

// Get returns a service by ID. Returns ErrNotFound if not found.
func (r *ServicesRepo) Get(ctx context.Context, id string) (*model.Service, error) {
	s := &model.Service{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, duration_minutes, price, is_active, created_at, updated_at
		 FROM services WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Description, &s.DurationMinutes, &s.Price,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get service %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("get service %s: %w", id, err)
	}
	return s, nil
}

// ListActive returns all services with is_active=1, ordered by name.
func (r *ServicesRepo) ListActive(ctx context.Context) ([]*model.Service, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, duration_minutes, price, is_active, created_at, updated_at
		 FROM services WHERE is_active = 1 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list active services: %w", err)
	}
	defer rows.Close()

	var services []*model.Service
	for rows.Next() {
		s := &model.Service{}
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.DurationMinutes,
			&s.Price, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list active services: scan: %w", err)
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active services: rows: %w", err)
	}
	return services, nil
}

// Update updates an existing service. Returns ErrNotFound if no row matches.
func (r *ServicesRepo) Update(ctx context.Context, s *model.Service) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE services SET name=?, description=?, duration_minutes=?, price=?,
		 is_active=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id=?`,
		s.Name, s.Description, s.DurationMinutes, s.Price, s.IsActive, s.ID,
	)
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update service rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("update service: %w", ErrNotFound)
	}
	return nil
}

// Delete removes a service by ID. Returns ErrNotFound if no row matches.
func (r *ServicesRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM services WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete service rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("delete service: %w", ErrNotFound)
	}
	return nil
}

// ftsQueryRe matches allowed FTS5 query characters: alphanumeric, spaces, and hyphens.
var ftsQueryRe = regexp.MustCompile(`[^a-zA-Z0-9\s\-]`)

// SearchFTS performs a full-text search on services using FTS5 MATCH.
// Results are ordered by FTS5 rank (most relevant first).
// Returns ErrInvalidInput if the query contains FTS5 operator characters.
func (r *ServicesRepo) SearchFTS(ctx context.Context, query string) ([]*model.Service, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("search services FTS: empty query: %w", ErrInvalidInput)
	}
	if ftsQueryRe.MatchString(query) {
		return nil, fmt.Errorf("search services FTS: query contains forbidden characters: %w", ErrInvalidInput)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT s.id, s.name, s.description, s.duration_minutes, s.price,
			s.is_active, s.created_at, s.updated_at
		 FROM services s
		 JOIN services_fts f ON s.rowid = f.rowid
		 WHERE services_fts MATCH ?
		 ORDER BY bm25(services_fts)`,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf("search services FTS: %w", err)
	}
	defer rows.Close()

	var services []*model.Service
	for rows.Next() {
		s := &model.Service{}
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.DurationMinutes,
			&s.Price, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("search services FTS: scan: %w", err)
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search services FTS: rows: %w", err)
	}
	return services, nil
}
