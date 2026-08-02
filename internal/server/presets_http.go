package server

import (
	"net/http"

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
