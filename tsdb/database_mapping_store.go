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

// DatabaseMapping represents a mapping between user-facing database names and internal storage names
type DatabaseMapping struct {
	UserName     string
	InternalName string
	Version      int64
	State        DatabaseMappingState
}

// DatabaseMappingState represents the state of a field mapping
type DatabaseMappingState int32

const (
	DatabaseMappingState_DATABASE_ACTIVE  DatabaseMappingState = 0
	DatabaseMappingState_DATABASE_DELETED DatabaseMappingState = 1
	DatabaseMappingState_DATABASE_RENAMED DatabaseMappingState = 2
)

// DatabaseMappingStore manages field mappings for an entire database
// Uses a writer-preferred RWMutex to prevent writer starvation
type DatabaseMappingStore struct {
	mu       sync.RWMutex
	mappings []*DatabaseMapping // list of database mappings
	path     string                // persistence file path
}

// DatabaseMappingInfo is returned by GetAllMappings for display purposes
type DatabaseMappingInfo struct {
	UserName     string
	InternalName string
	Version      int64
	State        DatabaseMappingState
}

// NewDatabaseMappingStore creates a new field mapping store
func NewDatabaseMappingStore(path string) (*DatabaseMappingStore, error) {
	store := &DatabaseMappingStore{
		mappings: make([]*DatabaseMapping, 0),
		path:     path,
	}
	return store, store.load()
}

