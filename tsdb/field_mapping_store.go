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
	mappings map[string][]*FieldMapping // measurement -> list of mappings
	path     string                     // persistence file path
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
		mappings: make(map[string][]*FieldMapping),
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

	hasAny := false
	for _, m := range measurementMappings {
		if m.UserName == userName {
			hasAny = true
			if m.State == FieldMappingState_ACTIVE {
				return m.InternalName, true
			}
		}
	}

	if hasAny {
		return "", false // field exists but is not active (deleted or renamed)
	}

	return userName, true // implicit identity mapping
}

// RenameField renames a field across the entire database
func (s *FieldMappingStore) RenameField(measurement, oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ValidateFieldName(oldName); err != nil {
		return fmt.Errorf("invalid old field name: %w", err)
	}
	if err := ValidateFieldName(newName); err != nil {
		return fmt.Errorf("invalid new field name: %w", err)
	}

	if _, exists := s.mappings[measurement]; !exists {
		s.mappings[measurement] = make([]*FieldMapping, 0)
	}
	measMappings := s.mappings[measurement]

	// Check if new name already exists and is active
	for _, m := range measMappings {
		if m.UserName == newName && m.State == FieldMappingState_ACTIVE {
			return fmt.Errorf("field %s already exists and is active", newName)
		}
	}

	// Find the active old mapping
	var oldMapping *FieldMapping
	for _, m := range measMappings {
		if m.UserName == oldName && m.State == FieldMappingState_ACTIVE {
			oldMapping = m
			break
		}
	}

	if oldMapping == nil {
		// Check if it was explicitly deleted
		for _, m := range measMappings {
			if m.UserName == oldName {
				return fmt.Errorf("field %s is not active", oldName)
			}
		}

		// Create explicit mapping for implicit field (v1)
		oldMapping = &FieldMapping{
			UserName:     oldName,
			InternalName: oldName,
			Version:      1,
			State:        FieldMappingState_ACTIVE,
		}
		s.mappings[measurement] = append(s.mappings[measurement], oldMapping)
	}

	// Mark old mapping as renamed
	oldMapping.State = FieldMappingState_RENAMED

	// Create new mapping pointing to same internal name
	newMapping := &FieldMapping{
		UserName:     newName,
		InternalName: oldMapping.InternalName,
		Version:      oldMapping.Version,
		State:        FieldMappingState_ACTIVE,
	}
	s.mappings[measurement] = append(s.mappings[measurement], newMapping)

	return s.save()
}

// SoftDeleteField marks a field as deleted across the entire database
func (s *FieldMappingStore) SoftDeleteField(measurement, userName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ValidateFieldName(userName); err != nil {
		return fmt.Errorf("invalid field name: %w", err)
	}

	if _, exists := s.mappings[measurement]; !exists {
		s.mappings[measurement] = make([]*FieldMapping, 0)
	}
	measMappings := s.mappings[measurement]

	// Find the active mapping
	var mapping *FieldMapping
	for _, m := range measMappings {
		if m.UserName == userName && m.State == FieldMappingState_ACTIVE {
			mapping = m
			break
		}
	}

	if mapping == nil {
		// Check if it was already deleted
		for _, m := range measMappings {
			if m.UserName == userName {
				return fmt.Errorf("field %s is not active", userName)
			}
		}

		// Create explicit mapping for implicit field (v1)
		mapping = &FieldMapping{
			UserName:     userName,
			InternalName: userName,
			Version:      1,
			State:        FieldMappingState_ACTIVE,
		}
		s.mappings[measurement] = append(s.mappings[measurement], mapping)
	}

	// Mark as deleted
	mapping.State = FieldMappingState_DELETED

	return s.save()
}

