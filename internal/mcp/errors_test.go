package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// bodyErrReader always fails on read, simulating a transport-level error that
// consumes the request body before jsonParseGuard sees it.
type bodyErrReader struct{}

func (bodyErrReader) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestJSONParseGuard_HTTPBodyMessagesSpanish pins the transport HTTP body
// messages in Spanish, matching the auth_translator sibling wording. The
// messages are the plain-text bodies of the 400/413 responses (http.Error);
// JSON-RPC callers never see them, but bilingual transport text is drift.
func TestJSONParseGuard_HTTPBodyMessagesSpanish(t *testing.T) {
	innerCalled := false
	handler := jsonParseGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
	}))

	t.Run("unreadable body answers 400 in Spanish", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", bodyErrReader{})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rec.Code)
		}
		if innerCalled {
			t.Error("inner handler ran on an unreadable body")
		}
		if body := strings.TrimSuffix(rec.Body.String(), "\n"); body != "no se pudo leer el cuerpo de la solicitud" {
			t.Errorf("body = %q; want the Spanish transport message", body)
		}
	})

	t.Run("oversized body answers 413 in Spanish", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(strings.Repeat("a", maxRequestBodyBytes+1)))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d; want 413", rec.Code)
		}
		if innerCalled {
			t.Error("inner handler ran on an oversized body")
		}
		if body := strings.TrimSuffix(rec.Body.String(), "\n"); body != "cuerpo de la solicitud demasiado grande" {
			t.Errorf("body = %q; want the Spanish transport message", body)
		}
	})
}

// newTestServerWithLogger builds the minimal Server the toMCPError contract
// needs (a cfg.Logger sink) plus the buffer capturing its records.
func newTestServerWithLogger() (*Server, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	return &Server{cfg: Config{Logger: logger}}, &buf
}

// golden cases for the SemanticError → JSON-RPC error mapping (T-08).
func TestToMCPError(t *testing.T) {
	t.Run("semantic error maps to -32002 with its Spanish message", func(t *testing.T) {
		srv, logs := newTestServerWithLogger()
		inner := &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: "el profesional ya tiene una reserva en ese horario",
			Cause:   errors.New("overlap"),
		}
		got := srv.toMCPError(inner)
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32002 {
			t.Errorf("code = %d; want -32002", got.Code)
		}
		if got.Message != inner.Message {
			t.Errorf("message = %q; want %q", got.Message, inner.Message)
		}
		// A semantic error is an expected business outcome: no error log.
		if logs.Len() != 0 {
			t.Errorf("semantic error logged %q; want no log record", logs.String())
		}
	})

	t.Run("wrapped semantic error still maps to -32002", func(t *testing.T) {
		srv, _ := newTestServerWithLogger()
		sem := &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada"}
		got := srv.toMCPError(errors.Join(sem, errors.New("outer")))
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32002 || got.Message != "reserva no encontrada" {
			t.Errorf("got code=%d msg=%q; want -32002 %q", got.Code, got.Message, "reserva no encontrada")
		}
	})

	t.Run("infrastructure error maps to generic -32603 and logs server-side", func(t *testing.T) {
		srv, logs := newTestServerWithLogger()
		infra := errors.New("sqlite: disk I/O error")
		got := srv.toMCPError(infra)
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32603 {
			t.Errorf("code = %d; want -32603", got.Code)
		}
		if got.Message != "error interno del servidor" {
			t.Errorf("message = %q; want %q", got.Message, "error interno del servidor")
		}
		// The generic message must be paired with a detailed server-side log.
		logged := logs.String()
		if !strings.Contains(logged, "msg=\"tool handler failed\"") {
			t.Errorf("log = %q; want msg \"tool handler failed\"", logged)
		}
		if !strings.Contains(logged, "level=ERROR") {
			t.Errorf("log = %q; want level=ERROR", logged)
		}
		if !strings.Contains(logged, infra.Error()) {
			t.Errorf("log = %q; want the original error %q", logged, infra.Error())
		}
		// Sanitization stance: the wire never carries the internal error.
		if strings.Contains(got.Message, infra.Error()) {
			t.Errorf("wire message %q leaks the internal error", got.Message)
		}
	})

	t.Run("raw domain sentinel without semantic wrapper is generic", func(t *testing.T) {
		srv, _ := newTestServerWithLogger()
		// Use cases map ErrNotFound to SemanticError themselves; a raw
		// sentinel reaching the mapper is an infra-level leak, so it stays
		// generic instead of leaking domain detail.
		got := srv.toMCPError(domain.ErrNotFound)
		if got == nil {
			t.Fatal("toMCPError() = nil; want *jsonrpc.Error")
		}
		if got.Code != -32603 {
			t.Errorf("code = %d; want -32603", got.Code)
		}
	})

	t.Run("nil logger falls back to the default logger without panicking", func(t *testing.T) {
		// Silence the fallback's default logger so the expected ERROR line
		// does not pollute test output; the branch under test is the nil
		// guard, not the default sink.
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		defer slog.SetDefault(prev)

		srv := &Server{}
		got := srv.toMCPError(errors.New("infra without logger"))
		if got == nil || got.Code != -32603 {
			t.Fatalf("got %+v; want -32603", got)
		}
	})

	t.Run("nil error stays nil", func(t *testing.T) {
		srv, _ := newTestServerWithLogger()
		if got := srv.toMCPError(nil); got != nil {
			t.Errorf("toMCPError(nil) = %+v; want nil", got)
		}
	})
}
