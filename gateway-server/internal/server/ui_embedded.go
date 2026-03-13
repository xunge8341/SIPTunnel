package server

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed embedded-ui/* embedded-ui/assets/* embedded-ui/errors/*
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
			serveFriendlyErrorPage(w, r, staticFS, http.StatusNotFound)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = path.Join(basePath, "/index.html")
		if serveEmbeddedFile(w, r2, staticFS, "index.html") {
			return
		}
		serveFriendlyErrorPage(w, r2, staticFS, http.StatusInternalServerError)
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
	setEmbeddedUICacheHeader(w, name)
	if ext := path.Ext(name); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
	}
	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = io.Copy(w, bytes.NewReader(data))
	return true
}

func serveFriendlyErrorPage(w http.ResponseWriter, r *http.Request, staticFS fs.FS, status int) {
	name := "errors/404.html"
	if status == http.StatusInternalServerError {
		name = "errors/500.html"
	}
	f, err := staticFS.Open(name)
	if err != nil {
		http.Error(w, http.StatusText(status), status)
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
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
