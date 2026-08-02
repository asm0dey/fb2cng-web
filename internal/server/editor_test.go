package server

import (
	"context"
	"errors"
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

	s := &Server{schema: testSchema(t), presets: store, tpl: editorTemplates(t), runner: effRunner{}}
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
	if !strings.Contains(body, `3 options · <span class="grp-changed">1</span> changed`) {
		t.Fatalf("document group count wrong: %s", body)
	}
	if !strings.Contains(body, `<b>2</b> of 3 changed`) {
		t.Fatalf("toolbar changed-summary wrong: %s", body)
	}
	if !strings.Contains(body, `option-row is-changed" data-path="document.images.jpeg_quality_level"`) {
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

func TestPresetEditorRendersSections(t *testing.T) {
	store := presets.NewStore(t.TempDir())
	p, err := store.Create("P")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	s := &Server{schema: testSchema(t), presets: store, tpl: editorTemplates(t), runner: effRunner{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings/preset/{id}", s.handlePresetEditor)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings/preset/"+p.ID, nil))
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="opt-section`) {
		t.Fatalf("no nested section rendered: %s", body)
	}
	if !strings.Contains(body, `data-section="images"`) {
		t.Fatal("images section header missing")
	}
	if !strings.Contains(body, `<span class="opt-section-name">Images</span>`) {
		t.Fatal("prettified Images label missing")
	}
	// optimize row still rendered (now nested inside the images section)
	if !strings.Contains(body, `name="document.images.optimize"`) {
		t.Fatal("optimize control missing after restructure")
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

// TestPresetSaveDropsNonNumericInt covers bean dbxm: a POST to the editor save
// route can be crafted (bypassing the <input type=number> widget) to submit a
// non-numeric value for a schema-declared KindInt field. parseValue must not
// let that leak through as a string override for an int field — it should be
// treated as "no override" and dropped, leaving the schema default in effect.
// A valid int in the same POST must still be stored as before (no regression).
func TestPresetSaveDropsNonNumericInt(t *testing.T) {
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
	form.Set("document.images.jpeg_quality_level", "abc") // non-numeric int -> must be dropped, not stored as string
	form.Set("document.toc_type", "detailed")             // valid string override -> must still be stored (no regression)

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
	doc, _ := got.Overrides["document"].(map[string]any)
	if doc == nil {
		t.Fatalf("overrides missing document: %+v", got.Overrides)
	}
	if img, _ := doc["images"].(map[string]any); img != nil {
		if v, ok := img["jpeg_quality_level"]; ok {
			t.Fatalf("non-numeric int override should be dropped, not stored: %v (%T)", v, v)
		}
	}
	if doc["toc_type"] != "detailed" {
		t.Fatalf("valid string override should still be saved: %v", doc["toc_type"])
	}
}

// TestPresetSaveKeepsValidInt confirms a valid int POST value is still stored
// as a Go int (not a string), so schema.Option.IsDefault's normalize-based
// comparison and downstream convert.BuildConfig still see the right type.
func TestPresetSaveKeepsValidInt(t *testing.T) {
	store := presets.NewStore(t.TempDir())
	p, err := store.Create("P")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{schema: testSchema(t), presets: store}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /settings/preset/{id}", s.handlePresetSave)

	form := url.Values{}
	form.Set("document.images.jpeg_quality_level", "40")

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
	doc, _ := got.Overrides["document"].(map[string]any)
	if doc == nil {
		t.Fatalf("overrides missing document: %+v", got.Overrides)
	}
	img, _ := doc["images"].(map[string]any)
	if img == nil {
		t.Fatalf("overrides missing document.images: %+v", doc)
	}
	v, ok := img["jpeg_quality_level"]
	if !ok {
		t.Fatal("valid int override should be stored")
	}
	if _, isInt := v.(int); !isInt {
		t.Fatalf("valid int override should be stored as Go int, got %T: %v", v, v)
	}
	if v.(int) != 40 {
		t.Fatalf("valid int override wrong value: %v", v)
	}
}

func TestRowLabel(t *testing.T) {
	cases := map[string]string{
		"document.images.optimize":                    "optimize",
		"document.output_name_template":               "output_name_template",
		"document.text_transformations.speech.enable": "speech.enable",
		"document.images.cover.default_image_path":    "cover.default_image_path",
		"document.stylesheet_path":                    "stylesheet_path",
		"document.vignettes.chapter.end":              "chapter.end",
		"version":                                     "version",
	}
	for k, want := range cases {
		if got := rowLabel(k); got != want {
			t.Errorf("rowLabel(%q) = %q, want %q", k, got, want)
		}
	}
}

// TestBuildEditorVMDeepLabels covers keys nested deeper than a section: they
// group under the section (2nd segment) but keep the remaining path as their
// row label so same-leaf keys stay distinguishable.
func TestBuildEditorVMDeepLabels(t *testing.T) {
	s := &Server{schema: &schema.Schema{Options: []schema.Option{
		{Key: "document.vignettes.chapter.end", Group: "document", Label: "end", Kind: schema.KindString, Default: ""},
		{Key: "document.vignettes.book.title_top", Group: "document", Label: "title_top", Kind: schema.KindString, Default: ""},
	}}}
	vm := s.buildEditorVM(&presets.Preset{}, map[string]any{})
	var vig *sectionVM
	for i := range vm.Groups {
		if vm.Groups[i].Name != "document" {
			continue
		}
		for j := range vm.Groups[i].Sections {
			if vm.Groups[i].Sections[j].Name == "vignettes" {
				vig = &vm.Groups[i].Sections[j]
			}
		}
	}
	if vig == nil {
		t.Fatalf("no vignettes section in %+v", vm.Groups)
	}
	if vig.Label != "Vignettes" {
		t.Errorf("section label = %q, want Vignettes", vig.Label)
	}
	if len(vig.Rows) != 2 || vig.Rows[0].Label != "chapter.end" || vig.Rows[1].Label != "book.title_top" {
		t.Errorf("row labels = %v, want [chapter.end book.title_top]",
			[]string{vig.Rows[0].Label, vig.Rows[1].Label})
	}
}

func TestPrettify(t *testing.T) {
	cases := map[string]string{
		"general":              "General",
		"images":               "Images",
		"text_transformations": "Text transformations",
		"page_map":             "Page map",
		"":                     "",
	}
	for in, want := range cases {
		if got := prettify(in); got != want {
			t.Errorf("prettify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildEditorVMSections(t *testing.T) {
	s := &Server{schema: &schema.Schema{Options: []schema.Option{
		{Key: "document.toc_type", Group: "document", Label: "toc_type", Kind: schema.KindString, Default: "normal"},
		{Key: "document.images.optimize", Group: "document", Label: "optimize", Kind: schema.KindBool, Default: true},
		{Key: "document.images.jpeg_quality_level", Group: "document", Label: "jpeg_quality_level", Kind: schema.KindInt, Default: float64(75)},
		{Key: "version", Group: "version", Label: "version", Kind: schema.KindString, Default: "2.0"},
	}}}
	// buildEditorVM's "flat" param is an already-flattened dotted-key map (as
	// produced by flattenOverrides and passed by every real caller), not a
	// nested map — use the dotted form here to match that contract.
	vm := s.buildEditorVM(&presets.Preset{}, map[string]any{
		"document.images.jpeg_quality_level": 40,
	})

	find := func(name string) groupVM {
		t.Helper()
		for _, g := range vm.Groups {
			if g.Name == name {
				return g
			}
		}
		t.Fatalf("group %q not found in %+v", name, vm.Groups)
		return groupVM{}
	}

	doc := find("document")
	if doc.Flat {
		t.Fatal("document has 2 sections, must not be Flat")
	}
	if len(doc.Sections) != 2 {
		t.Fatalf("want 2 sections, got %d: %+v", len(doc.Sections), doc.Sections)
	}
	if doc.Sections[0].Name != "general" || doc.Sections[0].Label != "General" {
		t.Fatalf("section 0 = %+v, want general/General", doc.Sections[0])
	}
	if doc.Sections[1].Name != "images" || doc.Sections[1].Label != "Images" {
		t.Fatalf("section 1 = %+v, want images/Images", doc.Sections[1])
	}
	if len(doc.Sections[1].Rows) != 2 || doc.Sections[1].Rows[0].Key != "document.images.optimize" {
		t.Fatalf("images section rows wrong: %+v", doc.Sections[1].Rows)
	}
	if doc.Sections[1].Changed != 1 { // jpeg override differs from default
		t.Fatalf("images Changed = %d, want 1", doc.Sections[1].Changed)
	}
	if doc.Total != 3 || doc.Changed != 1 {
		t.Fatalf("group totals wrong: Total=%d Changed=%d", doc.Total, doc.Changed)
	}
	if ver := find("version"); !ver.Flat || len(ver.Sections) != 1 {
		t.Fatalf("version must be Flat single-section: %+v", ver)
	}
}

// effRunner is a Runner whose Validate outcome is controlled by err.
type effRunner struct{ err error }

func (effRunner) DumpDefaults(context.Context) ([]byte, error) { return nil, nil }
func (effRunner) Convert(context.Context, string, string, string, string) ([]string, error) {
	return nil, nil
}
func (effRunner) ConvertLogged(context.Context, string, string, string, string, string) ([]string, error) {
	return nil, nil
}
func (e effRunner) Validate(context.Context, string) error { return e.err }

func TestEffectiveValid(t *testing.T) {
	store := presets.NewStore(t.TempDir())
	p, _ := store.Create("P")
	s := &Server{schema: testSchema(t), presets: store, tpl: editorTemplates(t), runner: effRunner{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /settings/preset/{id}/effective", s.handlePresetEffective)

	form := url.Values{}
	form.Set("document.images.jpeg_quality_level", "40")
	req := httptest.NewRequest("POST", "/settings/preset/"+p.ID+"/effective", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="validity valid"`) {
		t.Fatalf("expected valid chip: %s", body)
	}
	if !strings.Contains(body, "1 override") {
		t.Fatalf("override count wrong: %s", body)
	}
	if !strings.Contains(body, "jpeg_quality_level: 40") {
		t.Fatalf("merged YAML missing override: %s", body)
	}
}

func TestEffectiveInvalid(t *testing.T) {
	store := presets.NewStore(t.TempDir())
	p, _ := store.Create("P")
	s := &Server{schema: testSchema(t), presets: store, tpl: editorTemplates(t), runner: effRunner{err: errors.New("bad")}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /settings/preset/{id}/effective", s.handlePresetEffective)

	form := url.Values{}
	form.Set("document.images.jpeg_quality_level", "40")
	req := httptest.NewRequest("POST", "/settings/preset/"+p.ID+"/effective", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `class="validity invalid"`) {
		t.Fatalf("expected invalid chip: %s", rec.Body.String())
	}
}

func TestPresetEditorShowsEffectiveOnLoad(t *testing.T) {
	store := presets.NewStore(t.TempDir())
	p, _ := store.Create("P")
	p.Overrides = map[string]any{"document": map[string]any{"images": map[string]any{"jpeg_quality_level": 40}}}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	s := &Server{schema: testSchema(t), presets: store, tpl: editorTemplates(t), runner: effRunner{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings/preset/{id}", s.handlePresetEditor)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings/preset/"+p.ID, nil))
	if !strings.Contains(rec.Body.String(), `class="validity valid"`) {
		t.Fatalf("effective pane not populated on load: %s", rec.Body.String())
	}
}
