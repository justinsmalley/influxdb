package tsdb

import (
	"github.com/gogo/protobuf/proto"
	internal "github.com/influxdata/influxdb/tsdb/internal"
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
func (s *FieldMappingStore) SaveIfDirty() error {
	if !s.IsDirty() {
		return nil
	}
	s.ClearDirty()
	return s.Save()
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
		for _, meas := range s.GenericMappingStore.GetAllGroups() {
			for _, m := range s.GenericMappingStore.GetAllMappings(meas) {
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

// Save writes the mapping store to disk
func (s *FieldMappingStore) Save() error {
	return s.MarshalAndSave(
		func() ([]byte, error) {
			groups := s.GenericMappingStore.getAllGroupsLocked()
			pb := internal.FieldMappingSet{
				Measurements: make([]*internal.FieldMappingMeasurement, 0, len(groups)),
			}

			for _, measurement := range groups {
				measMappings := s.GenericMappingStore.getAllMappingsLocked(measurement)
				meas := &internal.FieldMappingMeasurement{
					Name:     []byte(measurement),
					Mappings: make([]*internal.FieldMapping, 0, len(measMappings)),
				}

				for _, mapping := range measMappings {
					meas.Mappings = append(meas.Mappings, &internal.FieldMapping{
						UserName:     mapping.UserName,
						InternalName: mapping.InternalName,
					})
				}
				pb.Measurements = append(pb.Measurements, meas)
			}

			return proto.Marshal(&pb)
		},
		func(b []byte) error {
			var pb internal.FieldMappingSet
			return proto.Unmarshal(b, &pb)
		},
	)
}

func (s *FieldMappingStore) load() error {
	return s.loadFromDisk(func(b []byte) error {
		var pb internal.FieldMappingSet
		if err := proto.Unmarshal(b, &pb); err != nil {
			return err
		}
		for _, meas := range pb.Measurements {
			measurement := string(meas.Name)
			var measMappings []*MappingEntry
			for _, mapping := range meas.Mappings {
				measMappings = append(measMappings, &MappingEntry{
					UserName:     mapping.UserName,
					InternalName: mapping.InternalName,
				})
			}
			s.SetMappings(measurement, measMappings)
		}
		return nil
	})
}
