package tsdb

import (
	"encoding/json"
	"sort"
)

type FieldMapping = MappingEntry

type FieldMappingInfo struct {
	Measurement  string
	UserName     string
	InternalName string
}

type FieldMappingStore struct {
	*GenericMappingStore
}

// jsonFieldMappingMeasurement is one measurement's entries in field_mappings.json.
type jsonFieldMappingMeasurement struct {
	Name     string             `json:"name"`
	Mappings []JSONMappingEntry `json:"mappings"`
}

// jsonFieldMappingFile is the on-disk JSON format for field_mappings.json.
type jsonFieldMappingFile struct {
	Measurements []jsonFieldMappingMeasurement `json:"measurements"`
}

func NewFieldMappingStore(path string) (*FieldMappingStore, error) {
	store := &FieldMappingStore{
		GenericMappingStore: NewGenericMappingStore(path),
	}
	return store, store.load()
}

func (s *FieldMappingStore) GetInternalFieldName(measurement, userName string) (string, bool) {
	return s.GetInternalName(measurement, userName)
}

func (s *FieldMappingStore) RenameField(measurement, oldName, newName string) error {
	if err := ValidateFieldName(oldName); err != nil {
		return err
	}
	if err := ValidateFieldName(newName); err != nil {
		return err
	}
	err := s.RenameMapping(measurement, oldName, newName)
	if err != nil {
		return err
	}
	return s.Save()
}

func (s *FieldMappingStore) SoftDeleteField(measurement, userName string) error {
	if err := ValidateFieldName(userName); err != nil {
		return err
	}
	err := s.SoftDeleteMapping(measurement, userName)
	if err != nil {
		return err
	}
	return s.Save()
}

func (s *FieldMappingStore) CreateFieldMapping(measurement, userName string) (string, error) {
	if err := ValidateFieldName(userName); err != nil {
		return "", err
	}
	name, err := s.CreateMapping(measurement, userName)
	if err != nil {
		return "", err
	}
	return name, s.Save()
}

// CreateFieldMappingDeferred creates the in-memory mapping but does not save to disk.
// Call SaveIfDirty after the batch completes to flush changes.
func (s *FieldMappingStore) CreateFieldMappingDeferred(measurement, userName string) (string, error) {
	if err := ValidateFieldName(userName); err != nil {
		return "", err
	}
	return s.CreateMappingDeferred(measurement, userName)
}

// SaveIfDirty writes to disk only if in-memory state has changed since last save.
// The dirty flag is cleared under the write lock before the save begins so that
// any concurrent write that sets dirty=true after the clear will be caught by
// the next SaveIfDirty call rather than being silently lost.
func (s *FieldMappingStore) SaveIfDirty() error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	s.dirty = false
	s.mu.Unlock()
	if err := s.Save(); err != nil {
		// Restore dirty so the next batch retries the flush.
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *FieldMappingStore) GetUserFieldNames(measurement string, internalFieldNames []string) []string {
	return s.GetUserNames(measurement, internalFieldNames)
}

func (s *FieldMappingStore) GetAllMappings(measurement string) []*FieldMappingInfo {
	var result []*FieldMappingInfo

	if measurement != "" {
		for _, m := range s.GenericMappingStore.GetAllMappings(measurement) {
			result = append(result, &FieldMappingInfo{
				Measurement:  measurement,
				UserName:     m.UserName,
				InternalName: m.InternalName,
			})
		}
	} else {
		// Hold the lock for the entire traversal so no group can be added
		// or removed between fetching the group list and iterating entries.
		for meas, mappings := range s.GenericMappingStore.GetAllMappingsAllGroups() {
			for _, m := range mappings {
				result = append(result, &FieldMappingInfo{
					Measurement:  meas,
					UserName:     m.UserName,
					InternalName: m.InternalName,
				})
			}
		}
	}
	return result
}

// Save writes the mapping store to disk.
func (s *FieldMappingStore) Save() error {
	return s.MarshalAndSave(
		func() ([]byte, error) {
			groups := s.GenericMappingStore.getAllGroupsLocked()
			sort.Strings(groups)

			jf := jsonFieldMappingFile{
				Measurements: make([]jsonFieldMappingMeasurement, 0, len(groups)),
			}

			for _, meas := range groups {
				entries := s.GenericMappingStore.getAllEntriesLocked(meas)
				sort.Slice(entries, func(i, j int) bool {
					return entries[i].InternalName < entries[j].InternalName
				})
				jf.Measurements = append(jf.Measurements, jsonFieldMappingMeasurement{
					Name:     meas,
					Mappings: entries,
				})
			}

			return json.Marshal(jf)
		},
		func(b []byte) error {
			var jf jsonFieldMappingFile
			return json.Unmarshal(b, &jf)
		},
	)
}

func (s *FieldMappingStore) load() error {
	return s.loadFromDisk(func(b []byte) error {
		var jf jsonFieldMappingFile
		if err := json.Unmarshal(b, &jf); err != nil {
			return err
		}
		for _, meas := range jf.Measurements {
			mappings := make([]*MappingEntry, 0, len(meas.Mappings))
			for _, e := range meas.Mappings {
				mappings = append(mappings, &MappingEntry{
					UserName:     e.UserName,
					InternalName: e.InternalName,
				})
			}
			s.SetMappings(meas.Name, mappings)
		}
		return nil
	})
}
