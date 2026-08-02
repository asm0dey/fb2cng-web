package server

import (
	"net/http"
	"strings"

	"fb2cng-web/internal/presets"
)

// settingsData is the Presets list page VM. layout is Plan 1's shared header struct.
type settingsData struct {
	layout
	DefaultID string
	Presets   []*presets.Preset
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	list, err := s.presets.List()
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	def := s.presets.DefaultID()
	if def == "" {
		def = presets.BuiltinID
	}
	s.render(w, settingsData{
		layout:    layout{Tab: "settings", ContentName: "settings"},
		DefaultID: def,
		Presets:   list,
	})
}

func (s *Server) handlePresetCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "New preset"
	}
	p, err := s.presets.Create(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/settings/preset/"+p.ID, http.StatusSeeOther)
}

func (s *Server) handlePresetDuplicate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src, err := s.presets.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, err := s.presets.Duplicate(id, src.Name+" copy"); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handlePresetDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.presets.Delete(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handlePresetDefault(w http.ResponseWriter, r *http.Request) {
	if err := s.presets.SetDefault(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
