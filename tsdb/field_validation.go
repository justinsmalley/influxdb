package tsdb

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxFieldNameLength = 255
	InternalDelimiter  = "#!~#"
)

var (
	// Reserved field names
	reservedFieldNames = map[string]bool{
		"_name":      true,
		"_tagKey":    true,
		"_tagValue":  true,
		"_seriesKey": true,
		"time":       true, // Reserved by InfluxDB
	}
)

// ValidateFieldName validates a user-provided field name
func ValidateFieldName(name string) error {
	// Check empty
	if name == "" {
		return fmt.Errorf("field name cannot be empty")
	}

	// Check length
	if len(name) > MaxFieldNameLength {
		return fmt.Errorf("field name exceeds maximum length of %d bytes", MaxFieldNameLength)
	}

	// Check valid UTF-8
	if !utf8.ValidString(name) {
		return fmt.Errorf("field name must be valid UTF-8")
	}

	// Check for null bytes
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("field name cannot contain null bytes")
	}

	// Check for internal delimiter
	if strings.Contains(name, InternalDelimiter) {
		return fmt.Errorf("field name cannot contain reserved pattern '%s'", InternalDelimiter)
	}

	// NOTE: Names ending in .v<N> (e.g., "temp.v1") are deliberately allowed.
	// The mapping layer uses .v<N> suffixes internally for collision avoidance,
	// but this is transparent to the user. If a user creates a field named
	// "temp.v1", the system handles it by cascading the internal suffix
	// (e.g., internal name becomes "temp.v1.v2" if needed).

	// Check for reserved names
	if reservedFieldNames[name] {
		return fmt.Errorf("field name '%s' is reserved", name)
	}

	// Check for leading/trailing whitespace
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("field name cannot have leading or trailing whitespace")
	}

	return nil
}
