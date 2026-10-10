package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// registrationMaxNameRunes caps the stored display name. design §1.2 pins the
// wire maximum at 80 runes; design §1.3 does not specify a use-case behavior,
// so the use case re-applies the cap deterministically by truncation (defense
// in depth: the transport rejects nothing, it only documents the limit).
const registrationMaxNameRunes = 80

// GetOrCreateClientResult is the registration outcome handed to the transport
// adapter: the resolved client id (byte-identical to the header phone), the
// stored display name and whether this call created the row.
type GetOrCreateClientResult struct {
	ClientID    string
	DisplayName string
	Created     bool
}

// GetOrCreateClientUseCase implements the anonymous self-registration path for
// the WhatsApp/Telegram gateway (change feat-whatsapp-bot, design §1.3).
//
// It is the one use case whose identity is a bare header phone with no role:
// Execute receives the phone explicitly and there is deliberately no
// auth.Caller / role parameter (compile-time guarantee). It never delegates to
// the legacy ClientsRepo.GetOrCreate UUID path, whose generated id could never
// be resolved again from a phone.
type GetOrCreateClientUseCase struct {
	clients repository.ClientsRepo
	lookup  repository.RegistrationLookup
	limiter *registrationRateLimiter
	logger  *slog.Logger
}

// NewGetOrCreateClientUseCase constructs a GetOrCreateClientUseCase from the
// client write port (GetOrCreateByPhone), the auth-free RegistrationLookup read
// port (accounts collision + phone lookup), the per-phone registration limiter
// (design §1.5, built from config by the composition root) and the process
// logger for the registration audit trail. All of them are wired ONLY here by
// the composition root; the authenticated repos keep their guards untouched.
//
// A nil limiter is normalized to the fail-closed "disabled" limiter and a nil
// logger to slog.Default(): a wiring mistake degrades safely instead of
// nil-panicking on the registration path.
func NewGetOrCreateClientUseCase(clients repository.ClientsRepo, lookup repository.RegistrationLookup, limiter *registrationRateLimiter, logger *slog.Logger) *GetOrCreateClientUseCase {
	if limiter == nil {
		limiter = newRegistrationRateLimiter(0, nil)
	}
	return &GetOrCreateClientUseCase{
		clients: clients,
		lookup:  lookup,
		limiter: limiter,
		logger:  registrationLogger(logger),
	}
}

// Execute validates the caller phone, rejects phones owned by a business
// account or an incompatible legacy row, then resolves or creates the client
// row with id == phone.
//
// Order of operations (design §1.3, fail closed: no write happens before every
// check passes):
//  1. phone validation via the entity-owned E.164 subset rule (optional '+',
//     4–15 digits);
//  2. accounts collision check (read-only; active and inactive accounts);
//  3. existing-client short-circuit (id == phone) and legacy-row conflict
//     (id != phone) — the pinned legacy-row decision for this change;
//  4. rate-limit check (design §1.5, per-phone fixed window, fail closed);
//  5. stored-name computation (trim + placeholder + rune cap);
//  6. adapter call GetOrCreateByPhone (creates/returns id == phone);
//  7. audit event client_registered / client_registration_noop;
//  8. return the result DTO.
func (uc *GetOrCreateClientUseCase) Execute(ctx context.Context, callerID, displayName string) (*GetOrCreateClientResult, error) {
	// 1. Validation: the entity-owned E.164 subset rule (optional '+', 4–15
	// digits) is the single source of truth for the registration format.
	if !validRegistrationPhone(callerID) {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el teléfono no tiene un formato válido (se esperan entre 4 y 15 dígitos, con + opcional).",
		}
	}

	// 2. Accounts collision check: an active OR inactive account owns this
	// phone, so it must never become a client. RegistrationLookup is auth-free
	// (no caller exists yet); existence is enough, and this use case is the
	// authority for the rejection.
	accountExists, err := uc.lookup.AccountExistsByID(ctx, callerID)
	switch {
	case err != nil:
		return nil, fmt.Errorf("registrar cliente: verificar cuenta: %w", err)
	case accountExists:
		logRegistrationEvent(uc.logger, registrationEventRejectedAccount, callerID)
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: "este número ya pertenece a una cuenta del negocio. Contactá al administrador.",
		}
	}

	// 3. Existing-client short-circuit / legacy-row conflict.
	identity, err := uc.lookup.FindClientByPhoneAny(ctx, callerID)
	switch {
	case err == nil:
		if identity.ID == callerID {
			// Idempotent registration: no insert, report created=false.
			logRegistrationEvent(uc.logger, registrationEventNoop, callerID)
			return &GetOrCreateClientResult{ClientID: identity.ID, DisplayName: identity.Name, Created: false}, nil
		}
		// Legacy row: its id is a UUID, but the resolver chain matches
		// clients.id == phone, so that row could never be resolved again.
		// Adopting it silently would be worse than failing closed.
		logRegistrationEvent(uc.logger, registrationEventRejectedLegacy, callerID)
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: "este número ya está registrado con un identificador incompatible; contactá al negocio.",
		}
	case !errors.Is(err, domain.ErrNotFound):
		return nil, fmt.Errorf("registrar cliente: buscar por teléfono: %w", err)
	}

	// 4. Rate-limit check (design §1.5): configurable per-phone fixed window,
	// enforced after every read-only rejection and before any write. Exhaustion
	// (and a limit of 0) fails closed with the semantic rate-limit error.
	if !uc.limiter.allow(callerID) {
		logRegistrationEvent(uc.logger, registrationEventRateLimited, callerID)
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeConflict,
			Message: "alcanzaste el límite de registros automáticos. Probá de nuevo más tarde o pedile al negocio que te registre.",
		}
	}

	// 5. Stored-name computation: trimmed display name, else the placeholder.
	storedName := strings.TrimSpace(displayName)
	if storedName == "" {
		storedName = fmt.Sprintf("Cliente %s", callerID)
	}
	if runes := []rune(storedName); len(runes) > registrationMaxNameRunes {
		storedName = string(runes[:registrationMaxNameRunes])
	}

	// 6. Adapter call: inserts or returns the row with id == phone.
	client, created, err := uc.clients.GetOrCreateByPhone(ctx, callerID, storedName)
	if err != nil {
		return nil, fmt.Errorf("registrar cliente: %w", err)
	}

	// 7. Audit the successful outcome: client_registered when this call created
	// the row, client_registration_noop when a concurrent call won the race.
	event := registrationEventRegistered
	if !created {
		event = registrationEventNoop
	}
	logRegistrationEvent(uc.logger, event, callerID)

	// 8. Result DTO.
	return &GetOrCreateClientResult{ClientID: client.ID, DisplayName: client.Name, Created: created}, nil
}

// validRegistrationPhone reports whether phone satisfies the registration
// format rule. That rule IS the entity-owned E.164 subset rule (optional
// leading '+', 4–15 digits), single-sourced in the domain via
// entity.Client.HasValidPhone; this wrapper only documents the registration
// path's intent.
func validRegistrationPhone(phone string) bool {
	c := entity.Client{Phone: phone}
	return c.HasValidPhone()
}
