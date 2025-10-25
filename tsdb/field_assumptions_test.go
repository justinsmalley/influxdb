package tsdb

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/influxdata/influxql"
)

// TestFieldNameCaseSensitivity tests whether field names are case-sensitive
func TestFieldNameCaseSensitivity(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create fields with different cases
	err1 := mf.CreateFieldIfNotExists([]byte("Temperature"), influxql.Float)
	err2 := mf.CreateFieldIfNotExists([]byte("temperature"), influxql.Float)

	if err1 != nil {
		t.Errorf("Failed to create 'Temperature': %v", err1)
	}
	if err2 != nil {
		t.Errorf("Failed to create 'temperature': %v", err2)
	}

	// Check if they are treated as different fields
	fields := mf.fields.Load().(map[string]*Field)
	if len(fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(fields))
	}

	// Verify both exist
	if fields["Temperature"] == nil {
		t.Error("Field 'Temperature' not found")
	}
	if fields["temperature"] == nil {
		t.Error("Field 'temperature' not found")
	}
}

// TestFieldNameSpecialCharacters tests field names with special characters
func TestFieldNameSpecialCharacters(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	testCases := []struct {
		name        string
		shouldFail  bool
		description string
	}{
		{"field with spaces", false, "field with spaces"},
		{"field-with-dashes", false, "field with dashes"},
		{"field.with.dots", false, "field with dots"},
		{"field👍with👍emoji", false, "field with emoji"},
		{"field_with_underscores", false, "field with underscores"},
		{"field123with456numbers", false, "field with numbers"},
		{"field!@#$%^&*()", false, "field with special chars"},
		{"field\nwith\nnewlines", true, "field with newlines"},
		{"field\twith\ttabs", true, "field with tabs"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := mf.CreateFieldIfNotExists([]byte(tc.name), influxql.Float)
			if tc.shouldFail && err == nil {
				t.Errorf("Expected error for field name '%s', but got none", tc.name)
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("Unexpected error for field name '%s': %v", tc.name, err)
			}
		})
	}
}

// TestFieldNameLengthLimits tests various field name lengths
func TestFieldNameLengthLimits(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	testCases := []struct {
		length      int
		shouldFail  bool
		description string
	}{
		{255, false, "exactly 255 characters"},
		{256, true, "256 characters (over limit)"},
		{1000, true, "1000 characters"},
		{10000, true, "10000 characters"},
		{1, false, "single character"},
		{0, true, "empty string"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			name := strings.Repeat("a", tc.length)
			err := mf.CreateFieldIfNotExists([]byte(name), influxql.Float)
			if tc.shouldFail && err == nil {
				t.Errorf("Expected error for field name length %d, but got none", tc.length)
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("Unexpected error for field name length %d: %v", tc.length, err)
			}
		})
	}
}

// TestConcurrentFieldOperations tests race conditions in field operations
func TestConcurrentFieldOperations(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Test concurrent field creation
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fieldName := fmt.Sprintf("field%d", i)
			err := mf.CreateFieldIfNotExists([]byte(fieldName), influxql.Float)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// Test concurrent field operations on same field
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := mf.CreateFieldIfNotExists([]byte("concurrent_field"), influxql.Float)
		if err != nil {
			errors <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait a bit then try to delete
		err := mf.SoftDeleteField("concurrent_field")
		if err != nil {
			errors <- err
		}
	}()

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify field count is reasonable
	fields := mf.fields.Load().(map[string]*Field)
	if len(fields) < 10 {
		t.Errorf("Expected at least 10 fields, got %d", len(fields))
	}
}

// TestFieldTypeConflicts tests field type conflict handling
func TestFieldTypeConflicts(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create field with integer type
	err := mf.CreateFieldIfNotExists([]byte("field"), influxql.Integer)
	if err != nil {
		t.Errorf("Failed to create integer field: %v", err)
	}

	// Try to create same field with float type
	err = mf.CreateFieldIfNotExists([]byte("field"), influxql.Float)
	if err != ErrFieldTypeConflict {
		t.Errorf("Expected ErrFieldTypeConflict, got: %v", err)
	}

	// Try to create same field with same type (should succeed)
	err = mf.CreateFieldIfNotExists([]byte("field"), influxql.Integer)
	if err != nil {
		t.Errorf("Expected no error for same type, got: %v", err)
	}
}

