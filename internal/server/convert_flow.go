package server

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fb2cng-web/internal/convert"
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

func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(128 << 20); err != nil {
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
	preset := r.FormValue("preset") // Plan 1: only "defaults" is offered
	if preset == "" {
		preset = "defaults"
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}

	id, err := s.jobs.Create(preset, format)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	// Persist inputs synchronously (the request body will not survive the goroutine).
	var names []string
	used := map[string]bool{}
	for _, fh := range files {
		name := filepath.Base(fh.Filename)
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".fb2") && !strings.HasSuffix(lower, ".zip") {
			http.Error(w, "only .fb2 and .zip files are accepted", http.StatusUnprocessableEntity)
			return
		}
		// De-dupe colliding basenames within this batch (e.g. two uploads both
		// named "book.fb2"): each persisted input and FileResult.Input must be
		// unique, or the second os.Create below overwrites the first upload
		// and the two StatePending rows sharing one Input hang the job (only
		// the first ever goes terminal — see updateFile).
		name = uniqueInputName(name, used)
		used[name] = true
		src, err := fh.Open()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		dst, err := os.Create(s.jobs.InputPath(id, name))
		if err != nil {
			src.Close()
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if _, err := copyAndClose(dst, src); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		names = append(names, name)
	}

	// Seed pending file rows.
	st, err := s.jobs.Load(id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	for _, n := range names {
		st.Files = append(st.Files, jobs.FileResult{Input: n, State: jobs.StatePending})
	}
	if err := s.jobs.Save(id, st); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	// Write the effective config once for the whole batch (Defaults => app defaults).
	cfgPath := filepath.Join(s.jobs.Dir, id, "config.yaml")
	cfgBytes, err := convert.BuildConfig("", convert.FormOptions{})
	if err != nil {
		http.Error(w, "invalid config: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	go s.processFiles(id, cfgPath, format, names)

	st, _ = s.jobs.Load(id)
	s.renderPartial(w, "convert_card", cardFor(st))
}

// uniqueInputName returns name, suffixed (book.fb2 -> book-1.fb2 ->
// book-2.fb2, ...) against the set of basenames already used in this batch,
// until it is unique. filepath.Base collisions between distinct uploads
// would otherwise overwrite each other's persisted input file and share one
// FileResult row.
func uniqueInputName(name string, used map[string]bool) string {
	if !used[name] {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	// Strip any pre-existing "-N" suffix so repeated collisions increment
	// cleanly (book-1.fb2 colliding again yields book-2.fb2, not book-1-1.fb2).
	if i := strings.LastIndex(stem, "-"); i >= 0 {
		if _, err := strconv.Atoi(stem[i+1:]); err == nil {
			stem = stem[:i]
		}
	}
	for n := 1; ; n++ {
		candidate := stem + "-" + strconv.Itoa(n) + ext
		if !used[candidate] {
			return candidate
		}
	}
}

// copyAndClose copies src into dst and closes both, returning the first error.
func copyAndClose(dst *os.File, src interface{ Read([]byte) (int, error) }) (int64, error) {
	n, err := ioCopy(dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if c, ok := src.(interface{ Close() error }); ok {
		if cerr := c.Close(); err == nil {
			err = cerr
		}
	}
	return n, err
}

func ioCopy(dst *os.File, src interface{ Read([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if rerr != nil {
			if rerr.Error() == "EOF" {
				return total, nil
			}
			return total, rerr
		}
	}
}

// processFiles runs fbc per input under the sem cap, capturing logs and updating status.
func (s *Server) processFiles(id, cfgPath, format string, inputs []string) {
	for _, in := range inputs {
		s.sem <- struct{}{}
		start := time.Now()
		outs, cerr := s.runner.ConvertLogged(
			context.Background(), s.jobs.InputPath(id, in), format, cfgPath,
			s.jobs.OutDir(id, in), s.jobs.LogPath(id, in))
		<-s.sem

		lines, firstErr := scanLog(s.jobs.LogPath(id, in))
		fr := jobs.FileResult{
			Input:      in,
			LogLines:   lines,
			FirstError: firstErr,
			Millis:     time.Since(start).Milliseconds(),
			Sizes:      map[string]int64{},
		}
		for _, o := range outs {
			base := filepath.Base(o)
			fr.Outputs = append(fr.Outputs, base)
			if fi, e := os.Stat(o); e == nil {
				fr.Sizes[base] = fi.Size()
			}
		}
		if cerr != nil {
			fr.State = jobs.StateFailed
			fr.Err = cerr.Error()
		} else {
			fr.State = jobs.StateDone
		}
		s.updateFile(id, fr)
	}
}

// updateFile merges one file's result into status.json and recomputes Done.
func (s *Server) updateFile(id string, fr jobs.FileResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.jobs.Load(id)
	if err != nil {
		return
	}
	found := false
	for i := range st.Files {
		if st.Files[i].Input == fr.Input {
			st.Files[i] = fr
			found = true
			break
		}
	}
	if !found {
		st.Files = append(st.Files, fr)
	}
	st.Done = allTerminal(st.Files)
	_ = s.jobs.Save(id, st)
}

func allTerminal(files []jobs.FileResult) bool {
	for _, f := range files {
		if f.State != jobs.StateDone && f.State != jobs.StateFailed {
			return false
		}
	}
	return len(files) > 0
}

// scanLog counts lines and returns the first line starting with "ERR".
func scanLog(path string) (lines int, firstError string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines++
		if firstError == "" && strings.HasPrefix(sc.Text(), "ERR") {
			firstError = sc.Text()
		}
	}
	return lines, firstError
}

// cardFor builds the convert-card view model from a status.
func cardFor(st *jobs.Status) *convertCardVM {
	vm := &convertCardVM{Status: st, Total: len(st.Files)}
	for _, f := range st.Files {
		vm.TotalOutputs += len(f.Outputs)
		switch f.State {
		case jobs.StateDone:
			vm.DoneCount++
		case jobs.StateFailed:
			vm.FailedCount++
		}
	}
	return vm
}

func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st, err := s.jobs.Load(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderPartial(w, "convert_card", cardFor(st))
}
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	file := r.PathValue("file")
	st, err := s.jobs.Load(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	for _, fr := range st.Files {
		for _, o := range fr.Outputs {
			if o == file {
				p, err := s.jobs.OutputFile(id, fr.Input, o)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				streamFile(w, p)
				return
			}
		}
	}
	http.NotFound(w, r)
}

func (s *Server) handleZip(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.jobs.Load(id); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(id+".zip"))
	if err := s.jobs.WriteZip(id, w); err != nil {
		// Header already sent; log-only. (self-hosted, low-hardening)
		return
	}
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	input := r.PathValue("file")
	if !jobs.ValidID(id) || strings.ContainsAny(input, `/\`) || strings.Contains(input, "..") {
		http.NotFound(w, r)
		return
	}
	if _, err := s.jobs.Load(id); err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(s.jobs.LogPath(id, input))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = ioCopyWriter(w, f)
}

func ioCopyWriter(w http.ResponseWriter, f *os.File) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if rerr != nil {
			return total, nil
		}
	}
}
func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// The failed->pending reset must be atomic with respect to updateFile's
	// read-modify-write of status.json (guarded by the same s.mu), or a
	// concurrent in-flight worker write can be lost. Holding the lock across
	// Load+Save also dedupes concurrent retry requests for free: a second
	// retry entering after the first finds no StateFailed files (they're
	// already pending) and launches no worker.
	s.mu.Lock()
	st, err := s.jobs.Load(id)
	if err != nil {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}

	var failed []string
	for i := range st.Files {
		if st.Files[i].State == jobs.StateFailed {
			failed = append(failed, st.Files[i].Input)
			st.Files[i].State = jobs.StatePending
			st.Files[i].Err = ""
			st.Files[i].FirstError = ""
		}
	}
	if len(failed) == 0 {
		s.mu.Unlock()
		s.renderPartial(w, "convert_card", cardFor(st))
		return
	}
	st.Done = false
	saveErr := s.jobs.Save(id, st)
	s.mu.Unlock()
	if saveErr != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	// Retry config = batch defaults + the broken-image tolerance flip.
	cfgBytes, err := convert.BuildConfig("use_broken_images: true", convert.FormOptions{})
	if err != nil {
		http.Error(w, "invalid retry config: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	cfgPath := filepath.Join(s.jobs.Dir, id, "retry-config.yaml")
	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	go s.processFiles(id, cfgPath, st.Format, failed)

	st, _ = s.jobs.Load(id)
	s.renderPartial(w, "convert_card", cardFor(st))
}
