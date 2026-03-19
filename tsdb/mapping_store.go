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
	mu     sync.RWMutex
	fileMu sync.Mutex   // serializes concurrent saves; held only during disk I/O
	path   string
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
	const maxVersionSuffix = 10000
	var internalName string
	for next := 1; ; next++ {
		if next > maxVersionSuffix {
			return "", fmt.Errorf("mapping: too many versions of name %q (max %d); drop old versions before creating new ones", userName, maxVersionSuffix)
		}
		if next == 1 {
			internalName = userName
		} else {
			internalName = fmt.Sprintf("%s.v%d", userName, next)
		}
		// Only use a slot that is entirely absent — never reuse a tombstone
		// (ByInternal[k] == "") because old TSM data for that key is still on
		// disk and would become visible again if we recycled the slot.
		if _, exists := groupMappings.ByInternal[internalName]; !exists {
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

	// 2. Find oldName — must already exist; silently creating an identity mapping
	// for a non-existent name would produce dangling entries.
	oldMapping, exists := groupMappings.ByUser[oldName]
	if !exists {
		return fmt.Errorf("mapping: %q not found", oldName)
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
	return fmt.Errorf("GenericMappingStore.Load must not be called directly; use the typed store's Load()")
}

func (s *GenericMappingStore) Save() error {
	return fmt.Errorf("GenericMappingStore.Save must not be called directly; use the typed store's Save()")
}

// GetAllMappings returns a list of all ACTIVE mappings
func (s *GenericMappingStore) GetAllMappings(group string) []*MappingEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getAllMappingsLocked(group)
}

// JSONMappingEntry is the on-disk JSON representation of a single name mapping.
// UserName == "" indicates a deleted entry: the internal slot is reserved and
// will return "" from GetUserNames (hiding the field/measurement from queries).
type JSONMappingEntry struct {
	UserName     string `json:"userName"`
	InternalName string `json:"internalName"`
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

// getAllEntriesLocked returns ALL entries including deleted ones (UserName == "").
// This is used by Save() to persist the full state, ensuring that deleted
// slots are preserved across restarts. Caller must hold at least s.mu.RLock().
func (s *GenericMappingStore) getAllEntriesLocked(group string) []JSONMappingEntry {
	groupMappings, exists := s.mappings[group]
	if !exists {
		return nil
	}

	result := make([]JSONMappingEntry, 0, len(groupMappings.ByInternal))
	for internalName, userName := range groupMappings.ByInternal {
		result = append(result, JSONMappingEntry{
			UserName:     userName,
			InternalName: internalName,
		})
	}
	return result
}

func (s *GenericMappingStore) MarshalAndSave(marshalFunc func() ([]byte, error), verifyFunc func([]byte) error) error {
	// Step 1: snapshot in-memory state under a read lock.
	// This is brief — just enough to serialize the marshal against concurrent mutations.
	// Readers and writers are not blocked during the subsequent disk I/O.
	s.mu.RLock()
	b, err := marshalFunc()
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal mapping data: %w", err)
	}

	// Step 2: all disk I/O under a separate mutex.
	// This serializes concurrent saves (e.g. two goroutines flushing the same
	// store) without blocking in-memory reads or writes.
	s.fileMu.Lock()
	defer s.fileMu.Unlock()

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
		// Try to restore from backup; if that also fails, surface both errors
		// so the operator knows the store is in an unreadable state.
		if _, statErr := os.Stat(backupFile); statErr == nil {
			if restoreErr := os.Rename(backupFile, s.path); restoreErr != nil {
				os.Remove(tempFile)
				return fmt.Errorf("rename temp to mapping file: %w; also failed to restore backup: %v", err, restoreErr)
			}
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

// saveIfDirty is the shared implementation for SaveIfDirty across all typed stores.
// It clears the dirty flag before saving so that any concurrent CreateMappingDeferred
// that fires in the window is caught by the next call rather than silently lost.
func (s *GenericMappingStore) saveIfDirty(saveFunc func() error) error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	s.dirty = false
	s.mu.Unlock()
	if err := saveFunc(); err != nil {
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return err
	}
	return nil
}

// loadFromDisk handles the shared load-with-backup-fallback pattern.
// unmarshalAndPopulate receives the raw bytes and must unmarshal the JSON
// and call SetMappings to populate the in-memory state.
//
// Recovery order:
//  1. Primary file present and valid — use it.
//  2. Primary file present but corrupt — fall back to .bak.
//  3. Primary file missing — check .bak and recover from it if present;
//     treat as empty store only if .bak also does not exist.
func (s *GenericMappingStore) loadFromDisk(unmarshalAndPopulate func([]byte) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0777); err != nil {
		return err
	}

	b, primaryErr := s.loadWithBackup()
	if primaryErr != nil {
		// IO error reading primary — hard failure, don't silently continue.
		return primaryErr
	}

	if b == nil {
		// Primary is missing or empty — check for a backup to recover from.
		backupBytes, backupErr := s.loadBackupFile()
		if backupErr != nil {
			if os.IsNotExist(backupErr) {
				return nil // Neither primary nor backup: genuinely new store.
			}
			// Backup exists but can't be read — hard failure.
			return fmt.Errorf("primary mapping file %s is missing and backup is unreadable: %v", s.path, backupErr)
		}
		// Backup is readable; recover from it.
		if unmarshalErr := unmarshalAndPopulate(backupBytes); unmarshalErr != nil {
			return fmt.Errorf("primary mapping file %s is missing and backup is corrupt: %v", s.path, unmarshalErr)
		}
		if err := ioutil.WriteFile(s.path, backupBytes, 0666); err != nil {
			return fmt.Errorf("failed to restore primary mapping file from backup: %w", err)
		}
		return nil
	}

	if err := unmarshalAndPopulate(b); err != nil {
		// Primary file corrupt — try backup.
		backupBytes, backupErr := s.loadBackupFile()
		if backupErr != nil {
			return fmt.Errorf("mapping file %s corrupt (%v) and no valid backup: %v", s.path, err, backupErr)
		}
		if unmarshalErr := unmarshalAndPopulate(backupBytes); unmarshalErr != nil {
			return fmt.Errorf("mapping file %s corrupt (%v) and backup also corrupt: %v", s.path, err, unmarshalErr)
		}
		// Recovered from backup — restore it as the primary.
		if err := ioutil.WriteFile(s.path, backupBytes, 0666); err != nil {
			return fmt.Errorf("failed to restore primary mapping file from backup: %w", err)
		}
	}

	return nil
}

// GetAllGroups returns all groups (e.g. measurements) in the store
func (s *GenericMappingStore) GetAllGroups() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getAllGroupsLocked()
}

