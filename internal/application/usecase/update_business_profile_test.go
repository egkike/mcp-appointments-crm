package usecase

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ─── Partial-merge semantics of applyProfileUpdates ────────────────────────

// mergeSeed is a stored profile whose every optional column carries a distinct
// value, so a merge that overwrites one column while silently dropping another
// is visible as a diff against this seed.
func mergeSeed() *entity.BusinessProfile {
	return &entity.BusinessProfile{
		ID:                     "singleton",
		Name:                   "Peluquería Vieja",
		Industry:               ptr("salud"),
		Country:                ptr("AR"),
		Address:                ptr("Calle Vieja 1"),
		Latitude:               f64(-34.6),
		Longitude:              f64(-58.4),
		CoverPhotoURL:          ptr("https://viejo.example.com/c.jpg"),
		PublicPhone:            ptr("+5491100000001"),
		MessengerPlatform:      ptr("whatsapp"),
		MessengerID:            ptr("5491100000001"),
		ContactEmail:           ptr("viejo@example.com"),
		WebsiteURL:             ptr("https://viejo.example.com"),
		GeneralDescription:     ptr("descripción vieja"),
		CurrencyCode:           "ARS",
		CurrencySymbol:         "$",
		AcceptedPaymentMethods: ptr(`["cash"]`),
		Timezone:               "America/Argentina/Buenos_Aires",
		SlotIntervalMinutes:    30,
		BusinessHours:          `{"1":{"open":"09:00","close":"18:00"}}`,
	}
}

// f64 mirrors ptr for the optional float columns.
func f64(v float64) *float64 { return &v }

// intPtr mirrors ptr for the optional int columns.
func intPtr(v int) *int { return &v }

// mergedColumns renders every optional profile column as a "column=value" line.
// The rendering is explicit (no reflection) and total (all 19 columns always
// present, in a fixed order), so two profiles can be diffed column by column
// even when they share no pointer identity.
func mergedColumns(p *entity.BusinessProfile) []string {
	text := func(v *string) string {
		if v == nil {
			return "<nil>"
		}
		return *v
	}
	number := func(v *float64) string {
		if v == nil {
			return "<nil>"
		}
		return strconv.FormatFloat(*v, 'g', -1, 64)
	}
	return []string{
		"name=" + p.Name,
		"industry=" + text(p.Industry),
		"country=" + text(p.Country),
		"address=" + text(p.Address),
		"latitude=" + number(p.Latitude),
		"longitude=" + number(p.Longitude),
		"cover_photo_url=" + text(p.CoverPhotoURL),
		"public_phone=" + text(p.PublicPhone),
		"messenger_platform=" + text(p.MessengerPlatform),
		"messenger_id=" + text(p.MessengerID),
		"contact_email=" + text(p.ContactEmail),
		"website_url=" + text(p.WebsiteURL),
		"general_description=" + text(p.GeneralDescription),
		"currency_code=" + p.CurrencyCode,
		"currency_symbol=" + p.CurrencySymbol,
		"accepted_payment_methods=" + text(p.AcceptedPaymentMethods),
		"timezone=" + p.Timezone,
		"slot_interval_minutes=" + strconv.Itoa(p.SlotIntervalMinutes),
		"business_hours=" + p.BusinessHours,
	}
}

// changedColumns returns the column names whose rendered value differs between
// before and after. The two snapshots always cover the same columns in the same
// order, so the comparison is positional.
func changedColumns(before, after []string) []string {
	var changed []string
	for i, line := range before {
		if line != after[i] {
			changed = append(changed, strings.SplitN(line, "=", 2)[0])
		}
	}
	return changed
}

