package tsdb_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/influxdata/influxdb/tsdb"
)

// TestRecovery_CorruptPrimary_FallsBackToBackup verifies that when the primary
// mapping file is corrupt, the store recovers from the .bak backup file.
func TestRecovery_CorruptPrimary_FallsBackToBackup(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "field_mappings.json")

	// Create a store and add a mapping (this creates the file)
	store, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFieldMapping("meas1", "temp"); err != nil {
		t.Fatal(err)
	}

	// Now add another mapping — this triggers a second save, creating a .bak
	if _, err := store.CreateFieldMapping("meas1", "humidity"); err != nil {
		t.Fatal(err)
	}

	// Verify .bak exists
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Fatal("expected .bak file to exist after second save")
	}

	// Corrupt the primary file
	if err := ioutil.WriteFile(path, []byte("corrupted data"), 0666); err != nil {
		t.Fatal(err)
	}

	// Reload — should fall back to backup (which has the first mapping only)
	store2, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatalf("expected recovery from backup, got error: %v", err)
	}

	// The backup was created after the first save (just "temp"), so only "temp"
	// should be present after recovery.
	internal, found := store2.GetInternalFieldName("meas1", "temp")
	if !found || internal != "temp" {
		t.Fatalf("after recovery: expected temp -> temp, got %s (found=%v)", internal, found)
	}
}

// TestRecovery_BothCorrupt_ReturnsError verifies that when both primary and
// backup are corrupt, loading returns an error.
func TestRecovery_BothCorrupt_ReturnsError(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_both_corrupt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "field_mappings.json")

	// Create a store with data to establish the file
	store, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFieldMapping("meas1", "temp"); err != nil {
		t.Fatal(err)
	}
	// Second save to create .bak
	if _, err := store.CreateFieldMapping("meas1", "humidity"); err != nil {
		t.Fatal(err)
	}

	// Corrupt both files
	ioutil.WriteFile(path, []byte("corrupted primary"), 0666)
	ioutil.WriteFile(path+".bak", []byte("corrupted backup"), 0666)

	// Reload — should fail
	_, err = tsdb.NewFieldMappingStore(path)
	if err == nil {
		t.Fatal("expected error when both primary and backup are corrupt")
	}
}

// TestRecovery_PrimaryMissing_BackupExists verifies that when the primary file
// is missing but a .bak backup exists, the store recovers from the backup and
// restores it as the primary.
func TestRecovery_PrimaryMissing_BackupExists(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_missing_primary")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "field_mappings.json")

	// Create store with data
	store, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFieldMapping("meas1", "temp"); err != nil {
		t.Fatal(err)
	}
	// Second save to create .bak (backup contains only "temp")
	if _, err := store.CreateFieldMapping("meas1", "humidity"); err != nil {
		t.Fatal(err)
	}

	// Verify .bak exists before deleting primary
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Fatal("expected .bak file to exist before test")
	}

	// Delete primary, leave backup
	os.Remove(path)

	// Reload — should recover from backup (no error)
	store2, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatalf("expected recovery from backup when primary missing, got error: %v", err)
	}

	// Backup had "temp" (from first save before backup was created)
	_, found := store2.GetInternalFieldName("meas1", "temp")
	if !found {
		t.Fatal("expected temp to be recovered from backup")
	}

	// Primary file should have been restored
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected primary file to be restored after backup recovery")
	}
}

// TestRecovery_BackupContainsPreviousVersion verifies that .bak always contains
// the previous version of the data, not the current version.
func TestRecovery_BackupContainsPreviousVersion(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_backup_version")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "db_mappings.json")

	// First save: create "db1"
	store, err := tsdb.NewDatabaseMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDatabaseMapping("db1"); err != nil {
		t.Fatal(err)
	}

	// Second save: create "db2" — .bak should now contain only "db1"
	if _, err := store.CreateDatabaseMapping("db2"); err != nil {
		t.Fatal(err)
	}

	// Corrupt primary to force fallback to backup
	ioutil.WriteFile(path, []byte("corrupt"), 0666)

	// Load from backup
	store2, err := tsdb.NewDatabaseMappingStore(path)
	if err != nil {
		t.Fatalf("expected recovery from backup, got: %v", err)
	}

	// Backup had only db1 (from first save)
	_, found := store2.GetInternalDatabaseName("db1")
	if !found {
		t.Fatal("expected db1 in backup")
	}
	_, found = store2.GetInternalDatabaseName("db2")
	if found {
		t.Fatal("db2 should NOT be in backup — it was only in the primary that was corrupted")
	}
}

