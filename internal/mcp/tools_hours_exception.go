package mcp

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Business-hours exception tools (feat-hours-exception-tool, REQ-MT-015):
// create_hours_exception, delete_hours_exception and list_hours_exceptions.
//
// All three are owner-only at BOTH layers: the ToolRBAC row in the composition
// root is the coarse transport gate and every use case re-asserts
// auth.RequireRole(RoleOwner) (defense in depth, ADR-0015 Decision 2). The
// repository's own admin+owner guard stays untouched, so the barriers are, from
// outer to inner: ToolRBAC {owner} → use case {owner} → repository
// {admin, owner} → SQL UNIQUE(exception_date).
//
// Validation split (ADR-0015 drift mitigation): the handlers enforce ONLY the
// transport schema shape, which the SDK derives from the struct tags. Every
// business rule (date format/calendar validity, is_closed↔times consistency,
// HH:MM shape, open<close, uniqueness) stays single-sourced in the repository
// (design §1.3), deliberately unlike the schedule tools: the rule set is
// multi-rule and stateful, so re-checking it here would be the drift ADR-0015
// warns about.
//
// There is no update tool: the repository documents exceptions as immutable, so
// changing a date is delete_hours_exception + create_hours_exception (D3).

// createHoursExceptionIn is the input of create_hours_exception. IsClosed and
// ExceptionDate are required; OpenTime/CloseTime are required when IsClosed is
// false and forbidden when true (enforced by the repository, which emits the
// Spanish semantic error). Reason is free text ("Navidad").
type createHoursExceptionIn struct {
	ExceptionDate string  `json:"exception_date"`
	IsClosed      bool    `json:"is_closed"`
	OpenTime      *string `json:"open_time,omitempty"`
	CloseTime     *string `json:"close_time,omitempty"`
	Reason        *string `json:"reason,omitempty"`
}

// deleteHoursExceptionIn is the input of delete_hours_exception. The integer id
// is the row identity create_hours_exception read back.
type deleteHoursExceptionIn struct {
	ExceptionID int `json:"exception_id"`
}

// listHoursExceptionsIn is the input of list_hours_exceptions: the required
// inclusive [From, To] range (both bounds YYYY-MM-DD, mirroring the repository
// contract — no invented default window).
type listHoursExceptionsIn struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// hoursExceptionOut is the pinned wire shape of one stored exception, shared by
// create_hours_exception and list_hours_exceptions. created_at is deliberately
// absent (PRD §513 "sin timestamps", OQ-3), mirroring serviceOut: the column
// stays in SQLite and in the repository row, but never reaches the LLM.
type hoursExceptionOut struct {
	ID            int     `json:"id"`
	ExceptionDate string  `json:"exception_date"`
	IsClosed      bool    `json:"is_closed"`
	OpenTime      *string `json:"open_time,omitempty"`
	CloseTime     *string `json:"close_time,omitempty"`
	Reason        *string `json:"reason,omitempty"`
}

// hoursExceptionsOut is the pinned WIRE shape of list_hours_exceptions: the
// rows travel nested under "exceptions" instead of as a bare JSON array.
//
// Why a carrier (defect-outputschema-array-root-contract, v0.9.1): the SDK's
// JSON Schema generator renders a bare-slice output type with the root
// {"type":["null","array"]}. Hermes' MCP client (pydantic mcp-types, protocol
// 2025-11-25) validates EVERY tool's outputSchema root as Literal["object"], so
// a single array-rooted tool rejects the whole ListToolsResult and the server
// stays parked without tools. The other reads already answer an object root
// ({schedules:[…]} in get_professional_schedules, {results:[…]} in the search
// tools); this carrier gives the list tool the same root shape. Only the
// nesting changed: every row still uses hoursExceptionOut unchanged.
type hoursExceptionsOut struct {
	Exceptions []hoursExceptionOut `json:"exceptions"`
}

// toHoursExceptionOut maps a stored row to the pinned wire shape.
func toHoursExceptionOut(e *entity.BusinessHoursException) hoursExceptionOut {
	return hoursExceptionOut{
		ID:            e.ID,
		ExceptionDate: e.ExceptionDate,
		IsClosed:      e.IsClosed,
		OpenTime:      e.OpenTime,
		CloseTime:     e.CloseTime,
		Reason:        e.Reason,
	}
}

// deleteHoursExceptionOut is the WIRE shape of delete_hours_exception
// (ADR-0013 boundary contract 4): the json tags live in the transport, the
// application carrier (dto.DeleteHoursExceptionResult) stays transport-neutral.
// The serialized bytes are identical to the previous shape — only the owner of
// the tags moved.
type deleteHoursExceptionOut struct {
	ExceptionID int    `json:"exception_id"`
	Status      string `json:"status"`
}

