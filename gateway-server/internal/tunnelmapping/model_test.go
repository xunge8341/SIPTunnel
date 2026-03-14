package tunnelmapping

import "testing"

func validMapping() TunnelMapping {
	return TunnelMapping{
		MappingID:            "map-1",
		Name:                 "core",
		Enabled:              true,
		PeerNodeID:           "peer-b",
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        18080,
		LocalBasePath:        "/api/core",
		RemoteTargetIP:       "10.0.0.2",
		RemoteTargetPort:     8080,
		RemoteBasePath:       "/v1/core",
		AllowedMethods:       []string{"GET", "POST"},
		ConnectTimeoutMS:     300,
		RequestTimeoutMS:     1000,
		ResponseTimeoutMS:    1000,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 2048,
	}
}

func TestTunnelMappingValidate(t *testing.T) {
	if err := validMapping().Validate(); err != nil {
		t.Fatalf("valid mapping should pass: %v", err)
	}
	invalid := validMapping()
	invalid.MappingID = ""
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected mapping_id error")
	}
	invalid = validMapping()
	invalid.LocalBindIP = "bad-ip"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected local bind ip error")
	}
	invalid = validMapping()
	invalid.RemoteBasePath = "v1"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected remote base path error")
	}
	invalid = validMapping()
	invalid.AllowedMethods = nil
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected methods error")
	}
}
