package sfurl

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  Parsed
		errOK bool // true if Parse should return an error
	}{
		{
			name:  "empty input",
			input: "",
			errOK: true,
		},
		{
			name:  "non-salesforce URL",
			input: "https://example.com/foo",
			errOK: true,
		},
		{
			name:  "bare 18-char Id, ApexClass prefix",
			input: "01p5g00000ABCDEAAA",
			want: Parsed{
				Kind:  devproject.KindApexClass,
				ID:    "01p5g00000ABCDEAAA",
				Extra: map[string]string{},
				Raw:   "01p5g00000ABCDEAAA",
			},
		},
		{
			name:  "bare 15-char Id, Account prefix → KindRecord (no SObject inferred)",
			input: "0011x00000ABCDE",
			want: Parsed{
				Kind:  devproject.KindRecord,
				ID:    "0011x00000ABCDE",
				Extra: map[string]string{},
				Raw:   "0011x00000ABCDE",
			},
		},
		{
			name:  "lightning record URL",
			input: "https://acme.lightning.force.com/lightning/r/Account/0011x00000ABCDE/view",
			want: Parsed{
				Kind:    devproject.KindRecord,
				SObject: "Account",
				ID:      "0011x00000ABCDE",
				Host:    "acme.lightning.force.com",
				Extra:   map[string]string{},
			},
		},
		{
			name:  "lightning sObject list",
			input: "https://acme.lightning.force.com/lightning/o/Contact/list",
			want: Parsed{
				Kind:    devproject.KindSObject,
				SObject: "Contact",
				Host:    "acme.lightning.force.com",
				Extra:   map[string]string{},
			},
		},
		{
			name:  "lightning sObject list with filterName",
			input: "https://acme.lightning.force.com/lightning/o/Account/list?filterName=00B5g00000ABCDE",
			want: Parsed{
				Kind:    devproject.KindSObject,
				SObject: "Account",
				ID:      "00B5g00000ABCDE",
				Host:    "acme.lightning.force.com",
				Extra:   map[string]string{"listViewId": "00B5g00000ABCDE"},
			},
		},
		{
			name:  "ObjectManager Details",
			input: "https://acme.lightning.force.com/lightning/setup/ObjectManager/Account/Details/view",
			want: Parsed{
				Kind:    devproject.KindSObject,
				SObject: "Account",
				Host:    "acme.lightning.force.com",
				Extra:   map[string]string{},
			},
		},
		{
			name:  "ObjectManager FieldsAndRelationships",
			input: "https://acme.lightning.force.com/lightning/setup/ObjectManager/Account/FieldsAndRelationships/00N5g00000ABCDE/view",
			want: Parsed{
				Kind:    devproject.KindField,
				SObject: "Account",
				ID:      "00N5g00000ABCDE",
				Host:    "acme.lightning.force.com",
				Extra:   map[string]string{"fieldId": "00N5g00000ABCDE"},
			},
		},
		{
			name:  "Setup Flows page with embedded id",
			input: "https://acme.lightning.force.com/lightning/setup/Flows/page?address=%2F3005g00000ABCDE",
			want: Parsed{
				Kind:  devproject.KindFlow,
				ID:    "3005g00000ABCDE",
				Host:  "acme.lightning.force.com",
				Extra: map[string]string{},
			},
		},
		{
			name:  "Setup PermSets page",
			input: "https://acme.lightning.force.com/lightning/setup/PermSets/page?address=%2F0PS5g00000ABCDE",
			want: Parsed{
				Kind:  devproject.KindPermissionSet,
				ID:    "0PS5g00000ABCDE",
				Host:  "acme.lightning.force.com",
				Extra: map[string]string{},
			},
		},
		{
			name:  "sandbox host",
			input: "https://acme--uat.sandbox.lightning.force.com/lightning/r/Account/0011x00000ABCDE/view",
			want: Parsed{
				Kind:    devproject.KindRecord,
				SObject: "Account",
				ID:      "0011x00000ABCDE",
				Host:    "acme--uat.sandbox.lightning.force.com",
				Sandbox: "uat",
				Extra:   map[string]string{},
			},
		},
		{
			name:  "classic my.salesforce.com record URL",
			input: "https://acme.my.salesforce.com/0011x00000ABCDE",
			want: Parsed{
				Kind:  devproject.KindRecord,
				ID:    "0011x00000ABCDE",
				Host:  "acme.my.salesforce.com",
				Extra: map[string]string{},
			},
		},
		{
			name:  "ListView Id (00B prefix) routes to SObject + listViewId",
			input: "00B5g00000ABCDE",
			want: Parsed{
				Kind:  devproject.KindSObject,
				ID:    "00B5g00000ABCDE",
				Extra: map[string]string{"listViewId": "00B5g00000ABCDE"},
				Raw:   "00B5g00000ABCDE",
			},
		},
		{
			name:  "garbage URL",
			input: "https://example.com/foo/bar",
			errOK: true,
		},
		{
			name:  "salesforce host but unknown path",
			input: "https://acme.lightning.force.com/foo/bar/baz",
			errOK: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.input)
			if c.errOK {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Raw is set by Parse to the trimmed input — fill in expected
			// when the test case doesn't set it explicitly.
			if c.want.Raw == "" {
				c.want.Raw = c.input
			}
			if got.Kind != c.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, c.want.Kind)
			}
			if got.SObject != c.want.SObject {
				t.Errorf("SObject = %q, want %q", got.SObject, c.want.SObject)
			}
			if got.ID != c.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, c.want.ID)
			}
			if got.Host != c.want.Host {
				t.Errorf("Host = %q, want %q", got.Host, c.want.Host)
			}
			if got.Sandbox != c.want.Sandbox {
				t.Errorf("Sandbox = %q, want %q", got.Sandbox, c.want.Sandbox)
			}
			for k, v := range c.want.Extra {
				if got.Extra[k] != v {
					t.Errorf("Extra[%q] = %q, want %q", k, got.Extra[k], v)
				}
			}
			for k := range got.Extra {
				if _, want := c.want.Extra[k]; !want {
					t.Errorf("unexpected Extra[%q] = %q", k, got.Extra[k])
				}
			}
		})
	}
}
