package tsdb

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/proto"
	internal "github.com/influxdata/influxdb/tsdb/internal"
)

type DatabaseMapping = MappingEntry

type DatabaseMappingInfo struct {
	UserName     string
	InternalName string
}

type DatabaseMappingStore struct {
	*GenericMappingStore
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

func (s *DatabaseMappingStore) SoftDeleteDatabase(userName string) error {
	err := s.SoftDeleteMapping("", userName)
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
	return s.MarshalAndSave(false, func() ([]byte, error) {
		mappings := s.GenericMappingStore.GetAllMappings("")
		pb := internal.DatabaseMappingSet{
			Mappings: make([]*internal.DatabaseMapping, 0, len(mappings)),
		}

		for _, mapping := range mappings {
			pb.Mappings = append(pb.Mappings, &internal.DatabaseMapping{
				UserName:     mapping.UserName,
				InternalName: mapping.InternalName,
			})
		}

		return proto.Marshal(&pb)
	})
}

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

	var mappings []*MappingEntry
	for _, mapping := range pb.Mappings {
		mappings = append(mappings, &MappingEntry{
			UserName:     mapping.UserName,
			InternalName: mapping.InternalName,
		})
	}
	s.SetMappings("", mappings)

	return nil
}