// registerHoursExceptionTools wires the three owner-only exception tools. Like
// every other sub-registrar it skips its tools when the matching port is nil,
// keeping the skeleton behavior (tool absent) that transport-level tests rely
// on.
func (s *Server) registerHoursExceptionTools() {
	if s.cfg.CreateHoursException != nil {
		mcp.AddTool(s.impl, s.mcpTool("create_hours_exception", "Crea una excepción de horario para una fecha: el negocio queda cerrado todo el día (is_closed=true, sin open_time/close_time) o abre con una ventana propia (is_closed=false, con open_time y close_time en HH:MM y open_time anterior a close_time). Una fecha ya existente devuelve conflicto: borrala y creala de nuevo para cambiarla. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in createHoursExceptionIn) (*mcp.CallToolResult, hoursExceptionOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, hoursExceptionOut{}, s.toMCPError(err)
				}
				stored, err := s.cfg.CreateHoursException.Execute(ctx, dto.CreateHoursExceptionInput{
					Caller:        *caller,
					ExceptionDate: in.ExceptionDate,
					IsClosed:      in.IsClosed,
					OpenTime:      in.OpenTime,
					CloseTime:     in.CloseTime,
					Reason:        in.Reason,
				})
				if err != nil {
					return nil, hoursExceptionOut{}, s.toMCPError(err)
				}
				if stored == nil {
					return nil, hoursExceptionOut{}, s.toMCPError(errNilMaintenanceEntity)
				}
				return nil, toHoursExceptionOut(stored), nil
			})
		s.toolNames["create_hours_exception"] = struct{}{}
	}

	if s.cfg.DeleteHoursException != nil {
		mcp.AddTool(s.impl, s.mcpTool("delete_hours_exception", "Elimina una excepción de horario por su id. Un id inexistente devuelve un error semántico. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in deleteHoursExceptionIn) (*mcp.CallToolResult, deleteHoursExceptionOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, deleteHoursExceptionOut{}, s.toMCPError(err)
				}
				result, err := s.cfg.DeleteHoursException.Execute(ctx, dto.DeleteHoursExceptionInput{
					Caller:      *caller,
					ExceptionID: in.ExceptionID,
				})
				if err != nil {
					return nil, deleteHoursExceptionOut{}, s.toMCPError(err)
				}
				if result == nil {
					return nil, deleteHoursExceptionOut{}, s.toMCPError(errNilMaintenanceEntity)
				}
				return nil, deleteHoursExceptionOut{ExceptionID: result.ExceptionID, Status: result.Status}, nil
			})
		s.toolNames["delete_hours_exception"] = struct{}{}
	}

	if s.cfg.ListHoursExceptions != nil {
		mcp.AddTool(s.impl, s.mcpTool("list_hours_exceptions", "Lista las excepciones de horario dentro de un rango de fechas inclusivo [from, to], ordenadas por fecha ascendente. Ambas fechas son obligatorias y usan formato YYYY-MM-DD; un rango sin excepciones devuelve una lista vacía. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in listHoursExceptionsIn) (*mcp.CallToolResult, hoursExceptionsOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, hoursExceptionsOut{}, s.toMCPError(err)
				}
				rows, err := s.cfg.ListHoursExceptions.Execute(ctx, dto.ListHoursExceptionsInput{
					Caller: *caller,
					From:   in.From,
					To:     in.To,
				})
				if err != nil {
					return nil, hoursExceptionsOut{}, s.toMCPError(err)
				}
				// A nil slice would break the port contract and serialize as
				// JSON null; the use case already returns an empty slice, so a
				// nil here is an infrastructure failure (fail closed, same
				// class as errNilMaintenanceEntity).
				if rows == nil {
					return nil, hoursExceptionsOut{}, s.toMCPError(errNilMaintenanceEntity)
				}
				out := hoursExceptionsOut{Exceptions: make([]hoursExceptionOut, 0, len(rows))}
				for _, row := range rows {
					// A nil row would panic inside toHoursExceptionOut: the
					// repository never produces one, so its presence is an
					// infrastructure failure, not a row to skip (same fail-closed
					// class as the nil-slice guard above).
					if row == nil {
						return nil, hoursExceptionsOut{}, s.toMCPError(errNilMaintenanceEntity)
					}
					out.Exceptions = append(out.Exceptions, toHoursExceptionOut(row))
				}
				return nil, out, nil
			})
		s.toolNames["list_hours_exceptions"] = struct{}{}
	}
}
