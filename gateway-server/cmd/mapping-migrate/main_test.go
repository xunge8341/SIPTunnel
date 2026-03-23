package main

import "testing"

func TestParseLegacyMappings(t *testing.T) {
	in := []byte(`{"routes":[{"api_code":"asset.sync","http_method":"POST","http_path":"/sync","enabled":true}]}`)
	items, err := parseLegacyMappings(in)
	if err != nil {
		t.Fatalf("parse legacy mappings failed: %v", err)
	}
	if len(items) != 1 || items[0].MappingID != "asset.sync" {
		t.Fatalf("unexpected result: %+v", items)
	}
}
