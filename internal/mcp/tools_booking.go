package mcp

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxNotesLen bounds the create_booking notes field at the transport layer
// (GGA W-1): the use case has no notes-length rule, so the tool handler is
// the enforcement point. Revisit if the domain ever grows a notes policy.
const maxNotesLen = 2000

// Tool input structs (T-09). Field names are the MCP contract
// (REQ-MT-015): they match the design spec, not the DTO json tags
// (e.g. create_booking takes "start_datetime" and maps it to dto.StartTime,
// whose json tag is "start_time"). Optional fields carry omitempty so the
// SDK schema marks them optional; required fields do not.

// checkAvailabilityIn is the input of check_availability. EndDatetime is
// accepted for MCP spec forward-compat (REQ-MT-015 input contract) but the
// availability use case checks the start slot only, so it is intentionally
// ignored: the dto.CheckAvailabilityInput has no end field.
type checkAvailabilityIn struct {
	ServiceID      string     `json:"service_id"`
	ProfessionalID string     `json:"professional_id"`
	StartDatetime  time.Time  `json:"start_datetime"`
	EndDatetime    *time.Time `json:"end_datetime,omitempty"`
}

type createBookingIn struct {
	ClientID       string    `json:"client_id"`
	ServiceID      string    `json:"service_id"`
	ProfessionalID string    `json:"professional_id"`
	StartDatetime  time.Time `json:"start_datetime"`
	Notes          *string   `json:"notes,omitempty"`
	// PaymentMethod is intentionally not exposed: the REQ-MT-015 input
	// contract does not include it, even though dto.CreateBookingInput and
	// entity.Booking support it. Revisit when the payment flow ships.
}

type getBookingIn struct {
	BookingID string `json:"booking_id"`
}

// cancelBookingIn is the input of cancel_booking. Reason is accepted for the
// MCP spec input contract (REQ-MT-015) but the cancellation use case does not
// persist it, so it is intentionally ignored: the dto.CancelBookingInput has
// no reason field.
type cancelBookingIn struct {
	BookingID string `json:"booking_id"`
	Reason    string `json:"reason"`
}

type rescheduleBookingIn struct {
	BookingID        string    `json:"booking_id"`
	NewStartDatetime time.Time `json:"new_start_datetime"`
}

// registerBookingTools wires the five booking tools onto the SDK server when
// the corresponding port is non-nil (T-09). Each handler resolves the
// authenticated caller from the request context (REQ-MT-007; AuthMiddleware
// guarantees its presence in production, RequireCaller fails closed
// otherwise) and maps use case failures with toMCPError (REQ-MT-015).
func (s *Server) registerBookingTools() {
	if s.cfg.CheckAvailability != nil {
		mcp.AddTool(s.impl, s.mcpTool("check_availability", "Verifica si un profesional está disponible en una fecha y hora determinadas. Nota: el parámetro opcional end_datetime se acepta por compatibilidad con el contrato MCP pero la disponibilidad se evalúa solo sobre la hora de inicio"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in checkAvailabilityIn) (*mcp.CallToolResult, dto.CheckAvailabilityResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.CheckAvailabilityResult{}, toMCPError(err)
				}
				result, err := s.cfg.CheckAvailability.Execute(ctx, dto.CheckAvailabilityInput{
					Caller:         *caller,
					ServiceID:      in.ServiceID,
					ProfessionalID: in.ProfessionalID,
					StartDatetime:  in.StartDatetime,
				})
				if err != nil {
					return nil, dto.CheckAvailabilityResult{}, toMCPError(err)
				}
				return nil, *result, nil
			})
		s.toolNames["check_availability"] = struct{}{}
	}

	if s.cfg.CreateBooking != nil {
		mcp.AddTool(s.impl, s.mcpTool("create_booking", "Crea una nueva reserva (booking) para un cliente con un profesional y servicio"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in createBookingIn) (*mcp.CallToolResult, dto.CreateBookingResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.CreateBookingResult{}, toMCPError(err)
				}
				if in.Notes != nil && len(*in.Notes) > maxNotesLen {
					return nil, dto.CreateBookingResult{}, toMCPError(&domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "Notas excede el largo máximo"})
				}
				result, err := s.cfg.CreateBooking.Execute(ctx, dto.CreateBookingInput{
					Caller:         *caller,
					ClientID:       in.ClientID,
					ServiceID:      in.ServiceID,
					ProfessionalID: in.ProfessionalID,
					StartTime:      in.StartDatetime,
					Notes:          in.Notes,
				})
				if err != nil {
					return nil, dto.CreateBookingResult{}, toMCPError(err)
				}
				return nil, *result, nil
			})
		s.toolNames["create_booking"] = struct{}{}
	}

	if s.cfg.GetBooking != nil {
		mcp.AddTool(s.impl, s.mcpTool("get_booking", "Obtiene los detalles de una reserva existente por su ID"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in getBookingIn) (*mcp.CallToolResult, dto.GetBookingResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.GetBookingResult{}, toMCPError(err)
				}
				result, err := s.cfg.GetBooking.Execute(ctx, dto.GetBookingInput{
					Caller:    *caller,
					BookingID: in.BookingID,
				})
				if err != nil {
					return nil, dto.GetBookingResult{}, toMCPError(err)
				}
				return nil, *result, nil
			})
		s.toolNames["get_booking"] = struct{}{}
	}

	if s.cfg.CancelBooking != nil {
		mcp.AddTool(s.impl, s.mcpTool("cancel_booking", "Cancela una reserva existente. Nota: el parámetro reason se acepta por compatibilidad con el contrato MCP pero no se persiste"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in cancelBookingIn) (*mcp.CallToolResult, dto.CancelBookingResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.CancelBookingResult{}, toMCPError(err)
				}
				result, err := s.cfg.CancelBooking.Execute(ctx, dto.CancelBookingInput{
					Caller:    *caller,
					BookingID: in.BookingID,
				})
				if err != nil {
					return nil, dto.CancelBookingResult{}, toMCPError(err)
				}
				return nil, *result, nil
			})
		s.toolNames["cancel_booking"] = struct{}{}
	}

	if s.cfg.RescheduleBooking != nil {
		mcp.AddTool(s.impl, s.mcpTool("reschedule_booking", "Reagenda una reserva existente a una nueva fecha y hora"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in rescheduleBookingIn) (*mcp.CallToolResult, dto.RescheduleBookingResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.RescheduleBookingResult{}, toMCPError(err)
				}
				result, err := s.cfg.RescheduleBooking.Execute(ctx, dto.RescheduleBookingInput{
					Caller:       *caller,
					BookingID:    in.BookingID,
					NewStartTime: in.NewStartDatetime,
				})
				if err != nil {
					return nil, dto.RescheduleBookingResult{}, toMCPError(err)
				}
				return nil, *result, nil
			})
		s.toolNames["reschedule_booking"] = struct{}{}
	}
}

// mcpTool builds the SDK tool descriptor with the shared tool metadata.
func (s *Server) mcpTool(name, description string) *mcp.Tool {
	return &mcp.Tool{Name: name, Description: description}
}
