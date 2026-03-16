package tsdb

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// MappingEntry represents the current mapping between a user-facing name and an internal name.
// No history: one entry per active alias. "Deleted" is represented by ByInternal[internalName] == ""
// and absence from ByUser. Version exists only as a suffix in InternalName (e.g. "name.v2") when
// needed for collision avoidance on recreate; no separate version field.
type MappingEntry struct {
	UserName     string
	InternalName string
}

// GroupMappings encapsulates the active mapping state for a single group (e.g. a measurement).
// It maintains O(1) maps for fast lookups in both directions.
type GroupMappings struct {
	ByUser     map[string]*MappingEntry // Fast lookup: User Name -> Internal Name
	ByInternal map[string]string        // Fast lookup: Internal Name -> User Name
}

func newGroupMappings() *GroupMappings {
	return &GroupMappings{
		ByUser:     make(map[string]*MappingEntry),
		ByInternal: make(map[string]string),
	}
}

type GenericMappingStore struct {
	mu       sync.RWMutex
	path     string
	mappings map[string]*GroupMappings
	dirty    bool // true when in-memory state is ahead of disk (deferred saves)
}

func NewGenericMappingStore(path string) *GenericMappingStore {
	return &GenericMappingStore{
		path:     path,
		mappings: make(map[string]*GroupMappings),
	}
}

func (s *GenericMappingStore) CreateMapping(group, userName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createMappingLocked(group, userName)
}

// createMappingLocked is the core mapping creation logic. Caller must hold s.mu.Lock().
func (s *GenericMappingStore) createMappingLocked(group, userName string) (string, error) {
	if userName == "" {
		return "", fmt.Errorf("invalid name")
	}

	if _, exists := s.mappings[group]; !exists {
		s.mappings[group] = newGroupMappings()
	}
	groupMappings := s.mappings[group]

	if existing, exists := groupMappings.ByUser[userName]; exists {
		return existing.InternalName, nil
	}

	// Find next free internal name: userName, userName.v2, ... (rare path; iterating map is fine)
	var internalName string
	for next := 1; ; next++ {
		if next == 1 {
			internalName = userName
		} else {
			internalName = fmt.Sprintf("%s.v%d", userName, next)
		}
		// Use slot if not present or if it was dropped (value "" means we can reuse)
		if v, exists := groupMappings.ByInternal[internalName]; !exists || v == "" {
			break
		}
	}

	newMapping := &MappingEntry{UserName: userName, InternalName: internalName}

	groupMappings.ByUser[userName] = newMapping
	groupMappings.ByInternal[internalName] = userName

	return internalName, nil
}

func (s *GenericMappingStore) GetInternalName(group, userName string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupMappings, exists := s.mappings[group]
	if !exists {
		return userName, false // Fallback
	}

	if mapping, exists := groupMappings.ByUser[userName]; exists {
		return mapping.InternalName, true
	}

	return userName, false
}

func (s *GenericMappingStore) GetUserNames(group string, internalNames []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupMappings, exists := s.mappings[group]
	if !exists {
		// No mappings, return as is
		return internalNames
	}

	result := make([]string, len(internalNames))
	for i, internalName := range internalNames {
		if userName, exists := groupMappings.ByInternal[internalName]; exists {
			result[i] = userName
		} else {
			// If not found (e.g., never mapped), return the internalName as a fallback.
			// (If it were explicitly deleted, exists would be true and userName would be "")
			result[i] = internalName
		}
	}
	return result
}

func (s *GenericMappingStore) RenameMapping(group, oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldName == "" || newName == "" {
		return fmt.Errorf("invalid name")
	}

	if _, exists := s.mappings[group]; !exists {
		s.mappings[group] = newGroupMappings()
	}
	groupMappings := s.mappings[group]

	// 1. Is newName already in use?
	if _, exists := groupMappings.ByUser[newName]; exists {
		return fmt.Errorf("field %s already exists", newName)
	}

	// 2. Find oldName
	oldMapping, exists := groupMappings.ByUser[oldName]
	if !exists {
		oldMapping = &MappingEntry{UserName: oldName, InternalName: oldName}
	}

	internalName := oldMapping.InternalName
	delete(groupMappings.ByUser, oldName)
	newMapping := &MappingEntry{UserName: newName, InternalName: internalName}

	groupMappings.ByUser[newName] = newMapping
	groupMappings.ByInternal[internalName] = newName

	return nil
}

