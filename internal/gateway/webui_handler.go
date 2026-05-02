package gateway

import (
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

// WebUIHandler serves the web UI and handles SPA routing
type WebUIHandler struct {
	// webUI will be set if web UI is embedded
	webUI fs.FS
}

// NewWebUIHandler creates a new web UI handler
func NewWebUIHandler(webUI fs.FS) *WebUIHandler {
	return &WebUIHandler{
		webUI: webUI,
	}
}

// ServeHTTP implements http.Handler for SPA routing
func (h *WebUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.webUI == nil {
		http.Error(w, "Web UI not available", http.StatusNotFound)
		return
	}

	// Remove leading slash and normalize path
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	// For API routes, don't serve UI
	if strings.HasPrefix(path, "api/") {
		http.NotFound(w, r)
		return
	}

	// Try to open the requested file
	file, err := h.webUI.Open(path)
	if err == nil {
		defer file.Close()

		// Check if it's a directory
		stat, _ := file.Stat()
		if stat.IsDir() {
			// Try index.html in directory
			indexPath := filepath.Join(path, "index.html")
			indexFile, err := h.webUI.Open(indexPath)
			if err == nil {
				defer indexFile.Close()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
				io.Copy(w, indexFile)
				return
			}
		} else {
			// Serve the file
			contentType := getContentType(path)
			w.Header().Set("Content-Type", contentType)
			if strings.HasSuffix(path, ".html") {
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			}
			io.Copy(w, file)
			return
		}
	}

	// For any other path, serve index.html (SPA routing)
	indexFile, err := h.webUI.Open("index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer indexFile.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	io.Copy(w, indexFile)
}

// getContentType returns the MIME type for a file
func getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}


// InstallWebUIHandler installs the web UI handler to the mux
func InstallWebUIHandler(mux *http.ServeMux, webUI fs.FS) {
	handler := NewWebUIHandler(webUI)
	mux.Handle("/", handler)
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to root which serves the SPA
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
}
