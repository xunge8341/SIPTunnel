package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type RouteTemplate struct {
	Method string
	Path   string
}

type TemplateRouter struct {
	baseURL string
	routes  map[string]RouteTemplate
}

func NewTemplateRouter(baseURL string, routes map[string]RouteTemplate) *TemplateRouter {
	copied := make(map[string]RouteTemplate, len(routes))
	for k, v := range routes {
		copied[k] = v
	}
	return &TemplateRouter{baseURL: baseURL, routes: copied}
}

func (r *TemplateRouter) BuildRequest(apiCode string, body map[string]any) (*http.Request, error) {
	template, ok := r.routes[apiCode]
	if !ok {
		return nil, fmt.Errorf("unknown api_code: %s", apiCode)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal routed body: %w", err)
	}
	url := r.baseURL + template.Path
	req, err := http.NewRequest(template.Method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
