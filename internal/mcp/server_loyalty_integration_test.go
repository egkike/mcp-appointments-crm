package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
)

// seedLoyaltyBookings inserts bookings relative to the current UTC time so
// window-based assertions are stable. It assumes professionals, services and
// clients are already seeded by newIntegrationMuxWithDB.
func seedLoyaltyBookings(t *testing.T, conn *sql.DB) {
	t.Helper()
	now := time.Now().UTC()

	insert := func(id, clientID, start, end, status string) {
		t.Helper()
		_, err := conn.ExecContext(ctx(),
			`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status)
			 VALUES (?, ?, 'p1', 's1', ?, ?, ?)`,
			id, clientID, start, end, status,
		)
		if err != nil {
			t.Fatalf("insert booking %s: %v", id, err)
		}
	}

	insert("b-recent", "c1", fmtDBTime(now.Add(-3*24*time.Hour)), fmtDBTime(now.Add(-3*24*time.Hour+time.Hour)), "confirmed")
	insert("b-month", "c2", fmtDBTime(now.Add(-20*24*time.Hour)), fmtDBTime(now.Add(-20*24*time.Hour+time.Hour)), "confirmed")
	insert("b-old", "c1", fmtDBTime(now.Add(-300*24*time.Hour)), fmtDBTime(now.Add(-300*24*time.Hour+time.Hour)), "confirmed")
	insert("b-cancelled", "c2", fmtDBTime(now.Add(-2*24*time.Hour)), fmtDBTime(now.Add(-2*24*time.Hour+time.Hour)), "cancelled")
}

func ctx() context.Context {
	return context.Background()
}

func fmtDBTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func decodeLoyaltyResult(t *testing.T, rec *httptest.ResponseRecorder) []dto.LoyaltyReportEntry {
	t.Helper()
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("get_loyalty_report failed: %d %q", code, msg)
	}
	var loyalty struct {
		StructuredContent struct {
			Results []dto.LoyaltyReportEntry `json:"results"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &loyalty); err != nil {
		t.Fatalf("unmarshal loyalty result: %v; body=%s", err, string(result))
	}
	return loyalty.StructuredContent.Results
}

func TestIntegrationLoyaltyReport(t *testing.T) {
	t.Run("tool is registered", func(t *testing.T) {
		mux := newIntegrationMux(t)
		rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		result, code, msg := decodeRPCEnvelope(t, rec)
		if code != 0 {
			t.Fatalf("tools/list failed: %d %q", code, msg)
		}
		var list struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(result, &list); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(list.Tools) != 11 {
			t.Errorf("tools = %d; want 11", len(list.Tools))
		}
		found := false
		for _, tool := range list.Tools {
			if tool.Name == "get_loyalty_report" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("get_loyalty_report not registered")
		}
	})

	t.Run("five windows", func(t *testing.T) {
		mux, conn := newIntegrationMuxWithDB(t)
		seedLoyaltyBookings(t, conn)

		cases := []struct {
			period   string
			wantRows int
			wantC1   int
			wantC2   int
		}{
			{"last_week", 1, 1, 0},
			{"last_month", 2, 1, 1},
			{"last_year", 2, 2, 1},
			{"all_time", 2, 2, 1},
		}
		for _, tc := range cases {
			t.Run(tc.period, func(t *testing.T) {
				body := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_loyalty_report","arguments":{"period":%q}}}`, tc.period)
				rec := postMCPCaller(t, mux, "owner-1", body)
				results := decodeLoyaltyResult(t, rec)
				if len(results) != tc.wantRows {
					t.Errorf("rows = %d; want %d", len(results), tc.wantRows)
				}
				var c1Count, c2Count int
				for _, r := range results {
					if r.ClientID == "c1" {
						c1Count = r.BookingCount
					}
					if r.ClientID == "c2" {
						c2Count = r.BookingCount
					}
				}
				if c1Count != tc.wantC1 || c2Count != tc.wantC2 {
					t.Errorf("counts c1=%d c2=%d; want c1=%d c2=%d", c1Count, c2Count, tc.wantC1, tc.wantC2)
				}
			})
		}
	})

	t.Run("omitted period defaults to last_month", func(t *testing.T) {
		mux, conn := newIntegrationMuxWithDB(t)
		seedLoyaltyBookings(t, conn)

		rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_loyalty_report","arguments":{}}}`)
		results := decodeLoyaltyResult(t, rec)
		if len(results) != 2 {
			t.Errorf("rows = %d; want 2", len(results))
		}
	})

	t.Run("empty result returns empty list", func(t *testing.T) {
		mux := newIntegrationMux(t)
		// No bookings at all; last_week should return an empty result, not an error.
		rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_loyalty_report","arguments":{"period":"last_week"}}}`)
		results := decodeLoyaltyResult(t, rec)
		if results == nil || len(results) != 0 {
			t.Errorf("results = %v, want empty non-nil slice", results)
		}
	})

	t.Run("role rejections", func(t *testing.T) {
		mux := newIntegrationMux(t)
		for _, caller := range []string{"staff-1", "c1"} {
			t.Run(caller, func(t *testing.T) {
				rec := postMCPCaller(t, mux, caller, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_loyalty_report","arguments":{"period":"all_time"}}}`)
				_, code, msg := decodeRPCEnvelope(t, rec)
				if code != -32001 || msg != "no tienes permiso para realizar esta acción" {
					t.Errorf("caller %s: got code=%d msg=%q; want -32001 %q", caller, code, msg, "no tienes permiso para realizar esta acción")
				}
			})
		}
	})
}
