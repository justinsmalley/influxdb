package tsdb_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/influxdata/influxdb/tsdb"
)

func TestFieldMappingStore_RenameDropRecreate(t *testing.T) {
	dir, err := os.MkdirTemp("", "fieldmapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "mappings.idx")
	store, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initially create a field "temp1"
	internalName, err := store.CreateFieldMapping("meas1", "temp1")
	if err != nil {
		t.Fatal(err)
	}
	if internalName != "temp1" {
		t.Fatalf("expected temp1, got %s", internalName)
	}

	// Verify GetInternalFieldName
	if name, ok := store.GetInternalFieldName("meas1", "temp1"); !ok || name != "temp1" {
		t.Fatalf("expected temp1, got %s", name)
	}

	// 2. Rename temp1 to temperature1
	if err := store.RenameField("meas1", "temp1", "temperature1"); err != nil {
		t.Fatal(err)
	}

	// Old name should no longer be active
	if _, ok := store.GetInternalFieldName("meas1", "temp1"); ok {
		t.Fatal("expected temp1 to be inactive")
	}

	// New name should point to old internal name
	if name, ok := store.GetInternalFieldName("meas1", "temperature1"); !ok || name != "temp1" {
		t.Fatalf("expected temperature1 -> temp1, got %s", name)
	}

	// 3. Drop temperature1
	if err := store.SoftDeleteField("meas1", "temperature1"); err != nil {
		t.Fatal(err)
	}

	if _, ok := store.GetInternalFieldName("meas1", "temperature1"); ok {
		t.Fatal("expected temperature1 to be inactive after drop")
	}

	// 4. Recreate temperature1
	internalName3, err := store.CreateFieldMapping("meas1", "temperature1")
	if err != nil {
		t.Fatal(err)
	}
	if internalName3 != "temperature1" {
		t.Fatalf("expected temperature1, got %s", internalName3)
	}

	// GetUserFieldNames (tests translation logic for Show Field Keys)
	userNames := store.GetUserFieldNames("meas1", []string{"temp1", "temperature1", "unmapped_field"})
	expected := []string{"", "temperature1", "unmapped_field"}
	if !reflect.DeepEqual(userNames, expected) {
		t.Fatalf("expected %v, got %v", expected, userNames)
	}

	// 5. Save/Load to ensure persistence works
	store2, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if name, ok := store2.GetInternalFieldName("meas1", "temperature1"); !ok || name != "temperature1" {
		t.Fatalf("expected temperature1 -> temperature1 after reload, got %s", name)
	}
}
