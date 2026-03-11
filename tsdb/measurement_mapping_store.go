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

// MeasurementMapping represents a mapping between user-facing measurement names and internal storage names
type MeasurementMapping struct {
	UserName     string
	InternalName string
	Version      int64
	State        MeasurementMappingState
}

// MeasurementMappingState represents the state of a field mapping
type MeasurementMappingState int32

const (
	MeasurementMappingState_MEASUREMENT_ACTIVE  MeasurementMappingState = 0
	MeasurementMappingState_MEASUREMENT_DELETED MeasurementMappingState = 1
	MeasurementMappingState_MEASUREMENT_RENAMED MeasurementMappingState = 2
)

// MeasurementMappingStore manages field mappings for an entire database
// Uses a writer-preferred RWMutex to prevent writer starvation
type MeasurementMappingStore struct {
	mu       sync.RWMutex
	mappings []*MeasurementMapping // list of measurement mappings
	path     string                // persistence file path
}

// MeasurementMappingInfo is returned by GetAllMappings for display purposes
type MeasurementMappingInfo struct {
	UserName     string
	InternalName string
	Version      int64
	State        MeasurementMappingState
}

// NewMeasurementMappingStore creates a new field mapping store
func NewMeasurementMappingStore(path string) (*MeasurementMappingStore, error) {
	store := &MeasurementMappingStore{
		mappings: make([]*MeasurementMapping, 0),
		path:     path,
	}
	return store, store.load()
}

// GetInternalMeasurementName returns the internal measurement name for a user-facing measurement name
// Returns the internal name and a boolean indicating if it is active.
func (s *MeasurementMappingStore) GetInternalMeasurementName(userName string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hasActive := false
	var internalName string
	hasDeletedOrRenamed := false

	// Iterate backwards to find the most recent mapping
	for i := len(s.mappings) - 1; i >= 0; i-- {
		m := s.mappings[i]
		if m.UserName == userName {
			if m.State == MeasurementMappingState_MEASUREMENT_ACTIVE {
				hasActive = true
				internalName = m.InternalName
				break // Found the active mapping
			} else {
				hasDeletedOrRenamed = true
				// We found a non-active mapping, but we should continue checking older ones
				// just in case there's an active one (which shouldn't happen if state transitions are correct, but good to be safe)
			}
		}
	}

	if hasActive {
		return internalName, true
	}

	if hasDeletedOrRenamed {
		return "", false // exists in history but is not currently active
	}

	return userName, true // implicit identity mapping
}

// RenameMeasurement renames a measurement across the entire database
func (s *MeasurementMappingStore) RenameMeasurement(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate both names
	if oldName == "" {
		return fmt.Errorf("invalid old measurement name")
	}
	if newName == "" {
		return fmt.Errorf("invalid new measurement name")
	}

	// Check if new name already exists and is active (or if it exists implicitly)
	// We need to verify if there is any active data for the newName, even if it doesn't have an explicit mapping.
	// For now, we only block if there's an explicit active mapping for the new name.
	// TODO: Consider checking if implicit data exists for the new name.
	for _, m := range s.mappings {
		if m.UserName == newName && m.State == MeasurementMappingState_MEASUREMENT_ACTIVE {
			return fmt.Errorf("measurement %s already exists and is active", newName)
		}
	}

	// Find the active old mapping
	var oldMapping *MeasurementMapping
	for _, m := range s.mappings {
		if m.UserName == oldName && m.State == MeasurementMappingState_MEASUREMENT_ACTIVE {
			oldMapping = m
			break
		}
	}

	if oldMapping == nil {
		for _, m := range s.mappings {
			if m.UserName == oldName {
				return fmt.Errorf("measurement %s is not active", oldName)
			}
		}

		oldMapping = &MeasurementMapping{
			UserName:     oldName,
			InternalName: oldName,
			Version:      1,
			State:        MeasurementMappingState_MEASUREMENT_ACTIVE,
		}
		s.mappings = append(s.mappings, oldMapping)
	}

	oldMapping.State = MeasurementMappingState_MEASUREMENT_RENAMED

	newMapping := &MeasurementMapping{
		UserName:     newName,
		InternalName: oldMapping.InternalName,
		Version:      oldMapping.Version,
		State:        MeasurementMappingState_MEASUREMENT_ACTIVE,
	}
	s.mappings = append(s.mappings, newMapping)

	return s.save()
}

