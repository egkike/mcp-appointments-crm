package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// maxServiceIDLen bounds the service_id filter. Service ids are UUIDs (36
// characters, idgen.NewUUID) or short seed slugs, so 64 is generous; the bound
// exists because every filter value is untrusted input and must be length-bounded
// at the boundary (AGENTS.md input validation), independently of the fact that it
// travels as a SQL placeholder and is never concatenated.
const maxServiceIDLen = 64

// SearchQueryValidator is the narrow consumer-side contract for query_text
// validation. The composition root injects the concrete adapter
// (repository.QueryValidator), which delegates to the shared FTS5 query
// validator (validateFTSQuery). The application layer must not import
// internal/repository (ADR-0013), so the contract lives here and the adapter is
// wired at the composition root: one text policy across the *_advanced tools,
// zero duplicated rules.
type SearchQueryValidator interface {
	Validate(query string) error
}

// SearchProfessionalsAdvancedUseCase reads the active professionals catalog with
// an optional service filter and an optional text query (feat-owner-chat S7a).
//
// Role: ANY authenticated caller (client, staff, admin, owner). This is a
// deliberate difference from SearchServicesAdvancedUseCase, which requires
// owner/admin: the conversational booking flow belongs to the client, and without
// this read the agent cannot resolve which professionals offer a service — the
// defect observed in the smoke (Hermes offered a hairdresser for a wash with all
// professionals listed).
type SearchProfessionalsAdvancedUseCase struct {
	professionals domainrepo.ProfessionalsRepo
	services      domainrepo.ServicesRepo
	validator     SearchQueryValidator
}

// NewSearchProfessionalsAdvancedUseCase constructs the use case. validator is
// mandatory when a query_text is supplied (see Execute): it is the shared text
// policy, not an optional extra.
func NewSearchProfessionalsAdvancedUseCase(
	professionals domainrepo.ProfessionalsRepo,
	services domainrepo.ServicesRepo,
	validator SearchQueryValidator,
) *SearchProfessionalsAdvancedUseCase {
	return &SearchProfessionalsAdvancedUseCase{professionals: professionals, services: services, validator: validator}
}

// Execute returns the active professionals matching the filters.
//
// Semantics (design §8.2):
//   - service_id present: the service must exist (semantic NOT_FOUND otherwise)
//     and each professional must declare it through entity.Professional.
//     HasSpecialty. Fail-closed: an empty, absent or unreadable specialties
//     value excludes the professional. There is NO fallback to the full catalog —
//     that fallback would reintroduce the very bug S7a fixes.
//   - query_text present: case-insensitive substring match over name and
//     role_specialty. role_specialty is free text and never a filter key.
//   - neither: the whole active catalog.
//
// The result carrier is PII-free by construction (dto.ProfessionalSearchEntry
// has no email/phone fields): the tool is readable by clients.
func (uc *SearchProfessionalsAdvancedUseCase) Execute(ctx context.Context, input dto.SearchProfessionalsAdvancedInput) (*dto.SearchProfessionalsAdvancedResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	ctx = auth.WithCaller(ctx, input.Caller)

	query := strings.TrimSpace(input.QueryText)
	serviceID := strings.TrimSpace(input.ServiceID)

	if query != "" {
		if uc.validator == nil {
			// Fail secure: never run an unvalidated query. The composition root
			// always injects repository.QueryValidator.
			return nil, fmt.Errorf("search_professionals_advanced: validador de consultas no configurado: %w", domain.ErrInvalidInput)
		}
		if err := uc.validator.Validate(query); err != nil {
			return nil, err
		}
	}

	if serviceID != "" {
		if utf8.RuneCountInString(serviceID) > maxServiceIDLen {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el identificador del servicio no es válido.",
				Cause:   fmt.Errorf("service_id excede %d caracteres: %w", maxServiceIDLen, domain.ErrInvalidInput),
			}
		}
		if _, err := uc.services.FindByID(ctx, serviceID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, &domain.SemanticError{
					Code:    domain.ErrCodeNotFound,
					Message: "el servicio indicado no existe",
					Cause:   err,
				}
			}
			return nil, fmt.Errorf("search_professionals_advanced: %w", err)
		}
	}

	professionals, err := uc.professionals.FindAllActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("search_professionals_advanced: %w", err)
	}

	results := make([]dto.ProfessionalSearchEntry, 0, len(professionals))
	for _, p := range professionals {
		if p == nil || !p.IsActive() {
			continue
		}
		if serviceID != "" && !p.HasSpecialty(serviceID) {
			continue
		}
		if query != "" && !matchesProfessionalText(p, query) {
			continue
		}
		results = append(results, dto.ProfessionalSearchEntry{
			ID:            p.ID,
			Name:          p.Name,
			RoleSpecialty: p.RoleSpecialty,
			Specialties:   professionalSpecialtyIDs(p),
		})
	}
	return &dto.SearchProfessionalsAdvancedResult{Results: results}, nil
}

// matchesProfessionalText reports whether query (already trimmed, non-empty)
// appears as a case-insensitive substring of the professional's name or free-text
// role_specialty. role_specialty is descriptive metadata here, never a filter key
// (design §8.2).
func matchesProfessionalText(p *entity.Professional, query string) bool {
	needle := strings.ToLower(query)
	if strings.Contains(strings.ToLower(p.Name), needle) {
		return true
	}
	return p.RoleSpecialty != nil && strings.Contains(strings.ToLower(*p.RoleSpecialty), needle)
}

// professionalSpecialtyIDs decodes the JSON-encoded service IDs stored in
// professionals.specialties. nil, empty, JSON null and invalid values all map to
// an empty (non-nil) list, mirroring entity.Professional.HasSpecialty's
// fail-safe rule so the wire always renders an array.
func professionalSpecialtyIDs(p *entity.Professional) []string {
	ids := []string{}
	if p.Specialties == nil || *p.Specialties == "" {
		return ids
	}
	if err := json.Unmarshal([]byte(*p.Specialties), &ids); err != nil {
		return []string{}
	}
	if ids == nil {
		return []string{}
	}
	return ids
}