// GetInternalDatabaseName returns the internal database name for a user-facing database name
func (s *DatabaseMappingStore) GetInternalDatabaseName(userName string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hasActive := false
	var internalName string
	hasDeletedOrRenamed := false

	// Iterate backwards to find the most recent mapping
	for i := len(s.mappings) - 1; i >= 0; i-- {
		m := s.mappings[i]
		if m.UserName == userName {
			if m.State == DatabaseMappingState_DATABASE_ACTIVE {
				hasActive = true
				internalName = m.InternalName
				break // Found the active mapping
			} else {
				hasDeletedOrRenamed = true
				// We found a non-active mapping, but we should continue checking older ones
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

// RenameDatabase renames a database globally
func (s *DatabaseMappingStore) RenameDatabase(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate both names
	if oldName == "" {
		return fmt.Errorf("invalid old database name")
	}
	if newName == "" {
		return fmt.Errorf("invalid new database name")
	}

	// Check if new name already exists and is active
	for _, m := range s.mappings {
		if m.UserName == newName && m.State == DatabaseMappingState_DATABASE_ACTIVE {
			return fmt.Errorf("database %s already exists and is active", newName)
		}
	}

	// Find the active old mapping
	var oldMapping *DatabaseMapping
	for _, m := range s.mappings {
		if m.UserName == oldName && m.State == DatabaseMappingState_DATABASE_ACTIVE {
			oldMapping = m
			break
		}
	}

	if oldMapping == nil {
		for _, m := range s.mappings {
			if m.UserName == oldName {
				return fmt.Errorf("database %s is not active", oldName)
			}
		}

		oldMapping = &DatabaseMapping{
			UserName:     oldName,
			InternalName: oldName,
			Version:      1,
			State:        DatabaseMappingState_DATABASE_ACTIVE,
		}
		s.mappings = append(s.mappings, oldMapping)
	}

	oldMapping.State = DatabaseMappingState_DATABASE_RENAMED

	newMapping := &DatabaseMapping{
		UserName:     newName,
		InternalName: oldMapping.InternalName,
		Version:      oldMapping.Version,
		State:        DatabaseMappingState_DATABASE_ACTIVE,
	}
	s.mappings = append(s.mappings, newMapping)

	return s.save()
}

// SoftDeleteField marks a field as deleted across the entire database
func (s *DatabaseMappingStore) SoftDeleteDatabase(userName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userName == "" {
		return fmt.Errorf("invalid database name")
	}

	// Find the active mapping
	var mapping *DatabaseMapping
	for _, m := range s.mappings {
		if m.UserName == userName && m.State == DatabaseMappingState_DATABASE_ACTIVE {
			mapping = m
			break
		}
	}

	if mapping == nil {
		for _, m := range s.mappings {
			if m.UserName == userName {
				return fmt.Errorf("database %s is not active", userName)
			}
		}

		mapping = &DatabaseMapping{
			UserName:     userName,
			InternalName: userName,
			Version:      1,
			State:        DatabaseMappingState_DATABASE_ACTIVE,
		}
		s.mappings = append(s.mappings, mapping)
	}

	mapping.State = DatabaseMappingState_DATABASE_DELETED
	return s.save()
}

// CreateDatabaseMapping creates a new versioned field mapping
func (s *DatabaseMappingStore) CreateDatabaseMapping(userName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userName == "" {
		return "", fmt.Errorf("invalid database name")
	}

	for _, m := range s.mappings {
		if m.UserName == userName && m.State == DatabaseMappingState_DATABASE_ACTIVE {
			return m.InternalName, nil
		}
	}

	nextVersion := int64(1)
	for _, m := range s.mappings {
		if m.UserName == userName && m.Version >= nextVersion {
			nextVersion = m.Version + 1
		}
	}

	internalName := userName
	if nextVersion > 1 {
		internalName = fmt.Sprintf("%s.v%d", userName, nextVersion)
	}

	newMapping := &DatabaseMapping{
		UserName:     userName,
		InternalName: internalName,
		Version:      nextVersion,
		State:        DatabaseMappingState_DATABASE_ACTIVE,
	}
	s.mappings = append(s.mappings, newMapping)

	if err := s.save(); err != nil {
		return "", err
	}

	return internalName, nil
}

// GetUserFieldNames returns user-facing field names for a database from internal field names
// This handles deduplication - only returns one field per mapping, showing user-facing names when available
func (s *DatabaseMappingStore) GetUserDatabaseNames(internalDatabaseNames []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userNames := make(map[string]bool)
	for _, internalName := range internalDatabaseNames {
		var activeUserName string
		hasAnyMapping := false

		for _, m := range s.mappings {
			if m.InternalName == internalName {
				hasAnyMapping = true
				if m.State == DatabaseMappingState_DATABASE_ACTIVE {
					activeUserName = m.UserName
					break
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

// GetAllMappings returns all mappings for a database (or all if database is empty)
func (s *DatabaseMappingStore) GetAllMappings() []*DatabaseMappingInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*DatabaseMappingInfo

	for _, m := range s.mappings {
		result = append(result, &DatabaseMappingInfo{
			UserName:     m.UserName,
			InternalName: m.InternalName,
			Version:      m.Version,
			State:        m.State,
		})
	}

	return result
}

// save serializes the mappings to disk using protobuf
func (s *DatabaseMappingStore) save() error {
	pb := internal.DatabaseMappingSet{
		Mappings: make([]*internal.DatabaseMapping, 0, len(s.mappings)),
	}

	for _, mapping := range s.mappings {
		pb.Mappings = append(pb.Mappings, &internal.DatabaseMapping{
			UserName:     mapping.UserName,
			InternalName: mapping.InternalName,
			Version:      mapping.Version,
			State:        internal.DatabaseMappingState(mapping.State),
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
func (s *DatabaseMappingStore) load() error {
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

	var pb internal.DatabaseMappingSet
	if err := proto.Unmarshal(b, &pb); err != nil {
		return err
	}

	s.mappings = make([]*DatabaseMapping, 0, len(pb.Mappings))
	for _, mapping := range pb.Mappings {
		s.mappings = append(s.mappings, &DatabaseMapping{
			UserName:     mapping.UserName,
			InternalName: mapping.InternalName,
			Version:      mapping.Version,
			State:        DatabaseMappingState(mapping.State),
		})
	}

	return nil
}