// TestApplyProfileUpdates_PartialMerge pins the merge contract for every
// optional column: a nil field keeps the stored value, and a non-nil field
// changes exactly its own column and nothing else. It covers both semantics
// groups — deref-assigned scalars and verbatim-copied pointers — individually,
// which is what makes the helper split safe.
func TestApplyProfileUpdates_PartialMerge(t *testing.T) {
	seed := mergedColumns(mergeSeed())

	tests := []struct {
		name   string
		input  dto.UpdateBusinessProfileInput
		column string // the single column the input must change
		value  string // the value that column must hold afterwards
	}{
		{
			name:  "empty input keeps every stored value",
			input: dto.UpdateBusinessProfileInput{},
			// column/value empty: no column may change.
		},

		// Group (b): verbatim pointer copies.
		{name: "industry", input: dto.UpdateBusinessProfileInput{Industry: ptr("retail")}, column: "industry", value: "retail"},
		{name: "country", input: dto.UpdateBusinessProfileInput{Country: ptr("UY")}, column: "country", value: "UY"},
		{name: "address", input: dto.UpdateBusinessProfileInput{Address: ptr("Calle Nueva 1")}, column: "address", value: "Calle Nueva 1"},
		{name: "latitude", input: dto.UpdateBusinessProfileInput{Latitude: f64(-33.9)}, column: "latitude", value: "-33.9"},
		{name: "longitude", input: dto.UpdateBusinessProfileInput{Longitude: f64(-56.2)}, column: "longitude", value: "-56.2"},
		{name: "cover_photo_url", input: dto.UpdateBusinessProfileInput{CoverPhotoURL: ptr("https://nuevo.example.com/c.jpg")}, column: "cover_photo_url", value: "https://nuevo.example.com/c.jpg"},
		{name: "public_phone", input: dto.UpdateBusinessProfileInput{PublicPhone: ptr("+5491100000002")}, column: "public_phone", value: "+5491100000002"},
		{name: "messenger_platform", input: dto.UpdateBusinessProfileInput{MessengerPlatform: ptr("telegram")}, column: "messenger_platform", value: "telegram"},
		{name: "messenger_id", input: dto.UpdateBusinessProfileInput{MessengerID: ptr("5491100000002")}, column: "messenger_id", value: "5491100000002"},
		{name: "contact_email", input: dto.UpdateBusinessProfileInput{ContactEmail: ptr("nuevo@example.com")}, column: "contact_email", value: "nuevo@example.com"},
		{name: "website_url", input: dto.UpdateBusinessProfileInput{WebsiteURL: ptr("https://nuevo.example.com")}, column: "website_url", value: "https://nuevo.example.com"},
		{name: "general_description", input: dto.UpdateBusinessProfileInput{GeneralDescription: ptr("descripción nueva")}, column: "general_description", value: "descripción nueva"},
		{name: "accepted_payment_methods", input: dto.UpdateBusinessProfileInput{AcceptedPaymentMethods: ptr(`["card"]`)}, column: "accepted_payment_methods", value: `["card"]`},

		// Group (a): deref-assigned scalars.
		{name: "name", input: dto.UpdateBusinessProfileInput{Name: ptr("Peluquería Nueva")}, column: "name", value: "Peluquería Nueva"},
		{name: "currency_code", input: dto.UpdateBusinessProfileInput{CurrencyCode: ptr("USD")}, column: "currency_code", value: "USD"},
		{name: "currency_symbol", input: dto.UpdateBusinessProfileInput{CurrencySymbol: ptr("US$")}, column: "currency_symbol", value: "US$"},
		{name: "timezone", input: dto.UpdateBusinessProfileInput{Timezone: ptr("UTC")}, column: "timezone", value: "UTC"},
		{name: "slot_interval_minutes", input: dto.UpdateBusinessProfileInput{SlotIntervalMinutes: intPtr(45)}, column: "slot_interval_minutes", value: "45"},
		{name: "business_hours", input: dto.UpdateBusinessProfileInput{BusinessHours: ptr(`{"2":{"open":"10:00","close":"19:00"}}`)}, column: "business_hours", value: `{"2":{"open":"10:00","close":"19:00"}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeSeed()
			applyProfileUpdates(got, tc.input)

			var wantChanged []string
			if tc.column != "" {
				wantChanged = []string{tc.column}
			}
			columns := mergedColumns(got)
			if diff := changedColumns(seed, columns); !slices.Equal(diff, wantChanged) {
				t.Fatalf("changed columns = %v, want %v (merged profile: %v)", diff, wantChanged, columns)
			}
			if tc.column == "" {
				return
			}
			want := tc.column + "=" + tc.value
			for _, line := range columns {
				if strings.HasPrefix(line, tc.column+"=") {
					if line != want {
						t.Errorf("%s = %q, want %q", tc.column, line, want)
					}
					return
				}
			}
			t.Errorf("column %q missing from the merged profile", tc.column)
		})
	}
}

// ─── Explicit-null clear of the location columns (F-4) ─────────────────────

// TestUpdateBusinessProfileUseCase_ClearLocation pins the explicit-null
// contract at the use-case boundary: a clear flag is a real update, so it passes
// the empty-update guard, writes nil on the entity (SQL NULL on the full-row
// repository Update) and leaves every other column untouched. The same table
// pins the two neighboring rules: a flagless empty payload still rejects with
// the semantic message, and a nil location pointer without its flag still means
// "keep the stored coordinate".
func TestUpdateBusinessProfileUseCase_ClearLocation(t *testing.T) {
	seed := mergedColumns(mergeSeed())

	tests := []struct {
		name        string
		input       dto.UpdateBusinessProfileInput
		wantErr     bool
		wantChanged []string
	}{
		{
			name:        "clear both coordinates",
			input:       dto.UpdateBusinessProfileInput{Caller: ownerCaller(), ClearLatitude: true, ClearLongitude: true},
			wantChanged: []string{"latitude", "longitude"},
		},
		{
			name:        "clear latitude only",
			input:       dto.UpdateBusinessProfileInput{Caller: ownerCaller(), ClearLatitude: true},
			wantChanged: []string{"latitude"},
		},
		{
			name:        "clear longitude only",
			input:       dto.UpdateBusinessProfileInput{Caller: ownerCaller(), ClearLongitude: true},
			wantChanged: []string{"longitude"},
		},
		{
			name:    "flagless empty payload still rejects",
			input:   dto.UpdateBusinessProfileInput{Caller: ownerCaller()},
			wantErr: true,
		},
		{
			// The pre-F-4 rule must survive: a nil pointer on its own is an
			// absent field, not a clear.
			name:        "nil coordinates without flags keep the stored values",
			input:       dto.UpdateBusinessProfileInput{Caller: ownerCaller(), Name: ptr("Peluquería Nueva")},
			wantChanged: []string{"name"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var saved *entity.BusinessProfile
			repo := &mockBusinessProfileRepo{
				GetFn: func(context.Context) (*entity.BusinessProfile, error) { return mergeSeed(), nil },
				UpdateFn: func(_ context.Context, p *entity.BusinessProfile) error {
					saved = p
					return nil
				},
			}
			uc := NewUpdateBusinessProfileUseCase(repo, nil)

			result, err := uc.Execute(context.Background(), tc.input)
			if tc.wantErr {
				if saved != nil {
					t.Fatal("Update must not be called for a payload that carries no updates")
				}
				assertSemanticError(t, err, domain.ErrCodeInvalidInput, "no se proporcionaron campos para actualizar")
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if saved == nil {
				t.Fatal("Update was not called")
			}
			if result != saved {
				t.Error("result is not the merged entity that was persisted")
			}
			if diff := changedColumns(seed, mergedColumns(saved)); !slices.Equal(diff, tc.wantChanged) {
				t.Fatalf("changed columns = %v, want %v (merged profile: %v)", diff, tc.wantChanged, mergedColumns(saved))
			}
		})
	}
}

// TestUpdateBusinessProfileUseCase_InvalidInputMessageMapping pins the error
// mapping split introduced with the coordinate range guards: a
// *domain.SemanticError raised by the repository (wrapped by it, as production
// does) reaches the caller with its own LLM-facing Spanish message, while a
// generic domain.ErrInvalidInput failure — the pre-existing business_hours,
// timezone, payment-method and messenger-platform rules — keeps the shared
// collapse. The generic half proves the pass-through did not regress them.
func TestUpdateBusinessProfileUseCase_InvalidInputMessageMapping(t *testing.T) {
	coordinateErr := &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: "la latitud debe estar entre -90 y 90",
		Cause:   domain.ErrInvalidInput,
	}

	tests := []struct {
		name        string
		repoErr     error
		wantMessage string
	}{
		{
			name:        "coordinate semantic error passes through verbatim",
			repoErr:     fmt.Errorf("actualizar perfil de negocio: %w", coordinateErr),
			wantMessage: "la latitud debe estar entre -90 y 90",
		},
		{
			name:        "generic invalid input still collapses to the shared message",
			repoErr:     fmt.Errorf("actualizar perfil de negocio: %w", domain.ErrInvalidInput),
			wantMessage: "los datos del perfil no son válidos: revisá los horarios, la zona horaria, los métodos de pago y la plataforma de mensajería",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockBusinessProfileRepo{
				GetFn:    func(context.Context) (*entity.BusinessProfile, error) { return mergeSeed(), nil },
				UpdateFn: func(context.Context, *entity.BusinessProfile) error { return tt.repoErr },
			}
			uc := NewUpdateBusinessProfileUseCase(repo, nil)

			_, err := uc.Execute(context.Background(), dto.UpdateBusinessProfileInput{
				Caller:   ownerCaller(),
				Latitude: f64(200),
			})
			assertSemanticError(t, err, domain.ErrCodeInvalidInput, tt.wantMessage)
		})
	}
}
