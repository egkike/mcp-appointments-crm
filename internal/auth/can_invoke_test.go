package auth

import "testing"

// TestCanInvoke pins the single authorization predicate (advisory S1, issue
// #268). It is a table over the THREE row shapes × the FOUR roles, because the
// rule is not "is the role listed" — it is "the role must be listed UNLESS the
// row is absent or empty".
//
// The populated row lists owner only on purpose: it is the shape that can tell
// "listed" apart from "allowed by absence". If it listed every role, a
// predicate that returned true unconditionally would still pass this table.
func TestCanInvoke(t *testing.T) {
	rbac := ToolRBAC{
		"populated": {RoleOwner},
		"empty":     {},
		// "absent" deliberately has no key: the third shape is the missing row.
	}

	allowed := map[string]bool{
		RoleOwner: true, RoleAdmin: true, RoleStaff: true, RoleClient: true,
	}
	ownerOnly := map[string]bool{
		RoleOwner: true, RoleAdmin: false, RoleStaff: false, RoleClient: false,
	}

	cases := []struct {
		shape string
		tool  string
		want  map[string]bool // role → expected verdict
	}{
		{"absent_row", "absent", allowed},
		{"empty_row", "empty", allowed},
		{"populated_row", "populated", ownerOnly},
	}

	for _, tc := range cases {
		for role, want := range tc.want {
			t.Run(tc.shape+"/"+role, func(t *testing.T) {
				if got := CanInvoke(rbac, tc.tool, role); got != want {
					t.Errorf("CanInvoke(%q, %q) = %v; want %v", tc.tool, role, got, want)
				}
			})
		}
	}

	// Membership inside a multi-role row, plus the case sensitivity both former
	// copies had: roles are the Role* constants, so a case-variant or padded
	// string is a DIFFERENT role and must stay refused. Asserting it keeps a
	// future "helpful" normalization from granting a role nobody wrote into the
	// policy.
	t.Run("membership_and_case", func(t *testing.T) {
		shared := ToolRBAC{"shared": {RoleOwner, RoleStaff}}

		for _, role := range []string{RoleOwner, RoleStaff} {
			if !CanInvoke(shared, "shared", role) {
				t.Errorf("CanInvoke(shared, %q) = false; the role is listed", role)
			}
		}
		for _, role := range []string{RoleAdmin, RoleClient} {
			if CanInvoke(shared, "shared", role) {
				t.Errorf("CanInvoke(shared, %q) = true; the role is not listed", role)
			}
		}
		if CanInvoke(ToolRBAC{"populated": {RoleOwner}}, "populated", "Owner") {
			t.Error("CanInvoke accepted a case-variant role; the match must stay case-sensitive")
		}
		if CanInvoke(ToolRBAC{"populated": {" owner"}}, "populated", RoleOwner) {
			t.Error("CanInvoke accepted a padded role; the match must stay byte-exact")
		}
	})
}