// CreateFieldMapping creates a new versioned field mapping
func (s *FieldMappingStore) CreateFieldMapping(measurement, userName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ValidateFieldName(userName); err != nil {
		return "", fmt.Errorf("invalid field name: %w", err)
	}

	if _, exists := s.mappings[measurement]; !exists {
		s.mappings[measurement] = make([]*FieldMapping, 0)
	}
	measMappings := s.mappings[measurement]

	// Check if there's already an active field with this user name
	for _, m := range measMappings {
		if m.UserName == userName && m.State == FieldMappingState_ACTIVE {
			return m.InternalName, nil // Return existing
		}
	}

	// Find next version number across all mappings for this user name
	nextVersion := int64(2) // Start from v2 since v1 is implicit
	for _, m := range measMappings {
		if m.UserName == userName && m.Version >= nextVersion {
			nextVersion = m.Version + 1
		}
	}

	internalName := fmt.Sprintf("%s.v%d", userName, nextVersion)

	newMapping := &FieldMapping{
		UserName:     userName,
		InternalName: internalName,
		Version:      nextVersion,
		State:        FieldMappingState_ACTIVE,
	}
	s.mappings[measurement] = append(s.mappings[measurement], newMapping)

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

	// Translate and deduplicate
	userNames := make(map[string]bool)
	for _, internalName := range internalFieldNames {
		// Find the active user name for this internal name
		var activeUserName string
		hasAnyMapping := false

		for _, m := range measurementMappings {
			if m.InternalName == internalName {
				hasAnyMapping = true
				if m.State == FieldMappingState_ACTIVE {
					activeUserName = m.UserName
					break
				}
			}
		}

		if activeUserName != "" {
			userNames[activeUserName] = true
		} else if !hasAnyMapping {
			// No mapping at all for this internal name (implicit field)
			userNames[internalName] = true
		}
		// If hasAnyMapping but no activeUserName, it's deleted/renamed, so we skip it entirely
	}

	// Convert map to slice
	result := make([]string, 0, len(userNames))
	for name := range userNames {
		result = append(result, name)
	}

	sort.Strings(result)
	return result
}

// GetAllMappings returns all mappings for a measurement (or all if measurement is empty)
func (s *FieldMappingStore) GetAllMappings(measurement string) []*FieldMappingInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*FieldMappingInfo

	if measurement != "" {
		if measMappings, exists := s.mappings[measurement]; exists {
			for _, m := range measMappings {
				result = append(result, &FieldMappingInfo{
					Measurement:  measurement,
					UserName:     m.UserName,
					InternalName: m.InternalName,
					Version:      m.Version,
					State:        m.State,
				})
			}
		}
	} else {
		// Return all mappings, sort measurements first
		measurementNames := make([]string, 0, len(s.mappings))
		for meas := range s.mappings {
			measurementNames = append(measurementNames, meas)
		}
		sort.Strings(measurementNames)

		for _, meas := range measurementNames {
			for _, m := range s.mappings[meas] {
				result = append(result, &FieldMappingInfo{
					Measurement:  meas,
					UserName:     m.UserName,
					InternalName: m.InternalName,
					Version:      m.Version,
					State:        m.State,
				})
			}
		}
	}

	return result
}

// save serializes the mappings to disk using protobuf
func (s *FieldMappingStore) save() error {
	pb := internal.FieldMappingSet{
		Measurements: make([]*internal.FieldMappingMeasurement, 0, len(s.mappings)),
	}

	for measurement, measMappings := range s.mappings {
		meas := &internal.FieldMappingMeasurement{
			Name:     []byte(measurement),
			Mappings: make([]*internal.FieldMapping, 0, len(measMappings)),
		}

		for _, mapping := range measMappings {
			meas.Mappings = append(meas.Mappings, &internal.FieldMapping{
				UserName:     mapping.UserName,
				InternalName: mapping.InternalName,
				Version:      mapping.Version,
				State:        internal.FieldMappingState(mapping.State),
			})
		}
		pb.Measurements = append(pb.Measurements, meas)
	}

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
	if err := os.MkdirAll(filepath.Dir(s.path), 0777); err != nil {
		return err
	}

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}

	if len(b) == 0 {
		return nil
	}

	var pb internal.FieldMappingSet
	if err := proto.Unmarshal(b, &pb); err != nil {
		return err
	}

	s.mappings = make(map[string][]*FieldMapping)
	for _, meas := range pb.Measurements {
		measurement := string(meas.Name)
		measMappings := make([]*FieldMapping, 0, len(meas.Mappings))

		for _, mapping := range meas.Mappings {
			measMappings = append(measMappings, &FieldMapping{
				UserName:     mapping.UserName,
				InternalName: mapping.InternalName,
				Version:      mapping.Version,
				State:        FieldMappingState(mapping.State),
			})
		}
		s.mappings[measurement] = measMappings
	}

	return nil
}
