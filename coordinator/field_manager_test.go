package coordinator

import (
	"testing"
)

func TestFieldManager_ParseFieldCommand(t *testing.T) {
	// Create a mock statement executor for testing
	executor := &StatementExecutor{}
	fm := NewFieldManager(executor)
	
	tests := []struct {
		name        string
		database    string
		command     string
		expectError bool
	}{
		{
			name:        "valid drop field command",
			database:    "testdb",
			command:     "DROP FIELD temperature FROM cpu",
			expectError: false,
		},
		{
			name:        "valid rename field command",
			database:    "testdb",
			command:     "ALTER MEASUREMENT cpu RENAME FIELD temp TO temperature",
			expectError: false,
		},
		{
			name:        "drop field with database in command",
			database:    "testdb",
			command:     "DROP FIELD temperature FROM cpu ON mydb",
			expectError: false,
		},
		{
			name:        "rename field with database in command",
			database:    "testdb",
			command:     "ALTER MEASUREMENT cpu ON mydb RENAME FIELD temp TO temperature",
			expectError: false,
		},
		{
			name:        "invalid drop field syntax",
			database:    "testdb",
			command:     "DROP FIELD temperature cpu",
			expectError: true,
		},
		{
			name:        "invalid rename field syntax",
			database:    "testdb",
			command:     "ALTER MEASUREMENT cpu RENAME FIELD temp temperature",
			expectError: true,
		},
		{
			name:        "unknown command",
			database:    "testdb",
			command:     "CREATE FIELD temperature IN cpu",
			expectError: true,
		},
		{
			name:        "empty database",
			database:    "",
			command:     "DROP FIELD temperature FROM cpu",
			expectError: true,
		},
		{
			name:        "empty command",
			database:    "testdb",
			command:     "",
			expectError: true,
		},
		{
			name:        "whitespace only command",
			database:    "testdb",
			command:     "   ",
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fm.ParseFieldCommand(tt.database, tt.command)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestFieldManager_DropField(t *testing.T) {
	executor := &StatementExecutor{}
	fm := NewFieldManager(executor)
	
	tests := []struct {
		name        string
		database    string
		measurement string
		fieldName   string
		expectError bool
	}{
		{
			name:        "valid parameters",
			database:    "testdb",
			measurement: "cpu",
			fieldName:   "temperature",
			expectError: false,
		},
		{
			name:        "empty database",
			database:    "",
			measurement: "cpu",
			fieldName:   "temperature",
			expectError: true,
		},
		{
			name:        "empty measurement",
			database:    "testdb",
			measurement: "",
			fieldName:   "temperature",
			expectError: true,
		},
		{
			name:        "empty field name",
			database:    "testdb",
			measurement: "cpu",
			fieldName:   "",
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fm.DropField(tt.database, tt.measurement, tt.fieldName)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestFieldManager_RenameField(t *testing.T) {
	executor := &StatementExecutor{}
	fm := NewFieldManager(executor)
	
	tests := []struct {
		name        string
		database    string
		measurement string
		oldName     string
		newName     string
		expectError bool
	}{
		{
			name:        "valid parameters",
			database:    "testdb",
			measurement: "cpu",
			oldName:     "temp",
			newName:     "temperature",
			expectError: false,
		},
		{
			name:        "empty database",
			database:    "",
			measurement: "cpu",
			oldName:     "temp",
			newName:     "temperature",
			expectError: true,
		},
		{
			name:        "empty measurement",
			database:    "testdb",
			measurement: "",
			oldName:     "temp",
			newName:     "temperature",
			expectError: true,
		},
		{
			name:        "empty old name",
			database:    "testdb",
			measurement: "cpu",
			oldName:     "",
			newName:     "temperature",
			expectError: true,
		},
		{
			name:        "empty new name",
			database:    "testdb",
			measurement: "cpu",
			oldName:     "temp",
			newName:     "",
			expectError: true,
		},
		{
			name:        "same old and new name",
			database:    "testdb",
			measurement: "cpu",
			oldName:     "temp",
			newName:     "temp",
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fm.RenameField(tt.database, tt.measurement, tt.oldName, tt.newName)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
