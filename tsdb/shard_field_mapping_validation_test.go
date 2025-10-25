package tsdb

import (
	"testing"

	"github.com/influxdata/influxql"
)

func TestMeasurementFields_CreateFieldIfNotExists_Validation(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	tests := []struct {
		name        string
		fieldName   string
		fieldType   influxql.DataType
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid field name",
			fieldName:   "temperature",
			fieldType:   influxql.Float,
			expectError: false,
		},
		{
			name:        "invalid field name - empty",
			fieldName:   "",
			fieldType:   influxql.Float,
			expectError: true,
			errorMsg:    "field name cannot be empty",
		},
		{
			name:        "invalid field name - contains delimiter",
			fieldName:   "temp#!~#value",
			fieldType:   influxql.Float,
			expectError: true,
			errorMsg:    "field name cannot contain reserved pattern",
		},
		{
			name:        "invalid field name - version suffix",
			fieldName:   "temperature.v1",
			fieldType:   influxql.Float,
			expectError: true,
			errorMsg:    "field name cannot end with versioning pattern",
		},
		{
			name:        "invalid field name - reserved name",
			fieldName:   "time",
			fieldType:   influxql.Float,
			expectError: true,
			errorMsg:    "field name 'time' is reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mf.CreateFieldIfNotExists([]byte(tt.fieldName), tt.fieldType)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("CreateFieldIfNotExists(%q, %v) expected error but got nil", tt.fieldName, tt.fieldType)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("CreateFieldIfNotExists(%q, %v) expected error containing '%s' but got '%s'", 
						tt.fieldName, tt.fieldType, tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("CreateFieldIfNotExists(%q, %v) expected no error but got: %v", tt.fieldName, tt.fieldType, err)
				}
			}
		})
	}
}

func TestMeasurementFields_SoftDeleteField_Validation(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create a valid field first
	err := mf.CreateFieldIfNotExists([]byte("temperature"), influxql.Float)
	if err != nil {
		t.Fatalf("Failed to create test field: %v", err)
	}

	tests := []struct {
		name        string
		fieldName   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid field name",
			fieldName:   "temperature",
			expectError: false,
		},
		{
			name:        "invalid field name - empty",
			fieldName:   "",
			expectError: true,
			errorMsg:    "field name cannot be empty",
		},
		{
			name:        "invalid field name - contains delimiter",
			fieldName:   "temp#!~#value",
			expectError: true,
			errorMsg:    "field name cannot contain reserved pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mf.SoftDeleteField(tt.fieldName)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("SoftDeleteField(%q) expected error but got nil", tt.fieldName)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("SoftDeleteField(%q) expected error containing '%s' but got '%s'", 
						tt.fieldName, tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("SoftDeleteField(%q) expected no error but got: %v", tt.fieldName, err)
				}
			}
		})
	}
}

func TestMeasurementFields_RenameField_Validation(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create a valid field first
	err := mf.CreateFieldIfNotExists([]byte("temperature"), influxql.Float)
	if err != nil {
		t.Fatalf("Failed to create test field: %v", err)
	}

	tests := []struct {
		name        string
		oldName     string
		newName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid rename",
			oldName:     "temperature",
			newName:     "air_temperature",
			expectError: false,
		},
		{
			name:        "invalid old name - empty",
			oldName:     "",
			newName:     "air_temperature",
			expectError: true,
			errorMsg:    "field name cannot be empty",
		},
		{
			name:        "invalid new name - empty",
			oldName:     "temperature",
			newName:     "",
			expectError: true,
			errorMsg:    "field name cannot be empty",
		},
		{
			name:        "invalid new name - version suffix",
			oldName:     "temperature",
			newName:     "air_temperature.v1",
			expectError: true,
			errorMsg:    "field name cannot end with versioning pattern",
		},
		{
			name:        "invalid new name - reserved",
			oldName:     "temperature",
			newName:     "time",
			expectError: true,
			errorMsg:    "field name 'time' is reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mf.RenameField(tt.oldName, tt.newName)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("RenameField(%q, %q) expected error but got nil", tt.oldName, tt.newName)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("RenameField(%q, %q) expected error containing '%s' but got '%s'", 
						tt.oldName, tt.newName, tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("RenameField(%q, %q) expected no error but got: %v", tt.oldName, tt.newName, err)
				}
			}
		})
	}
}

func TestMeasurementFields_CreateFieldMapping_Validation(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	tests := []struct {
		name        string
		fieldName   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid field name",
			fieldName:   "temperature",
			expectError: false,
		},
		{
			name:        "invalid field name - empty",
			fieldName:   "",
			expectError: true,
			errorMsg:    "field name cannot be empty",
		},
		{
			name:        "invalid field name - contains delimiter",
			fieldName:   "temp#!~#value",
			expectError: true,
			errorMsg:    "field name cannot contain reserved pattern",
		},
		{
			name:        "invalid field name - version suffix",
			fieldName:   "temperature.v1",
			expectError: true,
			errorMsg:    "field name cannot end with versioning pattern",
		},
		{
			name:        "invalid field name - reserved",
			fieldName:   "time",
			expectError: true,
			errorMsg:    "field name 'time' is reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mf.CreateFieldMapping(tt.fieldName)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("CreateFieldMapping(%q) expected error but got nil", tt.fieldName)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("CreateFieldMapping(%q) expected error containing '%s' but got '%s'", 
						tt.fieldName, tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("CreateFieldMapping(%q) expected no error but got: %v", tt.fieldName, err)
				}
			}
		})
	}
}

