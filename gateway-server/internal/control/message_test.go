package control

import "testing"

func TestEncodeEnvelopeHeadersMirrorIndexFields(t *testing.T) {
	msg := SIPBusinessMessage{TraceID: "t1", RequestID: "r1", ApiCode: "A", Payload: map[string]any{"k": "v"}}
	env, err := EncodeEnvelope(msg)
	if err != nil {
		t.Fatalf("EncodeEnvelope() error = %v", err)
	}
	if env.Headers["X-Trace-ID"] != "t1" || env.Headers["X-Request-ID"] != "r1" || env.Headers["X-Api-Code"] != "A" {
		t.Fatalf("headers not mirrored: %#v", env.Headers)
	}
	decoded, err := DecodeEnvelope(env)
	if err != nil {
		t.Fatalf("DecodeEnvelope() error = %v", err)
	}
	if decoded.Payload["k"] != "v" {
		t.Fatalf("unexpected payload %#v", decoded.Payload)
	}
}