func (s *GenericMappingStore) DropMapping(group, userName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupMappings, exists := s.mappings[group]
	if !exists {
		return nil // Nothing to drop
	}

	if mapping, exists := groupMappings.ByUser[userName]; exists {
		internalName := mapping.InternalName
		delete(groupMappings.ByUser, userName)
		// Instead of fully removing from ByInternal, we should mark it deleted by pointing to empty string
		// This ensures GetUserNames returns "" for this internal name, hiding it from queries.
		groupMappings.ByInternal[internalName] = ""
	}

	return nil
}

func (s *GenericMappingStore) DropMappingsByInternalName(group, internalName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupMappings, exists := s.mappings[group]
	if !exists {
		return nil
	}

	if userName, exists := groupMappings.ByInternal[internalName]; exists {
		delete(groupMappings.ByUser, userName)
		// Mark it deleted in ByInternal
		groupMappings.ByInternal[internalName] = ""
	}

	return nil
}

// SoftDeleteMapping is essentially a drop since we don't track state
func (s *GenericMappingStore) SoftDeleteMapping(group, userName string) error {
	return s.DropMapping(group, userName)
}

func (s *GenericMappingStore) DropGroup(group string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mappings, group)
	return nil
}

func (s *GenericMappingStore) DropDatabase(database string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mappings = make(map[string]*GroupMappings)
	return nil
}

func (s *GenericMappingStore) Load() error {
	panic("GenericMappingStore.Load must not be called directly; use the typed store's load()")
}

func (s *GenericMappingStore) Save() error {
	panic("GenericMappingStore.Save must not be called directly; use the typed store's Save()")
}

// GetAllMappings returns a list of all ACTIVE mappings
func (s *GenericMappingStore) GetAllMappings(group string) []*MappingEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getAllMappingsLocked(group)
}

// getAllMappingsLocked returns all active mappings. Caller must hold at least s.mu.RLock().
func (s *GenericMappingStore) getAllMappingsLocked(group string) []*MappingEntry {
	groupMappings, exists := s.mappings[group]
	if !exists {
		return nil
	}

	result := make([]*MappingEntry, 0, len(groupMappings.ByUser))
	for _, mapping := range groupMappings.ByUser {
		mCopy := *mapping
		result = append(result, &mCopy)
	}

	return result
}

func (s *GenericMappingStore) MarshalAndSave(marshalFunc func() ([]byte, error), verifyFunc func([]byte) error) error {
	// Hold the full lock for marshaling and file operations to prevent concurrent
	// saves from racing on file renames. This is acceptable because saves are
	// infrequent (once per batch for deferred writes, or on rare DDL operations).
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := marshalFunc()
	if err != nil {
		return fmt.Errorf("marshal mapping data: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return fmt.Errorf("create mapping directory: %w", err)
	}

	// 1. Write to temp file
	tempFile := s.path + ".tmp"
	if err := ioutil.WriteFile(tempFile, b, 0666); err != nil {
		return fmt.Errorf("write temp mapping file: %w", err)
	}

	// 2. Read back and verify round-trip integrity
	if verifyFunc != nil {
		readBack, err := ioutil.ReadFile(tempFile)
		if err != nil {
			os.Remove(tempFile)
			return fmt.Errorf("read back temp mapping file: %w", err)
		}
		if err := verifyFunc(readBack); err != nil {
			os.Remove(tempFile)
			return fmt.Errorf("verify temp mapping file: %w", err)
		}
	}

	// 3. Backup current file (if it exists) — exactly one rolling backup
	backupFile := s.path + ".bak"
	if _, err := os.Stat(s.path); err == nil {
		os.Remove(backupFile)
		if err := os.Rename(s.path, backupFile); err != nil {
			os.Remove(tempFile)
			return fmt.Errorf("backup current mapping file: %w", err)
		}
	}

	// 4. Atomic rename temp to current
	if err := os.Rename(tempFile, s.path); err != nil {
		// Try to restore from backup
		if _, statErr := os.Stat(backupFile); statErr == nil {
			os.Rename(backupFile, s.path)
		}
		os.Remove(tempFile)
		return fmt.Errorf("rename temp to mapping file: %w", err)
	}

	return nil
}

// loadWithBackup reads the primary mapping file. Returns nil bytes if the file
// does not exist (empty store).
func (s *GenericMappingStore) loadWithBackup() ([]byte, error) {
	b, err := ioutil.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read mapping file %s: %w", s.path, err)
	}
	if len(b) == 0 {
		return nil, nil
	}
	return b, nil
}

