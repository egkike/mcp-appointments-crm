package mcp

// End-to-end coverage for the anonymous self-registration path
// (feat-whatsapp-bot TASK-3.5): the REAL production composition (temp-file
// SQLite → repositories → use cases → tools → AuthMiddleware with the
// anonymous seam and the real RBAC map), driven through the HTTP transport with
// X-Caller-Id exactly like the rest of the integration suite
// (server_integration_maintenance_test.go).
//
// tools_client_registration_test.go pins the transport contract with a mocked
// port; these tests prove the seam, the resolver and the registration use case
// agree end-to-end: an unknown phone registers through the anonymous allowlist,
// the next call with the same header resolves as a `client`, an accounts phone
// is rejected, the per-phone limit holds and every other tool still answers 401.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// registrationOutput is the pinned register_client output
// {client_id, display_name, created} (REQ-MT-015).
type registrationOutput struct {
	ClientID    string `json:"client_id"`
	DisplayName string `json:"display_name"`
	Created     bool   `json:"created"`
}

// registerClientThroughMux performs the anonymous registration call through the
// real mux and returns the decoded output, or the JSON-RPC error code/message
// when the call failed.
func registerClientThroughMux(t *testing.T, h http.Handler, phone, args string) (registrationOutput, int64, string) {
	t.Helper()
	result, code, msg := callMCPTool(t, h, phone, "register_client", args)
	if code != 0 {
		return registrationOutput{}, code, msg
	}
	var out registrationOutput
	decodeToolStructured(t, result, &out)
	return out, 0, ""
}

// assertNoClientRow fails when any clients row exists for phone (id OR phone,
// so a legacy UUID row carrying the number also counts).
func assertNoClientRow(t *testing.T, conn *sql.DB, phone string) {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM clients WHERE id = ? OR phone = ?`, phone, phone).Scan(&n); err != nil {
		t.Fatalf("count clients for %q: %v", phone, err)
	}
	if n != 0 {
		t.Errorf("clients rows for %q = %d; want 0", phone, n)
	}
}

// countClientRows returns how many clients rows carry phone as id or phone
// (the idempotency assertion: the UNIQUE(phone) backstop allows exactly one).
func countClientRows(t *testing.T, conn *sql.DB, phone string) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM clients WHERE id = ? OR phone = ?`, phone, phone).Scan(&n); err != nil {
		t.Fatalf("count clients for %q: %v", phone, err)
	}
	return n
}

