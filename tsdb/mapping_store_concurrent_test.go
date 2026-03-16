package tsdb_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/influxdata/influxdb/tsdb"
)

// TestConcurrent_WriteDifferentMeasurements verifies that N goroutines can
// concurrently create field mappings for different measurements without races.
func TestConcurrent_WriteDifferentMeasurements(t *testing.T) {
	dir, err := os.MkdirTemp("", "concurrent_write_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := tsdb.NewFieldMappingStore(filepath.Join(dir, "field_mappings.json"))
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 20
	const fieldsPerGoroutine = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*fieldsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			meas := fmt.Sprintf("measurement_%d", idx)
			for f := 0; f < fieldsPerGoroutine; f++ {
				fieldName := fmt.Sprintf("field_%d", f)
				_, err := store.CreateFieldMapping(meas, fieldName)
				if err != nil {
					errs <- fmt.Errorf("goroutine %d, field %s: %v", idx, fieldName, err)
				}
			}
		}(g)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	// Verify all mappings were created
	for g := 0; g < goroutines; g++ {
		meas := fmt.Sprintf("measurement_%d", g)
		for f := 0; f < fieldsPerGoroutine; f++ {
			fieldName := fmt.Sprintf("field_%d", f)
			internal, found := store.GetInternalFieldName(meas, fieldName)
			if !found {
				t.Errorf("missing mapping: %s/%s", meas, fieldName)
			}
			if internal != fieldName {
				t.Errorf("expected identity mapping for %s/%s, got %s", meas, fieldName, internal)
			}
		}
	}
}

// TestConcurrent_ReadWhileRename verifies that concurrent reads don't panic
// or return corrupt data while a rename is happening.
func TestConcurrent_ReadWhileRename(t *testing.T) {
	dir, err := os.MkdirTemp("", "concurrent_rename_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := tsdb.NewFieldMappingStore(filepath.Join(dir, "field_mappings.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Create initial mappings
	const numFields = 100
	for i := 0; i < numFields; i++ {
		if _, err := store.CreateFieldMapping("cpu", fmt.Sprintf("field_%d", i)); err != nil {
			t.Fatal(err)
		}
	}

	// Start readers
	const readers = 10
	var wg sync.WaitGroup
	stop := make(chan struct{})
	readErrs := make(chan error, 1000)

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					// Read a random field — should never panic
					for i := 0; i < numFields; i++ {
						name, _ := store.GetInternalFieldName("cpu", fmt.Sprintf("field_%d", i))
						if name == "" {
							readErrs <- fmt.Errorf("got empty name for field_%d", i)
						}
					}
				}
			}
		}()
	}

	// Rename fields concurrently
	for i := 0; i < numFields; i++ {
		oldName := fmt.Sprintf("field_%d", i)
		newName := fmt.Sprintf("renamed_%d", i)
		if err := store.RenameField("cpu", oldName, newName); err != nil {
			t.Errorf("rename %s -> %s: %v", oldName, newName, err)
		}
	}

	close(stop)
	wg.Wait()
	close(readErrs)

	for err := range readErrs {
		t.Error(err)
	}

	// Verify all renames took effect
	for i := 0; i < numFields; i++ {
		newName := fmt.Sprintf("renamed_%d", i)
		internal, found := store.GetInternalFieldName("cpu", newName)
		if !found {
			t.Errorf("renamed field %s not found", newName)
		}
		expected := fmt.Sprintf("field_%d", i)
		if internal != expected {
			t.Errorf("expected internal name %s, got %s", expected, internal)
		}
	}
}

// TestConcurrent_SimultaneousRenameToSameTarget verifies that two goroutines
// trying to rename different fields to the same target name result in exactly
// one success and one failure (the second finds the target name already taken).
func TestConcurrent_SimultaneousRenameToSameTarget(t *testing.T) {
	dir, err := os.MkdirTemp("", "concurrent_conflict_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := tsdb.NewFieldMappingStore(filepath.Join(dir, "field_mappings.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateFieldMapping("cpu", "temp_a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFieldMapping("cpu", "temp_b"); err != nil {
		t.Fatal(err)
	}

	// Two goroutines try to rename different fields to the same target
	var wg sync.WaitGroup
	results := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		results <- store.RenameField("cpu", "temp_a", "temperature")
	}()
	go func() {
		defer wg.Done()
		results <- store.RenameField("cpu", "temp_b", "temperature")
	}()

	wg.Wait()
	close(results)

	var successes, failures int
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}

	// Exactly one should succeed; the other finds "temperature" already exists
	if successes != 1 || failures != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d successes and %d failures", successes, failures)
	}

	// Verify "temperature" maps to exactly one internal name
	internal, found := store.GetInternalFieldName("cpu", "temperature")
	if !found {
		t.Fatal("expected 'temperature' to exist")
	}
	if internal != "temp_a" && internal != "temp_b" {
		t.Errorf("expected internal name to be temp_a or temp_b, got %s", internal)
	}
}

// TestConcurrent_DropAndRecreateSameField verifies that concurrent drop and
// recreate operations on the same field don't corrupt the mapping state.
func TestConcurrent_DropAndRecreateSameField(t *testing.T) {
	dir, err := os.MkdirTemp("", "concurrent_drop_recreate_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := tsdb.NewFieldMappingStore(filepath.Join(dir, "field_mappings.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateFieldMapping("cpu", "temp"); err != nil {
		t.Fatal(err)
	}

	const iterations = 100
	var wg sync.WaitGroup
	errs := make(chan error, iterations*2)

	for i := 0; i < iterations; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := store.SoftDeleteField("cpu", "temp"); err != nil {
				errs <- fmt.Errorf("drop: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := store.CreateFieldMapping("cpu", "temp"); err != nil {
				errs <- fmt.Errorf("recreate: %v", err)
			}
		}()
		wg.Wait()
	}

	close(errs)
	for err := range errs {
		t.Error(err)
	}

	// The final state should be consistent: either "temp" exists or it doesn't,
	// but the internal maps should not be corrupt.
	mappings := store.GetAllMappings("cpu")
	for _, m := range mappings {
		if m.UserName == "" && m.InternalName == "" {
			t.Error("found corrupt empty mapping")
		}
	}
}

// TestConcurrent_DeferredCreateAndSave verifies that deferred creates from
// multiple goroutines followed by a single SaveIfDirty don't lose data.
func TestConcurrent_DeferredCreateAndSave(t *testing.T) {
	dir, err := os.MkdirTemp("", "concurrent_deferred_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := tsdb.NewFieldMappingStore(filepath.Join(dir, "field_mappings.json"))
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 10
	const fieldsPerGoroutine = 20
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for f := 0; f < fieldsPerGoroutine; f++ {
				fieldName := fmt.Sprintf("field_%d_%d", idx, f)
				if _, err := store.CreateFieldMappingDeferred("cpu", fieldName); err != nil {
					t.Errorf("deferred create: %v", err)
				}
			}
		}(g)
	}

	wg.Wait()

	// Flush to disk
	if err := store.SaveIfDirty(); err != nil {
		t.Fatal(err)
	}

	// Reload and verify all mappings persisted
	store2, err := tsdb.NewFieldMappingStore(filepath.Join(dir, "field_mappings.json"))
	if err != nil {
		t.Fatal(err)
	}

	for g := 0; g < goroutines; g++ {
		for f := 0; f < fieldsPerGoroutine; f++ {
			fieldName := fmt.Sprintf("field_%d_%d", g, f)
			_, found := store2.GetInternalFieldName("cpu", fieldName)
			if !found {
				t.Errorf("missing mapping after reload: %s", fieldName)
			}
		}
	}
}
