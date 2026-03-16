package coordinator_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/influxdata/influxdb/coordinator"
	"github.com/influxdata/influxdb/services/meta"
	"github.com/influxdata/influxdb/tsdb"
)

// TestDDL_RenameDatabase_CorruptMappingStore verifies that RenameDatabase
// returns an error when the database mapping file is corrupt (not silently skipped).
func TestDDL_RenameDatabase_CorruptMappingStore(t *testing.T) {
	dir, err := ioutil.TempDir("", "ddl_rename_db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Write corrupt data to the database mapping file BEFORE opening the store.
	// store.Open() calls reconcileMappings() which lazy-initializes and caches
	// the DatabaseMappingStore. By corrupting the file first, the load will fail
	// and Open() will log a warning (non-fatal for reconcile), but the store won't
	// be cached. The next access from DDL will attempt to load again and fail.
	mappingPath := filepath.Join(dir, "database_mappings.idx")
	if err := os.MkdirAll(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(mappingPath, []byte("corrupt protobuf data"), 0666); err != nil {
		t.Fatal(err)
	}

	store := tsdb.NewStore(dir)
	store.EngineOptions.Config.WALDir = filepath.Join(dir, "wal")
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Verify that DatabaseMappingStore() returns an error due to corrupt file
	_, dbErr := store.DatabaseMappingStore()
	if dbErr == nil {
		t.Fatal("expected error from DatabaseMappingStore() with corrupt file, got nil")
	}

	// Now test via RenameDatabase through the StatementExecutor
	se := &coordinator.StatementExecutor{
		MetaClient: &MetaClient{
			DatabaseFn: func(name string) *meta.DatabaseInfo {
				return &meta.DatabaseInfo{Name: name}
			},
		},
		TSDBStore: store,
	}

	err = se.RenameDatabase("old_db", "new_db")
	if err == nil {
		t.Fatal("expected error from RenameDatabase with corrupt mapping store, got nil")
	}
	if !strings.Contains(err.Error(), "mapping") {
		t.Fatalf("expected mapping-related error, got: %v", err)
	}
}

// TestDDL_DropField_CorruptMeasurementMapping verifies that DropField returns
// an error when the measurement mapping file is corrupt.
func TestDDL_DropField_CorruptMeasurementMapping(t *testing.T) {
	dir, err := ioutil.TempDir("", "ddl_drop_field")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store := tsdb.NewStore(dir)
	store.EngineOptions.Config.WALDir = filepath.Join(dir, "wal")
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// The store is open with an empty (valid) DatabaseMappingStore.
	// Create a valid database mapping so translateDatabaseName succeeds.
	dbMapping, err := store.DatabaseMappingStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbMapping.CreateDatabaseMapping("testdb"); err != nil {
		t.Fatal(err)
	}

	// Create the database directory and corrupt the measurement mapping file
	// BEFORE MeasurementMappingStore("testdb") is first accessed.
	// reconcileMappings() won't touch this because there are no shards for "testdb".
	dbDir := filepath.Join(dir, "testdb")
	if err := os.MkdirAll(dbDir, 0777); err != nil {
		t.Fatal(err)
	}
	measMappingPath := filepath.Join(dbDir, "measurement_mappings.idx")
	if err := ioutil.WriteFile(measMappingPath, []byte("corrupt measurement data"), 0666); err != nil {
		t.Fatal(err)
	}

	se := &coordinator.StatementExecutor{
		MetaClient: &MetaClient{
			DatabaseFn: func(name string) *meta.DatabaseInfo {
				return &meta.DatabaseInfo{Name: name}
			},
		},
		TSDBStore: store,
	}

	err = se.DropField("testdb", "cpu", "temperature")
	if err == nil {
		t.Fatal("expected error from DropField with corrupt measurement mapping, got nil")
	}
	if !strings.Contains(err.Error(), "mapping") {
		t.Fatalf("expected mapping-related error, got: %v", err)
	}
}

// TestDDL_RenameField_CorruptFieldMapping verifies that RenameField returns
// an error when the field mapping file is corrupt.
func TestDDL_RenameField_CorruptFieldMapping(t *testing.T) {
	dir, err := ioutil.TempDir("", "ddl_rename_field")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store := tsdb.NewStore(dir)
	store.EngineOptions.Config.WALDir = filepath.Join(dir, "wal")
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Establish valid database + measurement mappings.
	dbMapping, err := store.DatabaseMappingStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbMapping.CreateDatabaseMapping("testdb"); err != nil {
		t.Fatal(err)
	}

	// Create a valid measurement mapping store for this database.
	measMapping, err := store.MeasurementMappingStore("testdb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := measMapping.CreateMeasurementMapping("cpu"); err != nil {
		t.Fatal(err)
	}

	// Corrupt the field mapping file BEFORE FieldMappingStore("testdb") is first accessed.
	// Note: field mapping files don't use .idx extension — the path is just "field_mappings".
	dbDir := filepath.Join(dir, "testdb")
	fieldMappingPath := filepath.Join(dbDir, "field_mappings")
	if err := ioutil.WriteFile(fieldMappingPath, []byte("corrupt field data"), 0666); err != nil {
		t.Fatal(err)
	}

	se := &coordinator.StatementExecutor{
		MetaClient: &MetaClient{
			DatabaseFn: func(name string) *meta.DatabaseInfo {
				return &meta.DatabaseInfo{Name: name}
			},
		},
		TSDBStore: store,
	}

	err = se.RenameField("testdb", "cpu", "temp", "temperature")
	if err == nil {
		t.Fatal("expected error from RenameField with corrupt field mapping, got nil")
	}
	if !strings.Contains(err.Error(), "mapping") {
		t.Fatalf("expected mapping-related error, got: %v", err)
	}
}

// TestDDL_ShowMeasurements_GracefulDegradation verifies that when no mappings exist,
// measurement names are returned as-is (graceful degradation on read path).
func TestDDL_ShowMeasurements_GracefulDegradation(t *testing.T) {
	dir, err := ioutil.TempDir("", "ddl_show_meas")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store := tsdb.NewStore(dir)
	store.EngineOptions.Config.WALDir = filepath.Join(dir, "wal")
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// The MeasurementMappingStore for a database with no mapping file
	// should succeed — returning an empty store. This is the graceful
	// degradation path: names are returned as-is.
	measStore, err := store.MeasurementMappingStore("new_db")
	if err != nil {
		t.Fatalf("MeasurementMappingStore should succeed for new databases: %v", err)
	}

	// GetUserMeasurementNames should return internal names unchanged when no mappings exist
	internalNames := []string{"cpu", "mem", "disk"}
	userNames := measStore.GetUserMeasurementNames(internalNames)
	for i, name := range userNames {
		if name != internalNames[i] {
			t.Errorf("expected %s, got %s — graceful degradation should return names unchanged", internalNames[i], name)
		}
	}
}

// TestDDL_DropField_ValidPath verifies that DropField succeeds with valid mapping stores.
func TestDDL_DropField_ValidPath(t *testing.T) {
	dir, err := ioutil.TempDir("", "ddl_drop_field_valid")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store := tsdb.NewStore(dir)
	store.EngineOptions.Config.WALDir = filepath.Join(dir, "wal")
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Set up valid mappings
	dbMapping, err := store.DatabaseMappingStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbMapping.CreateDatabaseMapping("testdb"); err != nil {
		t.Fatal(err)
	}

	se := &coordinator.StatementExecutor{
		MetaClient: &MetaClient{
			DatabaseFn: func(name string) *meta.DatabaseInfo {
				return &meta.DatabaseInfo{Name: name}
			},
		},
		TSDBStore: store,
	}

	// DropField should succeed (dropping a field that doesn't exist is fine)
	err = se.DropField("testdb", "cpu", "nonexistent_field")
	if err != nil {
		t.Fatalf("expected DropField to succeed for valid mapping stores, got: %v", err)
	}
}
