package server

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/protocol/manscdp"
	"siptunnel/internal/protocol/siptext"
	"siptunnel/internal/tunnelmapping"
)

func testGBConfig() TunnelConfigPayload {
	return defaultTunnelConfigPayload(config.DefaultNetworkMode())
}

func TestGB28181HandleNotifySyncsCatalogRegistry(t *testing.T) {
	reg := NewCatalogRegistry()
	exposure := []tunnelmapping.TunnelMapping{{
		MappingID:             "relay-demo",
		DeviceID:              "34020000001320000001",
		LocalBindPort:         18080,
		MaxRequestBodyBytes:   2048,
		AllowedMethods:        []string{"GET"},
		ResponseMode:          "AUTO",
		MaxInlineResponseBody: 4096,
	}}
	svc := NewGB28181TunnelService(func() nodeconfig.LocalNodeConfig {
		return nodeconfig.LocalNodeConfig{NodeID: "upper", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}
	}, func() []tunnelmapping.TunnelMapping {
		return exposure
	}, nil, func() TunnelConfigPayload { return testGBConfig() }, reg, nil, nil)
	body, err := manscdp.Marshal(manscdp.CatalogNotify{
		CmdType:  "Catalog",
		SN:       1,
		DeviceID: "34020000002000000001",
		SumNum:   1,
		DeviceList: []manscdp.CatalogDevice{{
			DeviceID:              "34020000001320000001",
			Name:                  "demo-resource",
			Status:                "ON",
			MethodList:            "GET,POST",
			ResponseMode:          "RTP",
			MaxInlineResponseBody: 8192,
			MaxRequestBody:        4096,
		}},
	})
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	msg := siptext.NewRequest("NOTIFY", "sip:upper")
	msg.SetHeader("Event", "Catalog")
	msg.Body = body
	if _, err := svc.HandleSIP(context.Background(), "127.0.0.1:15060", msg); err != nil {
		t.Fatalf("handle notify: %v", err)
	}
	deviceID, ok := reg.ResolveDeviceID(18080)
	if !ok || deviceID != "34020000001320000001" {
		t.Fatalf("resolve device id failed: ok=%v device=%q", ok, deviceID)
	}
	resource, ok := reg.Resource("34020000001320000001")
	if !ok {
		t.Fatalf("resource not synced")
	}
	if resource.Name != "demo-resource" || resource.ResponseMode != "RTP" {
		t.Fatalf("unexpected resource: %+v", resource)
	}
	if len(resource.MethodList) != 2 || resource.MethodList[0] != "GET" || resource.MethodList[1] != "POST" {
		t.Fatalf("unexpected methods: %+v", resource.MethodList)
	}
}

func TestGB28181HandleNotifyTriggersCatalogChangeCallback(t *testing.T) {
	reg := NewCatalogRegistry()
	svc := NewGB28181TunnelService(func() nodeconfig.LocalNodeConfig {
		return nodeconfig.LocalNodeConfig{NodeID: "upper", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}
	}, nil, nil, func() TunnelConfigPayload { return testGBConfig() }, reg, nil, nil)
	changed := make(chan struct{}, 1)
	svc.SetCatalogChangeCallback(func() { changed <- struct{}{} })
	body, err := manscdp.Marshal(manscdp.CatalogNotify{CmdType: "Catalog", SN: 1, DeviceID: "34020000002000000001", SumNum: 1, DeviceList: []manscdp.CatalogDevice{{DeviceID: "34020000001320000001", Name: "demo-resource"}}})
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	msg := siptext.NewRequest("NOTIFY", "sip:upper")
	msg.SetHeader("Event", "Catalog")
	msg.Body = body
	if _, err := svc.HandleSIP(context.Background(), "127.0.0.1:15060", msg); err != nil {
		t.Fatalf("handle notify: %v", err)
	}
	select {
	case <-changed:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("catalog change callback not triggered")
	}
}

func TestGB28181HandleRegisterChallengesWhenAuthEnabled(t *testing.T) {
	cfg := testGBConfig()
	cfg.RegisterAuthEnabled = true
	cfg.RegisterAuthPassword = "secret"
	svc := NewGB28181TunnelService(func() nodeconfig.LocalNodeConfig {
		return nodeconfig.LocalNodeConfig{NodeID: "upper", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}
	}, nil, nil, func() TunnelConfigPayload { return cfg }, NewCatalogRegistry(), nil, nil)
	msg := siptext.NewRequest("REGISTER", "sip:upper")
	msg.SetHeader("Via", "SIP/2.0/TCP demo")
	msg.SetHeader("Contact", "<sip:34020000002000000001@127.0.0.1:15060>")
	msg.SetHeader("X-Device-ID", "34020000002000000001")
	raw, err := svc.HandleSIP(context.Background(), "127.0.0.1:15060", msg)
	if err != nil {
		t.Fatalf("handle register: %v", err)
	}
	resp, err := siptext.Parse(raw)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("unexpected status=%d", resp.StatusCode)
	}
	if got := resp.Header("WWW-Authenticate"); got == "" {
		t.Fatal("missing WWW-Authenticate header")
	}
}

