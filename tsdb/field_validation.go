package tsdb

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// InternalDelimiter is the separator used in internal series keys to join
	// measurement name, tag set, and field name. Names containing this sequence
	// would corrupt the key format and are therefore rejected.
	InternalDelimiter = "#!~#"
)

var (
	// Reserved field names — upstream InfluxDB reserves these for internal use.
	reservedFieldNames = map[string]bool{
		"_name":      true,
		"_tagKey":    true,
		"_tagValue":  true,
		"_seriesKey": true,
		"time":       true,
	}
)

// validateName is the shared core for ValidateFieldName and ValidateMeasurementName.
// It rejects names that are empty, not valid UTF-8, contain null bytes, or contain
// the internal key delimiter. No length limit is enforced: upstream InfluxDB 1.7.11
// does not impose one (only the combined measurement+tags key has a 65535-byte cap
// via models.MaxKeyLength in models/points.go).
func validateName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s name cannot be empty", kind)
	}
	if !utf8.ValidString(name) {
		return fmt.Errorf("%s name must be valid UTF-8", kind)
	}
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("%s name cannot contain null bytes", kind)
	}
	if strings.Contains(name, InternalDelimiter) {
		return fmt.Errorf("%s name cannot contain reserved pattern '%s'", kind, InternalDelimiter)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("%s name cannot have leading or trailing whitespace", kind)
	}
	return nil
}

// ValidateFieldName validates a user-provided field name.
// NOTE: Names ending in .v<N> (e.g., "temp.v1") are deliberately allowed.
// The mapping layer uses .v<N> suffixes internally for collision avoidance,
// but this is transparent to the user — if a user creates "temp.v1", the
// system cascades the suffix (e.g., internal name becomes "temp.v1.v2").
func ValidateFieldName(name string) error {
	if err := validateName("field", name); err != nil {
		return err
	}
	if reservedFieldNames[name] {
		return fmt.Errorf("field name '%s' is reserved", name)
	}
	return nil
}

// ValidateMeasurementName validates a user-provided measurement name.
// The same structural rules as field names apply (non-empty, valid UTF-8,
// no null bytes, no internal delimiter, no leading/trailing whitespace).
func ValidateMeasurementName(name string) error {
	return validateName("measurement", name)
}
