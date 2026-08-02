package server

import (
	"net/http"

	"fb2cng-web/internal/jobs"
)

// layout is the shared header VM embedded by every page VM. base.gohtml reads its
// promoted fields (.Tab, .ContentName, .User) directly.
type layout struct {
	Tab         string // "convert" | "settings"
	ContentName string // {{define}} name of the page body base dispatches to
	User        string // forward-auth display name, "" when disabled
}

// Page/card view models rendered by the Convert templates.
type presetOption struct {
	ID   string
	Name string
}

type convertCardVM struct {
	Status       *jobs.Status
	TotalOutputs int // total output files across the batch
	Total        int // number of input files
	DoneCount    int // files in state "done"
	FailedCount  int // files in state "failed"
}

type pageVM struct {
	layout  // embedded: promotes .Tab, .ContentName, .User
	Presets []presetOption
	Formats []string
	Format  string
	Card    *convertCardVM
}

var uiFormats = []string{"epub3", "epub2", "kepub", "kfx", "azw8", "pdf"}

func defaultPresetOptions() []presetOption {
	return []presetOption{{ID: "defaults", Name: "Defaults"}}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, pageVM{
		layout:  layout{Tab: "convert", ContentName: "convert", User: r.Header.Get("Remote-Name")},
		Presets: defaultPresetOptions(),
		Formats: uiFormats,
		Format:  "epub3",
		Card:    &convertCardVM{},
	})
}

// --- temporary stubs; real bodies land in Tasks 7–10 ---
func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not yet", http.StatusNotImplemented)
}
func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not yet", http.StatusNotImplemented)
}
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not yet", http.StatusNotImplemented)
}
func (s *Server) handleZip(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not yet", http.StatusNotImplemented)
}
func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not yet", http.StatusNotImplemented)
}
func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not yet", http.StatusNotImplemented)
}