func TestSIPDigestAuthorizationRoundTrip(t *testing.T) {
	cfg := testGBConfig()
	cfg.RegisterAuthEnabled = true
	cfg.RegisterAuthPassword = "secret"
	cfg.RegisterAuthRealm = "upper"
	local := nodeconfig.LocalNodeConfig{NodeID: "upper"}
	challenge := buildRegisterDigestChallenge(cfg, local)
	header := buildSIPDigestAuthorization("REGISTER", "sip:upper", "34020000002000000001", cfg.RegisterAuthPassword, challenge)
	if !verifySIPDigestAuthorization(header, "REGISTER", "sip:upper", "34020000002000000001", cfg.RegisterAuthPassword, challenge.Realm, local.NodeID) {
		t.Fatal("expected authorization to verify")
	}
}

func TestShouldForceInlineResponse(t *testing.T) {
	mapping := tunnelmapping.TunnelMapping{ResponseMode: "AUTO", MaxInlineResponseBody: 4096}
	if !shouldForceInlineResponse(http.StatusBadGateway, 74, "RTP", &gbInboundSession{transport: "UDP", remoteRTPIP: "127.0.0.1", remoteRTPPort: 20000}, mapping) {
		t.Fatal("expected small error response to downgrade to INLINE")
	}
	if shouldForceInlineResponse(http.StatusOK, 74, "RTP", &gbInboundSession{transport: "UDP", remoteRTPIP: "127.0.0.1", remoteRTPPort: 20000}, mapping) {
		t.Fatal("did not expect successful RTP response to downgrade")
	}
	if !shouldForceInlineResponse(http.StatusOK, 74, "RTP", &gbInboundSession{transport: "UDP", remoteRTPIP: "", remoteRTPPort: 0}, mapping) {
		t.Fatal("expected invalid rtp callback endpoint to force INLINE")
	}
	if shouldForceInlineResponse(http.StatusBadGateway, 8193, "RTP", &gbInboundSession{transport: "UDP", remoteRTPIP: "127.0.0.1", remoteRTPPort: 20000}, mapping) {
		t.Fatal("did not expect large error body to downgrade automatically")
	}
}

func TestGB28181HandleInviteFindsLocalResource(t *testing.T) {
	resources := []LocalResourceRecord{{
		ResourceCode: "34020000001320000001",
		Name:         "demo-resource",
		Enabled:      true,
		TargetURL:    "http://127.0.0.1:28080/api/demo",
		Methods:      []string{"GET"},
		ResponseMode: "AUTO",
	}}
	svc := NewGB28181TunnelService(func() nodeconfig.LocalNodeConfig {
		return nodeconfig.LocalNodeConfig{NodeID: "34020000002000000001", SIPListenIP: "127.0.0.1", SIPListenPort: 5060}
	}, nil, func() []LocalResourceRecord { return resources }, func() TunnelConfigPayload { return testGBConfig() }, NewCatalogRegistry(), nil, nil)
	invite := siptext.NewRequest("INVITE", "sip:34020000001320000001@127.0.0.1:6070")
	invite.SetHeader("To", "<sip:34020000001320000001@127.0.0.1:6070>")
	invite.SetHeader("From", "<sip:34020000002000000001@127.0.0.1:5060>;tag=abc")
	invite.SetHeader("Via", "SIP/2.0/UDP 127.0.0.1:5060;rport;branch=z9hG4bK-test")
	invite.SetHeader("Contact", "<sip:34020000002000000001@127.0.0.1:5060>")
	invite.SetHeader("Call-ID", "call-demo")
	invite.SetHeader("CSeq", "1 INVITE")
	invite.SetHeader("Subject", "34020000002000000001:0,34020000001320000001:0")
	invite.Body = manscdp.BuildRelaySDP("127.0.0.1", 20001, invite.Header("Subject"), "34020000001320000001", "recvonly")
	raw, err := svc.HandleSIP(context.Background(), "127.0.0.1:15060", invite)
	if err != nil {
		t.Fatalf("handle invite: %v", err)
	}
	resp, err := siptext.Parse(raw)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
}

