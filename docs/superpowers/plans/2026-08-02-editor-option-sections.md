# Editor Option Sub-Sections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split each editor option group into collapsible sub-sections derived from the option key path, so `document.images.optimize` lives under an `Images` section instead of loose in a flat 45-row `document` accordion.

**Architecture:** Purely presentational. `buildEditorVM` partitions each group's rows into `sectionVM`s using the 2nd key segment (`document.images.optimize` → `images`; bare `document.fix_zip` → `general`). The editor template renders each section as a nested `<details>`; groups with a single section render rows flat. Inline JS extends search/changed-only filtering and change-counting to sections. CSS styles the nested header. No schema changes, no new dependencies.

**Tech Stack:** Go, `html/template` (stdlib), plain CSS, vanilla JS inline in the template.

## Global Constraints

- No new dependencies — stdlib only.
- Sparse-override save path (`collectOverrides`/`unflatten`/`handlePresetSave`) is untouched; this change is read/render-only.
- Existing test assertions in `internal/server/editor_test.go` must stay green (row markup, `data-path`, group count string `N options · <span class="grp-changed">C</span> changed`, `<b>X</b> of Y changed`).
- Section labels are prettified (`_`→space, first letter upper). Group names stay raw.
- Sub-sections default collapsed (no `open` attribute).

## File Structure

- `internal/server/editor.go` — add `sectionVM`, add `Sections`/`Flat` to `groupVM`, partition rows into sections in `finalize`, section-name + prettify helpers.
- `internal/server/editor_test.go` — unit test for section partitioning; server-render test asserting nested section markup.
- `internal/web/templates/editor.gohtml` — extract row markup to `{{define "optrow"}}`; nested section rendering; section-aware JS.
- `internal/web/static/app.css` — `.opt-section*` styles.
- `internal/web/templates_test.go` — parse guard for new defines; CSS guard for section style.

---

### Task 1: Section data model + partitioning (Go)

**Files:**
- Modify: `internal/server/editor.go`
- Test: `internal/server/editor_test.go`

**Interfaces:**
- Consumes: existing `rowVM`, `groupVM`, `groupSet`, `(*Server).buildEditorVM(p *presets.Preset, flat map[string]any) editorVM`.
- Produces:
  - `sectionVM{ Name string; Label string; Total int; Changed int; Rows []rowVM }`
  - `groupVM` gains `Sections []sectionVM` and `Flat bool`; `Rows []rowVM` remains as build-time scratch (set to `nil` after partition).
  - `sectionName(key string) string` — `len(parts)>=3` → `parts[1]`, else `"general"`.
  - `prettify(name string) string` — `_`→space, upper first byte.

- [ ] **Step 1: Write the failing test**

Add to `internal/server/editor_test.go`:

```go
func TestBuildEditorVMSections(t *testing.T) {
	s := &Server{schema: &schema.Schema{Options: []schema.Option{
		{Key: "document.toc_type", Group: "document", Label: "toc_type", Kind: schema.KindString, Default: "normal"},
		{Key: "document.images.optimize", Group: "document", Label: "optimize", Kind: schema.KindBool, Default: true},
		{Key: "document.images.jpeg_quality_level", Group: "document", Label: "jpeg_quality_level", Kind: schema.KindInt, Default: float64(75)},
		{Key: "version", Group: "version", Label: "version", Kind: schema.KindString, Default: "2.0"},
	}}}
	vm := s.buildEditorVM(&presets.Preset{}, map[string]any{
		"document": map[string]any{"images": map[string]any{"jpeg_quality_level": 40}},
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestBuildEditorVMSections`
Expected: FAIL — `groupVM` has no field `Sections`/`Flat` (compile error).

- [ ] **Step 3: Add the section types and helpers**

In `internal/server/editor.go`, replace the `groupVM` struct with:

```go
type sectionVM struct {
	Name    string // raw key segment; used for data-section and search
	Label   string // prettified header
	Total   int
	Changed int
	Rows    []rowVM
}

type groupVM struct {
	Name     string
	Total    int
	Changed  int
	Flat     bool // len(Sections) == 1 -> template renders rows directly
	Rows     []rowVM // build-time scratch; partitioned into Sections in finalize
	Sections []sectionVM
}
```

Add these helpers near `finalize`:

```go
// sectionName derives a group's sub-section from an option key: the 2nd path
// segment when the key nests (document.images.optimize -> "images"), else
// "general" for bare group options (document.fix_zip -> "general").
func sectionName(key string) string {
	parts := strings.Split(key, ".")
	if len(parts) >= 3 {
		return parts[1]
	}
	return "general"
}

// prettify turns a raw section segment into a header label:
// "text_transformations" -> "Text transformations".
func prettify(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// partition splits a group's rows into sections, preserving first-seen order.
func partition(rows []rowVM) []sectionVM {
	var order []string
	byName := map[string]*sectionVM{}
	for _, r := range rows {
		name := sectionName(r.Key)
		sec, ok := byName[name]
		if !ok {
			sec = &sectionVM{Name: name, Label: prettify(name)}
			byName[name] = sec
			order = append(order, name)
		}
		sec.Rows = append(sec.Rows, r)
	}
	out := make([]sectionVM, 0, len(order))
	for _, n := range order {
		sec := byName[n]
		sec.Total = len(sec.Rows)
		for _, r := range sec.Rows {
			if r.Changed {
				sec.Changed++
			}
		}
		out = append(out, *sec)
	}
	return out
}
```

- [ ] **Step 4: Partition rows in `finalize`**

Replace the body of `finalize` (the `for _, name := range gs.order` loop) with:

```go
func (gs *groupSet) finalize(p *presets.Preset, totalOptions int) editorVM {
	vm := editorVM{Preset: p, TotalOptions: totalOptions}
	for _, name := range gs.order {
		g := gs.byName[name]
		g.Sections = partition(g.Rows)
		g.Rows = nil
		g.Flat = len(g.Sections) == 1
		for _, sec := range g.Sections {
			g.Total += sec.Total
			g.Changed += sec.Changed
			vm.ChangedCount += sec.Changed
		}
		vm.Groups = append(vm.Groups, *g)
	}
	return vm
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestBuildEditorVMSections`
Expected: PASS.

- [ ] **Step 6: Confirm no regression in the package (template still references `.Rows` — expect failures here, fixed in Task 2)**

Run: `go build ./... && go vet ./internal/server/`
Expected: build + vet PASS. (The template is validated at render time, not build time; render tests are fixed in Task 2. If `go test ./internal/server/` is run now, render tests fail because `editor.gohtml` still ranges `.Rows` — that is expected and resolved in Task 2.)

- [ ] **Step 7: Commit**

```bash
git add internal/server/editor.go internal/server/editor_test.go
git commit -m "feat(editor): partition option groups into key-derived sections"
```

---

### Task 2: Nested section rendering + section-aware JS (template)

**Files:**
- Modify: `internal/web/templates/editor.gohtml`
- Test: `internal/server/editor_test.go` (add render test), `internal/web/templates_test.go` (parse guard)

**Interfaces:**
- Consumes: `groupVM.Sections []sectionVM`, `groupVM.Flat bool`, `sectionVM{Name,Label,Total,Changed,Rows}` from Task 1.
- Produces: `{{define "optrow"}}` partial rendering one `rowVM`; nested `<details class="opt-section" data-section="...">` markup; `.sec-changed` badge span.

- [ ] **Step 1: Write the failing render test**

Add to `internal/server/editor_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestPresetEditorRendersSections`
Expected: FAIL — template still ranges `.Rows`; either an exec error or the `opt-section` string is absent.

- [ ] **Step 3: Extract the row markup into an `optrow` partial**

In `internal/web/templates/editor.gohtml`, add this define block at the top of the file, immediately after `{{define "editor"}}` line's block is fine — place it just before the final `{{end}}` of the file is NOT ok; put it right after line 1. Concretely, insert directly below `{{define "editor"}}`:

```gotemplate
{{define "optrow"}}
{{$row := .}}
<div class="option-row{{if .Changed}} is-changed{{end}}{{if .IsTemplate}} is-wide{{end}}" data-path="{{.Key}}">
  <span class="option-key">{{.Label}}{{if .Description}}<button type="button" class="help" aria-label="{{.Label}} — help"><span class="help-tip" role="tooltip">{{.Description}}</span>?</button>{{end}}<span class="changed-dot"> •</span></span>
  <div class="option-control">
    {{if eq .Kind "bool"}}
      <input type="hidden" name="{{.Key}}" value="false">
      <input type="checkbox" id="opt-{{.Key}}" name="{{.Key}}" value="true" data-default="{{.DefaultStr}}" aria-label="{{.Label}}" {{if .Checked}}checked{{end}}>
    {{else if eq .Kind "int"}}
      <input type="number" id="opt-{{.Key}}" name="{{.Key}}" value="{{.StrValue}}" data-default="{{.DefaultStr}}" aria-label="{{.Label}}">
    {{else if eq .Kind "enum"}}
      <select id="opt-{{.Key}}" name="{{.Key}}" data-default="{{.DefaultStr}}" aria-label="{{.Label}}">
        {{range .Enum}}<option value="{{.}}"{{if eq $row.StrValue .}} selected{{end}}>{{.}}</option>{{end}}
      </select>
    {{else if .IsTemplate}}
      <div class="template-wrap">
        <textarea id="opt-{{.Key}}" name="{{.Key}}" rows="3" class="template-input" data-default="{{.DefaultStr}}" aria-label="{{.Label}}">{{.StrValue}}</textarea>
        {{if eq .Label "output_name_template"}}<div class="filename-preview" data-for="opt-{{.Key}}"></div>{{end}}
      </div>
    {{else}}
      <input type="text" id="opt-{{.Key}}" name="{{.Key}}" value="{{.StrValue}}" data-default="{{.DefaultStr}}" aria-label="{{.Label}}">
    {{end}}
  </div>
  <span class="option-meta">{{if and .Known (not .IsTemplate)}}<span class="option-default">default {{.DefaultStr}}</span>{{end}}<a class="reset" href="#" data-target="opt-{{.Key}}">reset</a></span>
</div>
{{end}}
```

