package tunnelmapping

import "testing"

func TestMappingFromLegacyOpsRoute(t *testing.T) {
	mapping, err := MappingFromLegacyOpsRoute(LegacyOpsRoute{
		APICode:    "asset.sync",
		HTTPMethod: "post",
		HTTPPath:   "/sync",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("convert legacy ops route failed: %v", err)
	}
	if mapping.MappingID != "asset.sync" || mapping.LocalBasePath != "/sync" {
		t.Fatalf("unexpected mapping basic fields: %+v", mapping)
	}
	if got := mapping.AllowedMethods[0]; got != "POST" {
		t.Fatalf("expected upper-case method, got %s", got)
	}
	if mapping.Description == "" {
		t.Fatalf("expected migration description")
	}
}

func TestMappingFromLegacyRouteConfig(t *testing.T) {
	mapping, err := MappingFromLegacyRouteConfig(LegacyRouteConfig{
		APICode:       "api.health.ping",
		TargetService: "peer-b",
		TargetHost:    "10.10.1.12",
		TargetPort:    19001,
		HTTPMethod:    "post",
		HTTPPath:      "/v1/ping",
		TimeoutMS:     1200,
	})
	if err != nil {
		t.Fatalf("convert legacy route config failed: %v", err)
	}
	if mapping.PeerNodeID != "peer-b" || mapping.RemoteTargetIP != "10.10.1.12" || mapping.RemoteTargetPort != 19001 {
		t.Fatalf("unexpected target mapping fields: %+v", mapping)
	}
	if mapping.RequestTimeoutMS != 1200 || mapping.ResponseTimeoutMS != 1200 {
		t.Fatalf("expected timeout mapping, got %+v", mapping)
	}
}
