package tsdb

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type MappingState int32

const (
	MappingStateActive  MappingState = 1
	MappingStateDeleted MappingState = 2
	MappingStateRenamed MappingState = 3
)

type MappingEntry struct {
	UserName     string
	InternalName string
	Version      int64
	State        MappingState
}

type GenericMappingStore struct {
	mu       sync.RWMutex
	path     string
	mappings map[string][]*MappingEntry
}

func NewGenericMappingStore(path string) *GenericMappingStore {
	return &GenericMappingStore{
		path:     path,
		mappings: make(map[string][]*MappingEntry),
	}
}

func (s *GenericMappingStore) CreateMapping(group, userName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userName == "" {
		return "", fmt.Errorf("invalid name")
	}

	if _, exists := s.mappings[group]; !exists {
		s.mappings[group] = make([]*MappingEntry, 0)
	}
	groupMappings := s.mappings[group]

	for _, m := range groupMappings {
		if m.UserName == userName {
			if m.State == MappingStateActive {
				return m.InternalName, nil
			}
			// It was explicitly marked deleted or renamed away
			if m.State == MappingStateDeleted || m.State == MappingStateRenamed {
				// To support dropping and recreating databases/measurements,
				// we just generate a new version number for it rather than erroring out.
				// We continue checking the loop in case there are other active mappings.
			}
		}
	}

	// Find the highest version for this userName in the entire group.
	// This ensures we don't accidentally reuse a version number.
	var maxVersion int64 = 0
	hasAnyMapping := false
	for _, m := range groupMappings {
		// IMPORTANT: we have to check across ALL mappings that might have been
		// assigned this internalName pattern, even if they were renamed away.
		// It's safer to just check the versions of this userName historically.
		if m.UserName == userName {
			hasAnyMapping = true
			if m.Version > maxVersion {
				maxVersion = m.Version
			}
		}
	}
	
	// Start with version 1 if it's the very first time we see this name
	var internalName string
	var nextVersion int64
	
	if !hasAnyMapping {
		internalName = userName
		nextVersion = 1
	} else {
		nextVersion = maxVersion + 1
		if nextVersion < 2 {
			nextVersion = 2 // Recreated names start at v2
		}
		
		// Loop to ensure the internal name we generate is truly unique
		for {
			internalName = fmt.Sprintf("%s.v%d", userName, nextVersion)
			
			nameExists := false
			for _, m := range groupMappings {
				if m.InternalName == internalName {
					nameExists = true
					break
				}
			}
			
			if !nameExists {
				break
			}
			nextVersion++
		}
	}

	newMapping := &MappingEntry{
		UserName:     userName,
		InternalName: internalName,
		Version:      nextVersion,
		State:        MappingStateActive,
	}

	s.mappings[group] = append(s.mappings[group], newMapping)
	return internalName, nil
}

func (s *GenericMappingStore) GetInternalName(group, userName string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupMappings := s.mappings[group]
	// We iterate in chronological order to return the LAST state for this userName
	var lastMapping *MappingEntry
	for _, m := range groupMappings {
		if m.UserName == userName {
			lastMapping = m
		}
	}

	// If no mapping was found, fallback to the requested name.
	if lastMapping == nil {
		return userName, true // Fallback: if not mapped, it doesn't have an internal name.
	}
	
	if lastMapping.State == MappingStateDeleted {
		return "", false // Explicitly return empty string if it's currently deleted
	}
	
	// If it's MappingStateRenamed, we still return the internal name but indicate it is inactive.
	// This allows the query engine to properly mask historical queries for renamed fields.
	return lastMapping.InternalName, lastMapping.State == MappingStateActive
}

func (s *GenericMappingStore) GetUserNames(group string, internalNames []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupMappings := s.mappings[group]
	internalToUser := make(map[string]string)

	// If we're trying to resolve a name that's not currently active, we need to know what it was renamed to.
	// We trace the lineage forward: find the most recent mapping with the same internalName.
	
	internalHistory := make(map[string][]*MappingEntry)
	for _, m := range groupMappings {
		internalHistory[m.InternalName] = append(internalHistory[m.InternalName], m)
	}

	for internalName, history := range internalHistory {
		// First pass: try to find an active mapping.
		// Since mappings are appended chronologically, we want the active mapping for this internal name.
		var activeName string
		foundActive := false
		for _, m := range history {
			if m.State == MappingStateActive {
				activeName = m.UserName
				foundActive = true
			}
		}

		if foundActive {
			internalToUser[internalName] = activeName
		} else if len(history) > 0 {
			// If no active mapping, fallback to the *last known user name* for this internal name.
			// The last entry in the history slice is the most recent state.
			lastEntry := history[len(history)-1]
			
			if lastEntry.State == MappingStateDeleted {
				internalToUser[internalName] = "" // HIDE DELETED NAMES
			} else if lastEntry.State == MappingStateRenamed {
				// Fallback to the last known user name.
				// Under normal operations, a renamed entry is immediately followed by an active entry.
				internalToUser[internalName] = lastEntry.UserName
			} else {
				internalToUser[internalName] = lastEntry.UserName
			}
		}
	}

	result := make([]string, len(internalNames))
	for i, internalName := range internalNames {
		if userName, ok := internalToUser[internalName]; ok {
			result[i] = userName // Might be empty string if DELETED, which is what we want to hide it
		} else {
			result[i] = internalName
		}
	}
	return result
}

