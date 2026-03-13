package tsdb

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/proto"
	internal "github.com/influxdata/influxdb/tsdb/internal"
)

type FieldMappingState = MappingState

const (
	FieldMappingState_ACTIVE  = MappingStateActive
	FieldMappingState_DELETED = MappingStateDeleted
	FieldMappingState_RENAMED = MappingStateRenamed
)

type FieldMapping = MappingEntry

type FieldMappingInfo struct {
	Measurement  string
	UserName     string
	InternalName string
	Version      int64
	State        FieldMappingState
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

func (s *FieldMappingStore) GetUserFieldNames(measurement string, internalFieldNames []string) []string {
	return s.GetUserNames(measurement, internalFieldNames)
}

func (s *FieldMappingStore) GetAllMappings(measurement string) []*FieldMappingInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*FieldMappingInfo

	if measurement != "" {
		for _, m := range s.mappings[measurement] {
			result = append(result, &FieldMappingInfo{
				Measurement:  measurement,
				UserName:     m.UserName,
				InternalName: m.InternalName,
				Version:      m.Version,
				State:        m.State,
			})
		}
	} else {
		for meas, measMappings := range s.mappings {
			for _, m := range measMappings {
				result = append(result, &FieldMappingInfo{
					Measurement:  meas,
					UserName:     m.UserName,
					InternalName: m.InternalName,
					Version:      m.Version,
					State:        m.State,
				})
			}
		}
	}
	return result
}

// Save writes the mapping store to disk
func (s *FieldMappingStore) Save() error {
	return s.MarshalAndSave(true, func() ([]byte, error) {
		pb := internal.FieldMappingSet{
			Measurements: make([]*internal.FieldMappingMeasurement, 0, len(s.mappings)),
		}

		for measurement, measMappings := range s.mappings {
			meas := &internal.FieldMappingMeasurement{
				Name:     []byte(measurement),
				Mappings: make([]*internal.FieldMapping, 0, len(measMappings)),
			}

			for _, mapping := range measMappings {
				stateVal := internal.FieldMappingState(mapping.State)
				meas.Mappings = append(meas.Mappings, &internal.FieldMapping{
					UserName:     mapping.UserName,
					InternalName: mapping.InternalName,
					Version:      mapping.Version,
					State:        stateVal,
				})
			}
			pb.Measurements = append(pb.Measurements, meas)
		}

		return proto.Marshal(&pb)
	})
}

func (s *FieldMappingStore) load() error {
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

	var pb internal.FieldMappingSet
	if err := proto.Unmarshal(b, &pb); err != nil {
		return err
	}

	for _, meas := range pb.Measurements {
		measurement := string(meas.Name)
		var measMappings []*MappingEntry
		for _, mapping := range meas.Mappings {
			stateVal := MappingState(mapping.State)
			measMappings = append(measMappings, &MappingEntry{
				UserName:     mapping.UserName,
				InternalName: mapping.InternalName,
				Version:      mapping.Version,
				State:        stateVal,
			})
		}
		s.SetMappings(measurement, measMappings)
	}

	return nil
}
