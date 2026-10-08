package usecase

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Registration fixtures. validPhone carries 13 digits; maxPhone is the E.164
// registration boundary (exactly 15 digits, no plus); tooLongPhone is one digit
// past it (16 digits).
const (
	validPhone   = "+5491100999999"
	maxPhone     = "123456789012345"
	tooLongPhone = "1234567890123456"
)

// newMissingRegistrations returns a RegistrationLookup mock for the common
// registration path: no account owns the phone and no client row exists.
func newMissingRegistrations() *mockRegistrationLookup {
	return &mockRegistrationLookup{
		AccountExistsByIDFn: func(context.Context, string) (bool, error) {
			return false, nil
		},
		FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
			return nil, domain.ErrNotFound
		},
	}
}

// requireSemanticError asserts err is (or wraps) a *domain.SemanticError with
// the given code and a non-empty Spanish message.
func requireSemanticError(t *testing.T, err error, wantCode domain.ErrCode) *domain.SemanticError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sem *domain.SemanticError
	if !errors.As(err, &sem) {
		t.Fatalf("error type = %T; want *domain.SemanticError (err=%v)", err, err)
	}
	if sem.Code != wantCode {
		t.Errorf("SemanticError.Code = %q; want %q", sem.Code, wantCode)
	}
	if strings.TrimSpace(sem.Message) == "" {
		t.Error("SemanticError.Message must carry a Spanish message, got empty")
	}
	return sem
}

// newTestUsecase builds the registration use case for the validation,
// collision and placeholder tests: a limiter wide enough not to interfere and
// a discarded audit logger.
func newTestUsecase(clients *mockClientsRepo, lookup *mockRegistrationLookup) *GetOrCreateClientUseCase {
	return NewGetOrCreateClientUseCase(clients, lookup, newRegistrationRateLimiter(100, time.Now), slog.New(slog.DiscardHandler))
}

// captureClientStore returns a clients mock whose GetOrCreateByPhone records
// the phone and name it was called with, paired with a RegistrationLookup mock
// in the common "no account, no client" state.
func captureClientStore(onStore func(ctx context.Context, phone, name string) (entity.Client, bool, error)) (*mockClientsRepo, *mockRegistrationLookup, *string, *string) {
	storedPhone := new(string)
	storedName := new(string)
	clients := &mockClientsRepo{
		GetOrCreateByPhoneFn: func(ctx context.Context, phone, name string) (entity.Client, bool, error) {
			*storedPhone = phone
			*storedName = name
			return onStore(ctx, phone, name)
		},
	}
	return clients, newMissingRegistrations(), storedPhone, storedName
}

