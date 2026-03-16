package tsdb

import (
	"encoding/json"
	"sort"
)

type MeasurementMapping = MappingEntry

type MeasurementMappingInfo struct {
	UserName     string
	InternalName string
}

type MeasurementMappingStore struct {
	*GenericMappingStore
}

// jsonMeasurementMappingFile is the on-disk JSON format for measurement_mappings.json.
type jsonMeasurementMappingFile struct {
	Mappings []JSONMappingEntry `json:"mappings"`
}

func NewMeasurementMappingStore(path string) (*MeasurementMappingStore, error) {
	store := &MeasurementMappingStore{
		GenericMappingStore: NewGenericMappingStore(path),
	}
	return store, store.load()
}

func (s *MeasurementMappingStore) GetInternalMeasurementName(userName string) (string, bool) {
	return s.GetInternalName("", userName)
}

func (s *MeasurementMappingStore) RenameMeasurement(oldName, newName string) error {
	if err := ValidateMeasurementName(oldName); err != nil {
		return err
	}
	if err := ValidateMeasurementName(newName); err != nil {
		return err
	}
	if err := s.RenameMapping("", oldName, newName); err != nil {
		return err
	}
	return s.Save()
}

func (s *MeasurementMappingStore) SoftDeleteMeasurement(userName string) error {
	if err := ValidateMeasurementName(userName); err != nil {
		return err
	}
	if err := s.SoftDeleteMapping("", userName); err != nil {
		return err
	}
	return s.Save()
}

func (s *MeasurementMappingStore) CreateMeasurementMapping(userName string) (string, error) {
	if err := ValidateMeasurementName(userName); err != nil {
		return "", err
	}
	name, err := s.CreateMapping("", userName)
	if err != nil {
		return "", err
	}
	return name, s.Save()
}

// CreateMeasurementMappingDeferred creates the in-memory mapping but does not
// save to disk. Call SaveIfDirty after the batch completes to flush changes.
func (s *MeasurementMappingStore) CreateMeasurementMappingDeferred(userName string) (string, error) {
	if err := ValidateMeasurementName(userName); err != nil {
		return "", err
	}
	return s.CreateMappingDeferred("", userName)
}

// SaveIfDirty writes to disk only if in-memory state has changed since last save.
func (s *MeasurementMappingStore) SaveIfDirty() error {
	if !s.IsDirty() {
		return nil
	}
	if err := s.Save(); err != nil {
		// Leave dirty=true so the next batch retries the flush.
		return err
	}
	s.ClearDirty()
	return nil
}

func (s *MeasurementMappingStore) GetUserMeasurementNames(internalNames []string) []string {
	return s.GetUserNames("", internalNames)
}

func (s *MeasurementMappingStore) GetAllMappings() []*MeasurementMappingInfo {
	mappings := s.GenericMappingStore.GetAllMappings("")
	var result []*MeasurementMappingInfo
	for _, m := range mappings {
		result = append(result, &MeasurementMappingInfo{
			UserName:     m.UserName,
			InternalName: m.InternalName,
		})
	}
	return result
}

func (s *MeasurementMappingStore) Save() error {
	return s.MarshalAndSave(
		func() ([]byte, error) {
			entries := s.GenericMappingStore.getAllEntriesLocked("")
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].InternalName < entries[j].InternalName
			})
			jf := jsonMeasurementMappingFile{Mappings: entries}
			return json.Marshal(jf)
		},
		func(b []byte) error {
			var jf jsonMeasurementMappingFile
			return json.Unmarshal(b, &jf)
		},
	)
}

func (s *MeasurementMappingStore) load() error {
	return s.loadFromDisk(func(b []byte) error {
		var jf jsonMeasurementMappingFile
		if err := json.Unmarshal(b, &jf); err != nil {
			return err
		}
		mappings := make([]*MappingEntry, 0, len(jf.Mappings))
		for _, e := range jf.Mappings {
			mappings = append(mappings, &MappingEntry{
				UserName:     e.UserName,
				InternalName: e.InternalName,
			})
		}
		s.SetMappings("", mappings)
		return nil
	})
}