// GetAllMappingsAllGroups returns a consistent snapshot of all active mappings
// across every group. It holds the read lock for the entire traversal so no
// group can be added or removed mid-iteration.
func (s *GenericMappingStore) GetAllMappingsAllGroups() map[string][]*MappingEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	groups := s.getAllGroupsLocked()
	result := make(map[string][]*MappingEntry, len(groups))
	for _, g := range groups {
		result[g] = s.getAllMappingsLocked(g)
	}
	return result
}

// getAllGroupsLocked returns all groups. Caller must hold at least s.mu.RLock().
func (s *GenericMappingStore) getAllGroupsLocked() []string {
	var groups []string
	for group := range s.mappings {
		groups = append(groups, group)
	}
	return groups
}

// compactLocked removes tombstone entries (ByInternal[k] == "") from a group's
// in-memory map. It must only be called when the caller can guarantee that no
// TSM data exists for the tombstoned internal names — for example, immediately
// after a hard DROP that also deleted on-disk data. Caller must hold s.mu.Lock().
func (s *GenericMappingStore) compactLocked(group string) {
	gm, exists := s.mappings[group]
	if !exists {
		return
	}
	for k, v := range gm.ByInternal {
		if v == "" {
			delete(gm.ByInternal, k)
		}
	}
}

// Compact removes tombstone entries for a group, acquiring the write lock.
// Call this only after on-disk data for the tombstoned internal names has been
// deleted; it is a no-op if the group does not exist.
func (s *GenericMappingStore) Compact(group string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compactLocked(group)
}

// Helper methods for direct assignment, used mainly by tests or initialization
func (s *GenericMappingStore) SetMappings(group string, mappings []*MappingEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupMappings := newGroupMappings()
	for _, m := range mappings {
		mCopy := *m
		// UserName == "" means this internal slot is deleted; record it in
		// ByInternal so GetUserNames returns "" (hidden) rather than falling
		// back to the internal name. Do NOT add to ByUser.
		groupMappings.ByInternal[m.InternalName] = m.UserName
		if m.UserName != "" {
			groupMappings.ByUser[m.UserName] = &mCopy
		}
	}
	s.mappings[group] = groupMappings
}
