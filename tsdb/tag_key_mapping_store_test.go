package tsdb_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/influxdata/influxdb/tsdb"
)

func TestTagKeyMappingStore_IdentityOnFirstWrite(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// First write: identity mapping created.
	internal, err := store.CreateTagKeyMapping("cpu", "host")
	if err != nil {
		t.Fatal(err)
	}
	if internal != "host" {
		t.Fatalf("expected identity mapping 'host', got %q", internal)
	}

	// GetInternalTagKeyName should return the identity.
	if name, ok := store.GetInternalTagKeyName("cpu", "host"); !ok || name != "host" {
		t.Fatalf("expected ('host', true), got (%q, %v)", name, ok)
	}
}

func TestTagKeyMappingStore_Rename(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Create initial mapping.
	if _, err := store.CreateTagKeyMapping("cpu", "host"); err != nil {
		t.Fatal(err)
	}

	// Rename host -> hostname.
	if err := store.RenameTagKey("cpu", "host", "hostname"); err != nil {
		t.Fatalf("unexpected rename error: %v", err)
	}

	// Old user name should no longer be active.
	if _, ok := store.GetInternalTagKeyName("cpu", "host"); ok {
		t.Fatal("expected 'host' to be inactive after rename")
	}

	// New user name should point to the original internal name.
	internalName, ok := store.GetInternalTagKeyName("cpu", "hostname")
	if !ok {
		t.Fatal("expected 'hostname' to be active after rename")
	}
	if internalName != "host" {
		t.Fatalf("expected internal name 'host', got %q", internalName)
	}
}

func TestTagKeyMappingStore_RenameToActiveNameErrors(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateTagKeyMapping("cpu", "host"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTagKeyMapping("cpu", "region"); err != nil {
		t.Fatal(err)
	}

	// Rename host -> region should fail (region is already active).
	if err := store.RenameTagKey("cpu", "host", "region"); err == nil {
		t.Fatal("expected error when renaming to an active name, got nil")
	}
}

func TestTagKeyMappingStore_GetUserTagKeyNames(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateTagKeyMapping("cpu", "host"); err != nil {
		t.Fatal(err)
	}

	// Rename host -> hostname; internal name remains "host".
	if err := store.RenameTagKey("cpu", "host", "hostname"); err != nil {
		t.Fatal(err)
	}

	// GetUserTagKeyNames: internal "host" -> user "hostname"; unmapped -> fallback.
	userNames := store.GetUserTagKeyNames("cpu", []string{"host", "region"})
	expected := []string{"hostname", "region"}
	if !reflect.DeepEqual(userNames, expected) {
		t.Fatalf("expected %v, got %v", expected, userNames)
	}
}

func TestTagKeyMappingStore_ChainRename(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateTagKeyMapping("cpu", "host"); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameTagKey("cpu", "host", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameTagKey("cpu", "h1", "h2"); err != nil {
		t.Fatal(err)
	}

	// h2 -> internal "host"
	internalName, ok := store.GetInternalTagKeyName("cpu", "h2")
	if !ok || internalName != "host" {
		t.Fatalf("expected h2 -> host (internal), got (%q, %v)", internalName, ok)
	}

	// h1 should be gone.
	if _, ok := store.GetInternalTagKeyName("cpu", "h1"); ok {
		t.Fatal("expected h1 to be inactive")
	}
}

func TestTagKeyMappingStore_GetAllMappings(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateTagKeyMapping("cpu", "host"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTagKeyMapping("cpu", "region"); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameTagKey("cpu", "host", "hostname"); err != nil {
		t.Fatal(err)
	}

	mappings := store.GetAllMappings("cpu")
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].UserName < mappings[j].UserName
	})

	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}
	if mappings[0].UserName != "hostname" || mappings[0].InternalName != "host" {
		t.Errorf("unexpected mapping[0]: %+v", mappings[0])
	}
	if mappings[1].UserName != "region" || mappings[1].InternalName != "region" {
		t.Errorf("unexpected mapping[1]: %+v", mappings[1])
	}
}

func TestTagKeyMappingStore_SaveLoadRoundTrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateTagKeyMapping("cpu", "host"); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameTagKey("cpu", "host", "hostname"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTagKeyMapping("cpu", "region"); err != nil {
		t.Fatal(err)
	}

	// Reload from disk.
	store2, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// hostname -> host (internal)
	name, ok := store2.GetInternalTagKeyName("cpu", "hostname")
	if !ok || name != "host" {
		t.Fatalf("after reload: expected hostname->host, got (%q, %v)", name, ok)
	}

	// region -> region (identity)
	name, ok = store2.GetInternalTagKeyName("cpu", "region")
	if !ok || name != "region" {
		t.Fatalf("after reload: expected region->region, got (%q, %v)", name, ok)
	}

	// host user name should be inactive.
	if _, ok := store2.GetInternalTagKeyName("cpu", "host"); ok {
		t.Fatal("after reload: expected 'host' user name to be inactive")
	}
}

func TestTagKeyMappingStore_DeferredSave(t *testing.T) {
	dir, err := os.MkdirTemp("", "tagkeymapping_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tag_key_mappings.json")
	store, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Deferred — no disk write yet.
	internal, err := store.CreateTagKeyMappingDeferred("mem", "server")
	if err != nil {
		t.Fatal(err)
	}
	if internal != "server" {
		t.Fatalf("expected 'server', got %q", internal)
	}

	// Reload before SaveIfDirty: should not see the entry.
	store2, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store2.GetInternalTagKeyName("mem", "server"); ok {
		t.Fatal("expected 'server' to not be persisted before SaveIfDirty")
	}

	// Now flush.
	if err := store.SaveIfDirty(); err != nil {
		t.Fatal(err)
	}

	// Reload after SaveIfDirty: should see the entry.
	store3, err := tsdb.NewTagKeyMappingStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := store3.GetInternalTagKeyName("mem", "server"); !ok || name != "server" {
		t.Fatalf("after SaveIfDirty: expected server->server, got (%q, %v)", name, ok)
	}
}
