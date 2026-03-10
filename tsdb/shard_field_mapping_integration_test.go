package tsdb_test

import (
	"context"
	"testing"

	"github.com/influxdata/influxdb/query"
	"github.com/influxdata/influxdb/tsdb"
	"github.com/influxdata/influxql"
)

func TestShard_FieldMapping_Integration(t *testing.T) {
	test := func(index string) {
		s := MustOpenStore(index)
		defer s.Close()

		// 1. Create a shard and write initial data.
		// "meas,tag=a temp1=1.0 0"
		s.MustCreateShardWithData("db0", "rp0", 0,
			`meas,tag=a temp1=1.0 0`,
		)

		// Get the FieldMappingStore for the database
		fmStore, err := s.Store.FieldMappingStore("db0")
		if err != nil {
			t.Fatal(err)
		}

		// 2. Rename temp1 -> temperature1
		if err := fmStore.RenameField("meas", "temp1", "temperature1"); err != nil {
			t.Fatal(err)
		}

		// 3. Write new data using the NEW name (temperature1)
		s.MustCreateShardWithData("db0", "rp0", 0,
			`meas,tag=a temperature1=2.0 10`,
		)

		// 4. Verify we can read data using the new name
		sh := s.Shard(0)
		if sh == nil {
			t.Fatal("shard not found")
		}

		// We need to use engine.CreateIterator directly to test reading translated fields
		engine, err := sh.Engine()
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()

		// Read "temperature1"
		opt := query.IteratorOptions{
			Expr:      &influxql.VarRef{Val: "temperature1"},
			Aux:       []influxql.VarRef{},
			StartTime: influxql.MinTime,
			EndTime:   influxql.MaxTime,
			Ascending: true,
		}

		itr, err := engine.CreateIterator(ctx, "meas", opt)
		if err != nil {
			t.Fatal(err)
		}
		if itr == nil {
			t.Fatal("expected iterator, got nil")
		}
		defer itr.Close()

		fitr := itr.(query.FloatIterator)

		// Point 1 (written as temp1=1.0) should be returned as temperature1
		p, err := fitr.Next()
		if err != nil {
			t.Fatal(err)
		}
		if p == nil {
			t.Fatal("expected point, got nil")
		}
		if p.Value != 1.0 {
			t.Fatalf("expected value=1.0 at time=0, got value=%f time=%d", p.Value, p.Time)
		}

		// Point 2 (written as temperature1=2.0)
		p, err = fitr.Next()
		if err != nil {
			t.Fatal(err)
		}
		if p == nil {
			t.Fatal("expected point, got nil")
		}
		if p.Value != 2.0 {
			t.Fatalf("expected value=2.0 at time=10, got value=%f time=%d", p.Value, p.Time)
		}

		// 5. Check FieldDimensions to ensure it returns the correct user-facing field name
		fields, _, err := sh.FieldDimensions([]string{"meas"})
		if err != nil {
			t.Fatal(err)
		}
		if len(fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(fields))
		}
		if _, ok := fields["temperature1"]; !ok {
			t.Fatalf("expected 'temperature1' in FieldDimensions, got: %v", fields)
		}
	}

	for _, index := range tsdb.RegisteredIndexes() {
		t.Run(index, func(t *testing.T) { test(index) })
	}
}
