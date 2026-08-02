package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"enabled": s.cfg.ForwardAuth}
	if s.cfg.ForwardAuth {
		resp["user"] = r.Header.Get("Remote-User")
		resp["name"] = r.Header.Get("Remote-Name")
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// allowedFormats are the output types fbc supports.
var allowedFormats = map[string]bool{
	"epub2": true, "epub3": true, "kepub": true, "kfx": true, "azw8": true, "pdf": true,
}

func streamFile(w http.ResponseWriter, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	name := filepath.Base(path)
	w.Header().Set("Content-Type", contentTypeFor(name))
	w.Header().Set("Content-Disposition", contentDisposition(name))
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("stream %s: %v", path, err)
	}
}

func contentTypeFor(name string) string {
	if strings.HasSuffix(name, ".epub") {
		return "application/epub+zip"
	}
	if strings.HasSuffix(name, ".pdf") {
		return "application/pdf"
	}
	return "application/octet-stream"
}

// contentDisposition sets a safe attachment filename (RFC 5987 for UTF-8).
func contentDisposition(name string) string {
	return "attachment; filename*=UTF-8''" + url.PathEscape(name)
}
