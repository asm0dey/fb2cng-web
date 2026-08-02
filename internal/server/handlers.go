package server

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

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
