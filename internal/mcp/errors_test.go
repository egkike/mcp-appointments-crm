package mcp

import (
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// golden cases for the SemanticError → JSON-RPC error mapping (T-08).
func TestToMCPError(t *testing.T) {
	t.Run("semantic error maps to -32002 with its Spanish message", func(t *testing.T) {
		inner := &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: "el profesional ya tiene una reserva en ese horario",
			Cause:   errors.New("overlap"),
		}
		got := toMCPError(inner)
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32002 {
			t.Errorf("code = %d; want -32002", got.Code)
		}
		if got.Message != inner.Message {
			t.Errorf("message = %q; want %q", got.Message, inner.Message)
		}
	})

	t.Run("wrapped semantic error still maps to -32002", func(t *testing.T) {
		sem := &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada"}
		got := toMCPError(errors.Join(sem, errors.New("outer")))
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32002 || got.Message != "reserva no encontrada" {
			t.Errorf("got code=%d msg=%q; want -32002 %q", got.Code, got.Message, "reserva no encontrada")
		}
	})

	t.Run("infrastructure error maps to generic -32603", func(t *testing.T) {
		got := toMCPError(errors.New("sqlite: disk I/O error"))
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32603 {
			t.Errorf("code = %d; want -32603", got.Code)
		}
		if got.Message != "error interno del servidor" {
			t.Errorf("message = %q; want %q", got.Message, "error interno del servidor")
		}
	})

	t.Run("raw domain sentinel without semantic wrapper is generic", func(t *testing.T) {
		// Use cases map ErrNotFound to SemanticError themselves; a raw
		// sentinel reaching the mapper is an infra-level leak, so it stays
		// generic instead of leaking domain detail.
		got := toMCPError(domain.ErrNotFound)
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32603 {
			t.Errorf("code = %d; want -32603", got.Code)
		}
	})

	t.Run("nil error stays nil", func(t *testing.T) {
		if got := toMCPError(nil); got != nil {
			t.Errorf("toMCPError(nil) = %+v; want nil", got)
		}
	})
}
