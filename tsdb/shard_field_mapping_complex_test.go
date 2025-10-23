package tsdb

import (
	"testing"

	"github.com/influxdata/influxql"
)

func TestComplexFieldMappingScenario(t *testing.T) {
	// This test covers a complex field mapping scenario with multiple operations
	
	mf := NewMeasurementFields()
	
	// Step 1: Create existing field called "temperature"
	t.Run("Step1_CreateExistingField", func(t *testing.T) {
		fields := make(map[string]*Field)
		fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
		mf.fields.Store(fields)
		
		// Verify field exists and is accessible
		f := mf.Field("temperature")
		if f == nil || f.Name != "temperature" {
			t.Error("Expected temperature field to be accessible")
		}
		
		// Verify it's in field keys
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		if !keyMap["temperature"] {
			t.Error("Expected temperature to be in field keys")
		}
	})
	
	// Step 2: Rename "temperature" to "Air Temperature"
	t.Run("Step2_RenameTemperatureToAirTemperature", func(t *testing.T) {
		err := mf.RenameField("temperature", "Air Temperature")
		if err != nil {
			t.Errorf("Unexpected error renaming field: %v", err)
		}
		
		// Verify old name is inactive
		_, isActive := mf.GetInternalFieldName("temperature")
		if isActive {
			t.Error("Expected old temperature name to be inactive")
		}
		
		// Verify new name is active
		internalName, isActive := mf.GetInternalFieldName("Air Temperature")
		if !isActive {
			t.Error("Expected Air Temperature name to be active")
		}
		if internalName != "temperature" {
			t.Errorf("Expected internal name 'temperature', got '%s'", internalName)
		}
		
		// Verify both names access the same field
		f1 := mf.Field("temperature")
		f2 := mf.Field("Air Temperature")
		if f1 == nil || f2 == nil || f1 != f2 {
			t.Error("Expected both names to access the same field")
		}
		
		// Verify field keys only show active name
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		if keyMap["temperature"] {
			t.Error("Expected old temperature name to not be in field keys")
		}
		if !keyMap["Air Temperature"] {
			t.Error("Expected Air Temperature to be in field keys")
		}
	})
	
	// Step 3: Insert data for new field called "temperature"
	t.Run("Step3_CreateNewTemperatureField", func(t *testing.T) {
		// Create new mapping for recreated field
		internalName, err := mf.CreateFieldMapping("temperature")
		if err != nil {
			t.Errorf("Unexpected error creating field mapping: %v", err)
		}
		
		expectedInternalName := "temperature.v2"
		if internalName != expectedInternalName {
			t.Errorf("Expected internal name '%s', got '%s'", expectedInternalName, internalName)
		}
		
		// Verify new mapping is active
		_, isActive := mf.GetInternalFieldName("temperature")
		if !isActive {
			t.Error("Expected new temperature mapping to be active")
		}
		
		// Verify field keys include both active fields
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		if !keyMap["temperature"] {
			t.Error("Expected new temperature to be in field keys")
		}
		if !keyMap["Air Temperature"] {
			t.Error("Expected Air Temperature to still be in field keys")
		}
	})
	
	// Step 4: Rename new "temperature" field to "Water Temperature"
	t.Run("Step4_RenameNewTemperatureToWaterTemperature", func(t *testing.T) {
		err := mf.RenameField("temperature", "Water Temperature")
		if err != nil {
			t.Errorf("Unexpected error renaming field: %v", err)
		}
		
		// Verify old name is inactive
		_, isActive := mf.GetInternalFieldName("temperature")
		if isActive {
			t.Error("Expected old temperature name to be inactive")
		}
		
		// Verify new name is active
		internalName, isActive := mf.GetInternalFieldName("Water Temperature")
		if !isActive {
			t.Error("Expected Water Temperature name to be active")
		}
		if internalName != "temperature.v2" {
			t.Errorf("Expected internal name 'temperature.v2', got '%s'", internalName)
		}
		
		// Verify field keys show correct active fields
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		if keyMap["temperature"] {
			t.Error("Expected old temperature name to not be in field keys")
		}
		if !keyMap["Water Temperature"] {
			t.Error("Expected Water Temperature to be in field keys")
		}
		if !keyMap["Air Temperature"] {
			t.Error("Expected Air Temperature to still be in field keys")
		}
	})
	
	// Step 5: Delete "Air Temperature"
	t.Run("Step5_DeleteAirTemperature", func(t *testing.T) {
		err := mf.SoftDeleteField("Air Temperature")
		if err != nil {
			t.Errorf("Unexpected error deleting field: %v", err)
		}
		
		// Verify field is inactive
		_, isActive := mf.GetInternalFieldName("Air Temperature")
		if isActive {
			t.Error("Expected Air Temperature to be inactive after deletion")
		}
		
		// Verify field is not accessible in modified version
		f := mf.Field("Air Temperature")
		if f != nil {
			t.Error("Expected deleted Air Temperature field to be inaccessible")
		}
		
		// Verify field keys don't include deleted field
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		if keyMap["Air Temperature"] {
			t.Error("Expected deleted Air Temperature to not be in field keys")
		}
		if !keyMap["Water Temperature"] {
			t.Error("Expected Water Temperature to still be in field keys")
		}
	})
	
	// Step 6: Create new field called "temperature"
	t.Run("Step6_CreateAnotherTemperatureField", func(t *testing.T) {
		// Create new mapping for recreated field
		internalName, err := mf.CreateFieldMapping("temperature")
		if err != nil {
			t.Errorf("Unexpected error creating field mapping: %v", err)
		}
		
		expectedInternalName := "temperature.v3"
		if internalName != expectedInternalName {
			t.Errorf("Expected internal name '%s', got '%s'", expectedInternalName, internalName)
		}
		
		// Verify new mapping is active
		_, isActive := mf.GetInternalFieldName("temperature")
		if !isActive {
			t.Error("Expected new temperature mapping to be active")
		}
		
		// Verify field keys include all active fields
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		if !keyMap["temperature"] {
			t.Error("Expected new temperature to be in field keys")
		}
		if !keyMap["Water Temperature"] {
			t.Error("Expected Water Temperature to still be in field keys")
		}
	})
	
	// Step 7: Create new field called "Air Temperature"
	t.Run("Step7_CreateNewAirTemperatureField", func(t *testing.T) {
		// Create new mapping for recreated field
		internalName, err := mf.CreateFieldMapping("Air Temperature")
		if err != nil {
			t.Errorf("Unexpected error creating field mapping: %v", err)
		}
		
		expectedInternalName := "Air Temperature.v2"
		if internalName != expectedInternalName {
			t.Errorf("Expected internal name '%s', got '%s'", expectedInternalName, internalName)
		}
		
		// Verify new mapping is active
		_, isActive := mf.GetInternalFieldName("Air Temperature")
		if !isActive {
			t.Error("Expected new Air Temperature mapping to be active")
		}
		
		// Verify field keys include all active fields
		keys := mf.FieldKeys()
		keyMap := make(map[string]bool)
		for _, key := range keys {
			keyMap[key] = true
		}
		if !keyMap["Air Temperature"] {
			t.Error("Expected new Air Temperature to be in field keys")
		}
		if !keyMap["temperature"] {
			t.Error("Expected temperature to still be in field keys")
		}
		if !keyMap["Water Temperature"] {
			t.Error("Expected Water Temperature to still be in field keys")
		}
	})
	
	// Final verification: Check the complete mapping state
	t.Run("FinalVerification_MappingState", func(t *testing.T) {
		// Expected final mapping:
		// "temperature" --> "Air Temperature" (DELETED) - this is the original field that was renamed then deleted
		// "temperature.v2" --> "Water Temperature" (ACTIVE) - this is the second temperature field that was renamed
		// "temperature.v3" --> "temperature" (ACTIVE) - this is the third temperature field
		// "Air Temperature.v2" --> "Air Temperature" (ACTIVE) - this is the new Air Temperature field created after deletion
		
		// Verify all active fields are accessible
		activeFields := mf.GetActiveUserFields()
		expectedActiveFields := []string{"Air Temperature", "temperature", "Water Temperature"}
		
		if len(activeFields) != len(expectedActiveFields) {
			t.Errorf("Expected %d active fields, got %d", len(expectedActiveFields), len(activeFields))
		}
		
		activeMap := make(map[string]bool)
		for _, field := range activeFields {
			activeMap[field] = true
		}
		
		for _, expectedField := range expectedActiveFields {
			if !activeMap[expectedField] {
				t.Errorf("Expected active field '%s' not found", expectedField)
			}
		}
		
		// Verify field access works for all active fields
		for _, fieldName := range expectedActiveFields {
			f := mf.Field(fieldName)
			if f == nil {
				t.Errorf("Expected field '%s' to be accessible", fieldName)
			}
		}
		
		// Verify the specific internal mappings
		// "temperature" should map to "temperature.v3" (the latest active one)
		internalName, isActive := mf.GetInternalFieldName("temperature")
		if !isActive {
			t.Error("Expected temperature field to be active")
		}
		if internalName != "temperature.v3" {
			t.Errorf("Expected temperature to map to 'temperature.v3', got '%s'", internalName)
		}
		
		// "Water Temperature" should map to "temperature.v2"
		internalName, isActive = mf.GetInternalFieldName("Water Temperature")
		if !isActive {
			t.Error("Expected Water Temperature field to be active")
		}
		if internalName != "temperature.v2" {
			t.Errorf("Expected Water Temperature to map to 'temperature.v2', got '%s'", internalName)
		}
		
		// "Air Temperature" should map to "Air Temperature.v2"
		internalName, isActive = mf.GetInternalFieldName("Air Temperature")
		if !isActive {
			t.Error("Expected Air Temperature field to be active")
		}
		if internalName != "Air Temperature.v2" {
			t.Errorf("Expected Air Temperature to map to 'Air Temperature.v2', got '%s'", internalName)
		}
		
		// Verify that the original "temperature" field (now deleted) is not accessible
		// This should not be accessible because it was renamed to "Air Temperature" then deleted
		f := mf.Field("temperature") // This should access the new temperature field (v3)
		if f == nil {
			t.Error("Expected new temperature field to be accessible")
		}
		if f.Name != "temperature.v3" {
			t.Errorf("Expected new temperature field to have internal name 'temperature.v3', got '%s'", f.Name)
		}
	})
}