// TestIntegrationRegisterClientEndToEnd walks the full registration contract
// through the real mux: unknown phone → register_client → row with id == phone
// → the same header resolves role = client on a client-role tool.
func TestIntegrationRegisterClientEndToEnd(t *testing.T) {
	mux, conn := newIntegrationMuxWithDB(t)
	const phone = "+5491100999999"

	// GIVEN no accounts row and no clients row for the phone.
	assertNoClientRow(t, conn, phone)

	// WHEN the anonymous registration call carries X-Caller-Id: phone with no
	// body at all, THEN it succeeds with the placeholder name and created=true.
	out, code, msg := registerClientThroughMux(t, mux, phone, `{}`)
	if code != 0 {
		t.Fatalf("register_client failed: code=%d msg=%q", code, msg)
	}
	if out.ClientID != phone {
		t.Errorf("client_id = %q; want the header phone %q", out.ClientID, phone)
	}
	if out.DisplayName != "Cliente "+phone {
		t.Errorf("display_name = %q; want the placeholder %q", out.DisplayName, "Cliente "+phone)
	}
	if !out.Created {
		t.Error("created = false; want true for a first registration")
	}

	// AND the stored row carries the exact bytes of the header, with both the
	// id and the phone column set to it (the resolver matches on id).
	var storedID, storedName, storedPhone string
	if err := conn.QueryRowContext(context.Background(),
		`SELECT id, name, phone FROM clients WHERE id = ?`, phone,
	).Scan(&storedID, &storedName, &storedPhone); err != nil {
		t.Fatalf("read registered client: %v", err)
	}
	if storedID != phone || storedPhone != phone {
		t.Errorf("stored id/phone = %q/%q; want both %q", storedID, storedPhone, phone)
	}
	if storedName != "Cliente "+phone {
		t.Errorf("stored name = %q; want %q", storedName, "Cliente "+phone)
	}

	// AND a second call is idempotent: created=false and still one row.
	out, code, msg = registerClientThroughMux(t, mux, phone, `{"display_name":"Ana"}`)
	if code != 0 {
		t.Fatalf("second register_client failed: code=%d msg=%q", code, msg)
	}
	if out.Created {
		t.Error("created = true on the second call; want false (the row already existed)")
	}
	if out.DisplayName != "Cliente "+phone {
		t.Errorf("display_name = %q; want the stored name untouched by the retry", out.DisplayName)
	}
	if got := countClientRows(t, conn, phone); got != 1 {
		t.Errorf("clients rows = %d; want exactly 1 (no duplicate)", got)
	}

	// AND the SAME header now resolves as a client: the owner books the freshly
	// registered client in, and the client reads that booking back through a
	// client-role tool (a 401 or a cross-tenant denial would mean the resolver
	// did not map the phone to Role=client with ClientID=phone).
	start := nextMonday10AM(t)
	result := mustCallTool(t, mux, "owner-1", "create_booking",
		fmt.Sprintf(`{"client_id":%q,"service_id":"s1","professional_id":"p1","start_datetime":%q}`,
			phone, start.Format("2006-01-02T15:04:05-07:00")))
	var booking struct {
		BookingID string `json:"booking_id"`
	}
	decodeToolStructured(t, result, &booking)
	if booking.BookingID == "" {
		t.Fatal("create_booking returned an empty booking_id")
	}

	result = mustCallTool(t, mux, phone, "get_booking", fmt.Sprintf(`{"booking_id":%q}`, booking.BookingID))
	var view struct {
		Booking struct {
			ID       string `json:"id"`
			ClientID string `json:"client_id"`
		} `json:"booking"`
	}
	decodeToolStructured(t, result, &view)
	if view.Booking.ID != booking.BookingID || view.Booking.ClientID != phone {
		t.Errorf("client read-back = %+v; want the booking %q owned by %q",
			view.Booking, booking.BookingID, phone)
	}
}

// TestIntegrationRegisterClientRejectsAccountPhone pins the accounts collision
// over the real mux for an active AND an inactive account: both fail with the
// semantic Spanish message and neither creates a client row.
func TestIntegrationRegisterClientRejectsAccountPhone(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isActive int
	}{
		{"active account", 1},
		{"inactive account", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, conn := newIntegrationMuxWithDB(t)
			const phone = "+5491100000000"

			if _, err := conn.ExecContext(context.Background(),
				`INSERT INTO accounts (id, role, display_name, is_active) VALUES (?, 'admin', 'Admin Teléfono', ?)`,
				phone, tc.isActive); err != nil {
				t.Fatalf("seed account: %v", err)
			}

			_, code, msg := registerClientThroughMux(t, mux, phone, `{}`)
			if code != -32002 || msg != msgRegisterAccountPhone {
				t.Errorf("code=%d msg=%q; want -32002 %q", code, msg, msgRegisterAccountPhone)
			}
			assertNoClientRow(t, conn, phone)
		})
	}
}

