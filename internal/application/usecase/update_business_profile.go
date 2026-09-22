package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// UpdateBusinessProfileUseCase applies a partial merge to the singleton
// business profile: only the fields carried by the input are overwritten.
// Access is restricted to the owner role (ADR-0015 Decision 2).
type UpdateBusinessProfileUseCase struct {
	profiles domainrepo.BusinessProfileRepo
	logger   *slog.Logger
}

// NewUpdateBusinessProfileUseCase constructs the use case. A nil logger falls
// back to slog.Default() (see maintenanceLogger).
func NewUpdateBusinessProfileUseCase(profiles domainrepo.BusinessProfileRepo, logger *slog.Logger) *UpdateBusinessProfileUseCase {
	return &UpdateBusinessProfileUseCase{profiles: profiles, logger: maintenanceLogger(logger)}
}

// Execute merges the provided fields onto the stored profile and persists it.
//
// The returned value is the merged entity — the same read shape
// get_business_profile exposes (that use case also returns the entity), so the
// transport layer maps the profile to JSON in exactly one place.
//
// Business rules (business_hours JSON, payment-method JSON, timezone, and the
// messenger platform enum) are enforced by entity.BusinessProfile.Validate()
// inside the repository; this use case only assembles the merged entity.
func (uc *UpdateBusinessProfileUseCase) Execute(ctx context.Context, input dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	profile, err := uc.profiles.Get(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el perfil del negocio no existe", Cause: err}
		}
		return nil, fmt.Errorf("update_business_profile: %w", err)
	}

	if !hasProfileUpdates(input) {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "no se proporcionaron campos para actualizar"}
	}

	applyProfileUpdates(profile, input)

	if err := uc.profiles.Update(ctx, profile); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "los datos del perfil no son válidos: revisá los horarios, la zona horaria, los métodos de pago y la plataforma de mensajería",
				Cause:   err,
			}
		case errors.Is(err, domain.ErrNotFound):
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el perfil del negocio no existe", Cause: err}
		case errors.Is(err, domain.ErrConflict):
			return nil, &domain.SemanticError{Code: domain.ErrCodeConflict, Message: "conflicto al actualizar el perfil del negocio", Cause: err}
		}
		return nil, fmt.Errorf("update_business_profile: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "update_business_profile", "business_profile", profile.ID, "update")
	return profile, nil
}

// hasProfileUpdates reports whether the partial-merge payload carries at least
// one updatable field. Identity and read-only columns (id, created_at,
// updated_at) are not part of the input by design.
func hasProfileUpdates(in dto.UpdateBusinessProfileInput) bool {
	return in.Name != nil ||
		in.Industry != nil ||
		in.Country != nil ||
		in.Address != nil ||
		in.Latitude != nil ||
		in.Longitude != nil ||
		in.CoverPhotoURL != nil ||
		in.PublicPhone != nil ||
		in.MessengerPlatform != nil ||
		in.MessengerID != nil ||
		in.ContactEmail != nil ||
		in.WebsiteURL != nil ||
		in.GeneralDescription != nil ||
		in.CurrencyCode != nil ||
		in.CurrencySymbol != nil ||
		in.AcceptedPaymentMethods != nil ||
		in.Timezone != nil ||
		in.SlotIntervalMinutes != nil ||
		in.BusinessHours != nil
}

// applyProfileUpdates merges the fields present in the input onto the stored
// profile: the partial-merge rule is exactly "nil keeps the stored value", and
// only explicit non-nil fields are overwritten. It is the composition entry for
// the two semantics-grouped helpers below; their relative order is irrelevant
// because the two field sets are disjoint.
func applyProfileUpdates(p *entity.BusinessProfile, in dto.UpdateBusinessProfileInput) {
	applyProfileScalarUpdates(p, in)
	applyProfileReferenceUpdates(p, in)
}

// applyProfileScalarUpdates applies the partial merge to the scalar columns
// (Name, CurrencyCode, CurrencySymbol, Timezone, SlotIntervalMinutes,
// BusinessHours), where the entity stores the value and the DTO carries a
// pointer: a non-nil pointer is dereferenced and assigned, a nil pointer keeps
// the stored value. Its fields are disjoint from
// applyProfileReferenceUpdates, so calling either helper alone is safe.
func applyProfileScalarUpdates(p *entity.BusinessProfile, in dto.UpdateBusinessProfileInput) {
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.CurrencyCode != nil {
		p.CurrencyCode = *in.CurrencyCode
	}
	if in.CurrencySymbol != nil {
		p.CurrencySymbol = *in.CurrencySymbol
	}
	if in.Timezone != nil {
		p.Timezone = *in.Timezone
	}
	if in.SlotIntervalMinutes != nil {
		p.SlotIntervalMinutes = *in.SlotIntervalMinutes
	}
	if in.BusinessHours != nil {
		p.BusinessHours = *in.BusinessHours
	}
}

// applyProfileReferenceUpdates applies the partial merge to the pointer columns
// (Industry, Country, Address, Latitude, Longitude, CoverPhotoURL, PublicPhone,
// MessengerPlatform, MessengerID, ContactEmail, WebsiteURL,
// GeneralDescription, AcceptedPaymentMethods), which share the same type on the
// entity and the DTO: a non-nil pointer is copied verbatim (pointer and pointee
// alike), a nil pointer keeps the stored reference. Its fields are disjoint from
// applyProfileScalarUpdates, so calling either helper alone is safe.
func applyProfileReferenceUpdates(p *entity.BusinessProfile, in dto.UpdateBusinessProfileInput) {
	if in.Industry != nil {
		p.Industry = in.Industry
	}
	if in.Country != nil {
		p.Country = in.Country
	}
	if in.Address != nil {
		p.Address = in.Address
	}
	if in.Latitude != nil {
		p.Latitude = in.Latitude
	}
	if in.Longitude != nil {
		p.Longitude = in.Longitude
	}
	if in.CoverPhotoURL != nil {
		p.CoverPhotoURL = in.CoverPhotoURL
	}
	if in.PublicPhone != nil {
		p.PublicPhone = in.PublicPhone
	}
	if in.MessengerPlatform != nil {
		p.MessengerPlatform = in.MessengerPlatform
	}
	if in.MessengerID != nil {
		p.MessengerID = in.MessengerID
	}
	if in.ContactEmail != nil {
		p.ContactEmail = in.ContactEmail
	}
	if in.WebsiteURL != nil {
		p.WebsiteURL = in.WebsiteURL
	}
	if in.GeneralDescription != nil {
		p.GeneralDescription = in.GeneralDescription
	}
	if in.AcceptedPaymentMethods != nil {
		p.AcceptedPaymentMethods = in.AcceptedPaymentMethods
	}
}
