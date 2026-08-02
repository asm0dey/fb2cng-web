package server

import (
	"net/http"
	"net/http/httptest"
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
