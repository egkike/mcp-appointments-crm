package auth

import (
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// staffScopeCaller / staffScopeCallerWithoutProfessional are the two staff
// shapes the predicate must tell apart: the schema guarantees a professional for
// staff (internal/db/schema.go), but the predicate is the layer that must not
// panic if a caller somehow arrives without one.
func staffScopeCaller(professionalID string) Caller {
	return Caller{ID: "staff-1", Role: RoleStaff, ProfessionalID: &professionalID}
}

// TestAuthorizeProfessionalRead pins the ONE "may this caller read this
// professional?" predicate (feat-staff-self-read S1/S2/S3, issue #276). Both the
// record read (get_professional) and the agenda read (get_professional_schedules)
// route through it, and so does the SchedulesRepo guard, so this table is the
// truth table of all three layers.
//
// The staff foreign-id case pins the ERROR SHAPE, not just "an error": before
// this predicate the refusal travelled as ErrCodeUnauthenticated
// (ProfessionalsRepo.FindByID's staff branch), which told an authenticated caller
// it was not logged in. The scope refusal must be Forbidden with the shipped
// refusal message.
func TestAuthorizeProfessionalRead(t *testing.T) {
	cases := []struct {
		name      string
		caller    Caller
		requested string
		wantID    string
	}{
		{"owner names any professional", Caller{ID: "owner-1", Role: RoleOwner}, "p-any", "p-any"},
		{"admin names any professional", Caller{ID: "admin-1", Role: RoleAdmin}, "p-any", "p-any"},
		{"staff names its own record", staffScopeCaller("p-own"), "p-own", "p-own"},
		{"staff omits the id and reads itself", staffScopeCaller("p-own"), "", "p-own"},
		{"staff omits the id, padded request", staffScopeCaller("p-own"), "   ", "p-own"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AuthorizeProfessionalRead(tc.caller, tc.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantID {
				t.Errorf("id = %q; want %q", got, tc.wantID)
			}
		})
	}
}

// TestAuthorizeProfessionalReadForbidden pins the rejection half of the truth
// table: a staff caller asking for a FOREIGN professional, a client asking for
// any record, and an unknown role. All three are authenticated callers that lack
// the scope, so all three MUST answer Forbidden with the shipped message —
// never UNAUTHENTICATED (feat-staff-self-read S2).
func TestAuthorizeProfessionalReadForbidden(t *testing.T) {
	cases := map[string]Caller{
		"staff asks for a foreign id": staffScopeCaller("p-own"),
		"client asks for a record":    {ID: "c1", Role: RoleClient, ClientID: ptrString("c1")},
		"unknown role":                {ID: "m1", Role: "manager"},
	}

	for name, caller := range cases {
		t.Run(name, func(t *testing.T) {
			// Every caller here asks for an id that is NOT the one it may read:
			// the staff row is linked to p-own, so p-foreign is the foreign case.
			_, err := AuthorizeProfessionalRead(caller, "p-foreign")
			var sem *domain.SemanticError
			if !errors.As(err, &sem) {
				t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
			}
			if sem.Code != domain.ErrCodeForbidden {
				t.Errorf("Code = %q; want %q (an authenticated caller outside its scope is forbidden, not unauthenticated)",
					sem.Code, domain.ErrCodeForbidden)
			}
			if sem.Message != MsgForbiddenTool {
				t.Errorf("Message = %q; want the shipped refusal %q", sem.Message, MsgForbiddenTool)
			}
			if !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("sentinel lost: %v", err)
			}
		})
	}
}

// TestAuthorizeProfessionalReadSelfWithoutProfessional pins the third answer:
// a caller that is NOT a professional (owner/admin/client) omitting the id has
// no record to read. That is an input problem, not a permission problem — the
// caller must be told which field is missing instead of receiving an empty
// record or a panic on the nil ProfessionalID.
func TestAuthorizeProfessionalReadSelfWithoutProfessional(t *testing.T) {
	for name, caller := range map[string]Caller{
		"owner":  {ID: "owner-1", Role: RoleOwner},
		"admin":  {ID: "admin-1", Role: RoleAdmin},
		"client": {ID: "c1", Role: RoleClient, ClientID: ptrString("c1")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := AuthorizeProfessionalRead(caller, "")
			var sem *domain.SemanticError
			if !errors.As(err, &sem) {
				t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
			}
			if sem.Code != domain.ErrCodeInvalidInput {
				t.Errorf("Code = %q; want %q (a missing id is an input error, not a permission one)",
					sem.Code, domain.ErrCodeInvalidInput)
			}
			if sem.Message != MsgProfessionalScopeRequired {
				t.Errorf("Message = %q; want %q", sem.Message, MsgProfessionalScopeRequired)
			}
		})
	}
}

// TestCanReadProfessionalContact pins the field-level PII rule (S4): the contact
// data crosses only for the owner and for the professional itself. admin is
// deliberately excluded — oversight does not need the roster's phones (owner
// decision L2) — and so is every caller without a professional link.
func TestCanReadProfessionalContact(t *testing.T) {
	cases := []struct {
		name          string
		caller        Caller
		professional  string
		wantInclusion bool
	}{
		{"owner reads any contact", Caller{ID: "owner-1", Role: RoleOwner}, "p-any", true},
		{"staff reads its own contact", staffScopeCaller("p-own"), "p-own", true},
		{"admin reads no contact", Caller{ID: "admin-1", Role: RoleAdmin}, "p-any", false},
		// The accounts CHECK lets an ADMIN row carry a professional_id, so the
		// "self" branch cannot be a bare id comparison: an admin linked to p-own
		// would otherwise receive that professional's contact data (verifier
		// finding on #276). Only the staff role is schema-tied to a professional.
		{"admin linked to the professional still reads no contact", Caller{ID: "admin-1", Role: RoleAdmin, ProfessionalID: ptrString("p-own")}, "p-own", false},
		{"staff reads another professional", staffScopeCaller("p-own"), "p-other", false},
		// Trim symmetry (issue #278 A1): the requested id is trimmed by the scope
		// predicate, so the contact comparison must normalise both sides or the
		// same record would pass one question and fail the other.
		{"staff reads its own contact with a padded stored id", staffScopeCaller("  p-own  "), "p-own", true},
		{"client reads no contact", Caller{ID: "c1", Role: RoleClient, ClientID: ptrString("c1")}, "p-any", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanReadProfessionalContact(tc.caller, tc.professional); got != tc.wantInclusion {
				t.Errorf("CanReadProfessionalContact(%s, %q) = %v; want %v",
					tc.caller.Role, tc.professional, got, tc.wantInclusion)
			}
		})
	}
}

// ptrString is the local pointer helper for this file. helpers_test.go carries
// no shared one and the auth package must not grow a dependency for a fixture.
func ptrString(s string) *string { return &s }
