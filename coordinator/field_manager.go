package coordinator

import (
	"fmt"
	"strings"

	"github.com/influxdata/influxql"
)

// FieldManager provides methods to manage field operations
type FieldManager struct {
	executor *StatementExecutor
}

// NewFieldManager creates a new FieldManager
func NewFieldManager(executor *StatementExecutor) *FieldManager {
	return &FieldManager{
		executor: executor,
	}
}

// DropField soft deletes a field from a measurement
// Usage: DropField("database", "measurement", "field_name")
func (fm *FieldManager) DropField(database, measurement, fieldName string) error {
	if database == "" {
		return fmt.Errorf("database name is required")
	}
	if measurement == "" {
		return fmt.Errorf("measurement name is required")
	}
	if fieldName == "" {
		return fmt.Errorf("field name is required")
	}
	
	return fm.executor.DropField(database, measurement, fieldName)
}

// RenameField renames a field in a measurement
// Usage: RenameField("database", "measurement", "old_name", "new_name")
func (fm *FieldManager) RenameField(database, measurement, oldName, newName string) error {
	if database == "" {
		return fmt.Errorf("database name is required")
	}
	if measurement == "" {
		return fmt.Errorf("measurement name is required")
	}
	if oldName == "" {
		return fmt.Errorf("old field name is required")
	}
	if newName == "" {
		return fmt.Errorf("new field name is required")
	}
	if oldName == newName {
		return fmt.Errorf("old and new field names cannot be the same")
	}
	
	return fm.executor.RenameField(database, measurement, oldName, newName)
}

// ParseFieldCommand parses a field command string using the InfluxQL parser and executes it
// Supported commands:
//   "DROP FIELD field_name FROM measurement" 
//   "ALTER MEASUREMENT measurement RENAME FIELD old_name TO new_name"
func (fm *FieldManager) ParseFieldCommand(database, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}
	
	// Parse the command using InfluxQL parser
	stmt, err := influxql.ParseStatement(command)
	if err != nil {
		return fmt.Errorf("failed to parse command: %v", err)
	}
	
	// Execute based on statement type
	switch s := stmt.(type) {
	case *influxql.DropFieldStatement:
		// Use provided database if statement doesn't specify one
		db := s.Database
		if db == "" {
			db = database
		}
		if db == "" {
			return fmt.Errorf("database name is required")
		}
		return fm.DropField(db, s.Measurement, s.Name)
		
	case *influxql.RenameFieldStatement:
		// Use provided database if statement doesn't specify one
		db := s.Database
		if db == "" {
			db = database
		}
		if db == "" {
			return fmt.Errorf("database name is required")
		}
		return fm.RenameField(db, s.Measurement, s.OldName, s.NewName)
		
	default:
		return fmt.Errorf("unsupported statement type: %T. Only DROP FIELD and ALTER MEASUREMENT RENAME FIELD are supported", stmt)
	}
}
