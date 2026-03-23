package server

import (
	"testing"

	"siptunnel/internal/tunnelmapping"
)

func TestBuildCatalogExposurePlanKeepsManualMappingsAuthoritative(t *testing.T) {
	static := []tunnelmapping.TunnelMapping{{
		MappingID:            "manual-1",
		DeviceID:             "34020000001320000001",
		Enabled:              true,
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        18080,
		LocalBasePath:        "/",
		RemoteTargetIP:       "127.0.0.1",
		RemoteTargetPort:     80,
		RemoteBasePath:       "/",
		AllowedMethods:       []string{"GET"},
		ConnectTimeoutMS:     1000,
		RequestTimeoutMS:     1000,
		ResponseTimeoutMS:    1000,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 2048,
	}}
	resources := []VirtualResource{{DeviceID: "34020000001320000001", Name: "manual", MethodList: []string{"GET"}}, {DeviceID: "34020000001320000002", Name: "unmapped", MethodList: []string{"POST"}}}
	plan := buildCatalogExposurePlan(static, resources)
	if len(plan.EffectiveMappings) != 1 {
		t.Fatalf("expected 1 effective mapping, got %d", len(plan.EffectiveMappings))
	}
	if len(plan.Views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(plan.Views))
	}
	if plan.Views[0].ExposureMode != "MANUAL" {
		t.Fatalf("expected first resource to stay manual, got %s", plan.Views[0].ExposureMode)
	}
	if plan.Views[1].ExposureMode != "UNEXPOSED" {
		t.Fatalf("expected second resource to stay unexposed, got %s", plan.Views[1].ExposureMode)
	}
}

func TestBuildCatalogExposurePlanDoesNotSynthesizeAutoMappings(t *testing.T) {
	resources := []VirtualResource{{DeviceID: "34020000001320000002", Name: "remote-only", MethodList: []string{"GET"}}}
	plan := buildCatalogExposurePlan(nil, resources)
	if len(plan.EffectiveMappings) != 0 {
		t.Fatalf("expected no synthesized effective mappings, got %d", len(plan.EffectiveMappings))
	}
	if len(plan.Views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(plan.Views))
	}
	if plan.Views[0].ExposureMode != "UNEXPOSED" {
		t.Fatalf("expected unexposed view, got %s", plan.Views[0].ExposureMode)
	}
}
