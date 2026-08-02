package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/jobs"
	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/web"
)

// newPresetServer builds a Server whose presets store is rooted at dir.
//
// Server.New's signature is owned by Plan 1: New(cfg, runner, static, tpl, jobs). Plan 2 widens
// it to New(cfg, runner, static, tpl, jobs, presets) by appending the presets field (concretely
// typed *presets.Store). tpl MUST be a REAL parsed template set (base.gohtml + settings.gohtml +
// preset_edit.gohtml) — handleSettings/handlePresetEdit call s.tpl via s.render, so a nil tpl
// panics. web.Templates() is Plan 1's parse helper over the embedded templates; if Plan 1 names
// it differently, use that helper — never pass nil.
func newPresetServer(t *testing.T, dir string) (*Server, http.Handler) {
	t.Helper()
	tpl, err := web.Templates()
	if err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewStore(t.TempDir(), time.Hour)
	ps := presets.NewStore(dir)
	srv := New(config.Config{MaxConcurrent: 1}, stubRunner{}, web.FS, tpl, jobStore, ps)
	return srv, srv.Handler()
}

func TestSettingsListsPresets(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	if _, err := srv.presets.Create("Kindle"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/settings", nil))
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Kindle",           // user preset name
		"Defaults",         // built-in row
		"Built-in",         // built-in badge
		"options changed",  // overrides column phrasing (mockup SCREEN 4)
		"How presets work", // collapsed details
		"+ New preset",     // create control
		`name="default"`,   // default radio
	} {
		if !strings.Contains(body, want) {
			t.Errorf("settings page missing %q", want)
		}
	}
}

