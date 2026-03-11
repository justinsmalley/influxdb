package tsdb

import (
	"fmt"
	"regexp"
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

// internal version suffix regex (e.g., .v1, .v2)
var versionPattern = regexp.MustCompile(`\.v[1-9][0-9]*$`)

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

	// Restrict explicit user version suffixes
	if versionPattern.MatchString(name) {
		return fmt.Errorf("field name cannot end with versioning pattern '.v<number>'")
	}

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
