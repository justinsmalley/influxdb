package tsdb_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/influxdata/influxdb/tsdb"
)

func TestMeasurementMappingStore_RenameDropRecreate(t *testing.T) {
	dir, err := os.MkdirTemp("", "measmapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "measurement_mappings.idx")
	store, err := tsdb.NewMeasurementMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initially create a measurement "m1"
	internalName, err := store.CreateMeasurementMapping("m1")
	if err != nil {
		t.Fatal(err)
	}
	if internalName != "m1.v2" {
		t.Fatalf("expected m1.v2, got %s", internalName)
	}

	// Verify GetInternalMeasurementName
	if name, ok := store.GetInternalMeasurementName("m1"); !ok || name != "m1.v2" {
		t.Fatalf("expected m1.v2, got %s", name)
	}

	// 2. Rename m1 to m1_new
	if err := store.RenameMeasurement("m1", "m1_new"); err != nil {
		t.Fatal(err)
	}

	// Old name should no longer be active
	if _, ok := store.GetInternalMeasurementName("m1"); ok {
		t.Fatal("expected m1 to be inactive")
	}

	// New name should point to old internal name
	if name, ok := store.GetInternalMeasurementName("m1_new"); !ok || name != "m1.v2" {
		t.Fatalf("expected m1_new -> m1.v2, got %s", name)
	}

	// 3. Drop m1_new
	if err := store.SoftDeleteMeasurement("m1_new"); err != nil {
		t.Fatal(err)
	}

	if _, ok := store.GetInternalMeasurementName("m1_new"); ok {
		t.Fatal("expected m1_new to be inactive after drop")
	}

	// 4. Recreate m1_new
	internalName3, err := store.CreateMeasurementMapping("m1_new")
	if err != nil {
		t.Fatal(err)
	}
	if internalName3 != "m1_new.v3" {
		t.Fatalf("expected m1_new.v3, got %s", internalName3)
	}

	// GetUserMeasurementNames (tests translation logic for Show Measurements)
	userNames := store.GetUserMeasurementNames([]string{"m1.v2", "m1_new.v3", "unmapped_meas"})
	expected := []string{"m1_new", "unmapped_meas"}
	if !reflect.DeepEqual(userNames, expected) {
		t.Fatalf("expected %v, got %v", expected, userNames)
	}

	// 5. Save/Load to ensure persistence works
	store2, err := tsdb.NewMeasurementMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if name, ok := store2.GetInternalMeasurementName("m1_new"); !ok || name != "m1_new.v3" {
		t.Fatalf("expected m1_new -> m1_new.v3 after reload, got %s", name)
	}
}
