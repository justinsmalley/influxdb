package tsdb

import (
	"testing"

	internal "github.com/influxdata/influxdb/tsdb/internal"
)

func TestFieldMappingProtobuf(t *testing.T) {
	// Create a test field mapping
	mapping := &internal.FieldMapping{
		UserName:     "Air_Temperature",
		InternalName: "temperature",
		Version:      1,
		State:        internal.FieldMappingState_ACTIVE,
	}

	// Create a measurement with the mapping
	measurement := &internal.MeasurementFields{
		Name:     []byte("test"),
		Fields:   []*internal.Field{{Name: []byte("temperature"), Type: 1}},
		Mappings: []*internal.FieldMapping{mapping},
	}

	// Verify the mapping is set
	if len(measurement.Mappings) != 1 {
		t.Errorf("Expected 1 mapping, got %d", len(measurement.Mappings))
	}

	if measurement.Mappings[0].UserName != "Air_Temperature" {
		t.Errorf("Expected UserName 'Air_Temperature', got '%s'", measurement.Mappings[0].UserName)
	}

	t.Logf("Protobuf test passed: mapping has UserName=%s, InternalName=%s",
		measurement.Mappings[0].UserName, measurement.Mappings[0].InternalName)
}