func TestDuplicateActiveFieldNamePrevention(t *testing.T) {
	// This test ensures that there cannot be more than one active field with the same external name
	
	mf := NewMeasurementFields()
	
	// Create initial field
	fields := make(map[string]*Field)
	fields["temperature"] = &Field{Name: "temperature", Type: influxql.Float}
	mf.fields.Store(fields)
	
	// Try to create another active field with the same name
	// This should fail because the field already exists and is active
	_, err := mf.CreateFieldMapping("temperature")
	if err == nil {
		t.Error("Expected error when trying to create duplicate active field name")
	}
	if err.Error() != "field 'temperature' already exists and is active" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
	
	// Verify that only one temperature field exists
	keys := mf.FieldKeys()
	temperatureCount := 0
	for _, key := range keys {
		if key == "temperature" {
			temperatureCount++
		}
	}
	
	if temperatureCount != 1 {
		t.Errorf("Expected exactly 1 temperature field in keys, got %d", temperatureCount)
	}
	
	// Now test with rename - try to rename a field to an existing active field name
	err = mf.RenameField("temperature", "temperature") // Same name
	if err == nil {
		t.Error("Expected error when trying to rename field to same name")
	}
	
	// Create another field and try to rename it to temperature
	fields["humidity"] = &Field{Name: "humidity", Type: influxql.Float}
	mf.fields.Store(fields)
	
	err = mf.RenameField("humidity", "temperature")
	if err == nil {
		t.Error("Expected error when trying to rename field to existing active field name")
	}
	if err.Error() != "field temperature already exists and is active" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}
