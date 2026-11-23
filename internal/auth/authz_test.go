package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// TestAuthorizeBookingAccess pins the cross-tenant rule of the booking
// lifecycle (get/cancel/reschedule). It had no unit test at all: the observable
// chain was covered end to end, which means a change here could only be caught
// by a full booking flow (issue #284).
func TestAuthorizeBookingAccess(t *testing.T) {
	own := "p-1"
	other := "p-9"
	clientOwn := "c-1"
	clientOther := "c-9"

	bookingFor := func(professional, client string) *entity.Booking {
		return &entity.Booking{ProfessionalID: professional, ClientID: client}
	}

	cases := []struct {
		name    string
		caller  Caller
		booking *entity.Booking
		wantErr bool
	}{
		{"client reads its own booking", Caller{Role: RoleClient, ClientID: &clientOwn}, bookingFor(own, clientOwn), false},
		{"client reads another client's booking", Caller{Role: RoleClient, ClientID: &clientOwn}, bookingFor(own, clientOther), true},
		{"staff reads its professional's booking", Caller{Role: RoleStaff, ProfessionalID: &own}, bookingFor(own, clientOwn), false},
		{"staff reads another professional's booking", Caller{Role: RoleStaff, ProfessionalID: &own}, bookingFor(other, clientOwn), true},
		{"staff without a professional link", Caller{Role: RoleStaff}, bookingFor(own, clientOwn), true},
		{"admin reads any booking", Caller{Role: RoleAdmin}, bookingFor(other, clientOther), false},
		{"owner reads any booking", Caller{Role: RoleOwner}, bookingFor(other, clientOther), false},
		{"unknown role is refused", Caller{Role: "auditor"}, bookingFor(own, clientOwn), true},
		// The raw comparison admitted an empty caller link against an empty
		// booking professional by equality ("" == ""), which is the one case the
		// schema forbids for staff but a Caller struct can still carry. The
		// normalised form denies it, and this pins that tightening (issue #284).
		{"staff with an empty link on an empty booking professional", Caller{Role: RoleStaff, ProfessionalID: ptrString("")}, bookingFor("", clientOwn), true},
		{"staff with a blank link on a blank booking professional", Caller{Role: RoleStaff, ProfessionalID: ptrString("   ")}, bookingFor("   ", clientOwn), true},
		// Trim symmetry (issue #284): the read predicates already normalise the
		// caller's link, so these gates must too, or the same professional would
		// be in scope for a record read and out of scope for its own booking.
		{"staff with a padded stored link reads its own booking", Caller{Role: RoleStaff, ProfessionalID: ptrString("  p-1  ")}, bookingFor(own, clientOwn), false},
		{"staff reads its booking stored with a padded professional id", Caller{Role: RoleStaff, ProfessionalID: &own}, bookingFor(" p-1 ", clientOwn), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AuthorizeBookingAccess(tc.caller, tc.booking)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var sem *domain.SemanticError
			if !errors.As(err, &sem) {
				t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
			}
			if sem.Code != domain.ErrCodeForbidden {
				t.Errorf("code = %q; want %q", sem.Code, domain.ErrCodeForbidden)
			}
			if !strings.Contains(sem.Message, "no puede acceder a las reservas") &&
				!strings.Contains(sem.Message, "solo puede acceder") {
				t.Errorf("unexpected message %q", sem.Message)
			}
		})
	}
}
