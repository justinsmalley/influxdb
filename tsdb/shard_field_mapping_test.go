package tsdb

import (
	"testing"

	"github.com/influxdata/influxql"
)

func TestMeasurementFields_FieldMappings(t *testing.T) {
	mf := NewMeasurementFields()
	
	// Test initial state - no mappings
	activeFields := mf.GetActiveUserFields()
	if len(activeFields) != 0 {
		t.Errorf("Expected 0 active fields, got %d", len(activeFields))
	}
	
	// Test GetInternalFieldName with no mapping (implicit identity)
	_, isActive := mf.GetInternalFieldName("temperature")
	if !isActive {
		t.Error("Expected field to be active")
	}
}

func TestMeasurementFields_SoftDeleteField(t *testing.T) {
	mf := NewMeasurementFields()
	
	// Create a field first
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	mf.fields.Store(fields)
	
	// Test soft delete
	err := mf.SoftDeleteField("temperature")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// Verify field is deleted
	_, isActive := mf.GetInternalFieldName("temperature")
	if isActive {
		t.Error("Expected field to be inactive after soft delete")
	}
	
	// Verify field is NOT accessible in modified version (soft-deleted fields are hidden)
	f := mf.Field("temperature")
	if f != nil {
		t.Error("Expected soft-deleted field to be inaccessible in modified version")
	}
	
	// Note: The field data still exists internally and will be accessible when reverting to unmodified InfluxDB
}

func TestMeasurementFields_RenameField(t *testing.T) {
	mf := NewMeasurementFields()
	
	// Create a field first
	fields := make(map[string]*Field)
	fields["temp"] = &Field{Name: "temp", Type: influxql.Float}
	mf.fields.Store(fields)
	
	// Test rename
	err := mf.RenameField("temp", "temperature")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// Verify old name is inactive
	_, isActive := mf.GetInternalFieldName("temp")
	if isActive {
		t.Error("Expected old field name to be inactive")
	}
	
	// Verify new name is active
	internalName, isActive := mf.GetInternalFieldName("temperature")
	if !isActive {
		t.Error("Expected new field name to be active")
	}
	if internalName != "temp" {
		t.Errorf("Expected internal name 'temp', got '%s'", internalName)
	}
	
	// Verify both names can access the same field
	f1 := mf.Field("temp")
	f2 := mf.Field("temperature")
	if f1 == nil || f2 == nil || f1 != f2 {
		t.Error("Expected both names to access the same field")
	}
}

func TestMeasurementFields_CreateFieldMapping(t *testing.T) {
	mf := NewMeasurementFields()
	
	// Create initial field
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	mf.fields.Store(fields)
	
	// Soft delete the field
	err := mf.SoftDeleteField("temperature")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// Create new mapping for recreated field
	internalName, err := mf.CreateFieldMapping("temperature")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	expectedInternalName := "temperature.v2"
	if internalName != expectedInternalName {
		t.Errorf("Expected internal name '%s', got '%s'", expectedInternalName, internalName)
	}
	
	// Verify new mapping is active
	_, isActive := mf.GetInternalFieldName("temperature")
	if !isActive {
		t.Error("Expected new mapping to be active")
	}
}

func TestMeasurementFields_FieldKeys(t *testing.T) {
	mf := NewMeasurementFields()
	
	// Create fields
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	fields["humidity"] = &Field{Name: "humidity", Type: influxql.Float}
	mf.fields.Store(fields)
	
	// Test FieldKeys returns both user-facing and internal names
	keys := mf.FieldKeys()
	expectedKeys := []string{"humidity", "temperature"}
	if len(keys) != len(expectedKeys) {
		t.Errorf("Expected %d keys, got %d", len(expectedKeys), len(keys))
	}
	
	// Verify all expected keys are present
	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}
	for _, expectedKey := range expectedKeys {
		if !keyMap[expectedKey] {
			t.Errorf("Expected key '%s' not found", expectedKey)
		}
	}
}

func TestMeasurementFields_FieldSet(t *testing.T) {
	mf := NewMeasurementFields()
	
	// Create fields
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	fields["humidity"] = &Field{Name: "humidity", Type: influxql.Integer}
	mf.fields.Store(fields)
	
	// Test FieldSet returns both user-facing and internal names
	fieldSet := mf.FieldSet()
	
	// Should contain both internal names and user-facing names
	if fieldSet["temperature"] != influxql.Float {
		t.Error("Expected temperature field to be Float type")
	}
	if fieldSet["humidity"] != influxql.Integer {
		t.Error("Expected humidity field to be Integer type")
	}
	
	// Should have at least 2 fields (internal names)
	if len(fieldSet) < 2 {
		t.Errorf("Expected at least 2 fields, got %d", len(fieldSet))
	}
}

func TestMeasurementFields_BackwardCompatibility(t *testing.T) {
	mf := NewMeasurementFields()
	
	// Create fields (simulating existing database)
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	fields["humidity"] = &Field{Name: "humidity", Type: influxql.Integer}
	mf.fields.Store(fields)
	
	// Test that existing fields work (implicit identity mapping)
	f1 := mf.Field("temperature")
	f2 := mf.Field("humidity")
	
	if f1 == nil || f1.Name != "temperature" {
		t.Error("Expected temperature field to be accessible")
	}
	if f2 == nil || f2.Name != "humidity" {
		t.Error("Expected humidity field to be accessible")
	}
	
	// Test FieldKeys includes user-facing names (implicit identity mapping)
	keys := mf.FieldKeys()
	if len(keys) < 2 {
		t.Errorf("Expected at least 2 keys, got %d", len(keys))
	}
	
	// Verify user-facing names are accessible
	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}
	if !keyMap["temperature"] || !keyMap["humidity"] {
		t.Error("Expected user-facing field names to be accessible")
	}
}
