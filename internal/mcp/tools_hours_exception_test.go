package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/application/usecase"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Hours-exception tools (feat-hours-exception-tool, REQ-MT-005 / REQ-MT-015):
// create_hours_exception, delete_hours_exception and list_hours_exceptions.
// All three are owner-only, return the stored row WITHOUT wire timestamps
// (OQ-3 / PRD §513), and let only the use cases' Spanish semantic errors reach
// the client as -32002. The transport duplicates NO validation rule: the
// repository stays the single validator (design §1.3).

// Exact LLM-facing messages the three use cases produce (design §4). Pinning
// them here keeps the wire contract and the use-case wording from drifting
// apart: the transport must pass each one through unchanged.
const (
	msgHoursExceptionInvalid    = "la excepción no es válida. Si el negocio está cerrado, no envíes open_time/close_time; si está abierto, ambos son obligatorios en formato HH:MM y open_time debe ser anterior a close_time."
	msgHoursExceptionConflict   = "ya existe una excepción para esa fecha. Borrala y creala de nuevo si querés cambiar el horario de ese día."
	msgHoursExceptionNotFound   = "no existe una excepción con ese id."
	msgHoursExceptionForbidden  = "no tienes permiso para realizar esta acción"
	msgHoursExceptionRangeBound = "las fechas del rango deben tener formato YYYY-MM-DD."
)

// ── mock ports (fn-table, same pattern as the other tool tests) ──

type mockCreateHoursExceptionPort struct {
	executeFn func(ctx context.Context, in dto.CreateHoursExceptionInput) (*entity.BusinessHoursException, error)
}

func (m *mockCreateHoursExceptionPort) Execute(ctx context.Context, in dto.CreateHoursExceptionInput) (*entity.BusinessHoursException, error) {
	return m.executeFn(ctx, in)
}

type mockDeleteHoursExceptionPort struct {
	executeFn func(ctx context.Context, in dto.DeleteHoursExceptionInput) (*dto.DeleteHoursExceptionResult, error)
}

func (m *mockDeleteHoursExceptionPort) Execute(ctx context.Context, in dto.DeleteHoursExceptionInput) (*dto.DeleteHoursExceptionResult, error) {
	return m.executeFn(ctx, in)
}

type mockListHoursExceptionsPort struct {
	executeFn func(ctx context.Context, in dto.ListHoursExceptionsInput) ([]*entity.BusinessHoursException, error)
}

func (m *mockListHoursExceptionsPort) Execute(ctx context.Context, in dto.ListHoursExceptionsInput) ([]*entity.BusinessHoursException, error) {
	return m.executeFn(ctx, in)
}

// stubHoursExceptionPortsFail makes all three exception ports return (nil,
// err), so each test overrides only the port under test (same convention as
// stubMaintenancePortsFail).
func stubHoursExceptionPortsFail(ports *mockToolPorts, err error) {
	ports.createHoursException.executeFn = func(context.Context, dto.CreateHoursExceptionInput) (*entity.BusinessHoursException, error) {
		return nil, err
	}
	ports.deleteHoursException.executeFn = func(context.Context, dto.DeleteHoursExceptionInput) (*dto.DeleteHoursExceptionResult, error) {
		return nil, err
	}
	ports.listHoursExceptions.executeFn = func(context.Context, dto.ListHoursExceptionsInput) ([]*entity.BusinessHoursException, error) {
		return nil, err
	}
}

// fakeHoursExceptionRepo is an in-memory domainrepo.BusinessHoursExceptionRepo
// that mirrors the persistence contract the real adapter implements: rows are
// unique by date, a duplicate date is domain.ErrConflict, a missing delete is
// domain.ErrNotFound, and an inconsistent is_closed↔times pair is a wrapped
// domain.ErrInvalidInput. It exists so the tests below can drive the REAL use
// cases: the Spanish messages asserted here are the use cases' own output, not
// a mock's echo.
type fakeHoursExceptionRepo struct {
	rows     map[string]*entity.BusinessHoursException
	nextID   int
	lastFrom time.Time
	lastTo   time.Time
}

