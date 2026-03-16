package tsdb

import (
	"github.com/gogo/protobuf/proto"
	internal "github.com/influxdata/influxdb/tsdb/internal"
)

type MeasurementMapping = MappingEntry

type MeasurementMappingInfo struct {
	UserName     string
	InternalName string
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

// CreateMeasurementMappingDeferred creates the in-memory mapping but does not
// save to disk. Call SaveIfDirty after the batch completes to flush changes.
func (s *MeasurementMappingStore) CreateMeasurementMappingDeferred(userName string) (string, error) {
	return s.CreateMappingDeferred("", userName)
}

// SaveIfDirty writes to disk only if in-memory state has changed since last save.
func (s *MeasurementMappingStore) SaveIfDirty() error {
	if !s.IsDirty() {
		return nil
	}
	s.ClearDirty()
	return s.Save()
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
			mappings := s.GenericMappingStore.getAllMappingsLocked("")
			pb := internal.MeasurementMappingSet{
				Mappings: make([]*internal.MeasurementMapping, 0, len(mappings)),
			}

			for _, mapping := range mappings {
				pb.Mappings = append(pb.Mappings, &internal.MeasurementMapping{
					UserName:     mapping.UserName,
					InternalName: mapping.InternalName,
				})
			}

			return proto.Marshal(&pb)
		},
		func(b []byte) error {
			var pb internal.MeasurementMappingSet
			return proto.Unmarshal(b, &pb)
		},
	)
}

func (s *MeasurementMappingStore) load() error {
	return s.loadFromDisk(func(b []byte) error {
		var pb internal.MeasurementMappingSet
		if err := proto.Unmarshal(b, &pb); err != nil {
			return err
		}
		var mappings []*MappingEntry
		for _, m := range pb.Mappings {
			mappings = append(mappings, &MappingEntry{
				UserName:     m.UserName,
				InternalName: m.InternalName,
			})
		}
		s.SetMappings("", mappings)
		return nil
	})
}
