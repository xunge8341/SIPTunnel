package server

import (
	"testing"

	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/protocol/manscdp"
)

func TestBuildCatalogNotifyChunksUDPStaysUnderLimit(t *testing.T) {
	svc := NewGB28181TunnelService(func() nodeconfig.LocalNodeConfig {
		return nodeconfig.LocalNodeConfig{NodeID: "34020000002000000001", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}
	}, nil, nil, func() TunnelConfigPayload { return testGBConfig() }, NewCatalogRegistry(), nil, nil)
	state := sipDialogState{
		callID:         "catalog-call",
		remoteURI:      "sip:34020000001320000001@127.0.0.1:15060",
		localURI:       "sip:34020000002000000001@127.0.0.1:5060",
		contactURI:     "sip:34020000002000000001@127.0.0.1:5060",
		localTag:       "ltag",
		remoteTag:      "rtag",
		remoteTarget:   "127.0.0.1:15060",
		transport:      "UDP",
		subscriptionID: "sub-1",
		nextLocalCSeq:  1,
	}
	devices := []manscdp.CatalogDevice{
		{DeviceID: "31010900002000000003", Name: "web", Status: "ON", MethodList: "GET,POST", ResponseMode: "RTP", MaxInlineResponseBody: 65536, MaxRequestBody: 524288},
		{DeviceID: "31010900002000000004", Name: "web2", Status: "ON", MethodList: "GET,POST", ResponseMode: "RTP", MaxInlineResponseBody: 65536, MaxRequestBody: 524288},
		{DeviceID: "31010900002000000005", Name: "video", Status: "ON", MethodList: "GET,POST", ResponseMode: "RTP", MaxInlineResponseBody: 65536, MaxRequestBody: 524288},
	}
	chunks := svc.buildCatalogNotifyChunks(state, nodeconfig.LocalNodeConfig{NodeID: "34020000002000000001", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}, devices)
	if len(chunks) < 2 {
		t.Fatalf("expected chunking for udp catalog notify, got %d chunk(s)", len(chunks))
	}
	totalDevices := 0
	for i, chunk := range chunks {
		if len(chunk.devices) == 0 {
			t.Fatalf("chunk %d has no devices", i)
		}
		_, sipBytes, err := svc.catalogNotifyBodyAndSize(state, nodeconfig.LocalNodeConfig{NodeID: "34020000002000000001", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}, chunk.devices, len(devices))
		if err != nil {
			t.Fatalf("chunk %d size calc error: %v", i, err)
		}
		if sipBytes > udpCatalogMaxBytes() {
			t.Fatalf("chunk %d sip_bytes=%d > limit=%d", i, sipBytes, udpCatalogMaxBytes())
		}
		totalDevices += len(chunk.devices)
	}
	if totalDevices != len(devices) {
		t.Fatalf("total devices in chunks=%d want=%d", totalDevices, len(devices))
	}
}

func TestMergeCatalogNotifyFragmentAggregatesUntilComplete(t *testing.T) {
	svc := NewGB28181TunnelService(func() nodeconfig.LocalNodeConfig {
		return nodeconfig.LocalNodeConfig{NodeID: "upper", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}
	}, nil, nil, func() TunnelConfigPayload { return testGBConfig() }, NewCatalogRegistry(), nil, nil)

	if items, complete := svc.mergeCatalogNotifyFragment("peer-1", manscdp.CatalogNotify{DeviceID: "peer-1", SumNum: 3, DeviceList: []manscdp.CatalogDevice{{DeviceID: "A"}, {DeviceID: "B"}}}); complete || items != nil {
		t.Fatalf("expected incomplete fragment merge, got complete=%v items=%v", complete, items)
	}
	items, complete := svc.mergeCatalogNotifyFragment("peer-1", manscdp.CatalogNotify{DeviceID: "peer-1", SumNum: 3, DeviceList: []manscdp.CatalogDevice{{DeviceID: "C"}}})
	if !complete {
		t.Fatal("expected merge complete on second fragment")
	}
	if len(items) != 3 {
		t.Fatalf("merged items=%d want=3", len(items))
	}
	if items[0].DeviceID != "A" || items[1].DeviceID != "B" || items[2].DeviceID != "C" {
		t.Fatalf("unexpected merged order/items: %+v", items)
	}
}
