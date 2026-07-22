package ui

import "testing"

// TestApplyObjPermInvariants exercises the dependency-rule enforcement
// for the object-permissions grid. Each test case specifies a starting
// state, the field being toggled (with the toggle already applied before
// calling applyObjPermInvariants), and the expected final state.
func TestApplyObjPermInvariants(t *testing.T) {
	all := func(r, c, e, d, va, ma bool) objPermState {
		return objPermState{
			Read: r, Create: c, Edit: e, Delete: d,
			ViewAllRecords: va, ModifyAllRecords: ma,
		}
	}

	tests := []struct {
		name  string
		start objPermState // state after the toggle, before invariants
		which string
		want  objPermState
	}{
		{
			name:  "toggle Read on when all-off",
			start: all(true, false, false, false, false, false),
			which: "read",
			want:  all(true, false, false, false, false, false),
		},
		{
			name:  "toggle Edit on when all-off",
			start: all(false, false, true, false, false, false),
			which: "edit",
			want:  all(true, false, true, false, false, false),
		},
		{
			name:  "toggle Delete on when all-off",
			start: all(false, false, false, true, false, false),
			which: "delete",
			want:  all(true, false, true, true, false, false),
		},
		{
			name:  "toggle ModifyAll on when all-off",
			start: all(false, false, false, false, false, true),
			which: "modifyall",
			want:  all(true, true, true, true, true, true),
		},
		{
			name:  "toggle Read off when Edit is on",
			start: all(false, false, true, false, false, false),
			which: "read",
			want:  all(false, false, false, false, false, false),
		},
		{
			name:  "toggle ViewAll off when ModifyAll is on",
			start: all(true, true, true, true, false, true),
			which: "viewall",
			want:  all(true, true, true, true, false, false),
		},
		{
			name:  "toggle Edit off cascades Delete and ModifyAll",
			start: all(true, false, false, true, false, true),
			which: "edit",
			want:  all(true, false, false, false, false, false),
		},
		{
			name:  "toggle Read off cascades all",
			start: all(false, false, true, true, true, true),
			which: "read",
			want:  all(false, false, false, false, false, false),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyObjPermInvariants(tc.start, tc.which)
			if got != tc.want {
				t.Errorf("got  R=%v C=%v E=%v D=%v VA=%v MA=%v\nwant R=%v C=%v E=%v D=%v VA=%v MA=%v",
					got.Read, got.Create, got.Edit, got.Delete, got.ViewAllRecords, got.ModifyAllRecords,
					tc.want.Read, tc.want.Create, tc.want.Edit, tc.want.Delete, tc.want.ViewAllRecords, tc.want.ModifyAllRecords)
			}
		})
	}
}
