package server

import (
	"io/fs"
	"net/http"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
)

// Server wires configuration, the fbc runner, and the static frontend.
type Server struct {
	cfg    config.Config
	runner convert.Runner
	static fs.FS
	sem    chan struct{}
}

// New constructs a Server. static is the embedded frontend filesystem.
func New(cfg config.Config, runner convert.Runner, static fs.FS) *Server {
	n := cfg.MaxConcurrent
	if n < 1 {
		n = 1
	}
	return &Server{cfg: cfg, runner: runner, static: static, sem: make(chan struct{}, n)}
}

// Handler returns the full HTTP handler with routes and auth applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /defaults", s.handleDefaults)
	mux.HandleFunc("GET /me", s.handleMe)
	mux.HandleFunc("POST /convert", s.handleConvert)
	mux.Handle("/", http.FileServer(http.FS(s.static)))
	return ForwardAuth(s.cfg.ForwardAuth, s.cfg.TrustedProxies, mux)
}