This is the exact markup lifted verbatim from the current inline row block (former lines 33–55), with `{{$row := .}}` moved to the top of the partial.

- [ ] **Step 4: Replace the group loop body to render sections**

In the same file, replace the `.option-list` loop (the `{{range .Groups}} … {{end}}` block that currently renders `<details class="opt-group">` with an inline `{{range .Rows}}`) with:

```gotemplate
{{range .Groups}}
<details class="opt-group{{if gt .Changed 0}} has-changes{{end}}" data-group="{{.Name}}">
  <summary class="opt-group-sum">
    <span class="caret">▸</span>
    <span class="opt-group-name">{{.Name}}</span>
    <span class="opt-group-count">{{.Total}} options · <span class="grp-changed">{{.Changed}}</span> changed</span>
  </summary>
  {{if .Flat}}
    {{range (index .Sections 0).Rows}}{{template "optrow" .}}{{end}}
  {{else}}
    {{range .Sections}}
    <details class="opt-section{{if gt .Changed 0}} has-changes{{end}}" data-section="{{.Name}}">
      <summary class="opt-section-sum">
        <span class="caret">▸</span>
        <span class="opt-section-name">{{.Label}}</span>
        <span class="opt-section-count">{{.Total}} · <span class="sec-changed">{{.Changed}}</span> changed</span>
      </summary>
      {{range .Rows}}{{template "optrow" .}}{{end}}
    </details>
    {{end}}
  {{end}}
</details>
{{end}}
```

Note: the group `<summary>` keeps the exact `{{.Total}} options · <span class="grp-changed">{{.Changed}}</span> changed` string so `TestPresetEditorRendersGrid` stays green.

- [ ] **Step 5: Make the JS filter and counter section-aware**

In the same file's `<script>`, replace the `apply` function with:

```javascript
  function apply() {
    var q = (search && search.value || '').toLowerCase();
    var only = changedOnly && changedOnly.checked;
    var filtering = q !== '' || only;
    groups.forEach(function (g) {
      var gShown = 0;
      g.querySelectorAll('.option-row').forEach(function (row) {
        var path = (row.getAttribute('data-path') || '').toLowerCase();
        var match = path.indexOf(q) !== -1;
        if (only && !row.classList.contains('is-changed')) match = false;
        row.style.display = match ? '' : 'none';
        if (match) gShown++;
      });
      g.querySelectorAll('.opt-section').forEach(function (sec) {
        var visible = Array.prototype.some.call(
          sec.querySelectorAll('.option-row'),
          function (r) { return r.style.display !== 'none'; });
        sec.style.display = visible ? '' : 'none';
        if (filtering && visible) sec.open = true;
      });
      g.style.display = gShown ? '' : 'none';
      if (filtering && gShown) g.open = true;
    });
  }
```

And replace the `recount` function with:

```javascript
  function recount() {
    var total = 0;
    groups.forEach(function (g) {
      var c = g.querySelectorAll('.option-row.is-changed').length;
      total += c;
      var badge = g.querySelector('.grp-changed');
      if (badge) badge.textContent = c;
      g.classList.toggle('has-changes', c > 0);
      g.querySelectorAll('.opt-section').forEach(function (sec) {
        var sc = sec.querySelectorAll('.option-row.is-changed').length;
        var sb = sec.querySelector('.sec-changed');
        if (sb) sb.textContent = sc;
        sec.classList.toggle('has-changes', sc > 0);
      });
    });
    if (summary) summary.textContent = total;
    if (changedOnly && changedOnly.checked) apply();
  }
```