func TestGetOrCreateClientUseCase(t *testing.T) {
	t.Run("invalid phone is rejected and no repo is touched", func(t *testing.T) {
		for _, phone := range []string{"", "abc", "+123", "12 34"} {
			touched := false
			clients := &mockClientsRepo{
				GetOrCreateByPhoneFn: func(context.Context, string, string) (entity.Client, bool, error) {
					touched = true
					return entity.Client{}, false, nil
				},
			}
			lookup := &mockRegistrationLookup{
				AccountExistsByIDFn: func(context.Context, string) (bool, error) {
					touched = true
					return false, nil
				},
				FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
					touched = true
					return nil, domain.ErrNotFound
				},
			}
			uc := newTestUsecase(clients, lookup)

			_, err := uc.Execute(context.Background(), phone, "")
			sem := requireSemanticError(t, err, domain.ErrCodeInvalidInput)
			if !strings.Contains(sem.Message, "teléfono") {
				t.Errorf("phone %q: message = %q; want the invalid-format Spanish error", phone, sem.Message)
			}
			if touched {
				t.Errorf("phone %q: no repository access may happen before validation passes", phone)
			}
		}
	})

	t.Run("16-digit phone is rejected; 15-digit phone registers", func(t *testing.T) {
		touched := false
		rejectClients := &mockClientsRepo{
			GetOrCreateByPhoneFn: func(context.Context, string, string) (entity.Client, bool, error) {
				touched = true
				return entity.Client{}, false, nil
			},
		}
		rejectLookup := &mockRegistrationLookup{
			AccountExistsByIDFn: func(context.Context, string) (bool, error) {
				touched = true
				return false, nil
			},
			FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
				touched = true
				return nil, domain.ErrNotFound
			},
		}
		rejectUC := newTestUsecase(rejectClients, rejectLookup)

		_, err := rejectUC.Execute(context.Background(), tooLongPhone, "")
		sem := requireSemanticError(t, err, domain.ErrCodeInvalidInput)
		if !strings.Contains(sem.Message, "teléfono") {
			t.Errorf("message = %q; want the invalid-format Spanish error", sem.Message)
		}
		if touched {
			t.Error("a 16-digit phone must be rejected before any repository access")
		}

		// Exactly 15 digits is accepted (the cap is inclusive).
		clients, lookup, _, _ := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		acceptUC := newTestUsecase(clients, lookup)
		res, err := acceptUC.Execute(context.Background(), maxPhone, "")
		if err != nil {
			t.Fatalf("15-digit phone must register, got error: %v", err)
		}
		if !res.Created || res.ClientID != maxPhone {
			t.Errorf("got %+v; want created client with id %q", res, maxPhone)
		}
	})

	t.Run("phone owned by an accounts row is rejected", func(t *testing.T) {
		clientTouched := false
		lookup := &mockRegistrationLookup{
			AccountExistsByIDFn: func(context.Context, string) (bool, error) {
				return true, nil
			},
			FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
				clientTouched = true
				return nil, domain.ErrNotFound
			},
		}
		clients := &mockClientsRepo{
			GetOrCreateByPhoneFn: func(context.Context, string, string) (entity.Client, bool, error) {
				clientTouched = true
				return entity.Client{}, false, nil
			},
		}
		uc := newTestUsecase(clients, lookup)

		_, err := uc.Execute(context.Background(), validPhone, "")
		sem := requireSemanticError(t, err, domain.ErrCodeConflict)
		if !strings.Contains(sem.Message, "cuenta") {
			t.Errorf("message = %q; want the account-collision Spanish error", sem.Message)
		}
		if clientTouched {
			t.Error("the client port must not be touched when the phone belongs to an account")
		}
	})

	t.Run("account lookup failure is propagated wrapped", func(t *testing.T) {
		boom := errors.New("accounts table unavailable")
		clients := &mockClientsRepo{
			GetOrCreateByPhoneFn: func(context.Context, string, string) (entity.Client, bool, error) {
				return entity.Client{}, false, nil
			},
		}
		lookup := &mockRegistrationLookup{
			AccountExistsByIDFn: func(context.Context, string) (bool, error) {
				return false, boom
			},
			FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
				return nil, domain.ErrNotFound
			},
		}
		uc := newTestUsecase(clients, lookup)

		if _, err := uc.Execute(context.Background(), validPhone, ""); !errors.Is(err, boom) {
			t.Errorf("err = %v; want it to wrap the lookup failure", err)
		}
	})

	t.Run("existing client with id == phone is returned without insert", func(t *testing.T) {
		inserted := false
		lookup := &mockRegistrationLookup{
			AccountExistsByIDFn: func(context.Context, string) (bool, error) {
				return false, nil
			},
			FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
				return &entity.Client{ID: validPhone, Name: "Ana", Phone: validPhone}, nil
			},
		}
		clients := &mockClientsRepo{
			GetOrCreateByPhoneFn: func(context.Context, string, string) (entity.Client, bool, error) {
				inserted = true
				return entity.Client{}, false, nil
			},
		}
		uc := newTestUsecase(clients, lookup)

		res, err := uc.Execute(context.Background(), validPhone, "Otro nombre")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Created {
			t.Error("existing client must report created=false")
		}
		if res.ClientID != validPhone || res.DisplayName != "Ana" {
			t.Errorf("got %+v; want the stored existing row", res)
		}
		if inserted {
			t.Error("GetOrCreateByPhone must not be called when the client already exists")
		}
	})

	t.Run("legacy row with id != phone is an explicit conflict and is never adopted", func(t *testing.T) {
		inserted := false
		lookup := &mockRegistrationLookup{
			AccountExistsByIDFn: func(context.Context, string) (bool, error) {
				return false, nil
			},
			FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
				return &entity.Client{ID: "550e8400-e29b-41d4-a716-446655440000", Name: "Legacy", Phone: validPhone}, nil
			},
		}
		clients := &mockClientsRepo{
			GetOrCreateByPhoneFn: func(context.Context, string, string) (entity.Client, bool, error) {
				inserted = true
				return entity.Client{}, false, nil
			},
		}
		uc := newTestUsecase(clients, lookup)

		var auditBuf bytes.Buffer
		auditUC := NewGetOrCreateClientUseCase(clients, lookup, newRegistrationRateLimiter(100, time.Now), slog.New(slog.NewTextHandler(&auditBuf, nil)))
		_, _ = auditUC.Execute(context.Background(), validPhone, "")
		if !strings.Contains(auditBuf.String(), "registration_rejected_legacy_id") {
			t.Error("legacy rejection must be audited")
		}
		if strings.Contains(auditBuf.String(), validPhone) {
			t.Error("audit must not contain the unmasked phone")
		}

		_, err := uc.Execute(context.Background(), validPhone, "")
		sem := requireSemanticError(t, err, domain.ErrCodeConflict)
		if !strings.Contains(sem.Message, "número") {
			t.Errorf("message = %q; want the incompatible-identifier Spanish error", sem.Message)
		}
		if inserted {
			t.Error("a legacy row must never be adopted via GetOrCreateByPhone")
		}
	})

	t.Run("omitted name stores the Cliente {phone} placeholder", func(t *testing.T) {
		clients, lookup, storedPhone, storedName := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		uc := newTestUsecase(clients, lookup)

		res, err := uc.Execute(context.Background(), validPhone, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "Cliente " + validPhone
		if *storedName != want {
			t.Errorf("stored name = %q; want %q", *storedName, want)
		}
		if *storedPhone != validPhone {
			t.Errorf("stored phone = %q; want the byte-identical header %q", *storedPhone, validPhone)
		}
		if res.DisplayName != want || !res.Created {
			t.Errorf("got %+v; want created client named %q", res, want)
		}
	})

	t.Run("supplied name is trimmed before store", func(t *testing.T) {
		clients, lookup, _, storedName := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		uc := newTestUsecase(clients, lookup)

		if _, err := uc.Execute(context.Background(), validPhone, "  Ana Gómez  "); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *storedName != "Ana Gómez" {
			t.Errorf("stored name = %q; want %q", *storedName, "Ana Gómez")
		}
	})

	t.Run("whitespace-only name falls back to the placeholder", func(t *testing.T) {
		clients, lookup, _, storedName := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		uc := newTestUsecase(clients, lookup)

		if _, err := uc.Execute(context.Background(), validPhone, "   "); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "Cliente " + validPhone; *storedName != want {
			t.Errorf("stored name = %q; want %q", *storedName, want)
		}
	})

	t.Run("name longer than 80 runes is truncated to 80", func(t *testing.T) {
		clients, lookup, _, storedName := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		uc := newTestUsecase(clients, lookup)

		long := strings.Repeat("á", 90)
		if _, err := uc.Execute(context.Background(), validPhone, long); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := len([]rune(*storedName)); got != 80 {
			t.Errorf("stored name has %d runes; want 80 (deterministic cap)", got)
		}
	})

	t.Run("created registration is audited with a masked phone only", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		clients, lookup, _, _ := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		uc := NewGetOrCreateClientUseCase(clients, lookup, newRegistrationRateLimiter(10, time.Now), logger)

		if _, err := uc.Execute(context.Background(), validPhone, "Ana"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, registrationEventRegistered) {
			t.Errorf("audit log = %q; want the %s event", out, registrationEventRegistered)
		}
		if !strings.Contains(out, "***9999") {
			t.Errorf("audit log = %q; want the masked phone (last 4 digits)", out)
		}
		if strings.Contains(out, validPhone) {
			t.Errorf("audit log must never contain the full phone %q", validPhone)
		}
	})

	t.Run("existing client emits the no-op audit event with a masked phone", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		lookup := &mockRegistrationLookup{
			AccountExistsByIDFn: func(context.Context, string) (bool, error) { return false, nil },
			FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) {
				return &entity.Client{ID: validPhone, Name: "Ana", Phone: validPhone}, nil
			},
		}
		uc := NewGetOrCreateClientUseCase(&mockClientsRepo{}, lookup, newRegistrationRateLimiter(10, time.Now), logger)

		if _, err := uc.Execute(context.Background(), validPhone, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, registrationEventNoop) {
			t.Errorf("audit log = %q; want the %s event", out, registrationEventNoop)
		}
		if !strings.Contains(out, "***9999") || strings.Contains(out, validPhone) {
			t.Errorf("audit log = %q; want only the masked phone", out)
		}
	})

	t.Run("account rejection emits the rejection audit event with a masked phone", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		lookup := &mockRegistrationLookup{
			AccountExistsByIDFn:    func(context.Context, string) (bool, error) { return true, nil },
			FindClientByPhoneAnyFn: func(context.Context, string) (*entity.Client, error) { return nil, domain.ErrNotFound },
		}
		uc := NewGetOrCreateClientUseCase(&mockClientsRepo{}, lookup, newRegistrationRateLimiter(10, time.Now), logger)

		if _, err := uc.Execute(context.Background(), validPhone, ""); err == nil {
			t.Fatal("an account-owned phone must be rejected")
		}
		out := buf.String()
		if !strings.Contains(out, registrationEventRejectedAccount) {
			t.Errorf("audit log = %q; want the %s event", out, registrationEventRejectedAccount)
		}
		if !strings.Contains(out, "***9999") || strings.Contains(out, validPhone) {
			t.Errorf("audit log = %q; want only the masked phone", out)
		}
	})

	t.Run("exhausted limit rejects, audits and writes nothing", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		now := time.Now()
		limiter := newRegistrationRateLimiter(1, func() time.Time { return now })
		inserted := 0
		clients, lookup, _, _ := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			inserted++
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		uc := NewGetOrCreateClientUseCase(clients, lookup, limiter, logger)

		if _, err := uc.Execute(context.Background(), validPhone, ""); err != nil {
			t.Fatalf("first attempt must succeed: %v", err)
		}
		_, err := uc.Execute(context.Background(), validPhone, "")
		sem := requireSemanticError(t, err, domain.ErrCodeConflict)
		if !strings.Contains(sem.Message, "límite") {
			t.Errorf("message = %q; want the rate-limit Spanish error", sem.Message)
		}
		if inserted != 1 {
			t.Errorf("GetOrCreateByPhone calls = %d; want 1 (no write on the rejected attempt)", inserted)
		}
		if !strings.Contains(buf.String(), registrationEventRateLimited) {
			t.Errorf("audit log = %q; want the %s event", buf.String(), registrationEventRateLimited)
		}
	})

	t.Run("limit zero disables registration and writes nothing", func(t *testing.T) {
		inserted := false
		clients, lookup, _, _ := captureClientStore(func(_ context.Context, phone, name string) (entity.Client, bool, error) {
			inserted = true
			return entity.Client{ID: phone, Name: name, Phone: phone}, true, nil
		})
		uc := NewGetOrCreateClientUseCase(clients, lookup, newRegistrationRateLimiter(0, time.Now), slog.New(slog.DiscardHandler))

		_, err := uc.Execute(context.Background(), validPhone, "")
		_ = requireSemanticError(t, err, domain.ErrCodeConflict)
		if inserted {
			t.Error("limit 0 must fail closed before any write")
		}
	})
}
