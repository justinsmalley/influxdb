package tsdb

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	internal "github.com/influxdata/influxdb/tsdb/internal"
)

func TestProtobufSerialization(t *testing.T) {
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

	// DEBUG: Check if the Mappings field is actually set
	if measurement.Mappings == nil {
		t.Fatal("Mappings field is nil")
	}
	if len(measurement.Mappings) == 0 {
		t.Fatal("Mappings field is empty")
	}
	t.Logf("Mappings field has %d elements", len(measurement.Mappings))

	// Create the field set
	fieldSet := &internal.MeasurementFieldSet{
		Measurements: []*internal.MeasurementFields{measurement},
	}

	// DEBUG: Check what's in the field set before marshaling
	t.Logf("Before marshaling: %d measurements", len(fieldSet.Measurements))
	for i, meas := range fieldSet.Measurements {
		t.Logf("  Measurement %d: %d fields, %d mappings", i, len(meas.Fields), len(meas.Mappings))
		for j, mapping := range meas.Mappings {
			t.Logf("    Mapping %d: %s -> %s (v%d, state %d)", j, mapping.UserName, mapping.InternalName, mapping.Version, mapping.State)
		}
	}

	// Marshal the protobuf
	data, err := proto.Marshal(fieldSet)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Marshaled %d bytes", len(data))

	// Unmarshal the protobuf
	var fieldSet2 internal.MeasurementFieldSet
	if err := proto.Unmarshal(data, &fieldSet2); err != nil {
		t.Fatal(err)
	}

	// Verify the mapping was preserved
	if len(fieldSet2.Measurements) != 1 {
		t.Fatalf("Expected 1 measurement, got %d", len(fieldSet2.Measurements))
	}

	meas := fieldSet2.Measurements[0]
	if len(meas.Mappings) != 1 {
		t.Fatalf("Expected 1 mapping, got %d", len(meas.Mappings))
	}

	m := meas.Mappings[0]
	if m.UserName != "Air_Temperature" {
		t.Fatalf("Expected UserName 'Air_Temperature', got '%s'", m.UserName)
	}

	if m.InternalName != "temperature" {
		t.Fatalf("Expected InternalName 'temperature', got '%s'", m.InternalName)
	}

	if m.Version != 1 {
		t.Fatalf("Expected Version 1, got %d", m.Version)
	}

	if m.State != internal.FieldMappingState_ACTIVE {
		t.Fatalf("Expected State ACTIVE, got %d", m.State)
	}

	t.Log("Protobuf serialization test passed!")
}
