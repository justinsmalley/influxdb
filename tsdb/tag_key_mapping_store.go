package tsdb

import (
	"encoding/json"
	"sort"
)

// TagKeyMapping is an alias for MappingEntry used in tag key mapping stores.
type TagKeyMapping = MappingEntry

// TagKeyMappingInfo holds the resolved mapping for one tag key in a measurement.
type TagKeyMappingInfo struct {
	Measurement  string
	UserName     string
	InternalName string
}

// TagKeyMappingStore wraps GenericMappingStore for per-measurement tag key mappings.
// Unlike FieldMappingStore, there is no DROP support — tag keys can only be renamed.
type TagKeyMappingStore struct {
	*GenericMappingStore
}

// jsonTagKeyMappingMeasurement is one measurement's entries in tag_key_mappings.json.
type jsonTagKeyMappingMeasurement struct {
	Name     string             `json:"name"`
	Mappings []JSONMappingEntry `json:"mappings"`
}

// jsonTagKeyMappingFile is the on-disk JSON format for tag_key_mappings.json.
type jsonTagKeyMappingFile struct {
	Measurements []jsonTagKeyMappingMeasurement `json:"measurements"`
}

// NewTagKeyMappingStore creates a new TagKeyMappingStore backed by the given path.
func NewTagKeyMappingStore(path string) (*TagKeyMappingStore, error) {
	store := &TagKeyMappingStore{
		GenericMappingStore: NewGenericMappingStore(path),
	}
	return store, store.load()
}

// GetInternalTagKeyName returns the internal name for a user-facing tag key.
// The second return value is true if the mapping is active.
func (s *TagKeyMappingStore) GetInternalTagKeyName(measurement, userName string) (string, bool) {
	return s.GetInternalName(measurement, userName)
}

// GetUserTagKeyNames translates a slice of internal tag key names to user-facing names.
// Names that have no mapping are returned as-is.
func (s *TagKeyMappingStore) GetUserTagKeyNames(measurement string, internalNames []string) []string {
	return s.GetUserNames(measurement, internalNames)
}

// RenameTagKey renames a tag key within a measurement, immediately saving to disk.
// Returns an error if newName is already active for the measurement.
func (s *TagKeyMappingStore) RenameTagKey(measurement, oldName, newName string) error {
	if err := s.RenameMapping(measurement, oldName, newName); err != nil {
		return err
	}
	return s.Save()
}

// CreateTagKeyMapping creates an identity mapping for userName and saves immediately.
func (s *TagKeyMappingStore) CreateTagKeyMapping(measurement, userName string) (string, error) {
	name, err := s.CreateMapping(measurement, userName)
	if err != nil {
		return "", err
	}
	return name, s.Save()
}

// CreateTagKeyMappingDeferred creates an identity mapping but defers the disk write.
// Call SaveIfDirty after the batch completes to flush changes.
func (s *TagKeyMappingStore) CreateTagKeyMappingDeferred(measurement, userName string) (string, error) {
	return s.CreateMappingDeferred(measurement, userName)
}

// SaveIfDirty flushes deferred mapping changes to disk if any exist.
func (s *TagKeyMappingStore) SaveIfDirty() error { return s.saveIfDirty(s.Save) }

// GetAllMappings returns all active tag key mappings for the given measurement.
// If measurement is empty, returns mappings for all measurements.
func (s *TagKeyMappingStore) GetAllMappings(measurement string) []*TagKeyMappingInfo {
	var result []*TagKeyMappingInfo

	if measurement != "" {
		for _, m := range s.GenericMappingStore.GetAllMappings(measurement) {
			result = append(result, &TagKeyMappingInfo{
				Measurement:  measurement,
				UserName:     m.UserName,
				InternalName: m.InternalName,
			})
		}
	} else {
		for meas, mappings := range s.GenericMappingStore.GetAllMappingsAllGroups() {
			for _, m := range mappings {
				result = append(result, &TagKeyMappingInfo{
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
func (s *TagKeyMappingStore) Save() error {
	return s.MarshalAndSave(
		func() ([]byte, error) {
			groups := s.GenericMappingStore.getAllGroupsLocked()
			sort.Strings(groups)

			jf := jsonTagKeyMappingFile{
				Measurements: make([]jsonTagKeyMappingMeasurement, 0, len(groups)),
			}

			for _, meas := range groups {
				entries := s.GenericMappingStore.getAllEntriesLocked(meas)
				sort.Slice(entries, func(i, j int) bool {
					return entries[i].InternalName < entries[j].InternalName
				})
				jf.Measurements = append(jf.Measurements, jsonTagKeyMappingMeasurement{
					Name:     meas,
					Mappings: entries,
				})
			}

			return json.Marshal(jf)
		},
		func(b []byte) error {
			var jf jsonTagKeyMappingFile
			return json.Unmarshal(b, &jf)
		},
	)
}

func (s *TagKeyMappingStore) load() error {
	return s.loadFromDisk(func(b []byte) error {
		var jf jsonTagKeyMappingFile
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
