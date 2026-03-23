package manscdp

import "testing"

func TestMarshalCatalogAndDetectCmdType(t *testing.T) {
	body, err := Marshal(CatalogNotify{CmdType: "Catalog", SN: 1, DeviceID: "34020000002000000001", SumNum: 1, DeviceList: []CatalogDevice{{DeviceID: "34020000001320000001", Name: "demo"}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if DetectCmdType(body) != "Catalog" {
		t.Fatalf("cmd type mismatch: %q", DetectCmdType(body))
	}
}
