package siptext

import "testing"

func TestParseRequestRoundTrip(t *testing.T) {
	req := NewRequest("INVITE", "sip:34020000001320000001@10.0.0.8:5060")
	req.SetHeader("Call-ID", "call-1")
	req.SetHeader("Content-Type", "application/sdp")
	req.Body = []byte("v=0\r\n")
	parsed, err := Parse(req.Bytes())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.IsRequest || parsed.Method != "INVITE" || parsed.Header("Call-ID") != "call-1" {
		t.Fatalf("parsed mismatch: %+v", parsed)
	}
}

func TestParseResponseRoundTrip(t *testing.T) {
	resp := NewResponse(nil, 200, "OK")
	resp.SetHeader("Call-ID", "call-2")
	parsed, err := Parse(resp.Bytes())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.IsRequest || parsed.StatusCode != 200 || parsed.Header("Call-ID") != "call-2" {
		t.Fatalf("parsed mismatch: %+v", parsed)
	}
}
