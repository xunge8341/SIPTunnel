package httpinvoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ResultCodeOK                 = "OK"
	ResultCodeUpstreamTimeout    = "UPSTREAM_TIMEOUT"
	ResultCodeUpstreamRateLimit  = "UPSTREAM_RATE_LIMIT"
	ResultCodeUpstreamServerErr  = "UPSTREAM_SERVER_ERROR"
	ResultCodeUpstreamClientErr  = "UPSTREAM_CLIENT_ERROR"
	ResultCodeUpstreamUnexpected = "UPSTREAM_UNEXPECTED"
)

var ErrRouteNotAllowed = errors.New("api_code not allowed by route whitelist")

type RouteConfig struct {
	APICode       string            `yaml:"api_code" json:"api_code"`
	TargetService string            `yaml:"target_service" json:"target_service"`
	TargetHost    string            `yaml:"target_host" json:"target_host"`
	TargetPort    int               `yaml:"target_port" json:"target_port"`
	HTTPMethod    string            `yaml:"http_method" json:"http_method"`
	HTTPPath      string            `yaml:"http_path" json:"http_path"`
	ContentType   string            `yaml:"content_type" json:"content_type"`
	TimeoutMS     int               `yaml:"timeout_ms" json:"timeout_ms"`
	RetryTimes    int               `yaml:"retry_times" json:"retry_times"`
	HeaderMapping map[string]string `yaml:"header_mapping" json:"header_mapping"`
	BodyMapping   map[string]string `yaml:"body_mapping" json:"body_mapping"`
}

type Config struct {
	Routes []RouteConfig `yaml:"routes" json:"routes"`
}

func LoadConfig(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read httpinvoke config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal httpinvoke config: %w", err)
	}
	return cfg, nil
}

type RequestContext struct {
	RequestID     string
	TraceID       string
	SessionID     string
	TransferID    string
	SourceSystem  string
	IdempotentKey string
}

type InvokeInput struct {
	APICode string
	Context RequestContext
	Params  map[string]any
}

type InvokeResult struct {
	StatusCode int
	ResultCode string
	Body       []byte
}

type Invoker struct {
	routes map[string]RouteConfig
	client *http.Client
}

func NewInvoker(cfg Config, client *http.Client) (*Invoker, error) {
	if client == nil {
		client = &http.Client{}
	}
	routes := make(map[string]RouteConfig, len(cfg.Routes))
	for _, route := range cfg.Routes {
		if route.APICode == "" {
			return nil, fmt.Errorf("route api_code is required")
		}
		if _, exists := routes[route.APICode]; exists {
			return nil, fmt.Errorf("duplicate api_code route: %s", route.APICode)
		}
		routes[route.APICode] = route
	}
	return &Invoker{routes: routes, client: client}, nil
}

func (i *Invoker) Invoke(ctx context.Context, in InvokeInput) (InvokeResult, error) {
	route, ok := i.routes[in.APICode]
	if !ok {
		return InvokeResult{}, fmt.Errorf("%w: %s", ErrRouteNotAllowed, in.APICode)
	}

	bodyPayload := mapByTemplate(route.BodyMapping, in.Params)
	bodyBytes, err := json.Marshal(bodyPayload)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("marshal request body: %w", err)
	}

	url := fmt.Sprintf("http://%s:%d%s", route.TargetHost, route.TargetPort, route.HTTPPath)
	attempts := route.RetryTimes + 1
	if attempts <= 0 {
		attempts = 1
	}
	var lastResult InvokeResult
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		attemptCtx := ctx
		if route.TimeoutMS > 0 {
			var cancel context.CancelFunc
			attemptCtx, cancel = context.WithTimeout(ctx, time.Duration(route.TimeoutMS)*time.Millisecond)
			defer cancel()
		}

		req, err := http.NewRequestWithContext(attemptCtx, strings.ToUpper(route.HTTPMethod), url, bytes.NewReader(bodyBytes))
		if err != nil {
			return InvokeResult{}, fmt.Errorf("build request: %w", err)
		}

		if route.ContentType != "" {
			req.Header.Set("Content-Type", route.ContentType)
		}
		injectUnifiedHeaders(req.Header, in)
		for k, v := range mapByTemplate(route.HeaderMapping, in.Params) {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}

		resp, err := i.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < attempts-1 {
				continue
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return InvokeResult{ResultCode: ResultCodeUpstreamTimeout}, nil
			}
			return InvokeResult{}, fmt.Errorf("invoke upstream: %w", err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return InvokeResult{}, fmt.Errorf("read response: %w", readErr)
		}

		lastResult = InvokeResult{StatusCode: resp.StatusCode, ResultCode: MapHTTPStatusToResultCode(resp.StatusCode), Body: respBody}
		if shouldRetry(resp.StatusCode) && attempt < attempts-1 {
			continue
		}
		return lastResult, nil
	}
	if lastErr != nil {
		return InvokeResult{}, fmt.Errorf("invoke upstream after retries: %w", lastErr)
	}
	return lastResult, nil
}

func injectUnifiedHeaders(header http.Header, in InvokeInput) {
	header.Set("X-Request-ID", in.Context.RequestID)
	header.Set("X-Trace-ID", in.Context.TraceID)
	header.Set("X-Session-ID", in.Context.SessionID)
	header.Set("X-Transfer-ID", in.Context.TransferID)
	header.Set("X-Api-Code", in.APICode)
	header.Set("X-Source-System", in.Context.SourceSystem)
	header.Set("X-Idempotent-Key", in.Context.IdempotentKey)
}

func mapByTemplate(mapping map[string]string, params map[string]any) map[string]any {
	result := make(map[string]any, len(mapping))
	for target, source := range mapping {
		value, ok := resolvePath(params, source)
		if !ok {
			continue
		}
		result[target] = value
	}
	return result
}

func resolvePath(input map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var current any = input
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func MapHTTPStatusToResultCode(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode <= 299:
		return ResultCodeOK
	case statusCode == http.StatusTooManyRequests:
		return ResultCodeUpstreamRateLimit
	case statusCode == http.StatusGatewayTimeout:
		return ResultCodeUpstreamTimeout
	case statusCode >= 500 && statusCode <= 599:
		return ResultCodeUpstreamServerErr
	case statusCode >= 400 && statusCode <= 499:
		return ResultCodeUpstreamClientErr
	default:
		return ResultCodeUpstreamUnexpected
	}
}

func shouldRetry(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusGatewayTimeout {
		return true
	}
	return statusCode >= 500 && statusCode <= 599
}
