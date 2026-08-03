package server

import (
	"html/template"
	"io/fs"
	"net/http"
	"sync"

	"fb2cng-web/internal/auth"
	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/jobs"
	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/schema"
)

// Server wires configuration, the fbc runner, the job store, and the templated frontend.
type Server struct {
	cfg     config.Config
	runner  convert.Runner
	static  fs.FS
	sem     chan struct{}
	tpl     *template.Template
	jobs    *jobs.Store
	presets *presets.Store
	mu      sync.Mutex // guards status.json read-modify-write (self-hosted, low-hardening)
	schema  *schema.Schema
	auth    *auth.Authenticator
}

// Deps are the collaborators a Server needs, grouped so New keeps a small signature.
type Deps struct {
	Runner  convert.Runner
	Static  fs.FS
	Tpl     *template.Template
	Jobs    *jobs.Store
	Presets *presets.Store
	Schema  *schema.Schema
	Auth    *auth.Authenticator
}

// New constructs a Server from its config and dependencies.
func New(cfg config.Config, d Deps) *Server {
	n := cfg.MaxConcurrent
	if n < 1 {
		n = 1
	}
	if cfg.FBCTimeout <= 0 {
		cfg.FBCTimeout = config.DefaultFBCTimeout
	}
	return &Server{
		cfg:     cfg,
		runner:  d.Runner,
		static:  d.Static,
		sem:     make(chan struct{}, n),
		tpl:     d.Tpl,
		jobs:    d.Jobs,
		presets: d.Presets,
		schema:  d.Schema,
		auth:    d.Auth,
	}
}

// Handler returns the full HTTP handler with routes and auth applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /convert", s.handleConvert)
	mux.HandleFunc("GET /jobs/{id}", s.handleJobStatus)
	mux.HandleFunc("GET /jobs/{id}/download/{file}", s.handleDownload)
	mux.HandleFunc("GET /jobs/{id}/zip", s.handleZip)
	mux.HandleFunc("GET /jobs/{id}/log/{file}", s.handleLog)
	mux.HandleFunc("POST /jobs/{id}/retry", s.handleRetry)
	mux.HandleFunc("GET /settings", s.handleSettings)
	mux.HandleFunc("POST /settings/preset", s.handlePresetCreate)
	mux.HandleFunc("GET /settings/preset/{id}", s.handlePresetEditor)
	mux.HandleFunc("POST /settings/preset/{id}", s.handlePresetSave)
	mux.HandleFunc("POST /settings/preset/{id}/effective", s.handlePresetEffective)
	mux.HandleFunc("POST /settings/preset/{id}/duplicate", s.handlePresetDuplicate)
	mux.HandleFunc("POST /settings/preset/{id}/delete", s.handlePresetDelete)
	mux.HandleFunc("POST /settings/preset/{id}/default", s.handlePresetDefault)

	root := http.NewServeMux()
	root.Handle("GET /static/", http.FileServer(http.FS(s.static)))
	if s.auth.Enabled() {
		root.HandleFunc("/auth/login", s.auth.Login)
		root.HandleFunc("/auth/callback", s.auth.Callback)
		root.HandleFunc("/auth/logout", s.auth.Logout)
	}
	root.Handle("/", s.auth.Middleware(mux))
	return root
}

// userLabel is the header badge's display name: the session's name when
// authenticated, empty otherwise. s.auth is nil in editor_test.go's hand-built
// &Server{} unit-test literals (they exercise a single handler directly, never
// through New/Handler), so guard against that rather than requiring every such
// literal to carry an authenticator it doesn't otherwise need.
func (s *Server) userLabel(r *http.Request) string {
	if s.auth == nil {
		return ""
	}
	if sess, ok := s.auth.SessionFromRequest(r); ok {
		return sess.Name
	}
	return ""
}

// render executes the shared "base" layout, which dispatches to the page body named by
// the embedded layout.ContentName. Use it for full-page GETs.
func (s *Server) render(w http.ResponseWriter, data any) {
	w.Header().Set(hdrContentType, ctHTML)
	if err := s.tpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// renderPartial executes a single named template (an htmx fragment) directly, bypassing
// the base layout. Use it for the convert-card and other swap regions.
func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) {
	w.Header().Set(hdrContentType, ctHTML)
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}