var _ domainrepo.BusinessHoursExceptionRepo = (*fakeHoursExceptionRepo)(nil)

func newFakeHoursExceptionRepo() *fakeHoursExceptionRepo {
	return &fakeHoursExceptionRepo{rows: map[string]*entity.BusinessHoursException{}}
}

func (r *fakeHoursExceptionRepo) Get(_ context.Context, date time.Time) (*entity.BusinessHoursException, error) {
	row, ok := r.rows[date.Format("2006-01-02")]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return row, nil
}

func (r *fakeHoursExceptionRepo) Create(_ context.Context, e *entity.BusinessHoursException) (*entity.BusinessHoursException, error) {
	if _, exists := r.rows[e.ExceptionDate]; exists {
		return nil, fmt.Errorf("crear excepción: %w", domain.ErrConflict)
	}
	if e.IsClosed {
		if e.OpenTime != nil || e.CloseTime != nil {
			return nil, fmt.Errorf("crear excepción: %w", domain.ErrInvalidInput)
		}
	} else if e.OpenTime == nil || e.CloseTime == nil || *e.OpenTime >= *e.CloseTime {
		return nil, fmt.Errorf("crear excepción: %w", domain.ErrInvalidInput)
	}
	r.nextID++
	stored := *e
	stored.ID = r.nextID
	r.rows[stored.ExceptionDate] = &stored
	return &stored, nil
}

func (r *fakeHoursExceptionRepo) List(_ context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
	r.lastFrom, r.lastTo = from, to
	rows := []*entity.BusinessHoursException{}
	for _, row := range r.rows {
		date, err := time.Parse("2006-01-02", row.ExceptionDate)
		if err != nil || date.Before(from) || date.After(to) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ExceptionDate < rows[j].ExceptionDate })
	return rows, nil
}

func (r *fakeHoursExceptionRepo) Delete(_ context.Context, id int) error {
	for date, row := range r.rows {
		if row.ID == id {
			delete(r.rows, date)
			return nil
		}
	}
	return domain.ErrNotFound
}

// ── helpers ──

// newHoursExceptionUseCaseServer wires the twenty shared mock ports to a server
// whose three exception ports are the REAL use cases, so the assertions below
// exercise the messages the use cases actually produce. Only the repository is
// replaced (in-memory fake); no mock echo sits in the path under test.
func newHoursExceptionUseCaseServer(t *testing.T) (*Server, *fakeHoursExceptionRepo) {
	t.Helper()
	repo := newFakeHoursExceptionRepo()
	ports := newMockPorts()
	// Single-source mapper + per-site overrides: the three exception ports are
	// the REAL use cases over the fake repo so the assertions below exercise
	// the messages the use cases actually produce; the other twenty ports stay
	// the shared mocks.
	cfg := configFromMockPorts(ports)
	cfg.CreateHoursException = usecase.NewCreateHoursExceptionUseCase(repo, discardLogger())
	cfg.DeleteHoursException = usecase.NewDeleteHoursExceptionUseCase(repo, discardLogger())
	cfg.ListHoursExceptions = usecase.NewListHoursExceptionsUseCase(repo)
	return NewServer(cfg), repo
}

// hoursExceptionToolArgs holds one schema-valid payload per tool, so the
// cross-cutting tests reach the handler instead of failing SDK validation.
var hoursExceptionToolArgs = map[string]string{
	"create_hours_exception": `{"exception_date":"2026-12-25","is_closed":true}`,
	"delete_hours_exception": `{"exception_id":1}`,
	"list_hours_exceptions":  `{"from":"2026-12-01","to":"2026-12-31"}`,
}

