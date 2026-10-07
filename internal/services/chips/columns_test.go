package chips

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestRecordColumnCatalog_DefaultsFromDescribe(t *testing.T) {
	cat := RecordColumnCatalog(sf.SObjectDescribe{
		Name: "Request__c",
		Fields: []sf.Field{
			{Name: "Status__c", Label: "Status", Type: "picklist", Custom: true, Filterable: true, Sortable: true},
			{Name: "Id", Label: "Record ID", Type: "id", Filterable: true, Sortable: true},
			{Name: "Name", Label: "Request Name", Type: "string", Filterable: true, Sortable: true},
			{Name: "LastModifiedDate", Label: "Last Modified Date", Type: "datetime", Filterable: true, Sortable: true},
		},
	})
	if cat.Domain != "records" || cat.Scope != "Request__c" {
		t.Fatalf("catalog identity = %s/%s", cat.Domain, cat.Scope)
	}
	wantDefaults := []string{"Id", "Name", "LastModifiedDate"}
	if !reflect.DeepEqual(cat.DefaultColumns, wantDefaults) {
		t.Fatalf("defaults = %v, want %v", cat.DefaultColumns, wantDefaults)
	}
	if len(cat.Columns) == 0 || cat.Columns[0].ID != "Id" {
		t.Fatalf("first column = %+v, want Id first", cat.Columns)
	}
}

func TestValidateColumns_UnknownColumn(t *testing.T) {
	cat := ColumnCatalog{
		Domain: "records",
		Scope:  "Account",
		Columns: []Column{
			{ID: "Id"},
			{ID: "Name"},
		},
	}
	err := ValidateColumns(cat, []string{"Name", "Typo__c"})
	if err == nil {
		t.Fatal("ValidateColumns succeeded, want error")
	}
	if !errors.Is(err, ErrUnknownColumn) {
		t.Fatalf("error = %v, want ErrUnknownColumn", err)
	}
	var unknown UnknownColumnError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %T, want UnknownColumnError", err)
	}
	if len(unknown.Columns) != 1 || unknown.Columns[0] != "Typo__c" {
		t.Fatalf("unknown columns = %v", unknown.Columns)
	}
}

func TestStaticColumnCatalog_Flows(t *testing.T) {
	cat, ok := StaticColumnCatalog("flows")
	if !ok {
		t.Fatal("StaticColumnCatalog(flows) not found")
	}
	if !reflect.DeepEqual(cat.DefaultColumns, []string{"Name", "Type", "Status", "Version", "Label", "Marks"}) {
		t.Fatalf("defaults = %v", cat.DefaultColumns)
	}
	if err := ValidateColumns(cat, []string{"Name", "Marks"}); err != nil {
		t.Fatalf("ValidateColumns: %v", err)
	}
}
