package main

import (
	"fmt"
	"log"

	"siptunnel/internal/observability"
	"siptunnel/internal/router"
	"siptunnel/internal/security"
	"siptunnel/internal/service"
)

func main() {
	telemetry := observability.NewTelemetry()
	signer := security.NewHMACSigner("change-me")
	idStore := service.NewIdempotencyStore()
	limiter := service.NewRateLimiter(100, 200)
	r := router.NewTemplateRouter("http://127.0.0.1:8080", map[string]router.RouteTemplate{
		"PAYMENT_CREATE": {Method: "POST", Path: "/api/payment/create"},
		"ORDER_QUERY":    {Method: "POST", Path: "/api/order/query"},
	})

	sig, err := signer.Sign([]byte("boot"))
	if err != nil {
		log.Fatal(err)
	}

	telemetry.Audit(nil, "gateway_boot", "sign_alg", signer.Algorithm())
	fmt.Printf("gateway ready: sig=%s idempotency=%v limiter=%v router=%v\n", sig[:8], idStore.MarkOnce("boot"), limiter.Allow(), r != nil)
}
