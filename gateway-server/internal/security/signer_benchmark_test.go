package security

import "testing"

func BenchmarkSignerSign(b *testing.B) {
	signer := NewHMACSigner("benchmark-secret-key")
	payload := []byte(`{"api_code":"api.user.create","request_id":"req-bench-1","payload":"abcdefghijklmnopqrstuvwxyz0123456789"}`)

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := signer.Sign(payload); err != nil {
			b.Fatalf("sign payload: %v", err)
		}
	}
}

func BenchmarkSignerVerify(b *testing.B) {
	signer := NewHMACSigner("benchmark-secret-key")
	payload := []byte(`{"api_code":"api.user.create","request_id":"req-bench-1","payload":"abcdefghijklmnopqrstuvwxyz0123456789"}`)
	sig, err := signer.Sign(payload)
	if err != nil {
		b.Fatalf("prepare signature: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if ok := signer.Verify(payload, sig); !ok {
			b.Fatal("verify payload failed")
		}
	}
}
