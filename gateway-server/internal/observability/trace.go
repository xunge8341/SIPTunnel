package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var textMapPropagator = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

func ExtractTraceContext(r *http.Request) context.Context {
	ctx := textMapPropagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	if fields := CoreFieldsFromContext(ctx); fields.TraceID == "" {
		fields = BuildCoreFieldsFromRequest(r)
		if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
			fields.TraceID = spanCtx.TraceID().String()
		}
		ctx = WithCoreFields(ctx, fields)
	}
	return ctx
}

func InjectTraceContext(ctx context.Context, header http.Header) {
	textMapPropagator.Inject(ctx, propagation.HeaderCarrier(header))
}

func StartSpan(ctx context.Context, spanName string) (context.Context, trace.Span) {
	tracer := otel.Tracer("siptunnel/gateway-server")
	return tracer.Start(ctx, spanName)
}
