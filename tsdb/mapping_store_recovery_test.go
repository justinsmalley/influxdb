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

	path := filepath.Join(dir, "field_mappings")

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

	path := filepath.Join(dir, "field_mappings")

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

// TestRecovery_PrimaryMissing_BackupExists verifies that a missing primary
// file with an existing .bak does NOT auto-recover (we treat missing as empty).
func TestRecovery_PrimaryMissing_BackupExists(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_missing_primary")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "field_mappings")

	// Create store with data
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

	// Delete primary, leave backup
	os.Remove(path)

	// Reload — primary is missing, treated as empty store (no error)
	store2, err := tsdb.NewFieldMappingStore(path)
	if err != nil {
		t.Fatalf("expected empty store when primary missing, got error: %v", err)
	}

	// Should be empty — no auto-recovery from backup for missing files
	_, found := store2.GetInternalFieldName("meas1", "temp")
	if found {
		t.Fatal("expected empty store after primary deletion")
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

	path := filepath.Join(dir, "db_mappings.idx")

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

// TestRecovery_SaveCreatesBackup verifies that each save creates a .bak file
// after the first save.
func TestRecovery_SaveCreatesBackup(t *testing.T) {
	dir, err := os.MkdirTemp("", "recovery_backup_creation")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "meas_mappings.idx")

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

	path := filepath.Join(dir, "meas_mappings.idx")

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
