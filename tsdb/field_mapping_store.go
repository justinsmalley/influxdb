package tsdb

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/influxdata/influxdb/pkg/file"
	internal "github.com/influxdata/influxdb/tsdb/internal"
)

// FieldMapping represents a mapping between user-facing field names and internal storage names
type FieldMapping struct {
	UserName     string
	InternalName string
	Version      int64
	State        FieldMappingState
}

// FieldMappingState represents the state of a field mapping
type FieldMappingState int32

const (
	FieldMappingState_ACTIVE  FieldMappingState = 0
	FieldMappingState_DELETED FieldMappingState = 1
	FieldMappingState_RENAMED FieldMappingState = 2
)

// FieldMappingStore manages field mappings for an entire database
// Uses a writer-preferred RWMutex to prevent writer starvation
type FieldMappingStore struct {
	mu       sync.RWMutex
	mappings map[string]map[string]*FieldMapping // measurement -> fieldName -> mapping
	path     string                              // persistence file path
}

// FieldMappingInfo is returned by GetAllMappings for display purposes
type FieldMappingInfo struct {
	Measurement  string
	UserName     string
	InternalName string
	Version      int64
	State        FieldMappingState
}

// NewFieldMappingStore creates a new field mapping store
func NewFieldMappingStore(path string) (*FieldMappingStore, error) {
	store := &FieldMappingStore{
		mappings: make(map[string]map[string]*FieldMapping),
		path:     path,
	}
	return store, store.load()
}

// GetInternalFieldName returns the internal field name for a user-facing field name
func (s *FieldMappingStore) GetInternalFieldName(measurement, userName string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	measurementMappings, exists := s.mappings[measurement]
	if !exists {
		return userName, true // implicit identity mapping
	}

	mapping, exists := measurementMappings[userName]
	if !exists {
		return userName, true // implicit identity mapping
	}

	if mapping.State == FieldMappingState_ACTIVE {
		return mapping.InternalName, true
	}

	return "", false // field exists but is not active
}

// RenameField renames a field across the entire database
func (s *FieldMappingStore) RenameField(measurement, oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate both names
	if err := ValidateFieldName(oldName); err != nil {
		return fmt.Errorf("invalid old field name: %w", err)
	}
	if err := ValidateFieldName(newName); err != nil {
		return fmt.Errorf("invalid new field name: %w", err)
	}

	// Get or create measurement mappings
	if _, exists := s.mappings[measurement]; !exists {
		s.mappings[measurement] = make(map[string]*FieldMapping)
	}
	measurementMappings := s.mappings[measurement]

	// Check if old field exists and is active
	oldMapping, exists := measurementMappings[oldName]
	if !exists {
		// Create explicit mapping for implicit field (v1)
		oldMapping = &FieldMapping{
			UserName:     oldName,
			InternalName: oldName,
			Version:      1,
			State:        FieldMappingState_ACTIVE,
		}
	} else if oldMapping.State != FieldMappingState_ACTIVE {
		return fmt.Errorf("field %s is not active (state: %v)", oldName, oldMapping.State)
	}

	// Check if new name already exists
	if existingMapping, newExists := measurementMappings[newName]; newExists {
		if existingMapping.State == FieldMappingState_ACTIVE {
			return fmt.Errorf("field %s already exists and is active", newName)
		}
		return fmt.Errorf("field %s already exists", newName)
	}

	// Mark old mapping as renamed
	oldMapping.State = FieldMappingState_RENAMED
	measurementMappings[oldName] = oldMapping

	// Create new mapping pointing to same internal name
	newMapping := &FieldMapping{
		UserName:     newName,
		InternalName: oldMapping.InternalName,
		Version:      oldMapping.Version,
		State:        FieldMappingState_ACTIVE,
	}
	measurementMappings[newName] = newMapping

	return s.save()
}

// SoftDeleteField marks a field as deleted across the entire database
func (s *FieldMappingStore) SoftDeleteField(measurement, userName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate field name
	if err := ValidateFieldName(userName); err != nil {
		return fmt.Errorf("invalid field name: %w", err)
	}

	// Get or create measurement mappings
	if _, exists := s.mappings[measurement]; !exists {
		s.mappings[measurement] = make(map[string]*FieldMapping)
	}
	measurementMappings := s.mappings[measurement]

	// Check if field exists and is active
	mapping, exists := measurementMappings[userName]
	if !exists {
		// Create explicit mapping for implicit field (v1)
		mapping = &FieldMapping{
			UserName:     userName,
			InternalName: userName,
			Version:      1,
			State:        FieldMappingState_ACTIVE,
		}
	} else if mapping.State != FieldMappingState_ACTIVE {
		return fmt.Errorf("field %s is not active (state: %v)", userName, mapping.State)
	}

	// Mark as deleted
	mapping.State = FieldMappingState_DELETED
	measurementMappings[userName] = mapping

	return s.save()
}

// CreateFieldMapping creates a new versioned field mapping
func (s *FieldMappingStore) CreateFieldMapping(measurement, userName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate field name
	if err := ValidateFieldName(userName); err != nil {
		return "", fmt.Errorf("invalid field name: %w", err)
	}

	// Get or create measurement mappings
	if _, exists := s.mappings[measurement]; !exists {
		s.mappings[measurement] = make(map[string]*FieldMapping)
	}
	measurementMappings := s.mappings[measurement]

	// Check if there's already an active field with this user name
	if existingMapping, exists := measurementMappings[userName]; exists && existingMapping.State == FieldMappingState_ACTIVE {
		return existingMapping.InternalName, nil // Return existing mapping
	}

	// Find next version number
	nextVersion := int64(2) // Start from v2 since v1 is implicit
	for _, mapping := range measurementMappings {
		if mapping.UserName == userName && mapping.Version >= nextVersion {
			nextVersion = mapping.Version + 1
		}
	}

	internalName := fmt.Sprintf("%s.v%d", userName, nextVersion)

	// Create new mapping
	newMapping := &FieldMapping{
		UserName:     userName,
		InternalName: internalName,
		Version:      nextVersion,
		State:        FieldMappingState_ACTIVE,
	}
	measurementMappings[userName] = newMapping

	if err := s.save(); err != nil {
		return "", err
	}

	return internalName, nil
}

