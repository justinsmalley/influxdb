package tsdb

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidateFieldName(t *testing.T) {
	tests := []struct {
		name        string
		fieldName   string
		expectError bool
		errorMsg    string
	}{
		// Valid field names
		{
			name:        "valid simple name",
			fieldName:   "temperature",
			expectError: false,
		},
		{
			name:        "valid with underscore",
			fieldName:   "cpu_usage",
			expectError: false,
		},
		{
			name:        "valid with numbers",
			fieldName:   "sensor1",
			expectError: false,
		},
		{
			name:        "valid unicode",
			fieldName:   "温度",
			expectError: false,
		},
		{
			name:        "valid mixed unicode",
			fieldName:   "temp_温度_123",
			expectError: false,
		},

		// Empty and whitespace
		{
			name:        "empty string",
			fieldName:   "",
			expectError: true,
			errorMsg:    "field name cannot be empty",
		},
		{
			name:        "leading whitespace",
			fieldName:   " temperature",
			expectError: true,
			errorMsg:    "field name cannot have leading or trailing whitespace",
		},
		{
			name:        "trailing whitespace",
			fieldName:   "temperature ",
			expectError: true,
			errorMsg:    "field name cannot have leading or trailing whitespace",
		},
		{
			name:        "both leading and trailing whitespace",
			fieldName:   " temperature ",
			expectError: true,
			errorMsg:    "field name cannot have leading or trailing whitespace",
		},

		// Length validation
		{
			name:        "exceeds max length",
			fieldName:   strings.Repeat("a", MaxFieldNameLength+1),
			expectError: true,
			errorMsg:    "field name exceeds maximum length",
		},
		{
			name:        "exactly max length",
			fieldName:   strings.Repeat("a", MaxFieldNameLength),
			expectError: false,
		},

		// Invalid UTF-8
		{
			name:        "invalid UTF-8 sequence",
			fieldName:   "\xff\xfe",
			expectError: true,
			errorMsg:    "field name must be valid UTF-8",
		},

		// Null bytes
		{
			name:        "contains null byte",
			fieldName:   "temp\x00value",
			expectError: true,
			errorMsg:    "field name cannot contain null bytes",
		},

		// Internal delimiter
		{
			name:        "contains internal delimiter",
			fieldName:   "temp#!~#value",
			expectError: true,
			errorMsg:    "field name cannot contain reserved pattern '#!~#'",
		},
		{
			name:        "starts with internal delimiter",
			fieldName:   "#!~#value",
			expectError: true,
			errorMsg:    "field name cannot contain reserved pattern '#!~#'",
		},
		{
			name:        "ends with internal delimiter",
			fieldName:   "temp#!~#",
			expectError: true,
			errorMsg:    "field name cannot contain reserved pattern '#!~#'",
		},

		// Version suffix pattern — .v<N> names are deliberately allowed.
		// The mapping layer handles collisions transparently by cascading
		// internal suffixes (e.g., user "temp.v1" -> internal "temp.v1.v2").
		{
			name:        "ends with .v1",
			fieldName:   "temperature.v1",
			expectError: false,
		},
		{
			name:        "ends with .v2",
			fieldName:   "temperature.v2",
			expectError: false,
		},
		{
			name:        "ends with .v999",
			fieldName:   "temperature.v999",
			expectError: false,
		},
		{
			name:        "contains .v1 in middle",
			fieldName:   "temp.v1.value",
			expectError: false,
		},
		{
			name:        "ends with .v (no number)",
			fieldName:   "temperature.v",
			expectError: false,
		},

		// Reserved names
		{
			name:        "reserved _name",
			fieldName:   "_name",
			expectError: true,
			errorMsg:    "field name '_name' is reserved",
		},
		{
			name:        "reserved _tagKey",
			fieldName:   "_tagKey",
			expectError: true,
			errorMsg:    "field name '_tagKey' is reserved",
		},
		{
			name:        "reserved _tagValue",
			fieldName:   "_tagValue",
			expectError: true,
			errorMsg:    "field name '_tagValue' is reserved",
		},
		{
			name:        "reserved _seriesKey",
			fieldName:   "_seriesKey",
			expectError: true,
			errorMsg:    "field name '_seriesKey' is reserved",
		},
		{
			name:        "reserved time",
			fieldName:   "time",
			expectError: true,
			errorMsg:    "field name 'time' is reserved",
		},

		// Edge cases
		{
			name:        "single character",
			fieldName:   "a",
			expectError: false,
		},
		{
			name:        "single underscore",
			fieldName:   "_",
			expectError: false,
		},
		{
			name:        "all underscores",
			fieldName:   "___",
			expectError: false,
		},
		{
			name:        "mixed case",
			fieldName:   "Temperature",
			expectError: false,
		},
		{
			name:        "all caps",
			fieldName:   "TEMPERATURE",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFieldName(tt.fieldName)

			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateFieldName(%q) expected error but got nil", tt.fieldName)
					return
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("ValidateFieldName(%q) expected error containing '%s' but got '%s'",
						tt.fieldName, tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("ValidateFieldName(%q) expected no error but got: %v", tt.fieldName, err)
				}
			}
		})
	}
}

