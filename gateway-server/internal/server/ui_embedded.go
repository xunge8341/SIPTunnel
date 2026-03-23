package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed embedded-ui/* embedded-ui/assets/* embedded-ui/errors/* embedded-ui/.siptunnel-ui-embed.json
var embeddedUIFS embed.FS

type EmbeddedUIOptions struct {
	BasePath string
}

type EmbeddedUIDeliveryMetadata struct {
	MetadataPresent         bool   `json:"metadata_present"`
	ConsistencyStatus       string `json:"consistency_status"`
	ConsistencyDetail       string `json:"consistency_detail"`
	BuildNonce              string `json:"build_nonce"`
	EmbeddedAt              string `json:"embedded_at"`
	UISourceLatestWrite     string `json:"ui_source_latest_write"`
	EmbeddedHashSHA256      string `json:"embedded_hash_sha256"`
	AssetBaseMode           string `json:"asset_base_mode"`
	RouterBasePathPolicy    string `json:"router_base_path_policy"`
	DeliveryGuardStatus     string `json:"delivery_guard_status"`
	DeliveryGuardDetail     string `json:"delivery_guard_detail"`
	DeliveryGuardRemoved    int    `json:"delivery_guard_removed_count"`
	DeliveryGuardRemaining  int    `json:"delivery_guard_remaining_count"`
	DeliveryGuardActiveHits int    `json:"delivery_guard_hit_count"`
}

type embeddedUIBuildMetadata struct {
	BuildNonce             string `json:"build_nonce"`
	EmbeddedAtLocal        string `json:"embedded_at_local"`
	UISourceLatestWrite    string `json:"ui_source_latest_write_local"`
	EmbeddedHashSHA256     string `json:"embedded_hash_sha256"`
	DeliveryGuardStatus    string `json:"delivery_guard_status"`
	DeliveryGuardDetail    string `json:"delivery_guard_detail"`
	DeliveryGuardRemoved   int    `json:"delivery_guard_removed_count"`
	DeliveryGuardRemaining int    `json:"delivery_guard_remaining_count"`
	DeliveryGuardHitCount  int    `json:"delivery_guard_hit_count"`
}

var embeddedUIReservedPaths = map[string]struct{}{
	"/healthz":      {},
	"/readyz":       {},
	"/metrics":      {},
	"/audit/events": {},
}

func isReservedAPIBypassPath(requestPath string, basePath string) bool {
	trimmed := strings.TrimSpace(requestPath)
	if trimmed == "" {
		return false
	}
	reserved := embeddedUIReservedPaths
	if _, ok := reserved[trimmed]; ok {
		return true
	}
	if basePath != "/" && strings.HasPrefix(trimmed, basePath+"/") {
		trimmed = strings.TrimPrefix(trimmed, basePath)
		if trimmed == "" {
			trimmed = "/"
		}
		_, ok := reserved[trimmed]
		return ok
	}
	return false
}

func NewEmbeddedUIFallbackHandler(apiHandler http.Handler, opts EmbeddedUIOptions) (http.Handler, error) {
	staticFS, err := fs.Sub(embeddedUIFS, "embedded-ui")
	if err != nil {
		return nil, err
	}
	basePath := normalizeUIBasePath(opts.BasePath)
	staticHandler := http.FileServer(http.FS(staticFS))
	if basePath != "/" {
		staticHandler = http.StripPrefix(basePath, staticHandler)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			apiHandler.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, basePath) {
			apiHandler.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || isReservedAPIBypassPath(r.URL.Path, basePath) {
			apiHandler.ServeHTTP(w, r)
			return
		}

		target := strings.TrimPrefix(r.URL.Path, basePath)
		target = strings.TrimPrefix(target, "/")
		if target == "" {
			target = "index.html"
		}
		target = path.Clean(target)
		if target == "." || target == "/" {
			target = "index.html"
		}
		if strings.HasPrefix(target, "../") {
			apiHandler.ServeHTTP(w, r)
			return
		}

		if _, statErr := fs.Stat(staticFS, target); statErr == nil {
			if serveEmbeddedFile(w, r, staticFS, target, basePath) {
				return
			}
			staticHandler.ServeHTTP(w, r)
			return
		}
		if strings.Contains(path.Base(target), ".") {
			serveFriendlyErrorPage(w, r, staticFS, http.StatusNotFound)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = path.Join(basePath, "/index.html")
		if serveEmbeddedFile(w, r2, staticFS, "index.html", basePath) {
			return
		}
		serveFriendlyErrorPage(w, r2, staticFS, http.StatusInternalServerError)
	}), nil
}

func serveEmbeddedFile(w http.ResponseWriter, r *http.Request, staticFS fs.FS, name string, basePath string) bool {
	f, err := staticFS.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return false
	}
	setEmbeddedUICacheHeader(w, name)
	if ext := path.Ext(name); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
	}
	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data = injectEmbeddedUIBasePath(data, basePath)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = io.Copy(w, bytes.NewReader(data))
	return true
}

func injectEmbeddedUIBasePath(data []byte, basePath string) []byte {
	if len(data) == 0 {
		return data
	}
	normalized := normalizeUIBasePath(basePath)
	if normalized != "/" {
		normalized += "/"
	}
	return bytes.ReplaceAll(data, []byte("__SIPTUNNEL_UI_BASE_PATH__"), []byte(normalized))
}

func friendlyErrorHTML(status int) string {
	title := "404 Not Found"
	message := "页面未找到 / Requested resource was not found."
	if status == http.StatusInternalServerError {
		title = "500 Internal Server Error"
		message = "页面加载失败，请稍后重试 / The page could not be loaded."
	}
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>%s</title></head><body><h1>%s</h1><p>%s</p></body></html>`, title, title, message)
}

func serveFriendlyErrorPage(w http.ResponseWriter, r *http.Request, staticFS fs.FS, status int) {
	_ = staticFS
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, friendlyErrorHTML(status))
}

func setEmbeddedUICacheHeader(w http.ResponseWriter, name string) {
	if name == "index.html" || strings.HasPrefix(name, "errors/") {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	if strings.HasPrefix(name, "assets/") || name == "favicon.ico" || name == "favicon.svg" {
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
}

func normalizeUIBasePath(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "/"
	}
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	v = path.Clean(v)
	if v != "/" {
		v = strings.TrimSuffix(v, "/")
	}
	return v
}

func ReadEmbeddedUIDeliveryMetadata() EmbeddedUIDeliveryMetadata {
	meta := EmbeddedUIDeliveryMetadata{
		ConsistencyStatus:    "degraded",
		ConsistencyDetail:    "embedded ui metadata unavailable",
		RouterBasePathPolicy: "meta_base_path_or_relative_history",
	}
	indexBytes, indexErr := embeddedUIFS.ReadFile("embedded-ui/index.html")
	indexText := string(indexBytes)
	hasBaseMeta := strings.Contains(indexText, `meta name="siptunnel-ui-base-path"`)
	hasRelativeAssets := strings.Contains(indexText, `"./assets/`) || strings.Contains(indexText, `'./assets/`)
	switch {
	case hasBaseMeta && hasRelativeAssets:
		meta.AssetBaseMode = "relative_assets+basepath_meta"
	case hasBaseMeta:
		meta.AssetBaseMode = "basepath_meta_only"
	case hasRelativeAssets:
		meta.AssetBaseMode = "relative_assets_only"
	default:
		meta.AssetBaseMode = "legacy_or_unknown"
	}
	if indexErr != nil {
		meta.ConsistencyDetail = "embedded ui index missing"
		return meta
	}
	payload, err := embeddedUIFS.ReadFile("embedded-ui/.siptunnel-ui-embed.json")
	if err != nil {
		if hasBaseMeta || hasRelativeAssets {
			meta.ConsistencyDetail = "embedded ui metadata missing but index base-path signals exist"
		}
		return meta
	}
	var build embeddedUIBuildMetadata
	if err := json.Unmarshal(payload, &build); err != nil {
		meta.ConsistencyDetail = "embedded ui metadata unreadable"
		return meta
	}
	meta.MetadataPresent = true
	meta.BuildNonce = strings.TrimSpace(build.BuildNonce)
	meta.EmbeddedAt = strings.TrimSpace(build.EmbeddedAtLocal)
	meta.UISourceLatestWrite = strings.TrimSpace(build.UISourceLatestWrite)
	meta.EmbeddedHashSHA256 = strings.TrimSpace(build.EmbeddedHashSHA256)
	meta.DeliveryGuardStatus = strings.TrimSpace(build.DeliveryGuardStatus)
	meta.DeliveryGuardDetail = strings.TrimSpace(build.DeliveryGuardDetail)
	meta.DeliveryGuardRemoved = build.DeliveryGuardRemoved
	meta.DeliveryGuardRemaining = build.DeliveryGuardRemaining
	meta.DeliveryGuardActiveHits = build.DeliveryGuardHitCount
	if meta.BuildNonce != "" && meta.EmbeddedHashSHA256 != "" && hasBaseMeta && hasRelativeAssets && meta.DeliveryGuardRemaining == 0 && meta.DeliveryGuardActiveHits == 0 {
		meta.ConsistencyStatus = "aligned"
		meta.ConsistencyDetail = "source, build bundle, embedded ui metadata, and delivery guard are aligned"
		return meta
	}
	meta.ConsistencyDetail = "embedded ui metadata present but base-path delivery or source guard signals are incomplete"
	return meta
}
