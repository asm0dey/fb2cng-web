package server

import (
	"archive/zip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fb2cng-web/internal/convert"
)

func (s *Server) handleDefaults(w http.ResponseWriter, r *http.Request) {
	out, err := s.runner.DumpDefaults(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Write(out)
}

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

func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(128 << 20); err != nil { // 128 MiB in memory/disk
		http.Error(w, "invalid upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	format := r.FormValue("format")
	if format == "" {
		format = "epub3"
	}
	if !allowedFormats[format] {
		http.Error(w, "unsupported format: "+format, http.StatusUnprocessableEntity)
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	inName := filepath.Base(hdr.Filename)
	lower := strings.ToLower(inName)
	if !strings.HasSuffix(lower, ".fb2") && !strings.HasSuffix(lower, ".zip") {
		http.Error(w, "only .fb2 and .zip files are accepted", http.StatusUnprocessableEntity)
		return
	}

	// Build effective config from form fields (form wins over raw YAML).
	cfgBytes, err := convert.BuildConfig(r.FormValue("raw_yaml"), formOptions(r))
	if err != nil {
		http.Error(w, "invalid config: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	work, err := os.MkdirTemp("", "fb2conv-*")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(work)

	inPath := filepath.Join(work, inName)
	dst, err := os.Create(inPath)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	dst.Close()

	cfgPath := ""
	if cfgBytes != nil {
		cfgPath = filepath.Join(work, "config.yaml")
		if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
	}

	destDir := filepath.Join(work, "out")

	// Concurrency guard: cap simultaneous fbc processes.
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-r.Context().Done():
		http.Error(w, "client gone", http.StatusRequestTimeout)
		return
	}

	outs, err := s.runner.Convert(r.Context(), inPath, format, cfgPath, destDir)
	if err != nil {
		log.Printf("convert %q: %v", inName, err)
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	if len(outs) == 1 {
		streamFile(w, outs[0])
		return
	}
	streamZip(w, strings.TrimSuffix(inName, filepath.Ext(inName))+".zip", outs)
}

// formOptions reads the common-knob form fields. Absent/empty fields stay nil.
func formOptions(r *http.Request) convert.FormOptions {
	var o convert.FormOptions
	if v := r.FormValue("toc_type"); v != "" {
		o.TocType = &v
	}
	if v := r.FormValue("footnotes_mode"); v != "" {
		o.FootnotesMode = &v
	}
	if v := r.FormValue("images_optimize"); v != "" {
		b := v == "true"
		o.ImagesOptimize = &b
	}
	if v := r.FormValue("insert_soft_hyphen"); v != "" {
		b := v == "true"
		o.InsertSoftHyphen = &b
	}
	if v := r.FormValue("cover_generate"); v != "" {
		b := v == "true"
		o.CoverGenerate = &b
	}
	if v := r.FormValue("dropcaps_enable"); v != "" {
		b := v == "true"
		o.DropcapsEnable = &b
	}
	if v := r.FormValue("jpeg_quality"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			o.JpegQuality = &n
		}
	}
	return o
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

func streamZip(w http.ResponseWriter, zipName string, paths []string) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(zipName))
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return
		}
		hw, err := zw.Create(filepath.Base(p))
		if err != nil {
			f.Close()
			return
		}
		if _, err := io.Copy(hw, f); err != nil {
			log.Printf("stream zip entry %s: %v", p, err)
		}
		f.Close()
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