// hoursExceptionToolNames returns the three names in deterministic order.
func hoursExceptionToolNames() []string {
	names := make([]string, 0, len(hoursExceptionToolArgs))
	for name := range hoursExceptionToolArgs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func staffCallerPtr() *auth.Caller {
	c := auth.Caller{ID: "staff-1", Role: auth.RoleStaff}
	return &c
}

// ── registration (REQ-MT-005: 20 → 23) ──

func TestHoursExceptionToolsRegistered(t *testing.T) {
	srv, _ := newHoursExceptionUseCaseServer(t)

	// 23 = the 20 tools shipped before this change + the 3 new ones.
	if got := srv.ToolCount(); got != 23 {
		t.Errorf("ToolCount() = %d; want 23", got)
	}

	rec := callMethod(srv.Handler(), "tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal tools/list: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Result.Tools) != 23 {
		t.Fatalf("tools/list returned %d tools; want 23 (REQ-MT-005)", len(resp.Result.Tools))
	}
	got := make(map[string]bool, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"create_hours_exception", "delete_hours_exception", "list_hours_exceptions"} {
		if !got[want] {
			t.Errorf("tools/list missing %q; got %s", want, mustJSON(t, resp.Result.Tools))
		}
	}
}

// ── input decoding ──

func TestToolCreateHoursException(t *testing.T) {
	tests := []struct {
		name string
		args string
		want dto.CreateHoursExceptionInput
	}{
		{
			name: "closed day with reason",
			args: `{"exception_date":"2026-12-25","is_closed":true,"reason":"Navidad"}`,
			want: dto.CreateHoursExceptionInput{ExceptionDate: "2026-12-25", IsClosed: true, Reason: strPtr("Navidad")},
		},
		{
			name: "open day with a custom window",
			args: `{"exception_date":"2026-12-24","is_closed":false,"open_time":"09:00","close_time":"13:00"}`,
			want: dto.CreateHoursExceptionInput{ExceptionDate: "2026-12-24", OpenTime: strPtr("09:00"), CloseTime: strPtr("13:00")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubHoursExceptionPortsFail(ports, errors.New("port must not be called"))

			var got dto.CreateHoursExceptionInput
			ports.createHoursException.executeFn = func(_ context.Context, in dto.CreateHoursExceptionInput) (*entity.BusinessHoursException, error) {
				got = in
				return &entity.BusinessHoursException{
					ID: 7, ExceptionDate: in.ExceptionDate, IsClosed: in.IsClosed,
					OpenTime: in.OpenTime, CloseTime: in.CloseTime, Reason: in.Reason,
				}, nil
			}

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_hours_exception", tt.args))
			var out hoursExceptionOut
			if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}

			want := tt.want
			want.Caller = ownerCaller()
			if mustJSON(t, got) != mustJSON(t, want) {
				t.Errorf("input =\n%s\nwant\n%s", mustJSON(t, got), mustJSON(t, want))
			}
			if out.ID != 7 || out.ExceptionDate != tt.want.ExceptionDate || out.IsClosed != tt.want.IsClosed {
				t.Errorf("out = %+v; want the stored row read back (id 7)", out)
			}
		})
	}
}

func TestToolDeleteHoursException(t *testing.T) {
	srv, ports := newToolServer(t)
	stubHoursExceptionPortsFail(ports, errors.New("port must not be called"))

	var got dto.DeleteHoursExceptionInput
	ports.deleteHoursException.executeFn = func(_ context.Context, in dto.DeleteHoursExceptionInput) (*dto.DeleteHoursExceptionResult, error) {
		got = in
		return &dto.DeleteHoursExceptionResult{ExceptionID: in.ExceptionID, Status: "deleted"}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "delete_hours_exception", `{"exception_id":42}`))
	var out dto.DeleteHoursExceptionResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got.Caller != ownerCaller() || got.ExceptionID != 42 {
		t.Errorf("input = %+v; want owner-1/42", got)
	}
	if out.ExceptionID != 42 || out.Status != "deleted" {
		t.Errorf("out = %+v; want 42/deleted", out)
	}
}