func TestNormalizeResponseModeDefaultsToAuto(t *testing.T) {
	if got := normalizeResponseMode(""); got != "AUTO" {
		t.Fatalf("expected AUTO, got %s", got)
	}
	if got := normalizeResponseMode("inline"); got != "INLINE" {
		t.Fatalf("expected INLINE, got %s", got)
	}
}

func TestResponseModeDecisionForHeadersUsesInlineBudget(t *testing.T) {
	mapping := tunnelmapping.TunnelMapping{ResponseMode: "AUTO", MaxInlineResponseBody: 4096}
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: mustParseURL(t, "http://127.0.0.1/app")}
	resp := &http.Response{ContentLength: 128, Header: make(http.Header)}
	session := &gbInboundSession{transport: "UDP", remoteRTPIP: "127.0.0.1", remoteRTPPort: 20000}
	decision := responseModeDecisionForHeaders("AUTO", mapping, prepared, resp, session)
	if decision.Mode != "INLINE" {
		t.Fatalf("expected INLINE, got %+v", decision)
	}
}

func TestClassifyUDPRequestLane(t *testing.T) {
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: mustParseURL(t, "http://127.0.0.1/socket.io/?transport=polling")}
	lane, limit := classifyUDPRequestLane(prepared, tunnelmapping.TunnelMapping{ResponseMode: "AUTO"})
	if lane != "bulk" || limit <= 0 {
		t.Fatalf("unexpected lane=%s limit=%d", lane, limit)
	}
}

func TestResponseModeDecisionForHeadersPrefersRTPForBulkDownload(t *testing.T) {
	mapping := tunnelmapping.TunnelMapping{ResponseMode: "AUTO", MaxInlineResponseBody: 4096}
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: mustParseURL(t, "http://127.0.0.1/download/report.zip")}
	resp := &http.Response{ContentLength: 4097, Header: make(http.Header)}
	resp.Header.Set("Content-Type", "application/octet-stream")
	session := &gbInboundSession{transport: "UDP", remoteRTPIP: "127.0.0.1", remoteRTPPort: 20000}
	decision := responseModeDecisionForHeaders("AUTO", mapping, prepared, resp, session)
	if decision.Mode != "RTP" || decision.ResponseShape != string(responseShapeBulkDownload) {
		t.Fatalf("expected bulk RTP decision, got %+v", decision)
	}
}

func TestDynamicRelayBodyWaitUsesRangeBudgetForRangeRequest(t *testing.T) {
	ApplyTransportTuning(config.DefaultTransportTuningConfig())
	prepared := &mappingForwardRequest{
		Method:                http.MethodGet,
		TargetURL:             mustParseURL(t, "http://127.0.0.1/video.mp4"),
		Headers:               http.Header{"Range": []string{"bytes=0-1048575"}},
		ResponseHeaderTimeout: 500 * time.Millisecond,
		RequestTimeout:        500 * time.Millisecond,
	}
	wait := dynamicRelayBodyWait(prepared, manscdp.DeviceControl{ResponseMode: "RTP", ContentLength: 1024})
	if wait < boundaryRangeResponseStartWait() {
		t.Fatalf("dynamicRelayBodyWait=%v, want >= %v", wait, boundaryRangeResponseStartWait())
	}
}

func TestHasRTPDestination(t *testing.T) {
	if hasRTPDestination(nil) {
		t.Fatal("nil session should not have RTP destination")
	}
	if hasRTPDestination(&gbInboundSession{remoteRTPIP: "127.0.0.1", remoteRTPPort: 0}) {
		t.Fatal("zero RTP port should be treated as unavailable")
	}
	if !hasRTPDestination(&gbInboundSession{remoteRTPIP: "127.0.0.1", remoteRTPPort: 20000}) {
		t.Fatal("valid RTP endpoint should be available")
	}
}

func TestClassifyResponseShapeTreatsSmallJSONAsSmallPageData(t *testing.T) {
	prepared := &mappingForwardRequest{Method: http.MethodGet, TargetURL: mustParseURL(t, "http://127.0.0.1/api/summary")}
	resp := &http.Response{ContentLength: 128, Header: make(http.Header)}
	resp.Header.Set("Content-Type", "application/json")
	if got := classifyResponseShape(prepared, resp, resp.ContentLength, 360); got != responseShapeTinyControl {
		t.Fatalf("classifyResponseShape=%s, want %s", got, responseShapeTinyControl)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}
