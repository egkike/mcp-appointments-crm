package mcp

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getBusinessProfileIn is the input of get_business_profile: the profile is a
// singleton, so the tool takes no arguments. The SDK infers
// {"type":"object","additionalProperties":false} from an empty struct, so any
// non-empty argument object is rejected with -32602 before the handler runs
// (REQ-MT-015 input contract is exactly {}; behavior pinned by
// TestToolGetBusinessProfileRejectsArguments).
type getBusinessProfileIn struct{}

// businessProfileOut is the pinned output contract of get_business_profile
// (REQ-MT-015, JD fix B-3): snake_case JSON keys matching the spec key list
// exactly, independent of entity.BusinessProfile's Go field names (which have
// no json tags and would serialize as PascalCase). Optional entity fields map
// to omitempty pointers so absent values stay absent in the JSON output.
type businessProfileOut struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Industry               *string  `json:"industry,omitempty"`
	Country                *string  `json:"country,omitempty"`
	Address                *string  `json:"address,omitempty"`
	Latitude               *float64 `json:"latitude,omitempty"`
	Longitude              *float64 `json:"longitude,omitempty"`
	CoverPhotoURL          *string  `json:"cover_photo_url,omitempty"`
	PublicPhone            *string  `json:"public_phone,omitempty"`
	MessengerPlatform      *string  `json:"messenger_platform,omitempty"`
	MessengerID            *string  `json:"messenger_id,omitempty"`
	ContactEmail           *string  `json:"contact_email,omitempty"`
	WebsiteURL             *string  `json:"website_url,omitempty"`
	GeneralDescription     *string  `json:"general_description,omitempty"`
	CurrencyCode           string   `json:"currency_code"`
	CurrencySymbol         string   `json:"currency_symbol"`
	AcceptedPaymentMethods *string  `json:"accepted_payment_methods,omitempty"`
	Timezone               string   `json:"timezone"`
	SlotIntervalMinutes    int      `json:"slot_interval_minutes"`
	BusinessHours          string   `json:"business_hours"`
	CreatedAt              string   `json:"created_at"`
	UpdatedAt              string   `json:"updated_at"`
}

// toBusinessProfileOut maps the entity profile to the pinned output contract.
func toBusinessProfileOut(p *entity.BusinessProfile) businessProfileOut {
	return businessProfileOut{
		ID:                     p.ID,
		Name:                   p.Name,
		Industry:               p.Industry,
		Country:                p.Country,
		Address:                p.Address,
		Latitude:               p.Latitude,
		Longitude:              p.Longitude,
		CoverPhotoURL:          p.CoverPhotoURL,
		PublicPhone:            p.PublicPhone,
		MessengerPlatform:      p.MessengerPlatform,
		MessengerID:            p.MessengerID,
		ContactEmail:           p.ContactEmail,
		WebsiteURL:             p.WebsiteURL,
		GeneralDescription:     p.GeneralDescription,
		CurrencyCode:           p.CurrencyCode,
		CurrencySymbol:         p.CurrencySymbol,
		AcceptedPaymentMethods: p.AcceptedPaymentMethods,
		Timezone:               p.Timezone,
		SlotIntervalMinutes:    p.SlotIntervalMinutes,
		BusinessHours:          p.BusinessHours,
		CreatedAt:              p.CreatedAt,
		UpdatedAt:              p.UpdatedAt,
	}
}

// registerProfileTool wires the get_business_profile tool onto the SDK server
// when the port is non-nil (T-09). The profile is not tenant-scoped, so the
// handler only resolves the authenticated caller (fail-closed, REQ-MT-007)
// and passes no input to the use case; the role restriction (owner/admin/
// staff) is enforced at the RBAC transport layer, not here. The handler
// returns the pinned businessProfileOut output contract (JD fix B-3), not the
// raw entity.
func (s *Server) registerProfileTool() {
	if s.cfg.GetBusinessProfile == nil {
		return
	}
	mcp.AddTool(s.impl, s.mcpTool("get_business_profile", "Obtiene el perfil del negocio (nombre, descripción, horario y zona horaria)"),
		func(ctx context.Context, _ *mcp.CallToolRequest, _ getBusinessProfileIn) (*mcp.CallToolResult, businessProfileOut, error) {
			if _, err := auth.RequireCaller(ctx); err != nil {
				return nil, businessProfileOut{}, toMCPError(err)
			}
			profile, err := s.cfg.GetBusinessProfile.Execute(ctx)
			if err != nil {
				return nil, businessProfileOut{}, toMCPError(err)
			}
			if profile == nil {
				// Fail closed: the port documents no non-nil contract; a
				// (nil, nil) return is treated as not-found (GGA S-1).
				return nil, businessProfileOut{}, toMCPError(&domain.SemanticError{
					Code:    domain.ErrCodeNotFound,
					Message: "perfil del negocio no encontrado",
				})
			}
			return nil, toBusinessProfileOut(profile), nil
		})
	s.toolNames["get_business_profile"] = struct{}{}
}