// loadBackupFile reads the .bak backup file as a fallback.
func (s *GenericMappingStore) loadBackupFile() ([]byte, error) {
	backupPath := s.path + ".bak"
	b, err := ioutil.ReadFile(backupPath)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("backup file is empty")
	}
	return b, nil
}

// CreateMappingDeferred creates the in-memory mapping but does not save to disk.
// Call SaveIfDirty after the batch completes to flush changes.
func (s *GenericMappingStore) CreateMappingDeferred(group, userName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, err := s.createMappingLocked(group, userName)
	if err != nil {
		return "", err
	}
	s.dirty = true
	return name, nil
}

// SaveIfDirty is a no-op at the GenericMappingStore level.
// Typed stores override this to flush deferred changes to disk.
func (s *GenericMappingStore) SaveIfDirty() error {
	return nil
}

// IsDirty returns true if in-memory state has been modified since the last save.
func (s *GenericMappingStore) IsDirty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dirty
}

// ClearDirty resets the dirty flag after a successful save.
func (s *GenericMappingStore) ClearDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = false
}

// loadFromDisk handles the shared load-with-backup-fallback pattern.
// unmarshalAndPopulate receives the raw bytes and must unmarshal the protobuf
// and call SetMappings to populate the in-memory state.
func (s *GenericMappingStore) loadFromDisk(unmarshalAndPopulate func([]byte) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0777); err != nil {
		return err
	}

	b, err := s.loadWithBackup()
	if err != nil {
		return err
	}
	if b == nil {
		return nil
	}

	if err := unmarshalAndPopulate(b); err != nil {
		// Primary file corrupt — try backup
		backupBytes, backupErr := s.loadBackupFile()
		if backupErr != nil {
			return fmt.Errorf("mapping file %s corrupt (%v) and no valid backup: %v", s.path, err, backupErr)
		}
		if unmarshalErr := unmarshalAndPopulate(backupBytes); unmarshalErr != nil {
			return fmt.Errorf("mapping file %s corrupt (%v) and backup also corrupt: %v", s.path, err, unmarshalErr)
		}
		// Recovered from backup — restore it as the primary
		ioutil.WriteFile(s.path, backupBytes, 0666)
	}

	return nil
}

// GetAllGroups returns all groups (e.g. measurements) in the store
func (s *GenericMappingStore) GetAllGroups() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getAllGroupsLocked()
}

// getAllGroupsLocked returns all groups. Caller must hold at least s.mu.RLock().
func (s *GenericMappingStore) getAllGroupsLocked() []string {
	var groups []string
	for group := range s.mappings {
		groups = append(groups, group)
	}
	return groups
}

// Helper methods for direct assignment, used mainly by tests or initialization
func (s *GenericMappingStore) SetMappings(group string, mappings []*MappingEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupMappings := newGroupMappings()
	for _, m := range mappings {
		mCopy := *m
		groupMappings.ByUser[m.UserName] = &mCopy
		groupMappings.ByInternal[m.InternalName] = m.UserName
	}
	s.mappings[group] = groupMappings
}
