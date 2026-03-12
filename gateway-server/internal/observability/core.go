package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const (
	HeaderRequestID  = "X-Request-ID"
	HeaderTraceID    = "X-Trace-ID"
	HeaderSessionID  = "X-Session-ID"
	HeaderTransferID = "X-Transfer-ID"
	HeaderAPICode    = "X-Api-Code"
	HeaderSource     = "X-Source-System"
)

type CoreFields struct {
	TraceID      string `json:"trace_id"`
	RequestID    string `json:"request_id"`
	SessionID    string `json:"session_id"`
	TransferID   string `json:"transfer_id"`
	APICode      string `json:"api_code"`
	SourceSystem string `json:"source_system"`
	ResultCode   string `json:"result_code"`
}

func (f CoreFields) SlogAttrs() []any {
	return []any{
		"trace_id", f.TraceID,
		"request_id", f.RequestID,
		"session_id", f.SessionID,
		"transfer_id", f.TransferID,
		"api_code", f.APICode,
		"source_system", f.SourceSystem,
		"result_code", f.ResultCode,
	}
}

type ctxKey struct{}

func WithCoreFields(ctx context.Context, fields CoreFields) context.Context {
	return context.WithValue(ctx, ctxKey{}, fields)
}

func CoreFieldsFromContext(ctx context.Context) CoreFields {
	if v, ok := ctx.Value(ctxKey{}).(CoreFields); ok {
		return v
	}
	return CoreFields{}
}

func BuildCoreFieldsFromRequest(r *http.Request) CoreFields {
	return CoreFields{
		TraceID:      headerOrGenerate(r.Header.Get(HeaderTraceID)),
		RequestID:    headerOrGenerate(r.Header.Get(HeaderRequestID)),
		SessionID:    headerOrGenerate(r.Header.Get(HeaderSessionID)),
		TransferID:   headerOrGenerate(r.Header.Get(HeaderTransferID)),
		APICode:      headerOrDefault(r.Header.Get(HeaderAPICode), "unknown"),
		SourceSystem: headerOrDefault(r.Header.Get(HeaderSource), "unknown"),
		ResultCode:   "PROCESSING",
	}
}

func headerOrGenerate(v string) string {
	if v != "" {
		return v
	}
	return newID()
}

func headerOrDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(buf)
}