// TestFieldNameNormalization tests field name normalization
func TestFieldNameNormalization(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	testCases := []struct {
		name        string
		shouldFail  bool
		description string
	}{
		{"field ", true, "trailing space"},
		{" field", true, "leading space"},
		{" field ", true, "leading and trailing space"},
		{"field\t", true, "trailing tab"},
		{"field\n", true, "trailing newline"},
		{"field\r", true, "trailing carriage return"},
		{"field", false, "normal field"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := mf.CreateFieldIfNotExists([]byte(tc.name), influxql.Float)
			if tc.shouldFail && err == nil {
				t.Errorf("Expected error for field name '%s', but got none", tc.name)
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("Unexpected error for field name '%s': %v", tc.name, err)
			}
		})
	}
}

// TestFieldNameUTF8Encoding tests UTF-8 encoding handling
func TestFieldNameUTF8Encoding(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	testCases := []struct {
		name        string
		shouldFail  bool
		description string
	}{
		{"café", false, "é as single char"},
		{"cafe\u0301", false, "é as e + combining accent"},
		{"field\x00", true, "null byte"},
		{"field\xff", true, "invalid UTF-8"},
		{"field\xc0\x80", true, "overlong encoding"},
		{"field👍", false, "emoji"},
		{"fieldαβγ", false, "Greek letters"},
		{"field中文", false, "Chinese characters"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := mf.CreateFieldIfNotExists([]byte(tc.name), influxql.Float)
			if tc.shouldFail && err == nil {
				t.Errorf("Expected error for field name '%s', but got none", tc.name)
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("Unexpected error for field name '%s': %v", tc.name, err)
			}
		})
	}
}

// TestFieldNameCollisionEdgeCases tests edge cases in collision detection
func TestFieldNameCollisionEdgeCases(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	testCases := []struct {
		name        string
		shouldFail  bool
		description string
	}{
		{"temperature.v0", true, "version 0"},
		{"temperature.v-1", true, "negative version"},
		{"temperature.v999999999", true, "very large version"},
		{"temperature.v1.5", true, "decimal version"},
		{"temperature.v1a", true, "non-numeric version"},
		{"temperature.v", true, "incomplete version"},
		{"temperature.v1.", true, "trailing dot"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := mf.CreateFieldIfNotExists([]byte(tc.name), influxql.Float)
			if tc.shouldFail && err == nil {
				t.Errorf("Expected error for field name '%s', but got none", tc.name)
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("Unexpected error for field name '%s': %v", tc.name, err)
			}
		})
	}
}

// TestFieldNamePersistence tests field name persistence
func TestFieldNamePersistence(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create field with special name
	fieldName := "field👍with👍emoji"
	err := mf.CreateFieldIfNotExists([]byte(fieldName), influxql.Float)
	if err != nil {
		t.Errorf("Failed to create field: %v", err)
	}

	// Verify field exists
	fields := mf.fields.Load().(map[string]*Field)
	if fields[fieldName] == nil {
		t.Error("Field not found after creation")
	}

	// Test field retrieval
	field := mf.Field(fieldName)
	if field == nil {
		t.Error("Field() returned nil")
	}
	if field.Name != fieldName {
		t.Errorf("Field name mismatch: expected '%s', got '%s'", fieldName, field.Name)
	}
}

