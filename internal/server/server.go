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

// New constructs a Server. Plan 1 took five parameters, Plan 2 appended presets as
// the sixth, Plan 3 appended schema as the seventh, and Plan 4 appends the
// authenticator as the eighth (and final) parameter.
func New(cfg config.Config, runner convert.Runner, static fs.FS,
	tpl *template.Template, jobStore *jobs.Store, presetStore *presets.Store, sch *schema.Schema,
	authn *auth.Authenticator) *Server {
	n := cfg.MaxConcurrent
	if n < 1 {
		n = 1
	}
	if cfg.FBCTimeout <= 0 {
		cfg.FBCTimeout = config.DefaultFBCTimeout
	}
	return &Server{
		cfg:     cfg,
		runner:  runner,
		static:  static,
		sem:     make(chan struct{}, n),
		tpl:     tpl,
		jobs:    jobStore,
		presets: presetStore,
		schema:  sch,
		auth:    authn,
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