func TestMeasurementFields_CreateFieldMapping_LegacyCollision(t *testing.T) {
	mf := &MeasurementFields{}
	
	// Simulate a legacy database with fields named "temperature.v2" and "temperature.v3"
	legacyFields := map[string]*Field{
		"temperature.v2": {Name: "temperature.v2", Type: influxql.Float},
		"temperature.v3": {Name: "temperature.v3", Type: influxql.Float},
	}
	mf.fields.Store(legacyFields)
	
	// No explicit mappings (simulating old database)
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Try to create a new field mapping for "temperature" 
	// This should skip v2 and v3 (legacy fields) and create v4
	internalName, err := mf.CreateFieldMapping("temperature")
	
	if err != nil {
		t.Errorf("CreateFieldMapping('temperature') expected success but got error: %v", err)
		return
	}
	
	// Should create "temperature.v4" since v2 and v3 are taken by legacy fields
	expectedInternalName := "temperature.v4"
	if internalName != expectedInternalName {
		t.Errorf("CreateFieldMapping('temperature') expected internal name '%s' but got '%s'", 
			expectedInternalName, internalName)
	}
}

func TestMeasurementFields_CreateFieldMapping_LegacyCollision_WithExplicitMapping(t *testing.T) {
	mf := &MeasurementFields{}
	
	// Simulate a database with a field named "temperature.v2" that has an explicit mapping
	legacyFields := map[string]*Field{
		"temperature.v2": {Name: "temperature.v2", Type: influxql.Float},
	}
	mf.fields.Store(legacyFields)
	
	// Explicit mapping for the legacy field (not implicit)
	legacyMappings := map[string]*FieldMapping{
		"temperature.v2": {
			UserName:     "temperature.v2",
			InternalName: "temperature.v2",
			Version:      1,
			State:        FieldMappingState_ACTIVE,
		},
	}
	mf.mappings.Store(legacyMappings)

	// Try to create a new field mapping for "temperature" 
	// This should succeed because the legacy field has an explicit mapping
	internalName, err := mf.CreateFieldMapping("temperature")
	
	if err != nil {
		t.Errorf("CreateFieldMapping('temperature') expected success but got error: %v", err)
		return
	}
	
	// Should create "temperature.v3" since v2 is taken by explicit mapping
	expectedInternalName := "temperature.v3"
	if internalName != expectedInternalName {
		t.Errorf("CreateFieldMapping('temperature') expected internal name '%s' but got '%s'", 
			expectedInternalName, internalName)
	}
}

func TestMeasurementFields_CreateFieldMapping_ManyLegacyCollisions(t *testing.T) {
	mf := &MeasurementFields{}
	
	// Simulate a legacy database with many versioned fields
	legacyFields := map[string]*Field{
		"temperature.v2": {Name: "temperature.v2", Type: influxql.Float},
		"temperature.v3": {Name: "temperature.v3", Type: influxql.Float},
		"temperature.v4": {Name: "temperature.v4", Type: influxql.Float},
		"temperature.v5": {Name: "temperature.v5", Type: influxql.Float},
		"temperature.v6": {Name: "temperature.v6", Type: influxql.Float},
	}
	mf.fields.Store(legacyFields)
	
	// No explicit mappings (simulating old database)
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Try to create a new field mapping for "temperature" 
	// This should skip v2-v6 (legacy fields) and create v7
	internalName, err := mf.CreateFieldMapping("temperature")
	
	if err != nil {
		t.Errorf("CreateFieldMapping('temperature') expected success but got error: %v", err)
		return
	}
	
	// Should create "temperature.v7" since v2-v6 are taken by legacy fields
	expectedInternalName := "temperature.v7"
	if internalName != expectedInternalName {
		t.Errorf("CreateFieldMapping('temperature') expected internal name '%s' but got '%s'", 
			expectedInternalName, internalName)
	}
}

func TestMeasurementFields_CreateFieldMapping_MultipleVersions(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create multiple versions of the same field
	testFieldName := "temperature"
	
	// First version should be v2
	internalName1, err := mf.CreateFieldMapping(testFieldName)
	if err != nil {
		t.Fatalf("First CreateFieldMapping failed: %v", err)
	}
	if internalName1 != "temperature.v2" {
		t.Errorf("First version expected 'temperature.v2' but got '%s'", internalName1)
	}
	
	// Soft delete the field
	err = mf.SoftDeleteField(testFieldName)
	if err != nil {
		t.Fatalf("SoftDeleteField failed: %v", err)
	}
	
	// Second version should be v3
	internalName2, err := mf.CreateFieldMapping(testFieldName)
	if err != nil {
		t.Fatalf("Second CreateFieldMapping failed: %v", err)
	}
	if internalName2 != "temperature.v3" {
		t.Errorf("Second version expected 'temperature.v3' but got '%s'", internalName2)
	}
	
	// Soft delete again
	err = mf.SoftDeleteField(testFieldName)
	if err != nil {
		t.Fatalf("Second SoftDeleteField failed: %v", err)
	}
	
	// Third version should be v4
	internalName3, err := mf.CreateFieldMapping(testFieldName)
	if err != nil {
		t.Fatalf("Third CreateFieldMapping failed: %v", err)
	}
	if internalName3 != "temperature.v4" {
		t.Errorf("Third version expected 'temperature.v4' but got '%s'", internalName3)
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    len(s) > len(substr) && 
		    (s[:len(substr)] == substr || 
		     s[len(s)-len(substr):] == substr || 
		     containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
