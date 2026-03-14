package file

import (
	"path/filepath"
	"testing"

	"siptunnel/internal/tunnelmapping"
)

func sampleMapping(id string) tunnelmapping.TunnelMapping {
	return tunnelmapping.TunnelMapping{
		MappingID:            id,
		Name:                 "core-api",
		Enabled:              true,
		PeerNodeID:           "peer-b",
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        18080,
		LocalBasePath:        "/proxy/core",
		RemoteTargetIP:       "10.10.10.11",
		RemoteTargetPort:     8080,
		RemoteBasePath:       "/v1/core",
		AllowedMethods:       []string{"GET", "POST"},
		ConnectTimeoutMS:     500,
		RequestTimeoutMS:     1000,
		ResponseTimeoutMS:    1200,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 4096,
		Description:          "desc",
	}
}

func TestTunnelMappingStoreCRUDAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnel_mappings.json")
	store, err := NewTunnelMappingStore(path)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	created, err := store.Create(sampleMapping("m1"))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if created.MappingID != "m1" {
		t.Fatalf("unexpected mapping id: %s", created.MappingID)
	}
	if _, err := store.Create(sampleMapping("m1")); err != ErrMappingExists {
		t.Fatalf("expected ErrMappingExists, got %v", err)
	}
	updatedInput := sampleMapping("m1")
	updatedInput.Name = "changed"
	updated, err := store.Update("m1", updatedInput)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "changed" {
		t.Fatalf("update did not apply")
	}
	if err := store.Delete("m1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if err := store.Delete("m1"); err != ErrMappingNotFound {
		t.Fatalf("expected not found on second delete, got %v", err)
	}

	_, _ = store.Create(sampleMapping("m2"))
	reloaded, err := NewTunnelMappingStore(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	items := reloaded.List()
	if len(items) != 1 || items[0].MappingID != "m2" {
		t.Fatalf("unexpected items after reload: %+v", items)
	}
}
