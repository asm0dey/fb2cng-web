package server

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/schema"
)

// editorTemplates parses the editor content template + effective partial atop a
// STUB "base" that stands in for Plan 1's base.gohtml, so the editor is exercised
// through the base->content indirection (s.render executes "base") without
// coupling the test to Plan 1's chrome. Production parses the real base.gohtml
// from the embed glob instead.
//
// Go's {{template}} action requires a *constant* name, so dynamic dispatch by
// field (base.gohtml's .ContentName) goes through a "content" template func —
// see internal/web/embed.go's Templates(). This stub registers the same func
// so it exercises the identical base->content indirection production uses.
func editorTemplates(t *testing.T) *template.Template {
	t.Helper()
	tpl := template.New("")
	tpl.Funcs(template.FuncMap{
		"content": func(name string, data any) (template.HTML, error) {
			var b strings.Builder
			if err := tpl.ExecuteTemplate(&b, name, data); err != nil {
				return "", err
			}
			return template.HTML(b.String()), nil
		},
	})
	tpl = template.Must(tpl.Parse(
		`{{define "base"}}<!doctype html><html data-tab="{{.Tab}}">{{content .ContentName .}}</html>{{end}}`))
	if _, err := tpl.ParseFiles(
		"../web/templates/editor.gohtml",
		"../web/templates/_effective.gohtml",
	); err != nil {
		t.Fatal(err)
	}
	return tpl
}

func testSchema(t *testing.T) *schema.Schema {
	t.Helper()
	// schema.Load reads the embedded production options.json; tests build a small
	// fixed schema directly (Schema.Options is exported) to stay hermetic.
	return &schema.Schema{Options: []schema.Option{
		{Key: "document.toc_type", Group: "document", Label: "toc_type", Kind: schema.KindString, Default: "normal"},
		{Key: "document.images.optimize", Group: "document", Label: "optimize", Kind: schema.KindBool, Default: true},
		{Key: "document.images.jpeg_quality_level", Group: "document", Label: "jpeg_quality_level", Kind: schema.KindInt, Default: float64(75)},
	}}
}

func TestPresetEditorRendersGrid(t *testing.T) {
	store := presets.NewStore(t.TempDir())
	p, err := store.Create("My Preset")
	if err != nil {
		t.Fatal(err)
	}
	p.Overrides = map[string]any{
		"document": map[string]any{
			"images": map[string]any{"jpeg_quality_level": 40},
		},
		"unknown_key": "x",
	}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}

	s := &Server{schema: testSchema(t), presets: store, tpl: editorTemplates(t)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings/preset/{id}", s.handlePresetEditor)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings/preset/"+p.ID, nil))
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-tab="settings"`) {
		t.Fatalf("editor not rendered through base with Settings tab: %s", body)
	}
	if !strings.Contains(body, `data-group="document"`) {
		t.Fatal("missing document group header")
	}
	if !strings.Contains(body, "3 options · 1 changed") {
		t.Fatalf("document group count wrong: %s", body)
	}
	if !strings.Contains(body, "2 of 3 options changed") {
		t.Fatalf("toolbar changed-summary wrong: %s", body)
	}
	if !strings.Contains(body, `option-row changed" data-path="document.images.jpeg_quality_level"`) {
		t.Fatal("jpeg row not marked changed")
	}
	if !strings.Contains(body, `name="document.images.jpeg_quality_level" value="40"`) {
		t.Fatalf("jpeg override value not rendered: %s", body)
	}
	if !strings.Contains(body, `data-path="unknown_key"`) {
		t.Fatal("synthetic (unschematized) override row missing")
	}
	if !strings.Contains(body, `name="document.images.optimize"`) {
		t.Fatal("unchanged bool control missing")
	}
}

func TestPresetSaveSparse(t *testing.T) {
	store := presets.NewStore(t.TempDir())
	p, err := store.Create("P")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{schema: testSchema(t), presets: store}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /settings/preset/{id}", s.handlePresetSave)

	form := url.Values{}
	form.Set("name", "Renamed")
	form.Set("document.toc_type", "normal")              // == default -> dropped
	form.Set("document.images.jpeg_quality_level", "40") // != default 75 -> kept
	form["document.images.optimize"] = []string{"false"} // unchecked bool -> false, != default true -> kept

	req := httptest.NewRequest("POST", "/settings/preset/"+p.ID, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}

	got, err := store.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed" {
		t.Fatalf("name not saved: %q", got.Name)
	}
	doc, _ := got.Overrides["document"].(map[string]any)
	if doc == nil {
		t.Fatalf("overrides missing document: %+v", got.Overrides)
	}
	if _, ok := doc["toc_type"]; ok {
		t.Fatal("default toc_type should be dropped (sparse)")
	}
	img, _ := doc["images"].(map[string]any)
	if img == nil {
		t.Fatalf("overrides missing document.images: %+v", doc)
	}
	if fmt.Sprint(img["jpeg_quality_level"]) != "40" {
		t.Fatalf("jpeg override not saved: %v", img["jpeg_quality_level"])
	}
	if img["optimize"] != false {
		t.Fatalf("optimize should be saved as false: %v", img["optimize"])
	}
}