// GetUserFieldNames returns user-facing field names for a measurement from internal field names
// This handles deduplication - only returns one field per mapping, showing user-facing names when available
func (s *FieldMappingStore) GetUserFieldNames(measurement string, internalFieldNames []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	measurementMappings, exists := s.mappings[measurement]
	if !exists {
		// No mappings - return internal names as-is
		return internalFieldNames
	}

	// Build maps for translation
	internalToUser := make(map[string]string)
	for _, m := range measurementMappings {
		if m.State == FieldMappingState_ACTIVE {
			internalToUser[m.InternalName] = m.UserName
		}
	}

	// Translate and deduplicate
	userNames := make(map[string]bool)
	for _, internalName := range internalFieldNames {
		if userFacing, exists := internalToUser[internalName]; exists {
			// This internal name has a user-facing mapping
			userNames[userFacing] = true
		} else {
			// No mapping - use internal name
			userNames[internalName] = true
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(userNames))
	for name := range userNames {
		result = append(result, name)
	}

	// Sort to ensure deterministic iteration order
	sort.Strings(result)

	return result
}

// GetAllMappings returns all mappings for a measurement (or all if measurement is empty)
func (s *FieldMappingStore) GetAllMappings(measurement string) []*FieldMappingInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*FieldMappingInfo

	if measurement != "" {
		// Return mappings for specific measurement
		if measurementMappings, exists := s.mappings[measurement]; exists {
			// Sort user names to ensure deterministic iteration order
			userNames := make([]string, 0, len(measurementMappings))
			for userName := range measurementMappings {
				userNames = append(userNames, userName)
			}
			sort.Strings(userNames)

			for _, userName := range userNames {
				mapping := measurementMappings[userName]
				result = append(result, &FieldMappingInfo{
					Measurement:  measurement,
					UserName:     mapping.UserName,
					InternalName: mapping.InternalName,
					Version:      mapping.Version,
					State:        mapping.State,
				})
			}
		}
	} else {
		// Return all mappings
		// Sort measurement names to ensure deterministic iteration order
		measurementNames := make([]string, 0, len(s.mappings))
		for meas := range s.mappings {
			measurementNames = append(measurementNames, meas)
		}
		sort.Strings(measurementNames)

		for _, meas := range measurementNames {
			measurementMappings := s.mappings[meas]
			// Sort user names to ensure deterministic iteration order
			userNames := make([]string, 0, len(measurementMappings))
			for userName := range measurementMappings {
				userNames = append(userNames, userName)
			}
			sort.Strings(userNames)

			for _, userName := range userNames {
				mapping := measurementMappings[userName]
				result = append(result, &FieldMappingInfo{
					Measurement:  meas,
					UserName:     mapping.UserName,
					InternalName: mapping.InternalName,
					Version:      mapping.Version,
					State:        mapping.State,
				})
			}
		}
	}

	return result
}

// save serializes the mappings to disk using protobuf
func (s *FieldMappingStore) save() error {
	// Create protobuf message
	pb := internal.FieldMappingSet{
		Measurements: make([]*internal.FieldMappingMeasurement, 0, len(s.mappings)),
	}

	for measurement, measurementMappings := range s.mappings {
		meas := &internal.FieldMappingMeasurement{
			Name:     []byte(measurement),
			Mappings: make([]*internal.FieldMapping, 0, len(measurementMappings)),
		}

		for _, mapping := range measurementMappings {
			meas.Mappings = append(meas.Mappings, &internal.FieldMapping{
				UserName:     mapping.UserName,
				InternalName: mapping.InternalName,
				Version:      mapping.Version,
				State:        internal.FieldMappingState(mapping.State),
			})
		}

		pb.Measurements = append(pb.Measurements, meas)
	}

	// Marshal to protobuf
	b, err := proto.Marshal(&pb)
	if err != nil {
		return err
	}

	// Write to temp file then rename atomically
	tempPath := s.path + ".tmp"
	f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempPath)

	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}

	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	return file.RenameFile(tempPath, s.path)
}

// load deserializes mappings from disk
func (s *FieldMappingStore) load() error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(s.path), 0777); err != nil {
		return err
	}

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing mappings - that's OK
			return nil
		}
		return err
	}
	defer f.Close()

	// Read file
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}

	if len(b) == 0 {
		// Empty file - that's OK
		return nil
	}

	// Unmarshal protobuf
	var pb internal.FieldMappingSet
	if err := proto.Unmarshal(b, &pb); err != nil {
		return err
	}

	// Convert to internal structure
	s.mappings = make(map[string]map[string]*FieldMapping)
	for _, meas := range pb.Measurements {
		measurement := string(meas.Name)
		measurementMappings := make(map[string]*FieldMapping)

		for _, mapping := range meas.Mappings {
			measurementMappings[mapping.UserName] = &FieldMapping{
				UserName:     mapping.UserName,
				InternalName: mapping.InternalName,
				Version:      mapping.Version,
				State:        FieldMappingState(mapping.State),
			}
		}

		s.mappings[measurement] = measurementMappings
	}

	return nil
}
