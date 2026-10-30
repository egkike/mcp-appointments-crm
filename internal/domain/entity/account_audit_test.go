package entity

import (
	"reflect"
	"testing"
)

// TestAuditDetailIsWireFree pins the domain-boundary rule (GGA round 3 of the
// follow-ups commit): entity types carry NO persistence tags; the JSON shape
// belongs to the repository adapter's DTO, so the domain stays wire-free.
func TestAuditDetailIsWireFree(t *testing.T) {
	tType := reflect.TypeOf(AccountAuditDetail{})
	for i := 0; i < tType.NumField(); i++ {
		if tag, has := tType.Field(i).Tag.Lookup("json"); has {
			t.Errorf("entity field %s carries a persistence json tag %q - the wire shape belongs in the repository DTO", tType.Field(i).Name, tag)
		}
	}
}