(The `collapse-all` handler stays as-is — it toggles top-level `.opt-group`s only.)

- [ ] **Step 6: Run the render + existing editor tests**

Run: `go test ./internal/server/`
Expected: PASS — `TestPresetEditorRendersSections`, `TestPresetEditorRendersGrid`, and all save tests green.

- [ ] **Step 7: Add a parse guard for the new defines**

In `internal/web/templates_test.go`, extend the loop in `TestTemplatesParse`:

```go
	for _, name := range []string{"base", "convert", "convert_card", "editor", "optrow"} {
```

- [ ] **Step 8: Run the web package tests**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/web/templates/editor.gohtml internal/server/editor_test.go internal/web/templates_test.go
git commit -m "feat(editor): render option sections as nested collapsible details"
```

---

### Task 3: Section CSS

**Files:**
- Modify: `internal/web/static/app.css`
- Test: `internal/web/templates_test.go` (`TestAppCSSCarriesTokens` guard)

**Interfaces:**
- Consumes: markup classes `.opt-section`, `.opt-section-sum`, `.opt-section-name`, `.opt-section-count`, `.sec-changed`, and the existing `.caret` from Task 2.
- Produces: nested section styling; no JS/Go interface.

- [ ] **Step 1: Add the failing CSS guard**

In `internal/web/templates_test.go`, add `".opt-section-sum"` to the token slice in `TestAppCSSCarriesTokens`:

```go
		"[data-theme=",
		".opt-section-sum",
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/web/ -run TestAppCSSCarriesTokens`
Expected: FAIL — `app.css missing ".opt-section-sum"`.

- [ ] **Step 3: Add the section styles**

In `internal/web/static/app.css`, immediately after the `.opt-group.has-changes …` rules (around line 270), add:

```css
.opt-section { border-top: 1px solid var(--line); }
.opt-section-sum {
  display: flex; gap: 8px; align-items: center;
  padding: 8px 20px 8px 30px; cursor: pointer; list-style: none;
  font-size: 12px; text-transform: uppercase; letter-spacing: .04em;
  color: var(--fg-muted);
}
.opt-section-sum::-webkit-details-marker { display: none; }
.opt-section[open] > .opt-section-sum .caret { transform: rotate(90deg); }
.opt-section-name { color: var(--fg); }
.opt-section-count { font-weight: 400; text-transform: none; letter-spacing: 0; color: var(--fg-muted); }
.opt-section.has-changes > .opt-section-sum { border-left: 3px solid var(--changed); padding-left: 27px; }
.opt-section.has-changes > .opt-section-sum .sec-changed { color: var(--changed); font-weight: 600; }
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/web/ -run TestAppCSSCarriesTokens`
Expected: PASS.

- [ ] **Step 5: Full suite + build**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Manual visual verification (JS has no automated harness)**

Start the server (`go run . ` per README), open a preset editor, and confirm:
- Opening `document` shows collapsed section headers (`General`, `Images`, …).
- Opening `Images` reveals `optimize`.
- Typing `optimize` in search auto-opens the group + `Images` section and hides the rest.
- Toggling `changed only` and editing a value updates both the section `· N changed` badge and the group badge.
- A single-section group (e.g. `version`) renders its rows flat with no nested header.

- [ ] **Step 7: Commit**

```bash
git add internal/web/static/app.css internal/web/templates_test.go
git commit -m "feat(editor): style nested option sections"
```

---

## Self-Review

**Spec coverage:**
- Section-derivation rule → Task 1 (`sectionName`, `partition`), tested.
- `general` bucket + first-seen order → Task 1 test asserts order `general, images`.
- Prettified labels, raw group names → Task 1 `prettify`, Task 2 render test asserts `Images`; group summary keeps raw `{{.Name}}`.
- Data model (`sectionVM`, `Flat`, group totals = sum) → Task 1.
- Nested accordion, collapsed default, `optrow` extraction → Task 2.
- JS `apply`/`recount` section-aware, `collapse-all` group-level → Task 2 Step 5.
- CSS indent + mirrored header + caret reuse → Task 3.
- Tests: partition unit, single-section `Flat`, section render → Tasks 1–2.
- Out-of-scope items (group-name prettify, schema field, deeper nesting) — not implemented, correct.

**Placeholder scan:** none — all steps carry concrete code/commands.

**Type consistency:** `sectionVM`/`groupVM.Sections`/`Flat` defined in Task 1 and consumed identically in Task 2 template and JS class names (`.opt-section`, `.sec-changed`) match across Tasks 2–3.
