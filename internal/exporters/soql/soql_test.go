package soql

import (
	"testing"
)

func TestShape_PreservesPreferredOrder(t *testing.T) {
	records := []map[string]any{
		{"Id": "001", "Name": "Acme", "attributes": map[string]any{"type": "Account"}},
		{"Id": "002", "Name": "Globex"},
	}
	headers, rows := Shape(records, []string{"Id", "Name"})
	if len(headers) != 2 || headers[0] != "Id" || headers[1] != "Name" {
		t.Errorf("headers = %v, want [Id, Name]", headers)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0].Get("Id") != "001" || rows[0].Get("Name") != "Acme" {
		t.Errorf("row 0 = %v", rows[0].Columns)
	}
}

func TestShape_DiscoversExtraColumns(t *testing.T) {
	records := []map[string]any{
		{"Id": "001", "Name": "Acme", "Phone": "555"},
	}
	headers, _ := Shape(records, []string{"Id"})
	// Id first (preferred), then discovered columns alphabetically.
	if len(headers) != 3 || headers[0] != "Id" {
		t.Errorf("headers = %v", headers)
	}
}

func TestShape_FlattensNestedMaps(t *testing.T) {
	records := []map[string]any{
		{
			"Id": "001",
			"Owner": map[string]any{
				"Name":       "Alice",
				"Email":      "a@x",
				"attributes": map[string]any{"type": "User"},
			},
		},
	}
	headers, rows := Shape(records, []string{"Id", "Owner.Name", "Owner.Email"})
	got := rows[0]
	if got.Get("Owner.Name") != "Alice" || got.Get("Owner.Email") != "a@x" {
		t.Errorf("flatten failed: %v (headers %v)", got.Columns, headers)
	}
}

func TestShape_SkipsAttributesEnvelope(t *testing.T) {
	records := []map[string]any{
		{"Id": "001", "attributes": map[string]any{"type": "Account", "url": "/x"}},
	}
	headers, rows := Shape(records, nil)
	for _, h := range headers {
		if h == "attributes" || h == "attributes.type" || h == "attributes.url" {
			t.Errorf("attributes leaked into headers: %v", headers)
		}
	}
	_ = rows
}

func TestShape_FormatsCellTypes(t *testing.T) {
	records := []map[string]any{
		{"S": "abc", "B": true, "N": float64(42), "Nil": nil, "L": []any{"a", "b"}},
	}
	_, rows := Shape(records, []string{"S", "B", "N", "Nil", "L"})
	r := rows[0]
	if r.Get("S") != "abc" {
		t.Errorf("string: %q", r.Get("S"))
	}
	if r.Get("B") != "true" {
		t.Errorf("bool: %q", r.Get("B"))
	}
	if r.Get("N") != "42" {
		t.Errorf("number: %q", r.Get("N"))
	}
	if r.Get("Nil") != "" {
		t.Errorf("nil: %q", r.Get("Nil"))
	}
	if r.Get("L") != "[a, b]" {
		t.Errorf("list: %q", r.Get("L"))
	}
}