func postForm(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateRedirectsToEditor(t *testing.T) {
	_, h := newPresetServer(t, t.TempDir())
	rec := postForm(h, "/settings/preset", url.Values{"name": {"Kindle"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/settings/preset/kindle" {
		t.Fatalf("Location=%q, want /settings/preset/kindle", loc)
	}
}

func TestSetDefaultRoute(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")
	rec := postForm(h, "/settings/preset/"+p.ID+"/default", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d", rec.Code)
	}
	if srv.presets.DefaultID() != p.ID {
		t.Fatalf("default not set, got %q", srv.presets.DefaultID())
	}
}

func TestDuplicateRoute(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")
	rec := postForm(h, "/settings/preset/"+p.ID+"/duplicate", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d", rec.Code)
	}
	list, _ := srv.presets.List()
	if len(list) != 3 { // Defaults + Kindle + copy
		t.Fatalf("after duplicate List len=%d, want 3", len(list))
	}
}

func TestDeleteRoute(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")
	rec := postForm(h, "/settings/preset/"+p.ID+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, p.ID+".yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still present: %v", err)
	}
}

func TestDeleteDefaultBlocked(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")
	srv.presets.SetDefault(p.ID)
	rec := postForm(h, "/settings/preset/"+p.ID+"/delete", url.Values{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("deleting default should be 422, got %d", rec.Code)
	}
}

func TestEditorSeedsYAML(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")
	p.Overrides = map[string]any{"document": map[string]any{"toc_type": "inline"}}
	srv.presets.Save(p)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/settings/preset/"+p.ID, nil))
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "toc_type") || !strings.Contains(body, "Kindle") {
		t.Fatalf("editor did not seed name/overrides:\n%s", body)
	}
}

func TestEditBuiltinRedirects(t *testing.T) {
	_, h := newPresetServer(t, t.TempDir())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/settings/preset/defaults", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("editing builtin should redirect, got %d", rec.Code)
	}
}

func TestSaveParsesYAMLBackIntoOverrides(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")

	form := url.Values{
		"name":           {"Kindle Pro"},
		"overrides_yaml": {"document:\n  images:\n    optimize: true\n"},
	}
	rec := postForm(h, "/settings/preset/"+p.ID, form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save code=%d body=%q", rec.Code, rec.Body.String())
	}
	got, _ := srv.presets.Get(p.ID)
	if got.Name != "Kindle Pro" || got.ChangedCount() != 1 {
		t.Fatalf("saved preset = %+v", got)
	}
}

func TestBuildConfigForPreset(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")
	p.Overrides = map[string]any{"document": map[string]any{"toc_type": "inline"}}
	srv.presets.Save(p)

	out, err := srv.buildConfigForPreset(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(out)
	// Preset override is present:
	if !strings.Contains(cfg, "toc_type: inline") {
		t.Errorf("preset override missing:\n%s", cfg)
	}
	// Application defaults still layered underneath (from convert.applicationDefaults):
	if !strings.Contains(cfg, "insert_soft_hyphen: true") {
		t.Errorf("application defaults missing:\n%s", cfg)
	}
}

func TestBuildConfigForDefaults(t *testing.T) {
	srv, _ := newPresetServer(t, t.TempDir())
	out, err := srv.buildConfigForPreset("defaults")
	if err != nil {
		t.Fatal(err)
	}
	// Empty overrides -> only the application defaults:
	if !strings.Contains(string(out), "insert_soft_hyphen: true") {
		t.Errorf("defaults config missing app defaults:\n%s", string(out))
	}
}

func TestIndexListsRealPresets(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, err := srv.presets.Create("Kindle")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.presets.SetDefault(p.ID); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("index code=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="defaults"`) || !strings.Contains(body, "Defaults") {
		t.Errorf("dropdown missing Defaults option:\n%s", body)
	}
	if !strings.Contains(body, `value="`+p.ID+`"`) || !strings.Contains(body, "Kindle") {
		t.Errorf("dropdown missing created preset:\n%s", body)
	}
	if !strings.Contains(body, `value="`+p.ID+`" selected`) {
		t.Errorf("default preset not marked selected in dropdown:\n%s", body)
	}
}

func TestConvertUsesChosenPresetOverrides(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, err := srv.presets.Create("Kindle")
	if err != nil {
		t.Fatal(err)
	}
	p.Overrides = map[string]any{"document": map[string]any{"toc_type": "inline"}}
	if err := srv.presets.Save(p); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3", "preset": p.ID}))
	if rec.Code != 200 {
		t.Fatalf("convert POST code=%d body=%q", rec.Code, rec.Body.String())
	}
	id := extractJobID(t, rec.Body.String())

	cfg, err := os.ReadFile(filepath.Join(srv.jobs.Dir, id, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "toc_type: inline") {
		t.Errorf("chosen preset's override missing from job config:\n%s", cfg)
	}

	st, err := srv.jobs.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Preset != p.ID {
		t.Errorf("Status.Preset = %q, want %q", st.Preset, p.ID)
	}
}

func TestConvertAbsentPresetUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3"})) // no preset field
	if rec.Code != 200 {
		t.Fatalf("convert POST code=%d body=%q", rec.Code, rec.Body.String())
	}
	id := extractJobID(t, rec.Body.String())

	cfg, err := os.ReadFile(filepath.Join(srv.jobs.Dir, id, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "insert_soft_hyphen: true") {
		t.Errorf("missing application defaults for absent preset:\n%s", cfg)
	}

	st, err := srv.jobs.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Preset != "defaults" {
		t.Errorf("Status.Preset = %q, want %q", st.Preset, "defaults")
	}
}

// TestConvertUnknownPresetRejected is the regression test for bean i5e5: a
// well-formed but unknown preset id must be rejected (422) BEFORE any job
// state is created. Pre-fix, handleConvert created the job dir, persisted
// inputs, and seeded status.json with StatePending rows before ever calling
// buildConfigForPreset, so a rejected preset left an orphaned job with no
// worker ever launched to flip it to StateFailed — it would sit Pending,
// unreachable and unretryable, until TTL sweep.
func TestConvertUnknownPresetRejected(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3", "preset": "nonexistent"}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown preset should be 422, got %d body=%q", rec.Code, rec.Body.String())
	}
	entries, err := os.ReadDir(srv.jobs.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unknown preset must not leave an orphaned job dir, found %d entries: %v", len(entries), entries)
	}
}

// TestConvertUnsafePresetIDRejected covers the other buildConfigForPreset
// failure path: an id that fails presets.validID (path-traversal shaped)
// rather than a merely-unknown-but-safe id. Same orphaning bug, same fix.
func TestConvertUnsafePresetIDRejected(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3", "preset": "../evil"}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe preset id should be 422, got %d body=%q", rec.Code, rec.Body.String())
	}
	entries, err := os.ReadDir(srv.jobs.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unsafe preset id must not leave an orphaned job dir, found %d entries: %v", len(entries), entries)
	}
}

func TestSaveInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	srv, h := newPresetServer(t, dir)
	p, _ := srv.presets.Create("Kindle")
	form := url.Values{"name": {"Kindle"}, "overrides_yaml": {"key: : broken:\n  - ]["}}
	rec := postForm(h, "/settings/preset/"+p.ID, form)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid YAML should be 422, got %d", rec.Code)
	}
}
