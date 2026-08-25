package mcp

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// searchClientsAdvancedIn is the input of search_clients_advanced.
type searchClientsAdvancedIn struct {
	QueryText string `json:"query_text"`
}

// searchServicesAdvancedIn is the input of search_services_advanced.
type searchServicesAdvancedIn struct {
	QueryText string `json:"query_text"`
}

const (
	msgQueryTextRequired = "query_text es obligatorio"
	// maxFTSQueryLen mirrors repository.MaxFTSQueryLen to keep the transport
	// package free of repository imports (REQ-MT-012).
	maxFTSQueryLen = 200
)

// registerSearchTools wires the two FTS search tools onto the SDK server when
// the corresponding port is non-nil. Both tools admit any authenticated caller
// at the transport layer (no RBAC entry); role enforcement is delegated to the
// use case (search_services_advanced) or the repository (search_clients_advanced).
func (s *Server) registerSearchTools() {
	if s.cfg.SearchClientsAdvanced != nil {
		mcp.AddTool(s.impl, s.mcpTool("search_clients_advanced", "Busca clientes por nombre o preferencias. Disponible para todos los roles autenticados; el alcance depende del rol del llamante"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in searchClientsAdvancedIn) (*mcp.CallToolResult, dto.SearchClientsAdvancedResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.SearchClientsAdvancedResult{}, toMCPError(err)
				}
				query := strings.TrimSpace(in.QueryText)
				if query == "" {
					return nil, dto.SearchClientsAdvancedResult{}, toMCPError(&domain.SemanticError{
						Code:    domain.ErrCodeInvalidInput,
						Message: msgQueryTextRequired,
					})
				}
				if utf8.RuneCountInString(query) > maxFTSQueryLen {
					return nil, dto.SearchClientsAdvancedResult{}, toMCPError(&domain.SemanticError{
						Code:    domain.ErrCodeInvalidInput,
						Message: "query_text excede la longitud máxima",
					})
				}
				result, err := s.cfg.SearchClientsAdvanced.Execute(ctx, dto.SearchClientsAdvancedInput{

					Caller:    *caller,
					QueryText: query,
				})

				if err != nil {
					return nil, dto.SearchClientsAdvancedResult{}, toMCPError(err)
				}
				if result == nil {
					return nil, dto.SearchClientsAdvancedResult{Results: []dto.ClientSearchEntry{}}, nil
				}
				return nil, *result, nil
			})
		s.toolNames["search_clients_advanced"] = struct{}{}
	}

	if s.cfg.SearchServicesAdvanced != nil {
		mcp.AddTool(s.impl, s.mcpTool("search_services_advanced", "Busca servicios por nombre o descripción. Solo disponible para owner y admin"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in searchServicesAdvancedIn) (*mcp.CallToolResult, dto.SearchServicesAdvancedResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.SearchServicesAdvancedResult{}, toMCPError(err)
				}
				query := strings.TrimSpace(in.QueryText)
				if query == "" {
					return nil, dto.SearchServicesAdvancedResult{}, toMCPError(&domain.SemanticError{
						Code:    domain.ErrCodeInvalidInput,
						Message: msgQueryTextRequired,
					})
				}
				if utf8.RuneCountInString(query) > maxFTSQueryLen {
					return nil, dto.SearchServicesAdvancedResult{}, toMCPError(&domain.SemanticError{
						Code:    domain.ErrCodeInvalidInput,
						Message: "query_text excede la longitud máxima",
					})
				}
				result, err := s.cfg.SearchServicesAdvanced.Execute(ctx, dto.SearchServicesAdvancedInput{

					Caller:    *caller,
					QueryText: query,
				})

				if err != nil {
					return nil, dto.SearchServicesAdvancedResult{}, toMCPError(err)
				}
				if result == nil {
					return nil, dto.SearchServicesAdvancedResult{Results: []dto.ServiceSearchEntry{}}, nil
				}
				return nil, *result, nil
			})
		s.toolNames["search_services_advanced"] = struct{}{}
	}
}
