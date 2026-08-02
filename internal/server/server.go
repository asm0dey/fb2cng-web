package server

import (
	"html/template"
	"io/fs"
	"net/http"
	"sync"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/jobs"
)

// Server wires configuration, the fbc runner, the job store, and the templated frontend.
// Plan 2 appends `presets *presets.Store`; Plan 3 appends `schema *schema.Schema`.
type Server struct {
	cfg    config.Config
	runner convert.Runner
	static fs.FS
	sem    chan struct{}
	tpl    *template.Template
	jobs   *jobs.Store
	mu     sync.Mutex // guards status.json read-modify-write (self-hosted, low-hardening)
}

// New constructs a Server. Plans 2 and 3 widen this signature (adding presets, then
// schema) when those packages land; Plan 1 takes exactly five parameters.
func New(cfg config.Config, runner convert.Runner, static fs.FS,
	tpl *template.Template, jobStore *jobs.Store) *Server {
	n := cfg.MaxConcurrent
	if n < 1 {
		n = 1
	}
	return &Server{
		cfg:    cfg,
		runner: runner,
		static: static,
		sem:    make(chan struct{}, n),
		tpl:    tpl,
		jobs:   jobStore,
	}
}

// Handler returns the full HTTP handler with routes and auth applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /defaults", s.handleDefaults)
	mux.HandleFunc("GET /me", s.handleMe)
	mux.HandleFunc("POST /convert", s.handleConvert)
	mux.HandleFunc("GET /jobs/{id}", s.handleJobStatus)
	mux.HandleFunc("GET /jobs/{id}/download/{file}", s.handleDownload)
	mux.HandleFunc("GET /jobs/{id}/zip", s.handleZip)
	mux.HandleFunc("GET /jobs/{id}/log/{file}", s.handleLog)
	mux.HandleFunc("POST /jobs/{id}/retry", s.handleRetry)
	mux.Handle("GET /static/", http.FileServer(http.FS(s.static)))
	return ForwardAuth(s.cfg.ForwardAuth, s.cfg.TrustedProxies, mux)
}

// render executes the shared "base" layout, which dispatches to the page body named by
// the embedded layout.ContentName. Use it for full-page GETs.
func (s *Server) render(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// renderPartial executes a single named template (an htmx fragment) directly, bypassing
// the base layout. Use it for the convert-card and other swap regions.
func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}