// TestIntegrationRegisterClientRateLimited pins the per-phone limit through the
// real mux: a configured limit of 0 disables auto-registration (fail closed) and
// an exhausted window rejects the next attempt. Both answer the semantic Spanish
// rate-limit message and create no row.
func TestIntegrationRegisterClientRateLimited(t *testing.T) {
	t.Run("disabled limit rejects every registration", func(t *testing.T) {
		mux, conn := newIntegrationMuxWithRegistrationLimit(t, 0)
		const phone = "+5491100999999"

		_, code, msg := registerClientThroughMux(t, mux, phone, `{}`)
		if code != -32002 || msg != msgRegisterRateLimited {
			t.Errorf("code=%d msg=%q; want -32002 %q", code, msg, msgRegisterRateLimited)
		}
		assertNoClientRow(t, conn, phone)
	})

	t.Run("exhausted window rejects the next attempt", func(t *testing.T) {
		mux, conn := newIntegrationMuxWithRegistrationLimit(t, 1)
		const phone = "+5491100999999"

		out, code, msg := registerClientThroughMux(t, mux, phone, `{}`)
		if code != 0 || !out.Created {
			t.Fatalf("first registration: code=%d created=%t msg=%q; want a created row", code, out.Created, msg)
		}

		// The limiter budget is consumed by the attempt that REACHED it, and an
		// existing row short-circuits the use case before the limiter (the
		// idempotent no-op path). The phone therefore has to be unknown again for
		// a second attempt to reach the limiter inside the same window.
		if _, err := conn.ExecContext(context.Background(), `DELETE FROM clients WHERE id = ?`, phone); err != nil {
			t.Fatalf("delete registered client: %v", err)
		}

		_, code, msg = registerClientThroughMux(t, mux, phone, `{}`)
		if code != -32002 || msg != msgRegisterRateLimited {
			t.Errorf("second attempt: code=%d msg=%q; want -32002 %q", code, msg, msgRegisterRateLimited)
		}
		assertNoClientRow(t, conn, phone)
	})
}

// TestIntegrationRegistrationSeamBoundaries pins the seam boundaries through the
// real mux: the allowlisted path is the ONLY anonymous one, an empty
// X-Caller-Id is answered by the universal 401 before the seam, a body phone
// cannot register, and other tools keep rejecting an unknown phone.
func TestIntegrationRegistrationSeamBoundaries(t *testing.T) {
	mux, conn := newIntegrationMuxWithDB(t)
	const phone = "+5491100999999"

	// The tool is registered in the production composition.
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
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	registered := false
	for _, tool := range list.Tools {
		if tool.Name == "register_client" {
			registered = true
		}
	}
	if !registered {
		t.Errorf("register_client is not registered in the production composition: %s", string(result))
	}

	t.Run("missing X-Caller-Id is the universal 401 before the seam", func(t *testing.T) {
		_, code, msg := callMCPTool(t, mux, "", "register_client", `{}`)
		if code != -32000 || msg != "no se proporcionó X-Caller-Id" {
			t.Errorf("code=%d msg=%q; want -32000 %q", code, msg, "no se proporcionó X-Caller-Id")
		}
	})

	t.Run("a body phone cannot register either number", func(t *testing.T) {
		result, code, msg := callMCPTool(t, mux, phone, "register_client",
			`{"display_name":"Ana","phone":"+5491100000000"}`)
		if code != 0 {
			t.Fatalf("code=%d msg=%q; want a tool isError envelope (unknown property)", code, msg)
		}
		if text := toolErrorText(t, result); !strings.Contains(text, `"phone"`) {
			t.Errorf("tool error %q does not name the rejected property \"phone\"", text)
		}
		assertNoClientRow(t, conn, phone)
		assertNoClientRow(t, conn, "+5491100000000")
	})

	t.Run("every other tool still rejects an unknown phone", func(t *testing.T) {
		cases := []struct {
			tool string
			args string
		}{
			{"get_booking", `{"booking_id":"b-missing"}`},
			{"search_clients_advanced", `{"query_text":"Cliente"}`},
			{"check_availability", `{"service_id":"s1","professional_id":"p1","start_datetime":"2027-03-01T13:00:00Z"}`},
		}
		for _, tc := range cases {
			_, code, msg := callMCPTool(t, mux, phone, tc.tool, tc.args)
			if code != -32000 || msg != "no te reconozco. Por favor regístrate primero." {
				t.Errorf("%s: code=%d msg=%q; want -32000 unknown-caller", tc.tool, code, msg)
			}
		}
		assertNoClientRow(t, conn, phone)
	})
}
