package tsdb

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/proto"
	internal "github.com/influxdata/influxdb/tsdb/internal"
)

type MeasurementMappingState = MappingState

const (
	MeasurementMappingState_MEASUREMENT_ACTIVE  = MappingStateActive
	MeasurementMappingState_MEASUREMENT_DELETED = MappingStateDeleted
	MeasurementMappingState_MEASUREMENT_RENAMED = MappingStateRenamed
)

type MeasurementMapping = MappingEntry

type MeasurementMappingInfo struct {
	UserName     string
	InternalName string
	Version      int64
	State        MeasurementMappingState
}

type MeasurementMappingStore struct {
	*GenericMappingStore
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
	err := s.RenameMapping("", oldName, newName)
	if err != nil {
		return err
	}
	return s.Save()
}

func (s *MeasurementMappingStore) SoftDeleteMeasurement(userName string) error {
	err := s.SoftDeleteMapping("", userName)
	if err != nil {
		return err
	}
	return s.Save()
}

func (s *MeasurementMappingStore) CreateMeasurementMapping(userName string) (string, error) {
	name, err := s.CreateMapping("", userName)
	if err != nil {
		return "", err
	}
	return name, s.Save()
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
			Version:      m.Version,
			State:        m.State,
		})
	}
	return result
}

func (s *MeasurementMappingStore) Save() error {
	return s.MarshalAndSave(false, func() ([]byte, error) {
		mappings := s.GenericMappingStore.mappings[""]
		pb := internal.MeasurementMappingSet{
			Mappings: make([]*internal.MeasurementMapping, 0, len(mappings)),
		}

		for _, mapping := range mappings {
			pb.Mappings = append(pb.Mappings, &internal.MeasurementMapping{
				UserName:     mapping.UserName,
				InternalName: mapping.InternalName,
				Version:      mapping.Version,
				State:        internal.MeasurementMappingState(mapping.State),
			})
		}

		return proto.Marshal(&pb)
	})
}

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

	var mappings []*MappingEntry
	for _, mapping := range pb.Mappings {
		mappings = append(mappings, &MappingEntry{
			UserName:     mapping.UserName,
			InternalName: mapping.InternalName,
			Version:      mapping.Version,
			State:        MappingState(mapping.State),
		})
	}
	s.SetMappings("", mappings)

	return nil
}
