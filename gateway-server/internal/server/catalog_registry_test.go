package server

import (
	"testing"

	"siptunnel/internal/protocol/manscdp"
	"siptunnel/internal/tunnelmapping"
)

func TestCatalogRegistrySyncMappings(t *testing.T) {
	reg := NewCatalogRegistry()
	reg.SyncMappings([]tunnelmapping.TunnelMapping{{MappingID: "map-1", DeviceID: "34020000001320000001", Name: "demo", Enabled: true, LocalBindPort: 18080, AllowedMethods: []string{"GET"}, ResponseMode: "auto", MaxInlineResponseBody: 4096, MaxRequestBodyBytes: 2048}})
	deviceID, ok := reg.ResolveDeviceID(18080)
	if !ok || deviceID != "34020000001320000001" {
		t.Fatalf("resolve device id: %v %q", ok, deviceID)
	}
	item, ok := reg.Resource("34020000001320000001")
	if !ok || item.ResponseMode != "AUTO" {
		t.Fatalf("resource mismatch: %+v", item)
	}
}

func TestCatalogRegistrySyncRemoteCatalogPreservesExposurePorts(t *testing.T) {
	reg := NewCatalogRegistry()
	exposure := []tunnelmapping.TunnelMapping{{MappingID: "map-1", DeviceID: "34020000001320000001", LocalBindPort: 18080, MaxRequestBodyBytes: 2048}}
	reg.SyncRemoteCatalog([]manscdp.CatalogDevice{{DeviceID: "34020000001320000001", Name: "remote-demo", MethodList: "GET,POST", ResponseMode: "RTP", MaxRequestBody: 4096}}, exposure)
	deviceID, ok := reg.ResolveDeviceID(18080)
	if !ok || deviceID != "34020000001320000001" {
		t.Fatalf("resolve exposure port: ok=%v device=%q", ok, deviceID)
	}
	item, ok := reg.Resource("34020000001320000001")
	if !ok || item.Name != "remote-demo" || item.ResponseMode != "RTP" {
		t.Fatalf("remote resource mismatch: %+v", item)
	}
}

func TestCatalogRegistrySyncRemoteCatalogPreservesExistingAutoPorts(t *testing.T) {
	reg := NewCatalogRegistry()
	reg.SyncExposureMappings([]tunnelmapping.TunnelMapping{{MappingID: "catalog.34020000001320000001", DeviceID: "34020000001320000001", LocalBindPort: 18123}})
	reg.SyncRemoteCatalog([]manscdp.CatalogDevice{{DeviceID: "34020000001320000001", Name: "remote-demo"}}, nil)
	deviceID, ok := reg.ResolveDeviceID(18123)
	if !ok || deviceID != "34020000001320000001" {
		t.Fatalf("preserved auto port mismatch: ok=%v device=%q", ok, deviceID)
	}
}
