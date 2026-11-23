package auth

import (
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// MsgProfessionalScopeRequired is the refusal a caller receives when it asks for
// "my professional record" without being linked to one (owner, admin and client
// rows carry no professional_id). It is a semantic INPUT error, not a permission
// error: the caller was admitted by the role gate and simply named no record.
const MsgProfessionalScopeRequired = "Debes indicar professional_id: tu usuario no está vinculado a un profesional."

// AuthorizeProfessionalRead is the ONE predicate behind "may this caller read
// this professional?" (feat-staff-self-read S1/S2/S3, issue #276).
//
// Same doctrine as CanInvoke (advisory S1, issue #268): the question is answered
// here and nowhere else. Three call sites consume it —
//
//   - the get_professional use case (a staff caller naming a foreign id);
//   - the get_professional_schedules use case (same scope, agenda resource);
//   - the SchedulesRepo.FindByProfessional guard (the repository is the third
//     gate of the agenda read, so it cannot re-derive the rule either).
//
// Why it matters: the rule used to live ONLY inside ProfessionalsRepo.FindByID,
// and it answered ErrCodeUnauthenticated for a staff caller asking about a
// foreign id. That is the wrong answer for an authenticated caller (the token
// was fine; the scope was not), and worse, it made the refusal unreachable for
// every tool that does not go through FindByID. Centralizing it here lets each
// layer enforce the scope BEFORE the read, so "forbidden" is decided on the
// request, not inferred from a lookup that returned nothing.
//
// Contract:
//   - requestedID == "" (or blank) asks for the CALLER'S OWN record: a caller
//     without Caller.ProfessionalID has no such record → INVALID_INPUT naming
//     the missing field, never a panic and never an empty success;
//   - requestedID named: owner and admin may read anyone, staff may read only
//     its own professional — anything else is FORBIDDEN with the shipped
//     refusal message (auth.MsgForbiddenTool), never UNAUTHENTICATED.
//
// The returned id is the effective one, trimmed, which is what the caller should
// pass to the repository.
func AuthorizeProfessionalRead(caller Caller, requestedID string) (string, error) {
	id := strings.TrimSpace(requestedID)
	own := CallerProfessionalID(caller)
	if id == "" {
		if own == "" {
			return "", &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: MsgProfessionalScopeRequired,
				Cause:   domain.ErrInvalidInput,
			}
		}
		return own, nil
	}

	switch caller.Role {
	case RoleOwner, RoleAdmin:
		return id, nil
	case RoleStaff:
		if own != "" && own == id {
			return id, nil
		}
	}

	// Client, unknown role, or a staff caller naming someone else: the caller is
	// authenticated but outside the scope of this record.
	return "", &domain.SemanticError{
		Code:    domain.ErrCodeForbidden,
		Message: MsgForbiddenTool,
		Cause:   domain.ErrForbidden,
	}
}

// CanReadProfessionalContact answers the FIELD-level PII question for the
// professional record read (feat-staff-self-read S4, issue #276): may this caller
// see email and phone?
//
// It is deliberately a different question from AuthorizeProfessionalRead — the
// admin may read the record and still MUST NOT receive the contact fields
// (owner decision L2: oversight does not need the roster's phones, and exposing
// them would widen the PII surface for no operational purpose). Keeping it as its
// own predicate means the field rule is stated once, next to the record rule it
// qualifies, instead of being re-invented inside a use case.
//
// Inclusion set: owner, and the professional itself. "Itself" is deliberately
// narrower than "the caller's linked professional id matches" (verifier finding
// on #276): the accounts CHECK lets an ADMIN row carry a professional_id, so a
// bare id comparison would hand that admin the contact data of the professional
// it happens to be linked to, contradicting the owner decision this predicate
// exists to encode. The self branch therefore requires the staff role, which is
// the only role the schema ties to a professional.
func CanReadProfessionalContact(caller Caller, professionalID string) bool {
	if caller.Role == RoleOwner {
		return true
	}
	if caller.Role != RoleStaff {
		return false
	}
	own := CallerProfessionalID(caller)
	return own != "" && own == strings.TrimSpace(professionalID)
}

// CallerProfessionalID is the caller's own professional link, trimmed, or "" when
// it carries none.
//
// It is EXPORTED because the normalisation is not the read predicates' private
// business: the three remaining gates of the booking edge (AuthorizeBookingAccess,
// RequireClientMatch, CreateBookingUseCase) compare the same stored value against
// a named target, and each of them had its own raw comparison — so a padded link
// was in scope for a record read and out of scope for its own booking (issue
// #284). One normalisation, every layer.
//
// Since issue #286 the repository adapters read the link through this function
// too: the two auth filters (bookings, clients) scope their subqueries with it,
// ProfessionalsRepo.FindByID and FindActive use it instead of their own inline
// trim, and SchedulesRepo.FindByProfessionalAndDay normalises the override with
// it. Converging them closed a fail-open on the way — FindActive answered a staff
// caller with no link by running the UNSCOPED query, so it received the whole
// roster instead of a refusal.
//
// What remains outside is caller.ClientID, whose provenance differs (for a client,
// Caller.ID is the phone and ClientID the UUID) and whose filters are already
// fail-closed.
func CallerProfessionalID(caller Caller) string {
	if caller.ProfessionalID == nil {
		return ""
	}
	return strings.TrimSpace(*caller.ProfessionalID)
}
