package tsdb

import (
	"io/ioutil"
	"os"
	"testing"
	
	"github.com/influxdata/influxql"
)

func TestFieldMappingSaveLoad(t *testing.T) {
	// Create a temporary file for the field set
	tmpfile, err := ioutil.TempFile("", "fields_test_*.idx")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpfile.Name()
	tmpfile.Close()
	os.Remove(tmpPath) // Remove the empty file so NewMeasurementFieldSet doesn't try to load it
	defer os.Remove(tmpPath)
	
	// Create a field set
	fs, err := NewMeasurementFieldSet(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	
	// Create a measurement with a field
	mf := fs.CreateFieldsIfNotExists([]byte("test"))
	if err := mf.CreateFieldIfNotExists([]byte("temperature"), influxql.Float); err != nil {
		t.Fatal(err)
	}
	
	// Rename the field
	if err := mf.RenameField("temperature", "Air_Temperature"); err != nil {
		t.Fatal(err)
	}
	
	// Verify the mapping was created
	mappings := mf.mappings.Load().(map[string]*FieldMapping)
	if len(mappings) == 0 {
		t.Fatal("Expected mappings to be created, but got none")
	}
	
	t.Logf("Mappings before save: %d", len(mappings))
	for userName, mapping := range mappings {
		t.Logf("  %s -> %s (version %d, state %d)", userName, mapping.InternalName, mapping.Version, mapping.State)
	}
	
	// Save the field set
	if err := fs.Save(); err != nil {
		t.Fatal(err)
	}
	
	// Read the saved file to see what was actually saved
	data, err := ioutil.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Saved file size: %d bytes", len(data))
	
	// Load the field set
	fs2, err := NewMeasurementFieldSet(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	// Note: NewMeasurementFieldSet already calls load(), so we don't need to call it again
	
	// Verify the mapping was loaded
	mf2 := fs2.CreateFieldsIfNotExists([]byte("test"))
	mappings2 := mf2.mappings.Load().(map[string]*FieldMapping)
	
	t.Logf("Mappings after load: %d", len(mappings2))
	for userName, mapping := range mappings2 {
		t.Logf("  %s -> %s (version %d, state %d)", userName, mapping.InternalName, mapping.Version, mapping.State)
	}
	
	if len(mappings2) == 0 {
		t.Fatal("Expected mappings to be loaded, but got none")
	}
	
	// Verify the mapping is correct
	if mapping, exists := mappings2["Air_Temperature"]; !exists {
		t.Fatal("Expected mapping for 'Air_Temperature' to exist")
	} else if mapping.InternalName != "temperature" {
		t.Fatalf("Expected internal name 'temperature', got '%s'", mapping.InternalName)
	}
}

