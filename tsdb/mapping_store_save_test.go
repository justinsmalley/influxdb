package tsdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/influxdata/influxdb/tsdb"
)

// TestMarshalAndSave_PersistsAndReloads verifies that MarshalAndSave correctly
// writes data that can be reloaded by a new store instance.
func TestMarshalAndSave_PersistsAndReloads(t *testing.T) {
	dir, err := os.MkdirTemp("", "marshalsave_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Field mapping store
	path := filepath.Join(dir, "field_mappings.json")
	store, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateFieldMapping("meas1", "temp"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFieldMapping("meas1", "humidity"); err != nil {
		t.Fatal(err)
	}

	// Reload from disk
	store2, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	internal, found := store2.GetInternalFieldName("meas1", "temp")
	if !found || internal != "temp" {
		t.Fatalf("expected temp -> temp, got %s (found=%v)", internal, found)
	}
	internal, found = store2.GetInternalFieldName("meas1", "humidity")
	if !found || internal != "humidity" {
		t.Fatalf("expected humidity -> humidity, got %s (found=%v)", internal, found)
	}
}

// TestMarshalAndSave_DatabaseStore verifies database mapping persistence.
func TestMarshalAndSave_DatabaseStore(t *testing.T) {
	dir, err := os.MkdirTemp("", "marshalsave_db_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "database_mappings.json")
	store, err := tsdb.NewDatabaseMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateDatabaseMapping("mydb"); err != nil {
		t.Fatal(err)
	}

	// Reload
	store2, err := tsdb.NewDatabaseMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	internal, found := store2.GetInternalDatabaseName("mydb")
	if !found || internal != "mydb" {
		t.Fatalf("expected mydb -> mydb, got %s (found=%v)", internal, found)
	}
}

// TestMarshalAndSave_MeasurementStore verifies measurement mapping persistence.
func TestMarshalAndSave_MeasurementStore(t *testing.T) {
	dir, err := os.MkdirTemp("", "marshalsave_meas_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "measurement_mappings.json")
	store, err := tsdb.NewMeasurementMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateMeasurementMapping("cpu"); err != nil {
		t.Fatal(err)
	}

	// Rename then persist
	if err := store.RenameMeasurement("cpu", "cpu_usage"); err != nil {
		t.Fatal(err)
	}

	// Reload
	store2, err := tsdb.NewMeasurementMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	internal, found := store2.GetInternalMeasurementName("cpu_usage")
	if !found || internal != "cpu" {
		t.Fatalf("expected cpu_usage -> cpu, got %s (found=%v)", internal, found)
	}

	// Old name should not resolve
	_, found = store2.GetInternalMeasurementName("cpu")
	if found {
		t.Fatal("old name 'cpu' should not be found after rename")
	}
}

// TestMarshalAndSave_DropRecreateReusesSlot verifies that dropping and
// recreating a field reuses the freed internal name slot.
func TestMarshalAndSave_DropRecreateReusesSlot(t *testing.T) {
	dir, err := os.MkdirTemp("", "marshalsave_reuse_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "field_mappings.json")
	store, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Create, drop, recreate — dropped slot (v=="") is reusable
	if _, err := store.CreateFieldMapping("meas1", "temp"); err != nil {
		t.Fatal(err)
	}
	if err := store.SoftDeleteField("meas1", "temp"); err != nil {
		t.Fatal(err)
	}
	internalName, err := store.CreateFieldMapping("meas1", "temp")
	if err != nil {
		t.Fatal(err)
	}
	if internalName != "temp" {
		t.Fatalf("expected reused slot 'temp', got %s", internalName)
	}

	// Reload and verify
	store2, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	internal, found := store2.GetInternalFieldName("meas1", "temp")
	if !found || internal != "temp" {
		t.Fatalf("after reload: expected temp -> temp, got %s (found=%v)", internal, found)
	}
}

// TestMarshalAndSave_RenameCreatesVersionSuffix verifies that renaming a field
// and then creating a new field with the old name produces a .v2 suffix since
// the original internal slot is still occupied.
func TestMarshalAndSave_RenameCreatesVersionSuffix(t *testing.T) {
	dir, err := os.MkdirTemp("", "marshalsave_version_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "field_mappings.json")
	store, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Create "temp", rename to "temperature" (internal "temp" slot is now
	// occupied by user name "temperature"), then create new "temp" which
	// must get a .v2 suffix.
	if _, err := store.CreateFieldMapping("meas1", "temp"); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameField("meas1", "temp", "temperature"); err != nil {
		t.Fatal(err)
	}
	internalName, err := store.CreateFieldMapping("meas1", "temp")
	if err != nil {
		t.Fatal(err)
	}
	if internalName != "temp.v2" {
		t.Fatalf("expected temp.v2, got %s", internalName)
	}

	// Reload and verify both mappings survived
	store2, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	internal, found := store2.GetInternalFieldName("meas1", "temperature")
	if !found || internal != "temp" {
		t.Fatalf("after reload: expected temperature -> temp, got %s (found=%v)", internal, found)
	}
	internal, found = store2.GetInternalFieldName("meas1", "temp")
	if !found || internal != "temp.v2" {
		t.Fatalf("after reload: expected temp -> temp.v2, got %s (found=%v)", internal, found)
	}
}
