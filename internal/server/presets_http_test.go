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