func TestValidateFieldName_UTF8EdgeCases(t *testing.T) {
	// Test various UTF-8 sequences
	validUTF8Tests := []string{
		"café",            // Latin with accent
		"温度",              // Chinese characters
		"температура",     // Cyrillic
		"🌡️",              // Emoji
		"αβγ",             // Greek
		"температура_123", // Mixed scripts
	}

	for _, fieldName := range validUTF8Tests {
		t.Run("valid_utf8_"+fieldName, func(t *testing.T) {
			if !utf8.ValidString(fieldName) {
				t.Errorf("Test case %q is not valid UTF-8", fieldName)
				return
			}
			err := ValidateFieldName(fieldName)
			if err != nil {
				t.Errorf("ValidateFieldName(%q) expected no error but got: %v", fieldName, err)
			}
		})
	}
}

func TestValidateFieldName_VersionSuffixAllowed(t *testing.T) {
	// Version suffix patterns (.v<N>) are deliberately allowed in user field names.
	// The mapping layer handles collisions transparently by cascading the internal
	// suffix (e.g., user "temp.v1" maps internally to "temp.v1.v2" if needed).
	allowedNames := []string{
		"temp.v1",
		"temp.v2",
		"temp.v10",
		"temp.v999",
		"temperature.v1",
		"a.v1",
		"_.v1",
		"temp.v",
		"temp.v1a",
		"temp.v1.2",
		"tempv1",
		"temp.1",
	}

	for _, name := range allowedNames {
		t.Run("allowed_"+name, func(t *testing.T) {
			err := ValidateFieldName(name)
			if err != nil {
				t.Errorf("ValidateFieldName(%q) should be allowed but got error: %v", name, err)
			}
		})
	}
}

func TestValidateFieldName_Constants(t *testing.T) {
	// Test that our constants are reasonable
	if MaxFieldNameLength <= 0 {
		t.Errorf("MaxFieldNameLength should be positive, got %d", MaxFieldNameLength)
	}

	if MaxFieldNameLength > 1000 {
		t.Errorf("MaxFieldNameLength seems too large: %d", MaxFieldNameLength)
	}

	if InternalDelimiter == "" {
		t.Errorf("InternalDelimiter should not be empty")
	}

	// Test that reserved names map is populated
	if len(reservedFieldNames) == 0 {
		t.Errorf("reservedFieldNames map should not be empty")
	}

	// Test that all reserved names are actually reserved
	for reservedName := range reservedFieldNames {
		err := ValidateFieldName(reservedName)
		if err == nil {
			t.Errorf("Reserved name '%s' should be rejected by validation", reservedName)
		}
	}
}