func TestToolListHoursExceptions(t *testing.T) {
	srv, ports := newToolServer(t)
	stubHoursExceptionPortsFail(ports, errors.New("port must not be called"))

	var got dto.ListHoursExceptionsInput
	ports.listHoursExceptions.executeFn = func(_ context.Context, in dto.ListHoursExceptionsInput) ([]*entity.BusinessHoursException, error) {
		got = in
		return []*entity.BusinessHoursException{
			{ID: 1, ExceptionDate: "2026-12-01", IsClosed: true},
			{ID: 2, ExceptionDate: "2026-12-25", IsClosed: false, OpenTime: strPtr("09:00"), CloseTime: strPtr("13:00")},
		}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "list_hours_exceptions",
		`{"from":"2026-12-01","to":"2026-12-31"}`))
	var out []hoursExceptionOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got.Caller != ownerCaller() || got.From != "2026-12-01" || got.To != "2026-12-31" {
		t.Errorf("input = %+v; want owner-1/2026-12-01..2026-12-31 forwarded verbatim", got)
	}
	if len(out) != 2 || out[0].ExceptionDate != "2026-12-01" || out[1].ExceptionDate != "2026-12-25" {
		t.Errorf("out = %+v; want the rows in exception_date ASC order", out)
	}
}

// TestHoursExceptionOutWireShape pins the output contract: no timestamps
// (OQ-3), and the conditional time fields follow is_closed.
func TestHoursExceptionOutWireShape(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantKeys []string
	}{
		{
			name:     "closed day without times",
			args:     `{"exception_date":"2026-12-25","is_closed":true}`,
			wantKeys: []string{"id", "exception_date", "is_closed"},
		},
		{
			name:     "open day with times and reason",
			args:     `{"exception_date":"2026-12-24","is_closed":false,"open_time":"09:00","close_time":"13:00","reason":"Nochebuena"}`,
			wantKeys: []string{"id", "exception_date", "is_closed", "open_time", "close_time", "reason"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubHoursExceptionPortsFail(ports, errors.New("port must not be called"))
			ports.createHoursException.executeFn = func(_ context.Context, in dto.CreateHoursExceptionInput) (*entity.BusinessHoursException, error) {
				// CreatedAt is populated on the stored row; it must stay off the wire.
				return &entity.BusinessHoursException{
					ID: 9, ExceptionDate: in.ExceptionDate, IsClosed: in.IsClosed,
					OpenTime: in.OpenTime, CloseTime: in.CloseTime, Reason: in.Reason,
					CreatedAt: "2026-12-01T10:00:00Z",
				}, nil
			}

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_hours_exception", tt.args))
			var out map[string]json.RawMessage
			if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}
			if len(out) != len(tt.wantKeys) {
				t.Fatalf("output has %d keys; want exactly %d: %s", len(out), len(tt.wantKeys), mustJSON(t, out))
			}
			for _, key := range tt.wantKeys {
				if _, ok := out[key]; !ok {
					t.Errorf("output missing key %q; got %s", key, mustJSON(t, out))
				}
			}
			if _, ok := out["created_at"]; ok {
				t.Error("created_at must stay off the wire (PRD §513, OQ-3)")
			}
		})
	}
}

// ── validation split: required fields at the SDK, every business rule in the
// repository/use case (the transport duplicates no rule) ──

func TestHoursExceptionToolsRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args string
	}{
		{name: "create without exception_date", tool: "create_hours_exception", args: `{"is_closed":true}`},
		{name: "create without is_closed", tool: "create_hours_exception", args: `{"exception_date":"2026-12-25"}`},
		{name: "delete without exception_id", tool: "delete_hours_exception", args: `{}`},
		{name: "list without to", tool: "list_hours_exceptions", args: `{"from":"2026-12-01"}`},
		{name: "list without from", tool: "list_hours_exceptions", args: `{"to":"2026-12-31"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubHoursExceptionPortsFail(ports, errors.New("port must not be called"))

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), tt.tool, tt.args))
			wantToolError(t, resp, "missing properties")
		})
	}
}

// ── cross-cutting: caller, error mapping and the fail-closed guard ──

func TestHoursExceptionToolsMissingCaller(t *testing.T) {
	for _, name := range hoursExceptionToolNames() {
		t.Run(name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubHoursExceptionPortsFail(ports, errors.New("port must not be called"))

			resp := decodeToolResponse(t, callTool(srv.Handler(), nil, name, hoursExceptionToolArgs[name]))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != "se requiere autenticación" {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, "se requiere autenticación")
			}
		})
	}
}

// TestHoursExceptionToolsSemanticErrorPassthrough pins the only messages that
// may reach the client: the use cases' Spanish semantic errors, unchanged.
func TestHoursExceptionToolsSemanticErrorPassthrough(t *testing.T) {
	tests := []struct {
		tool    string
		message string
	}{
		{tool: "create_hours_exception", message: msgHoursExceptionInvalid},
		{tool: "delete_hours_exception", message: msgHoursExceptionNotFound},
		{tool: "list_hours_exceptions", message: msgHoursExceptionRangeBound},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubHoursExceptionPortsFail(ports, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: tt.message})

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), tt.tool, hoursExceptionToolArgs[tt.tool]))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != tt.message {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, tt.message)
			}
		})
	}
}

func TestHoursExceptionToolsInfraErrorMapsToInternal(t *testing.T) {
	for _, name := range hoursExceptionToolNames() {
		t.Run(name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubHoursExceptionPortsFail(ports, errors.New("sqlite: disk I/O error"))

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), name, hoursExceptionToolArgs[name]))
			wantErrorCode(t, resp, -32603)
			if resp.Error.Message != msgInternal {
				t.Errorf("error.message = %q; want %q (no internals leak)", resp.Error.Message, msgInternal)
			}
		})
	}
}

func TestHoursExceptionToolsNilResultFailsClosed(t *testing.T) {
	for _, name := range hoursExceptionToolNames() {
		t.Run(name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			stubHoursExceptionPortsFail(ports, nil)

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), name, hoursExceptionToolArgs[name]))
			wantErrorCode(t, resp, -32603)
		})
	}
}

// ── the real use cases over the transport ──

// TestHoursExceptionUseCasesThroughTransport drives create → list → delete with
// the production use cases (only the repository is an in-memory fake): the id
// read-back, the inclusive range pass-through, the date ASC order and the
// delete confirmation are the real behavior, not a mock's echo.
func TestHoursExceptionUseCasesThroughTransport(t *testing.T) {
	srv, repo := newHoursExceptionUseCaseServer(t)

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_hours_exception",
		`{"exception_date":"2026-12-25","is_closed":true,"reason":"Navidad"}`))
	var first hoursExceptionOut
	if err := json.Unmarshal(wantStructured(t, resp), &first); err != nil {
		t.Fatalf("unmarshal create output: %v", err)
	}
	if first.ID <= 0 || first.ExceptionDate != "2026-12-25" || !first.IsClosed {
		t.Fatalf("created = %+v; want the stored closing exception with its read-back id", first)
	}
	if first.Reason == nil || *first.Reason != "Navidad" {
		t.Errorf("created.Reason = %v; want Navidad", first.Reason)
	}

	resp = decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_hours_exception",
		`{"exception_date":"2026-12-31","is_closed":false,"open_time":"09:00","close_time":"13:00"}`))
	var second hoursExceptionOut
	if err := json.Unmarshal(wantStructured(t, resp), &second); err != nil {
		t.Fatalf("unmarshal second create output: %v", err)
	}
	if second.ID <= first.ID {
		t.Errorf("second id = %d; want > %d (distinct stored rows)", second.ID, first.ID)
	}

	resp = decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "list_hours_exceptions",
		`{"from":"2026-12-01","to":"2027-01-31"}`))
	var listed []hoursExceptionOut
	if err := json.Unmarshal(wantStructured(t, resp), &listed); err != nil {
		t.Fatalf("unmarshal list output: %v", err)
	}
	if len(listed) != 2 || listed[0].ExceptionDate != "2026-12-25" || listed[1].ExceptionDate != "2026-12-31" {
		t.Fatalf("listed = %+v; want the two rows in exception_date ASC order", listed)
	}
	wantFrom := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	if !repo.lastFrom.Equal(wantFrom) || !repo.lastTo.Equal(wantTo) {
		t.Errorf("repository range = %v..%v; want %v..%v (forwarded verbatim, inclusive)", repo.lastFrom, repo.lastTo, wantFrom, wantTo)
	}

	resp = decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "delete_hours_exception",
		fmt.Sprintf(`{"exception_id":%d}`, first.ID)))
	var deleted dto.DeleteHoursExceptionResult
	if err := json.Unmarshal(wantStructured(t, resp), &deleted); err != nil {
		t.Fatalf("unmarshal delete output: %v", err)
	}
	if deleted.ExceptionID != first.ID || deleted.Status != "deleted" {
		t.Errorf("deleted = %+v; want %d/deleted", deleted, first.ID)
	}
	if _, err := repo.Get(context.Background(), wantFrom); err == nil {
		t.Error("the row survived the delete in the repository")
	}
}

// TestHoursExceptionListEmptyRangeIsArray pins "empty range is [] and not an
// error": List (unlike Get) has no not-found outcome.
func TestHoursExceptionListEmptyRangeIsArray(t *testing.T) {
	srv, _ := newHoursExceptionUseCaseServer(t)

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "list_hours_exceptions",
		`{"from":"2030-01-01","to":"2030-12-31"}`))
	if got := string(wantStructured(t, resp)); got != "[]" {
		t.Errorf("empty range = %s; want []", got)
	}
}

// TestHoursExceptionUseCaseSemanticMessages drives the failure branches of the
// three real use cases: every one must surface as -32002 with its exact
// Spanish message (design §4) and leave the store untouched.
func TestHoursExceptionUseCaseSemanticMessages(t *testing.T) {
	tests := []struct {
		name    string
		caller  func() *auth.Caller
		tool    string
		args    string
		wantMsg string
		seed    bool
	}{
		{
			name: "duplicate date", caller: ownerCallerPtr, tool: "create_hours_exception",
			args: `{"exception_date":"2026-12-25","is_closed":true}`, wantMsg: msgHoursExceptionConflict, seed: true,
		},
		{
			name: "open day without times", caller: ownerCallerPtr, tool: "create_hours_exception",
			args: `{"exception_date":"2026-12-25","is_closed":false}`, wantMsg: msgHoursExceptionInvalid,
		},
		{
			name: "open_time not before close_time", caller: ownerCallerPtr, tool: "create_hours_exception",
			args: `{"exception_date":"2026-12-25","is_closed":false,"open_time":"13:00","close_time":"13:00"}`, wantMsg: msgHoursExceptionInvalid,
		},
		{
			name: "closed day with times", caller: ownerCallerPtr, tool: "create_hours_exception",
			args: `{"exception_date":"2026-12-25","is_closed":true,"open_time":"09:00","close_time":"13:00"}`, wantMsg: msgHoursExceptionInvalid,
		},
		{
			name: "delete unknown id", caller: ownerCallerPtr, tool: "delete_hours_exception",
			args: `{"exception_id":99}`, wantMsg: msgHoursExceptionNotFound,
		},
		{
			name: "malformed list bound", caller: ownerCallerPtr, tool: "list_hours_exceptions",
			args: `{"from":"2026-13-01","to":"2026-12-31"}`, wantMsg: msgHoursExceptionRangeBound,
		},
		{
			name: "staff caller is forbidden", caller: staffCallerPtr, tool: "create_hours_exception",
			args: `{"exception_date":"2026-12-25","is_closed":true}`, wantMsg: msgHoursExceptionForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, repo := newHoursExceptionUseCaseServer(t)
			if tt.seed {
				if _, err := repo.Create(context.Background(), &entity.BusinessHoursException{ExceptionDate: "2026-12-25", IsClosed: true}); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			before := len(repo.rows)

			resp := decodeToolResponse(t, callTool(srv.Handler(), tt.caller(), tt.tool, tt.args))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != tt.wantMsg {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, tt.wantMsg)
			}
			if got := len(repo.rows); got != before {
				t.Errorf("repository rows = %d; want %d (no write before the gate passes)", got, before)
			}
		})
	}
}