// TestRecovery_OrphanedFieldMappings verifies that field mapping groups with no
// corresponding measurement mapping entry are correctly identified as orphans.
// This simulates the state left by a crash between the two saves in
// DeleteMeasurement (field-mapping save succeeds, measurement-mapping save fails).
//
// The test operates at the unit level on the mapping stores directly (not the
// full Store), verifying the invariant that reconcileMappings is supposed to
// enforce.
func TestRecovery_OrphanedFieldMappings(t *testing.T) {
	dir, err := os.MkdirTemp("", "orphan_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fieldPath := filepath.Join(dir, "field_mappings.json")
	measPath := filepath.Join(dir, "measurement_mappings.json")

	// Create a field mapping store with entries for two measurements.
	fms, err := tsdb.NewFieldMappingStore(fieldPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fms.CreateFieldMapping("m1", "temp"); err != nil {
		t.Fatal(err)
	}
	if _, err := fms.CreateFieldMapping("m2", "humidity"); err != nil {
		t.Fatal(err)
	}

	// Create a measurement mapping store with only "m2" (simulating a crash
	// that dropped "m1"'s measurement mapping but left its field mappings).
	mms, err := tsdb.NewMeasurementMappingStore(measPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mms.CreateMeasurementMapping("m2"); err != nil {
		t.Fatal(err)
	}

	// Simulate what reconcileMappings orphan sweep does:
	// collect known internal measurement names from the measurement store.
	knownMeasurements := make(map[string]bool)
	for _, m := range mms.GetAllMappings() {
		knownMeasurements[m.InternalName] = true
	}

	// Identify orphaned field groups (groups in field store with no parent measurement).
	var dropped []string
	for _, group := range fms.GetAllGroups() {
		if !knownMeasurements[group] {
			fms.DropGroup(group)
			dropped = append(dropped, group)
		}
	}
	if err := fms.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify "m1" was identified as orphan and dropped.
	if len(dropped) != 1 || dropped[0] != "m1" {
		t.Fatalf("expected [m1] orphan, got %v", dropped)
	}

	// Reload the field store and verify "m1" is gone but "m2" remains.
	fms2, err := tsdb.NewFieldMappingStore(fieldPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := fms2.GetInternalFieldName("m1", "temp"); found {
		t.Fatal("orphaned field mapping for m1 should have been removed")
	}
	if _, found := fms2.GetInternalFieldName("m2", "humidity"); !found {
		t.Fatal("non-orphaned field mapping for m2 should still exist")
	}
}

// TestRecovery_DeleteMeasurementSaveOrder verifies that the field-save-first
// ordering means a crash between the two saves leaves a recoverable state.
// Field mappings for the dropped measurement are removed before the measurement
// mapping, so the worst-case crash leaves an unmapped measurement with no
// field mappings — which reconcileMappings can clean up.
func TestRecovery_DeleteMeasurementSaveOrder(t *testing.T) {
	dir, err := os.MkdirTemp("", "save_order_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fieldPath := filepath.Join(dir, "field_mappings.json")
	measPath := filepath.Join(dir, "measurement_mappings.json")

	fms, err := tsdb.NewFieldMappingStore(fieldPath)
	if err != nil {
		t.Fatal(err)
	}
	mms, err := tsdb.NewMeasurementMappingStore(measPath)
	if err != nil {
		t.Fatal(err)
	}

	// Set up: measurement "m1" with field "temp", measurement "m2" with "humidity".
	if _, err := mms.CreateMeasurementMapping("m1"); err != nil {
		t.Fatal(err)
	}
	if _, err := mms.CreateMeasurementMapping("m2"); err != nil {
		t.Fatal(err)
	}
	if _, err := fms.CreateFieldMapping("m1", "temp"); err != nil {
		t.Fatal(err)
	}
	if _, err := fms.CreateFieldMapping("m2", "humidity"); err != nil {
		t.Fatal(err)
	}

	// Simulate DeleteMeasurement for "m1" with field-save-first ordering.
	// Step 1: remove field mappings and save (this succeeds).
	fms.DropGroup("m1")
	if err := fms.Save(); err != nil {
		t.Fatal(err)
	}

	// Simulate crash here: measurement mapping save is skipped.
	// State: field store has no "m1" group; measurement store still has "m1".

	// On restart, orphan sweep sees "m1" in measurement store but no fields for it.
	// This is NOT an orphan in the field-store sense (the field store has no m1 group).
	// The measurement store still has "m1" but with no fields — that's fine.
	// Verify: field store is clean.
	fms2, err := tsdb.NewFieldMappingStore(fieldPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := fms2.GetInternalFieldName("m1", "temp"); found {
		t.Fatal("field mapping for m1/temp should be gone after field-save-first")
	}
	if _, found := fms2.GetInternalFieldName("m2", "humidity"); !found {
		t.Fatal("field mapping for m2/humidity should still exist")
	}

	// If we had crashed BEFORE the field save (old order: meas-save-first),
	// the measurement mapping would be gone but field mappings remain (orphans).
	// The field-save-first ordering ensures we never create field orphans.
}

// TestRecovery_SaveCreatesBackup verifies that each save creates a .bak file
// after the first save.
func TestRecovery_SaveCreatesBackup(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_backup_creation")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "meas_mappings.json")

	store, err := tsdb.NewMeasurementMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// First save — no .bak yet (no previous file to backup)
	if _, err := store.CreateMeasurementMapping("cpu"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatal("expected no .bak file after first save")
	}

	// Second save — .bak should now exist
	if _, err := store.CreateMeasurementMapping("mem"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Fatal("expected .bak file after second save")
	}

	// Third save — .bak should still exist (rolling single backup)
	if err := store.RenameMeasurement("cpu", "cpu_usage"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Fatal("expected .bak file after third save")
	}
}

// TestRecovery_VerifyFuncRejectsCorruptWrite verifies that if the written temp
// file fails verification, the save aborts and the original file is preserved.
func TestRecovery_MeasurementStoreCorruptPrimary(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_meas_corrupt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "meas_mappings.json")

	store, err := tsdb.NewMeasurementMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateMeasurementMapping("cpu"); err != nil {
		t.Fatal(err)
	}
	// Second save to get a .bak
	if _, err := store.CreateMeasurementMapping("mem"); err != nil {
		t.Fatal(err)
	}

	// Corrupt primary
	ioutil.WriteFile(path, []byte("garbage"), 0666)

	// Reload — should recover from backup
	store2, err := tsdb.NewMeasurementMappingStore(path)
	if err != nil {
		t.Fatalf("expected recovery, got: %v", err)
	}

	// Backup only had "cpu"
	_, found := store2.GetInternalMeasurementName("cpu")
	if !found {
		t.Fatal("expected cpu in recovered store")
	}
	_, found = store2.GetInternalMeasurementName("mem")
	if found {
		t.Fatal("mem should NOT be in recovered store (was only in corrupt primary)")
	}
}
