package usecase

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
)

// ─── reflect-based parity guard for the parallel field enumerations ──────────
//
// UpdateBusinessProfileInput is enumerated in three places that must stay in
// lockstep: hasProfileUpdates (the empty-update guard),
// applyProfileScalarUpdates and applyProfileReferenceUpdates (the two
// partial-merge appliers). The per-column table in
// update_business_profile_test.go pins the merge semantics, but it is itself a
// manual enumeration: a field added to the DTO without being added to the guard
// is silently dropped (accepted input, no effect), and a field added to an
// applier without the guard is rejected as an empty update. This test iterates
// the struct with reflection instead, so a future field is automatically
// covered the moment it is declared — and fails until all three enumerations
// list it.

// clearFlagColumns maps the two explicit-null input flags to the coordinate
// column handle they null out in mergedColumns rendering.
var clearFlagColumns = map[string]string{
	"ClearLatitude":  "latitude",
	"ClearLongitude": "longitude",
}

// toProfileColumn maps an input field name to the column handle used by
// mergedColumns: camelCase to snake_case, with the two clear flags resolving to
// the coordinates they clear.
func toProfileColumn(field string) string {
	if column, ok := clearFlagColumns[field]; ok {
		return column
	}
	runes := []rune(field)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 && (!unicode.IsUpper(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// sentinelInput builds an input that declares only field at index i. Each
// supported field type gets a value guaranteed to differ from every mergeSeed
// stored value, so the applied merge is observable as a column diff. A future
// field of an unhandled type fails the parity test explicitly instead of being
// skipped silently.
func sentinelInput(t *testing.T, inputType reflect.Type, index int, name string) dto.UpdateBusinessProfileInput {
	t.Helper()
	input := dto.UpdateBusinessProfileInput{}
	target := reflect.ValueOf(&input).Elem().Field(index)
	switch target.Type() {
	case reflect.TypeFor[bool]():
		target.SetBool(true)
	case reflect.TypeFor[*string]():
		s := "sentinel-" + name
		target.Set(reflect.ValueOf(&s))
	case reflect.TypeFor[*float64]():
		f := -123.456
		target.Set(reflect.ValueOf(&f))
	case reflect.TypeFor[*int]():
		n := 999
		target.Set(reflect.ValueOf(&n))
	default:
		t.Fatalf("parity test has no sentinel case for input field %q of type %s: extend sentinelInput", name, target.Type())
	}
	return input
}

// TestUpdateBusinessProfileInput_FieldParity walks every exported field of the
// partial-merge DTO (except the Caller identity field) and requires:
//  1. hasProfileUpdates counts the declared field as an update — the guard can
//     never reject a payload whose only field is a declared one;
//  2. applyProfileUpdates changes exactly the field's own profile column — a
//     declared field can never be accepted and then silently dropped.
func TestUpdateBusinessProfileInput_FieldParity(t *testing.T) {
	inputType := reflect.TypeFor[dto.UpdateBusinessProfileInput]()

	for i := 0; i < inputType.NumField(); i++ {
		field := inputType.Field(i)
		if field.Name == "Caller" {
			continue // identity field, deliberately not part of the payload
		}

		t.Run(field.Name, func(t *testing.T) {
			input := sentinelInput(t, inputType, i, field.Name)
			column := toProfileColumn(field.Name)

			if !hasProfileUpdates(input) {
				t.Errorf("hasProfileUpdates does not count field %s: a payload carrying only %s is rejected as an empty update", field.Name, field.Name)
			}

			before := mergeSeed()
			after := mergeSeed()
			applyProfileUpdates(after, input)
			changed := changedColumns(mergedColumns(before), mergedColumns(after))

			if len(changed) == 0 {
				t.Fatalf("field %s passes the guard but no profile column changed: it is silently dropped by applyProfileUpdates", field.Name)
			}
			if len(changed) != 1 || changed[0] != column {
				t.Fatalf("field %s changed columns %v, want exactly [%s]", field.Name, changed, column)
			}
		})
	}
}
