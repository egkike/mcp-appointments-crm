package auth

import (
	"context"
	"testing"
	"time"
)

func TestCaller_StaffWithProfessionalID(t *testing.T) {
	t.Parallel()
	pID := "p-001"
	c := Caller{ID: "+5491155554444", Role: RoleStaff, ProfessionalID: &pID}

	if c.ID != "+5491155554444" {
		t.Errorf("ID = %q; want %q", c.ID, "+5491155554444")
	}
	if c.Role != RoleStaff {
		t.Errorf("Role = %q; want %q", c.Role, RoleStaff)
	}
	if c.ProfessionalID == nil || *c.ProfessionalID != "p-001" {
		t.Errorf("ProfessionalID = %v; want pointer to %q", c.ProfessionalID, "p-001")
	}
	if c.ClientID != nil {
		t.Errorf("ClientID = %v; want nil", c.ClientID)
	}
}

func TestCaller_ClientWithClientID(t *testing.T) {
	t.Parallel()
	id := "+5491100001111"
	c := Caller{ID: id, Role: RoleClient, ClientID: &id}

	if c.Role != RoleClient {
		t.Errorf("Role = %q; want %q", c.Role, RoleClient)
	}
	if c.ProfessionalID != nil {
		t.Errorf("ProfessionalID = %v; want nil", c.ProfessionalID)
	}
	if c.ClientID == nil || *c.ClientID != id {
		t.Errorf("ClientID = %v; want pointer to %q", c.ClientID, id)
	}
}

func TestRoleConstants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"RoleOwner", RoleOwner, "owner"},
		{"RoleAdmin", RoleAdmin, "admin"},
		{"RoleStaff", RoleStaff, "staff"},
		{"RoleClient", RoleClient, "client"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("%s = %q; want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestWithCaller_ReturnsNewContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := Caller{ID: "+5491155554444", Role: RoleStaff}

	newCtx := WithCaller(ctx, c)

	// The returned context must be different from the original.
	if newCtx == ctx {
		t.Fatal("WithCaller returned the same context; want a new one")
	}

	got, ok := FromContext(newCtx)
	if !ok {
		t.Fatal("FromContext returned false; want true")
	}
	if got.ID != c.ID || got.Role != c.Role {
		t.Errorf("FromContext = %+v; want %+v", got, c)
	}
}

func TestWithCaller_ZeroValueNoPanic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Must not panic with zero-value Caller.
	newCtx := WithCaller(ctx, Caller{})

	got, ok := FromContext(newCtx)
	if !ok {
		t.Fatal("FromContext returned false for zero-value caller; want true")
	}
	if got != (Caller{}) {
		t.Errorf("FromContext = %+v; want zero Caller", got)
	}
}

func TestFromContext_Present(t *testing.T) {
	t.Parallel()
	pID := "p-001"
	cID := "+5491100001111"
	c := Caller{ID: "+5491100000000", Role: RoleOwner, ProfessionalID: &pID, ClientID: &cID}

	ctx := WithCaller(context.Background(), c)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext returned false; want true")
	}
	if got.ID != c.ID {
		t.Errorf("ID = %q; want %q", got.ID, c.ID)
	}
	if got.Role != c.Role {
		t.Errorf("Role = %q; want %q", got.Role, c.Role)
	}
	if got.ProfessionalID == nil || *got.ProfessionalID != pID {
		t.Errorf("ProfessionalID = %v; want pointer to %q", got.ProfessionalID, pID)
	}
	if got.ClientID == nil || *got.ClientID != cID {
		t.Errorf("ClientID = %v; want pointer to %q", got.ClientID, cID)
	}
}

func TestFromContext_Absent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	got, ok := FromContext(ctx)
	if ok {
		t.Fatal("FromContext returned true for empty context; want false")
	}
	if got != (Caller{}) {
		t.Errorf("FromContext = %+v; want zero Caller", got)
	}
}

func TestFromContext_CancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	got, ok := FromContext(ctx)
	if ok {
		t.Fatal("FromContext returned true for cancelled context without caller; want false")
	}
	if got != (Caller{}) {
		t.Errorf("FromContext = %+v; want zero Caller", got)
	}
}

func TestFromContext_PropagatesThroughWithCancel(t *testing.T) {
	t.Parallel()
	c := Caller{ID: "+5491155554444", Role: RoleAdmin}
	ctx := WithCaller(context.Background(), c)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	got, ok := FromContext(cancelCtx)
	if !ok {
		t.Fatal("FromContext returned false after WithCancel; want true")
	}
	if got.ID != c.ID {
		t.Errorf("ID = %q; want %q", got.ID, c.ID)
	}

	// Even after cancel, the value survives.
	cancel()
	got2, ok2 := FromContext(cancelCtx)
	if !ok2 {
		t.Fatal("FromContext returned false after cancel(); want true")
	}
	if got2.ID != c.ID {
		t.Errorf("ID after cancel = %q; want %q", got2.ID, c.ID)
	}
}

func TestFromContext_PropagatesThroughWithTimeout(t *testing.T) {
	t.Parallel()
	c := Caller{ID: "+5491155554444", Role: RoleStaff}
	ctx := WithCaller(context.Background(), c)

	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	got, ok := FromContext(timeoutCtx)
	if !ok {
		t.Fatal("FromContext returned false after WithTimeout; want true")
	}
	if got.ID != c.ID {
		t.Errorf("ID = %q; want %q", got.ID, c.ID)
	}
}
