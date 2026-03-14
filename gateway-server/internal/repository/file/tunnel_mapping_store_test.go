package file

import (
	"os"
	"path/filepath"
	"strings"
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
	if created.UpdatedAt == "" {
		t.Fatalf("expected updated_at to be set")
	}
	if _, err := store.Create(sampleMapping("m1")); err != ErrMappingExists {
		t.Fatalf("expected ErrMappingExists, got %v", err)
	}
	updatedInput := sampleMapping("m1")
	updatedInput.Name = "changed"
	updatedInput.AllowedMethods = nil
	updated, err := store.Update("m1", updatedInput)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "changed" {
		t.Fatalf("update did not apply")
	}
	if len(updated.AllowedMethods) != 1 || updated.AllowedMethods[0] != "*" {
		t.Fatalf("expected default methods after update, got %+v", updated.AllowedMethods)
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

func TestTunnelMappingStoreLoadLegacyOpsRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnel_mappings.json")
	legacy := `{"routes":[{"api_code":"asset.sync","http_method":"POST","http_path":"/sync","enabled":true}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy file failed: %v", err)
	}
	store, err := NewTunnelMappingStore(path)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	items := store.List()
	if len(items) != 1 || items[0].MappingID != "asset.sync" {
		t.Fatalf("unexpected migrated items: %+v", items)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file failed: %v", err)
	}
	if strings.Contains(string(persisted), "api_code") {
		t.Fatalf("expected rewritten tunnel mapping payload, got %s", persisted)
	}
}

func TestTunnelMappingStoreLoadLegacyRouteConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnel_mappings.json")
	legacy := `{"routes":[{"api_code":"api.health.ping","target_service":"peer-b","target_host":"10.10.1.12","target_port":19001,"http_method":"POST","http_path":"/v1/ping","timeout_ms":800}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy file failed: %v", err)
	}
	store, err := NewTunnelMappingStore(path)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	items := store.List()
	if len(items) != 1 {
		t.Fatalf("unexpected migrated items len: %+v", items)
	}
	item := items[0]
	if item.MappingID != "api.health.ping" || item.PeerNodeID != "peer-b" || item.RemoteTargetIP != "10.10.1.12" {
		t.Fatalf("unexpected migrated mapping: %+v", item)
	}
}

func TestTunnelMappingStoreCreateAutoIncrementIDAndCursorPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnel_mappings.json")
	store, err := NewTunnelMappingStore(path)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	m1 := sampleMapping("")
	created1, err := store.Create(m1)
	if err != nil {
		t.Fatalf("create1 failed: %v", err)
	}
	if created1.MappingID != "1" {
		t.Fatalf("expected first auto id=1, got %s", created1.MappingID)
	}
	m2 := sampleMapping("")
	created2, err := store.Create(m2)
	if err != nil {
		t.Fatalf("create2 failed: %v", err)
	}
	if created2.MappingID != "2" {
		t.Fatalf("expected second auto id=2, got %s", created2.MappingID)
	}
	reloaded, err := NewTunnelMappingStore(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	m3 := sampleMapping("")
	created3, err := reloaded.Create(m3)
	if err != nil {
		t.Fatalf("create3 failed: %v", err)
	}
	if created3.MappingID != "3" {
		t.Fatalf("expected third auto id=3 after reload, got %s", created3.MappingID)
	}
}
