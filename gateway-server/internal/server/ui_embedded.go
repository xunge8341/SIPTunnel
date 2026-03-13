package server

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed embedded-ui/* embedded-ui/assets/*
var embeddedUIFS embed.FS

type EmbeddedUIOptions struct {
	BasePath string
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
		if strings.HasPrefix(r.URL.Path, "/api/") {
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
			if serveEmbeddedFile(w, r, staticFS, target) {
				return
			}
			staticHandler.ServeHTTP(w, r)
			return
		}
		if strings.Contains(path.Base(target), ".") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = path.Join(basePath, "/index.html")
		if serveEmbeddedFile(w, r2, staticFS, "index.html") {
			return
		}
		staticHandler.ServeHTTP(w, r2)
	}), nil
}

func serveEmbeddedFile(w http.ResponseWriter, r *http.Request, staticFS fs.FS, name string) bool {
	f, err := staticFS.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return false
	}
	if ext := path.Ext(name); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
	}
	_, _ = w.Write(data)
	return true
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
