package server

import (
	"encoding/json"
	"net/http"
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

func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
