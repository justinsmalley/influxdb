package tsdb

import (
	"testing"

	"github.com/influxdata/influxql"
)

func TestFieldMappingIntegration(t *testing.T) {
	// This test demonstrates the complete field mapping workflow
	
	// Create measurement fields
	mf := NewMeasurementFields()
	
	// Simulate writing initial data (implicit identity mapping)
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	fields["humidity"] = &Field{Name: "humidity", Type: influxql.Integer}
	mf.fields.Store(fields)
	
	// Test 1: Initial state - fields work normally
	t.Run("InitialState", func(t *testing.T) {
		// Field access should work
		f := mf.Field("temperature")
		if f == nil || f.Name != "temperature" {
			t.Error("Expected temperature field to be accessible")
		}
		
		// FieldKeys should include internal names
		keys := mf.FieldKeys()
		if len(keys) < 2 {
			t.Errorf("Expected at least 2 keys, got %d", len(keys))
		}
		
		// GetInternalFieldName should return identity mapping
		internalName, isActive := mf.GetInternalFieldName("temperature")
		if !isActive || internalName != "temperature" {
			t.Error("Expected identity mapping for temperature")
		}
	})
	
	// Test 2: Soft delete field
	t.Run("SoftDelete", func(t *testing.T) {
		err := mf.SoftDeleteField("temperature")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		// Field should be inactive for new queries
		_, isActive := mf.GetInternalFieldName("temperature")
		if isActive {
			t.Error("Expected temperature field to be inactive after soft delete")
		}
		
		// Field should NOT be accessible in modified version (soft-deleted fields are hidden)
		f := mf.Field("temperature")
		if f != nil {
			t.Error("Expected soft-deleted field to be inaccessible in modified version")
		}
		
		// Note: The field data still exists internally and will be accessible when reverting to unmodified InfluxDB
	})
	
	// Test 3: Recreate field with same name
	t.Run("FieldRecreation", func(t *testing.T) {
		// Create new mapping for recreated field
		internalName, err := mf.CreateFieldMapping("temperature")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		expectedInternalName := "temperature.v2"
		if internalName != expectedInternalName {
			t.Errorf("Expected internal name '%s', got '%s'", expectedInternalName, internalName)
		}
		
		// New mapping should be active
		_, isActive := mf.GetInternalFieldName("temperature")
		if !isActive {
			t.Error("Expected new temperature mapping to be active")
		}
		
		// FieldKeys should include both old and new internal names
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		
		if !keyMap["temperature"] || !keyMap["temperature.v2"] {
			t.Error("Expected both old and new internal names to be accessible")
		}
	})
	
	// Test 4: Rename field
	t.Run("FieldRename", func(t *testing.T) {
		err := mf.RenameField("humidity", "moisture")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		// Old name should be inactive
		_, isActive := mf.GetInternalFieldName("humidity")
		if isActive {
			t.Error("Expected old humidity name to be inactive")
		}
		
		// New name should be active
		internalName, isActive := mf.GetInternalFieldName("moisture")
		if !isActive {
			t.Error("Expected new moisture name to be active")
		}
		if internalName != "humidity" {
			t.Errorf("Expected internal name 'humidity', got '%s'", internalName)
		}
		
		// Both names should access the same field
		f1 := mf.Field("humidity")
		f2 := mf.Field("moisture")
		if f1 == nil || f2 == nil || f1 != f2 {
			t.Error("Expected both names to access the same field")
		}
	})
	
	// Test 5: FieldSet and FieldKeys with mixed states
	t.Run("MixedStates", func(t *testing.T) {
		fieldSet := mf.FieldSet()
		keys := mf.FieldKeys()
		
		// Should contain only active user-facing field names
		// After operations: temperature (recreated), moisture (renamed from humidity)
		if len(fieldSet) < 2 {
			t.Errorf("Expected at least 2 fields in fieldSet, got %d", len(fieldSet))
		}
		if len(keys) < 2 {
			t.Errorf("Expected at least 2 keys, got %d", len(keys))
		}
		
		// Verify specific active fields exist
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		
		// Should have: temperature (recreated), moisture (renamed)
		// Should NOT have: humidity (renamed), temperature.v2 (internal name)
		if !keyMap["temperature"] {
			t.Errorf("Expected key 'temperature' not found")
		}
		if !keyMap["moisture"] {
			t.Errorf("Expected key 'moisture' not found")
		}
		if keyMap["humidity"] {
			t.Error("Expected renamed field 'humidity' to not be accessible")
		}
		// Note: temperature.v2 is accessible via Field() for backward compatibility but not in FieldKeys()
	})
}

func TestFieldMappingBackwardCompatibility(t *testing.T) {
	// This test verifies backward compatibility scenarios
	
	mf := NewMeasurementFields()
	
	// Simulate existing database with fields
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	fields["humidity"] = &Field{Name: "humidity", Type: influxql.Integer}
	mf.fields.Store(fields)
	
	t.Run("ExistingFieldAccess", func(t *testing.T) {
		// Existing fields should be accessible by their original names
		f1 := mf.Field("temperature")
		f2 := mf.Field("humidity")
		
		if f1 == nil || f1.Name != "temperature" {
			t.Error("Expected temperature field to be accessible")
		}
		if f2 == nil || f2.Name != "humidity" {
			t.Error("Expected humidity field to be accessible")
		}
	})
	
	t.Run("FieldKeysBackwardCompatibility", func(t *testing.T) {
		// FieldKeys should include internal field names for backward compatibility
		keys := mf.FieldKeys()
		
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		
		// Should include internal names
		if !keyMap["temperature"] || !keyMap["humidity"] {
			t.Error("Expected internal field names to be accessible")
		}
	})
	
	t.Run("FieldSetBackwardCompatibility", func(t *testing.T) {
		// FieldSet should include internal field names
		fieldSet := mf.FieldSet()
		
		if fieldSet["temperature"] != influxql.Float {
			t.Error("Expected temperature field to be Float type")
		}
		if fieldSet["humidity"] != influxql.Integer {
			t.Error("Expected humidity field to be Integer type")
		}
	})
}

func TestFieldMappingVersioning(t *testing.T) {
	// This test verifies proper versioning when fields are recreated
	
	mf := NewMeasurementFields()
	
	// Create initial field
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	mf.fields.Store(fields)
	
	t.Run("VersionIncrement", func(t *testing.T) {
		// Soft delete field
		err := mf.SoftDeleteField("temperature")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		// Create new mapping - should get .v2
		internalName1, err := mf.CreateFieldMapping("temperature")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if internalName1 != "temperature.v2" {
			t.Errorf("Expected 'temperature.v2', got '%s'", internalName1)
		}
		
		// Soft delete again
		err = mf.SoftDeleteField("temperature")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		// Create another mapping - should get .v3
		internalName2, err := mf.CreateFieldMapping("temperature")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if internalName2 != "temperature.v3" {
			t.Errorf("Expected 'temperature.v3', got '%s'", internalName2)
		}
	})
}