// TestFieldNameQueryCompatibility tests field name compatibility with queries
func TestFieldNameQueryCompatibility(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create field with special name
	fieldName := "field_with_special_chars"
	err := mf.CreateFieldIfNotExists([]byte(fieldName), influxql.Float)
	if err != nil {
		t.Errorf("Failed to create field: %v", err)
	}

	// Test field keys
	keys := mf.FieldKeys()
	found := false
	for _, key := range keys {
		if key == fieldName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Field '%s' not found in field keys", fieldName)
	}

	// Test field set
	fieldSet := mf.FieldSet()
	if fieldSet[fieldName] != influxql.Float {
		t.Errorf("Field type mismatch: expected %v, got %v", influxql.Float, fieldSet[fieldName])
	}

	// Test field existence
	if !mf.HasField(fieldName) {
		t.Error("HasField() returned false for existing field")
	}
}

// TestFieldNameReservedNames tests reserved field names
func TestFieldNameReservedNames(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	reservedNames := []string{
		"_name",
		"_tagKey",
		"_tagValue",
		"_seriesKey",
		"time",
	}

	for _, name := range reservedNames {
		t.Run(name, func(t *testing.T) {
			err := mf.CreateFieldIfNotExists([]byte(name), influxql.Float)
			if err == nil {
				t.Errorf("Expected error for reserved field name '%s', but got none", name)
			}
		})
	}
}

// TestFieldNameInternalDelimiter tests internal delimiter conflicts
func TestFieldNameInternalDelimiter(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	testCases := []struct {
		name        string
		shouldFail  bool
		description string
	}{
		{"field#!~#value", true, "contains internal delimiter"},
		{"field#!~#", true, "ends with internal delimiter"},
		{"#!~#field", true, "starts with internal delimiter"},
		{"field#!~#value#!~#more", true, "multiple internal delimiters"},
		{"field", false, "normal field"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := mf.CreateFieldIfNotExists([]byte(tc.name), influxql.Float)
			if tc.shouldFail && err == nil {
				t.Errorf("Expected error for field name '%s', but got none", tc.name)
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("Unexpected error for field name '%s': %v", tc.name, err)
			}
		})
	}
}

// TestFieldNameUnicodeNormalization tests Unicode normalization
func TestFieldNameUnicodeNormalization(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Test if different Unicode representations of the same character
	// are treated as different fields
	name1 := "café"                    // é as single char
	name2 := "cafe\u0301"              // é as e + combining accent

	err1 := mf.CreateFieldIfNotExists([]byte(name1), influxql.Float)
	err2 := mf.CreateFieldIfNotExists([]byte(name2), influxql.Float)

	if err1 != nil {
		t.Errorf("Failed to create field '%s': %v", name1, err1)
	}
	if err2 != nil {
		t.Errorf("Failed to create field '%s': %v", name2, err2)
	}

	// Check if they are treated as different fields
	fields := mf.fields.Load().(map[string]*Field)
	if len(fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(fields))
	}

	// Verify both exist
	if fields[name1] == nil {
		t.Errorf("Field '%s' not found", name1)
	}
	if fields[name2] == nil {
		t.Errorf("Field '%s' not found", name2)
	}
}

// TestFieldNamePerformance tests performance with many fields
func TestFieldNamePerformance(t *testing.T) {
	mf := &MeasurementFields{}
	mf.fields.Store(make(map[string]*Field))
	mf.mappings.Store(make(map[string]*FieldMapping))

	// Create many fields to test performance
	numFields := 1000
	for i := 0; i < numFields; i++ {
		fieldName := fmt.Sprintf("field%d", i)
		err := mf.CreateFieldIfNotExists([]byte(fieldName), influxql.Float)
		if err != nil {
			t.Errorf("Failed to create field %d: %v", i, err)
		}
	}

	// Verify all fields exist
	fields := mf.fields.Load().(map[string]*Field)
	if len(fields) != numFields {
		t.Errorf("Expected %d fields, got %d", numFields, len(fields))
	}

	// Test field lookup performance
	for i := 0; i < numFields; i++ {
		fieldName := fmt.Sprintf("field%d", i)
		field := mf.Field(fieldName)
		if field == nil {
			t.Errorf("Field '%s' not found", fieldName)
		}
	}
}
