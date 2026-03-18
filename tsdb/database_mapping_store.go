package tsdb

import (
	"encoding/json"
	"sort"
)

type DatabaseMappingInfo struct {
	UserName     string
	InternalName string
}

type DatabaseMappingStore struct {
	*GenericMappingStore
}

// jsonDatabaseMappingFile is the on-disk JSON format for database_mappings.json.
type jsonDatabaseMappingFile struct {
	Mappings []JSONMappingEntry `json:"mappings"`
}

func NewDatabaseMappingStore(path string) (*DatabaseMappingStore, error) {
	store := &DatabaseMappingStore{
		GenericMappingStore: NewGenericMappingStore(path),
	}
	return store, store.load()
}

func (s *DatabaseMappingStore) GetInternalDatabaseName(userName string) (string, bool) {
	return s.GetInternalName("", userName)
}

func (s *DatabaseMappingStore) RenameDatabase(oldName, newName string) error {
	err := s.RenameMapping("", oldName, newName)
	if err != nil {
		return err
	}
	return s.Save()
}

func (s *DatabaseMappingStore) DropDatabaseMapping(userName string) error {
	err := s.DropMapping("", userName)
	if err != nil {
		return err
	}
	return s.Save()
}

func (s *DatabaseMappingStore) DropDatabaseMappingByInternal(internalName string) error {
	err := s.DropMappingsByInternalName("", internalName)
	if err != nil {
		return err
	}
	return s.Save()
}

func (s *DatabaseMappingStore) CreateDatabaseMapping(userName string) (string, error) {
	name, err := s.CreateMapping("", userName)
	if err != nil {
		return "", err
	}
	return name, s.Save()
}

// CreateDatabaseMappingDeferred creates the in-memory mapping without an
// immediate disk write. Call SaveIfDirty after the batch completes to flush.
func (s *DatabaseMappingStore) CreateDatabaseMappingDeferred(userName string) (string, error) {
	return s.CreateMappingDeferred("", userName)
}

func (s *DatabaseMappingStore) SaveIfDirty() error { return s.saveIfDirty(s.Save) }

func (s *DatabaseMappingStore) GetUserDatabaseNames(internalNames []string) []string {
	return s.GetUserNames("", internalNames)
}

func (s *DatabaseMappingStore) GetAllMappings() []*DatabaseMappingInfo {
	mappings := s.GenericMappingStore.GetAllMappings("")
	var result []*DatabaseMappingInfo
	for _, m := range mappings {
		result = append(result, &DatabaseMappingInfo{
			UserName:     m.UserName,
			InternalName: m.InternalName,
		})
	}
	return result
}

func (s *DatabaseMappingStore) Save() error {
	return s.MarshalAndSave(
		func() ([]byte, error) {
			entries := s.GenericMappingStore.getAllEntriesLocked("")
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].InternalName < entries[j].InternalName
			})
			jf := jsonDatabaseMappingFile{Mappings: entries}
			return json.Marshal(jf)
		},
		func(b []byte) error {
			var jf jsonDatabaseMappingFile
			return json.Unmarshal(b, &jf)
		},
	)
}

func (s *DatabaseMappingStore) load() error {
	return s.loadFromDisk(func(b []byte) error {
		var jf jsonDatabaseMappingFile
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
