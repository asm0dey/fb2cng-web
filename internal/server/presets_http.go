package server

import (
	"net/http"
	"strings"

	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/presets"

	"gopkg.in/yaml.v3"
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
		http.Error(w, msgServerError, http.StatusInternalServerError)
		return
	}
	def := s.presets.DefaultID()
	if def == "" {
		def = presets.BuiltinID
	}
	s.render(w, settingsData{
		layout:    layout{Tab: "settings", ContentName: "settings", User: s.userLabel(r)},
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

// buildConfigForPreset marshals a preset's sparse overrides to YAML and feeds them to
// convert.BuildConfig as the raw-YAML layer (no form options). The Builtin "defaults" preset
// has empty overrides, so this yields just the application defaults.
func (s *Server) buildConfigForPreset(id string) ([]byte, error) {
	p, err := s.presets.Get(id)
	if err != nil {
		return nil, err
	}
	raw := ""
	if len(p.Overrides) > 0 {
		b, err := yaml.Marshal(p.Overrides)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	return convert.BuildConfig(raw, convert.FormOptions{})
}

// presetOptions returns Plan 1's convert-page dropdown options from the real preset store,
// with the default preset marked selected. presetOption is Plan 1's type (fields ID, Name,
// Selected — align the field names below if Plan 1 differs).
func (s *Server) presetOptions() []presetOption {
	list, err := s.presets.List()
	if err != nil {
		return []presetOption{{ID: presets.BuiltinID, Name: "Defaults", Selected: true}}
	}
	def := s.presets.DefaultID()
	if def == "" {
		def = presets.BuiltinID
	}
	opts := make([]presetOption, 0, len(list))
	for _, p := range list {
		opts = append(opts, presetOption{ID: p.ID, Name: p.Name, Selected: p.ID == def})
	}
	return opts
}