// SoftDeleteField marks a field as deleted across the entire database
func (s *MeasurementMappingStore) SoftDeleteMeasurement(userName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userName == "" {
		return fmt.Errorf("invalid measurement name")
	}

	// Find the active mapping
	var mapping *MeasurementMapping
	for _, m := range s.mappings {
		if m.UserName == userName && m.State == MeasurementMappingState_MEASUREMENT_ACTIVE {
			mapping = m
			break
		}
	}

	if mapping == nil {
		for _, m := range s.mappings {
			if m.UserName == userName {
				return fmt.Errorf("measurement %s is not active", userName)
			}
		}

		mapping = &MeasurementMapping{
			UserName:     userName,
			InternalName: userName,
			Version:      1,
			State:        MeasurementMappingState_MEASUREMENT_ACTIVE,
		}
		s.mappings = append(s.mappings, mapping)
	}

	mapping.State = MeasurementMappingState_MEASUREMENT_DELETED
	return s.save()
}

// CreateMeasurementMapping creates a new versioned field mapping
func (s *MeasurementMappingStore) CreateMeasurementMapping(userName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userName == "" {
		return "", fmt.Errorf("invalid measurement name")
	}

	for _, m := range s.mappings {
		if m.UserName == userName && m.State == MeasurementMappingState_MEASUREMENT_ACTIVE {
			return m.InternalName, nil
		}
	}

	nextVersion := int64(2)
	for _, m := range s.mappings {
		if m.UserName == userName && m.Version >= nextVersion {
			nextVersion = m.Version + 1
		}
	}

	internalName := fmt.Sprintf("%s.v%d", userName, nextVersion)

	newMapping := &MeasurementMapping{
		UserName:     userName,
		InternalName: internalName,
		Version:      nextVersion,
		State:        MeasurementMappingState_MEASUREMENT_ACTIVE,
	}
	s.mappings = append(s.mappings, newMapping)

	if err := s.save(); err != nil {
		return "", err
	}

	return internalName, nil
}

// GetUserFieldNames returns user-facing field names for a measurement from internal field names
// This handles deduplication - only returns one field per mapping, showing user-facing names when available
func (s *MeasurementMappingStore) GetUserMeasurementNames(internalMeasurementNames []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userNames := make(map[string]bool)
	for _, internalName := range internalMeasurementNames {
		var activeUserName string
		hasAnyMapping := false

		// Iterate backwards to find the most recent active mapping for this internal name
		for i := len(s.mappings) - 1; i >= 0; i-- {
			m := s.mappings[i]
			if m.InternalName == internalName {
				hasAnyMapping = true
				if m.State == MeasurementMappingState_MEASUREMENT_ACTIVE {
					activeUserName = m.UserName
					break // Found the active mapping
				}
			}
		}

		if activeUserName != "" {
			userNames[activeUserName] = true
		} else if !hasAnyMapping {
			userNames[internalName] = true
		}
	}

	result := make([]string, 0, len(userNames))
	for name := range userNames {
		result = append(result, name)
	}

	sort.Strings(result)
	return result
}

// GetAllMappings returns all mappings for a measurement (or all if measurement is empty)
func (s *MeasurementMappingStore) GetAllMappings() []*MeasurementMappingInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*MeasurementMappingInfo

	for _, m := range s.mappings {
		result = append(result, &MeasurementMappingInfo{
			UserName:     m.UserName,
			InternalName: m.InternalName,
			Version:      m.Version,
			State:        m.State,
		})
	}

	return result
}

// save serializes the mappings to disk using protobuf
func (s *MeasurementMappingStore) save() error {
	pb := internal.MeasurementMappingSet{
		Mappings: make([]*internal.MeasurementMapping, 0, len(s.mappings)),
	}

	for _, mapping := range s.mappings {
		pb.Mappings = append(pb.Mappings, &internal.MeasurementMapping{
			UserName:     mapping.UserName,
			InternalName: mapping.InternalName,
			Version:      mapping.Version,
			State:        internal.MeasurementMappingState(mapping.State),
		})
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
func (s *MeasurementMappingStore) load() error {
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

	var pb internal.MeasurementMappingSet
	if err := proto.Unmarshal(b, &pb); err != nil {
		return err
	}

	s.mappings = make([]*MeasurementMapping, 0, len(pb.Mappings))
	for _, mapping := range pb.Mappings {
		s.mappings = append(s.mappings, &MeasurementMapping{
			UserName:     mapping.UserName,
			InternalName: mapping.InternalName,
			Version:      mapping.Version,
			State:        MeasurementMappingState(mapping.State),
		})
	}

	return nil
}