func (s *GenericMappingStore) RenameMapping(group, oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldName == newName {
		return fmt.Errorf("old and new names cannot be the same")
	}

	if _, exists := s.mappings[group]; !exists {
		s.mappings[group] = make([]*MappingEntry, 0)
	}
	groupMappings := s.mappings[group]

	var activeOld *MappingEntry
	for _, m := range groupMappings {
		if m.UserName == oldName && m.State == MappingStateActive {
			activeOld = m
			break
		}
	}

	if activeOld == nil {
		for _, m := range groupMappings {
			if m.UserName == oldName {
				// If the source name exists in the mappings but is not active, it cannot be renamed.
				return fmt.Errorf("%s is not active", oldName)
			}
		}

		activeOld = &MappingEntry{
			UserName:     oldName,
			InternalName: oldName,
			Version:      1,
			State:        MappingStateActive,
		}
		s.mappings[group] = append(s.mappings[group], activeOld)
		groupMappings = s.mappings[group]
	}

	// We only prevent renaming to an ACTIVE name.
	// Reclaiming inactive names is explicitly permitted and will
	// bump the version number to ensure the new data doesn't collide
	// with the historical data of the inactive name.
	for _, m := range groupMappings {
		if m.UserName == newName && m.State == MappingStateActive {
			// This matches what the coordinator used to return, so the python tests 
			// checking for "field_b already exists" will pass.
			return fmt.Errorf("field %s already exists", newName)
		}
	}
	
	// Mark the old mapping as renamed. The internal name and version stay the same,
	// maintaining the physical mapping link while releasing the user-facing name.
	activeOld.State = MappingStateRenamed

	newMapping := &MappingEntry{
		UserName:     newName,
		InternalName: activeOld.InternalName,
		Version:      activeOld.Version,
		State:        MappingStateActive,
	}

	s.mappings[group] = append(s.mappings[group], newMapping)
	return nil
}

func (s *GenericMappingStore) DropMapping(group, userName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupMappings, exists := s.mappings[group]
	if !exists {
		return nil
	}

	var pruned []*MappingEntry
	for _, m := range groupMappings {
		if m.UserName != userName {
			pruned = append(pruned, m)
		}
	}
	s.mappings[group] = pruned
	return nil
}

func (s *GenericMappingStore) DropMappingsByInternalName(group, internalName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupMappings, exists := s.mappings[group]
	if !exists {
		return nil
	}

	var pruned []*MappingEntry
	for _, m := range groupMappings {
		if m.InternalName != internalName {
			pruned = append(pruned, m)
		}
	}
	s.mappings[group] = pruned
	return nil
}

func (s *GenericMappingStore) SoftDeleteMapping(group, userName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userName == "" {
		return fmt.Errorf("invalid name")
	}

	if _, exists := s.mappings[group]; !exists {
		s.mappings[group] = make([]*MappingEntry, 0)
	}
	groupMappings := s.mappings[group]

	var mapping *MappingEntry
	for _, m := range groupMappings {
		if m.UserName == userName && m.State == MappingStateActive {
			mapping = m
			break
		}
	}

	if mapping == nil {
		for _, m := range groupMappings {
			if m.UserName == userName {
				return fmt.Errorf("%s is not active", userName)
			}
		}

		mapping = &MappingEntry{
			UserName:     userName,
			InternalName: userName,
			Version:      1,
			State:        MappingStateActive,
		}
		s.mappings[group] = append(s.mappings[group], mapping)
	}

	mapping.State = MappingStateDeleted
	return nil
}

func (s *GenericMappingStore) GarbageCollect(retainDeleted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for group, groupMappings := range s.mappings {
		seenUser := make(map[string]bool)
		seenInternal := make(map[string]bool)
		var pruned []*MappingEntry

		for i := len(groupMappings) - 1; i >= 0; i-- {
			m := groupMappings[i]
			
			// Always keep active mappings
			if m.State == MappingStateActive {
				pruned = append(pruned, m)
				seenUser[m.UserName] = true
				seenInternal[m.InternalName] = true
				continue
			}

			// If inactive, only keep it if we haven't seen an active/newer mapping
			// for this specific internal name (needed to resolve historical data)
			// AND only if we are instructed to retain deleted mappings (i.e. fields)
			if m.State == MappingStateRenamed {
				// We MUST keep ALL renamed mappings so old names don't expose underlying data
				pruned = append(pruned, m)
				seenInternal[m.InternalName] = true
			} else if m.State == MappingStateDeleted && retainDeleted {
				// For deleted states, we only keep them if instructed (fields)
				pruned = append(pruned, m)
				seenInternal[m.InternalName] = true
			}
		}

		for i, j := 0, len(pruned)-1; i < j; i, j = i+1, j-1 {
			pruned[i], pruned[j] = pruned[j], pruned[i]
		}

		s.mappings[group] = pruned
	}
}

func (s *GenericMappingStore) GetAllMappings(group string) []*MappingEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupMappings := s.mappings[group]
	result := make([]*MappingEntry, len(groupMappings))
	for i, m := range groupMappings {
		mCopy := *m
		result[i] = &mCopy
	}
	return result
}

func (s *GenericMappingStore) SetMappings(group string, mappings []*MappingEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mappings[group] = mappings
}

func (s *GenericMappingStore) MarshalAndSave(retainDeleted bool, marshalFunc func() ([]byte, error)) error {
	s.GarbageCollect(retainDeleted)

	s.mu.RLock()
	b, err := marshalFunc()
	s.mu.RUnlock()
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	tempFile := s.path + ".tmp"
	if err := os.WriteFile(tempFile, b, 0666); err != nil {
		return err
	}

	if err := os.Rename(tempFile, s.path); err != nil {
		os.Remove(tempFile)
		return err
	}
	return nil
}
