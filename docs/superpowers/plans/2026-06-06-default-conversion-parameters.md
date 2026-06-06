# Default Conversion Parameters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the web converter apply four product defaults to every conversion — footnotes `floatRenumbered`, soft hyphens on, cover generation on, drop caps on — while leaving each one overridable by the UI form and by raw YAML.

**Architecture:** Introduce an application-defaults config layer in `internal/convert`. `BuildConfig` now layers config lowest-to-highest precedence: **fbc embedded defaults < app defaults < user raw YAML < form fields**, via a small recursive `deepMerge`. `BuildConfig` always returns a non-nil config (no more "send nothing, let fbc decide"), so defaults reach even non-browser (curl/API) clients. The HTML form is pre-filled/pre-checked to mirror the defaults; default-on checkboxes send an explicit `true`/`false` so a user can actually turn them off (an omitted field falls back to the app default).

**Tech Stack:** Go 1.26.3, `gopkg.in/yaml.v3`, Go standard `testing`. Frontend is plain HTML + vanilla JS served from an embedded FS. External `fbc` (github.com/rupor-github/fb2cng) consumes the YAML via `-c`.

---

## Background: how config flows today

- `internal/convert/config_yaml.go` — `FormOptions` (tri-state pointers; `nil` = "user didn't set it") and `BuildConfig(rawYAML, FormOptions) ([]byte, error)`. Today it returns `(nil, nil)` when nothing is set, and the handler then passes no `-c`, so fbc uses its own embedded defaults. Form values win over raw YAML; `nil` form fields are left untouched.
- `internal/server/handlers.go` — `formOptions(r)` reads form fields into `FormOptions`; `handleConvert` calls `BuildConfig`, writes `config.yaml` only `if cfgBytes != nil`, and passes `-c <path>` to the runner.
- `internal/web/index.html` — the Settings `<details>` form (selects + checkboxes).
- `internal/web/app.js` — `buildForm(file)` assembles the `FormData`; `optionalField(form, key, el, kind)` omits empty values and, for `kind === 'check'`, sends `'true'` only when checked (omits when unchecked).
- `internal/convert/runner.go` — `Runner` interface: `DumpDefaults(ctx)` and `Convert(ctx, inPath, format, cfgPath, destDir)`.

The fbc config keys (confirmed against the fb2cng user guide):

```yaml
version: 1
document:
  footnotes:
    mode: default | float | floatRenumbered
  insert_soft_hyphen: true | false
  dropcaps:
    enable: true | false          # fbc default already true
  images:
    cover:
      generate: true | false      # fbc default already true
```

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `internal/convert/config_yaml.go` | `FormOptions`, `applicationDefaults`, `deepMerge`, `BuildConfig` | Modify: add 2 fields, add defaults layer + merge, drop `empty()` / `(nil,nil)` shortcut, wire cover + dropcaps |
| `internal/convert/config_yaml_test.go` | Unit tests for the config builder | Modify: replace the empty-case test, add defaults / override / merge tests |
| `internal/server/handlers.go` | `formOptions` form parsing | Modify: parse `cover_generate`, `dropcaps_enable` |
| `internal/server/handlers_test.go` | Handler tests | Modify: add a capturing runner + 2 end-to-end config tests |
| `internal/web/index.html` | Settings form | Modify: pre-select footnotes, pre-check soft hyphen, add cover + dropcaps checkboxes |
| `internal/web/app.js` | Form assembly | Modify: add `booleanField`, send explicit booleans for the 3 default-on checkboxes |

No new files. No new dependencies.

---

### Task 1: `deepMerge` helper

**Files:**
- Modify: `internal/convert/config_yaml.go` (add function)
- Test: `internal/convert/config_yaml_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/convert/config_yaml_test.go`:

```go
func TestDeepMerge(t *testing.T) {
	dst := map[string]any{
		"version": 1,
		"document": map[string]any{
			"insert_soft_hyphen": true,
			"footnotes":          map[string]any{"mode": "floatRenumbered"},
		},
	}
	src := map[string]any{
		"document": map[string]any{
			"insert_soft_hyphen": false,    // scalar override
			"toc_type":           "flat",   // new key
		},
	}
	deepMerge(dst, src)

	doc := dst["document"].(map[string]any)
	if doc["insert_soft_hyphen"] != false {
		t.Errorf("src should override scalar, got %v", doc["insert_soft_hyphen"])
	}
	if doc["toc_type"] != "flat" {
		t.Errorf("src key should be added, got %v", doc["toc_type"])
	}
	fn := doc["footnotes"].(map[string]any)
	if fn["mode"] != "floatRenumbered" {
		t.Errorf("untouched nested key should be preserved, got %v", fn["mode"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/convert/ -run TestDeepMerge -v`
Expected: FAIL — `undefined: deepMerge` (compile error).

- [ ] **Step 3: Write minimal implementation**

Add to `internal/convert/config_yaml.go` (next to the existing `child` helper):

```go
// deepMerge recursively merges src into dst. When both sides hold a nested map,
// it merges them key by key; otherwise src's value replaces dst's. dst is
// mutated in place.
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
		}
		dst[k] = sv
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/convert/ -run TestDeepMerge -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/convert/config_yaml.go internal/convert/config_yaml_test.go
git commit -m "feat(convert): add deepMerge for layered config"
```

---

### Task 2: Application defaults + `BuildConfig` rewrite

**Files:**
- Modify: `internal/convert/config_yaml.go:10-69`
- Test: `internal/convert/config_yaml_test.go`

This task adds the `CoverGenerate` and `DropcapsEnable` fields, the `applicationDefaults` baseline, layers defaults under raw-YAML-under-form, removes the `empty()` shortcut so a config is always produced, and wires the two new fields into the YAML tree.

- [ ] **Step 1: Update the failing tests**

In `internal/convert/config_yaml_test.go`, **replace** `TestBuildConfigEmpty` (lines 10–18) with:

```go
func TestBuildConfigAppliesDefaults(t *testing.T) {
	out, err := BuildConfig("", FormOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected defaults config, got nil")
	}
	s := string(out)
	for _, want := range []string{
		"version: 1",
		"mode: floatRenumbered",
		"insert_soft_hyphen: true",
		"generate: true", // document.images.cover.generate
		"enable: true",   // document.dropcaps.enable
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}
```

Then **add** these tests at the end of the file:

```go
func TestBuildConfigCoverAndDropcapsFormOverride(t *testing.T) {
	out, err := BuildConfig("", FormOptions{
		CoverGenerate:  ptr(false),
		DropcapsEnable: ptr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"generate: false", "enable: false"} {
		if !strings.Contains(s, want) {
			t.Errorf("form should override default, missing %q in:\n%s", want, s)
		}
	}
}

func TestBuildConfigRawOverridesDefault(t *testing.T) {
	raw := "document:\n  footnotes:\n    mode: float\n"
	out, err := BuildConfig(raw, FormOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// raw "float" must win over the "floatRenumbered" default; guard against the
	// substring trap ("mode: float" is a prefix of "mode: floatRenumbered").
	if !strings.Contains(s, "mode: float") || strings.Contains(s, "mode: floatRenumbered") {
		t.Errorf("raw YAML should override the footnotes default, got:\n%s", s)
	}
	if !strings.Contains(s, "generate: true") {
		t.Errorf("untouched cover default should remain, got:\n%s", s)
	}
}
```

> Note: existing `TestBuildConfigFormWinsOverRaw` still passes — its raw sets `insert_soft_hyphen: false` and no form field touches it, so raw (`false`) beats the app default (`true`). `TestBuildConfigFormOnly`, `TestBuildConfigRawOnlyEnsuresVersion`, `TestBuildConfigFootnotesMode`, and `TestBuildConfigInvalidYAML` all use `strings.Contains` and remain valid.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/convert/ -run 'TestBuildConfig' -v`
Expected: FAIL — `TestBuildConfigAppliesDefaults` (got nil), `TestBuildConfigCoverAndDropcapsFormOverride` (unknown fields `CoverGenerate`/`DropcapsEnable` → compile error).

- [ ] **Step 3: Write the implementation**

In `internal/convert/config_yaml.go`, **replace** the `FormOptions` struct, the `empty()` method, and `BuildConfig` (current lines 10–69) with:

```go
// FormOptions are the common knobs the web form exposes. A nil pointer means
// "not set by the user"; BuildConfig then falls back to the application default
// (see applicationDefaults), then to the raw YAML, then to fbc's own default.
type FormOptions struct {
	TocType          *string // document.toc_type
	ImagesOptimize   *bool   // document.images.optimize
	JpegQuality      *int    // document.images.jpeg_quality_level
	FootnotesMode    *string // document.footnotes.mode
	InsertSoftHyphen *bool   // document.insert_soft_hyphen
	CoverGenerate    *bool   // document.images.cover.generate
	DropcapsEnable   *bool   // document.dropcaps.enable
}

// applicationDefaults is the web app's baseline config. It sits above fbc's
// embedded defaults and below the user's raw YAML and form fields, so any of
// those can override it. These are the product defaults we always apply.
func applicationDefaults() map[string]any {
	return map[string]any{
		"version": 1,
		"document": map[string]any{
			"footnotes":          map[string]any{"mode": "floatRenumbered"},
			"insert_soft_hyphen": true,
			"dropcaps":           map[string]any{"enable": true},
			"images":             map[string]any{"cover": map[string]any{"generate": true}},
		},
	}
}

// BuildConfig returns the effective fbc config (for -c) by layering, lowest to
// highest precedence: application defaults, the user's raw YAML, then the form
// fields. It always returns a non-nil config so our defaults are applied even
// when the user supplies nothing.
func BuildConfig(rawYAML string, o FormOptions) ([]byte, error) {
	root := applicationDefaults()

	if strings.TrimSpace(rawYAML) != "" {
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(rawYAML), &raw); err != nil {
			return nil, fmt.Errorf("parse config YAML: %w", err)
		}
		deepMerge(root, raw)
	}

	doc := child(root, "document")
	if o.TocType != nil {
		doc["toc_type"] = *o.TocType
	}
	if o.InsertSoftHyphen != nil {
		doc["insert_soft_hyphen"] = *o.InsertSoftHyphen
	}
	if o.ImagesOptimize != nil || o.JpegQuality != nil {
		img := child(doc, "images")
		if o.ImagesOptimize != nil {
			img["optimize"] = *o.ImagesOptimize
		}
		if o.JpegQuality != nil {
			img["jpeg_quality_level"] = *o.JpegQuality
		}
	}
	if o.FootnotesMode != nil {
		child(doc, "footnotes")["mode"] = *o.FootnotesMode
	}
	if o.CoverGenerate != nil {
		child(child(doc, "images"), "cover")["generate"] = *o.CoverGenerate
	}
	if o.DropcapsEnable != nil {
		child(doc, "dropcaps")["enable"] = *o.DropcapsEnable
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal config YAML: %w", err)
	}
	return out, nil
}
```

Leave the `child` helper (and the `deepMerge` from Task 1) below this, unchanged.

- [ ] **Step 4: Run the full convert package tests**

Run: `go test ./internal/convert/ -v`
Expected: PASS for all (the smoke test is build-tagged and excluded without `-tags smoke`).

- [ ] **Step 5: Commit**

```bash
git add internal/convert/config_yaml.go internal/convert/config_yaml_test.go
git commit -m "feat(convert): apply app defaults for footnotes, soft hyphen, cover, dropcaps"
```

---

### Task 3: Wire the new form fields in the handler

**Files:**
- Modify: `internal/server/handlers.go:137-160`
- Test: `internal/server/handlers_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/server/handlers_test.go` (the package already imports `os`, `path/filepath`, `strings`, `context`):

```go
// capturingRunner records the config bytes that handleConvert writes and passes
// via -c, so we can assert the effective config end-to-end.
type capturingRunner struct {
	cfgBytes []byte
}

func (c *capturingRunner) DumpDefaults(context.Context) ([]byte, error) { return nil, nil }
func (c *capturingRunner) Convert(_ context.Context, _, _, cfgPath, dest string) ([]string, error) {
	if cfgPath != "" {
		c.cfgBytes, _ = os.ReadFile(cfgPath)
	}
	os.MkdirAll(dest, 0o755)
	out := filepath.Join(dest, "x.epub")
	os.WriteFile(out, []byte("x"), 0o644)
	return []string{out}, nil
}

func TestConvertWritesDefaultConfig(t *testing.T) {
	cr := &capturingRunner{}
	h := newTestServer(t, config.Config{MaxConcurrent: 1}, cr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3"}))
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	s := string(cr.cfgBytes)
	for _, want := range []string{"mode: floatRenumbered", "insert_soft_hyphen: true", "generate: true", "enable: true"} {
		if !strings.Contains(s, want) {
			t.Errorf("default config not written, missing %q in:\n%s", want, s)
		}
	}
}

func TestConvertCheckboxFalseDisablesDefault(t *testing.T) {
	cr := &capturingRunner{}
	h := newTestServer(t, config.Config{MaxConcurrent: 1}, cr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{
		"format":          "epub3",
		"cover_generate":  "false",
		"dropcaps_enable": "false",
	}))
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	s := string(cr.cfgBytes)
	if !strings.Contains(s, "generate: false") || !strings.Contains(s, "enable: false") {
		t.Errorf("explicit false should disable, got:\n%s", s)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestConvert(WritesDefaultConfig|CheckboxFalseDisablesDefault)' -v`
Expected: FAIL — `TestConvertCheckboxFalseDisablesDefault` (handler ignores `cover_generate`/`dropcaps_enable`, so `generate`/`enable` stay `true`).

> `TestConvertWritesDefaultConfig` may already pass once Task 2 is in (defaults are written regardless of the handler change) — that is fine; it locks the behavior in.

- [ ] **Step 3: Write the implementation**

In `internal/server/handlers.go`, in `formOptions`, add the two blocks after the `insert_soft_hyphen` block (after current line 153, before the `jpeg_quality` block):

```go
	if v := r.FormValue("cover_generate"); v != "" {
		b := v == "true"
		o.CoverGenerate = &b
	}
	if v := r.FormValue("dropcaps_enable"); v != "" {
		b := v == "true"
		o.DropcapsEnable = &b
	}
```

- [ ] **Step 4: Run the full server package tests**

Run: `go test ./internal/server/ -v`
Expected: PASS for all.

- [ ] **Step 5: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go
git commit -m "feat(server): parse cover_generate and dropcaps_enable form fields"
```

---

### Task 4: Pre-fill the Settings form (HTML)

**Files:**
- Modify: `internal/web/index.html:69-80`

No automated unit test (static HTML). The existing `internal/server/static_test.go` continues to verify the page is served and embedded; a manual checklist is in Task 6.

- [ ] **Step 1: Pre-select the footnotes default**

In `internal/web/index.html`, replace the footnotes `<select>` block (lines 69–76) with:

```html
      <label>Footnotes
        <select id="footnotes_mode">
          <option value="">(default)</option>
          <option value="default">default</option>
          <option value="float">float</option>
          <option value="floatRenumbered" selected>floatRenumbered</option>
        </select>
      </label>
```

- [ ] **Step 2: Pre-check soft hyphens and add the two new checkboxes**

Replace the soft-hyphen line (line 80):

```html
      <label><input id="insert_soft_hyphen" type="checkbox"> Insert soft hyphens</label>
```

with:

```html
      <label><input id="insert_soft_hyphen" type="checkbox" checked> Insert soft hyphens</label>
      <label><input id="cover_generate" type="checkbox" checked> Generate cover</label>
      <label><input id="dropcaps_enable" type="checkbox" checked> Drop caps</label>
```

- [ ] **Step 3: Verify the server tests still pass (page still serves)**

Run: `go test ./internal/server/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/web/index.html
git commit -m "feat(web): pre-fill default footnotes, soft hyphen, cover, dropcaps in form"
```

---

### Task 5: Send explicit booleans for default-on checkboxes (JS)

**Files:**
- Modify: `internal/web/app.js:51-70`

The current `optionalField(..., 'check')` omits a checkbox value when unchecked. With a default-on backend, an omitted field falls back to the default (`true`), so the user could never turn cover/dropcaps/soft-hyphen **off**. Default-on checkboxes must send an explicit `'true'`/`'false'`.

- [ ] **Step 1: Add the `booleanField` helper**

In `internal/web/app.js`, add directly after the existing `optionalField` function (after line 56):

```js
// booleanField always sends true/false so a pre-checked (default-on) box can be
// turned off. Omitting it would let the server-side default re-apply.
function booleanField(form, key, el) {
  form.append(key, el.checked ? 'true' : 'false');
}
```

- [ ] **Step 2: Wire the three default-on checkboxes**

In `buildForm`, replace the soft-hyphen line (line 66):

```js
  optionalField(form, 'insert_soft_hyphen', $('insert_soft_hyphen'), 'check');
```

with:

```js
  booleanField(form, 'insert_soft_hyphen', $('insert_soft_hyphen'));
  booleanField(form, 'cover_generate', $('cover_generate'));
  booleanField(form, 'dropcaps_enable', $('dropcaps_enable'));
```

Leave `images_optimize` on `optionalField(..., 'check')` — it is a default-**off** knob with no app default, so omit-when-unchecked is still correct.

- [ ] **Step 3: Commit**

```bash
git add internal/web/app.js
git commit -m "feat(web): send explicit booleans for cover, dropcaps, soft hyphen"
```

---

### Task 6: Full verification

**Files:** none (verification + manual UI check)

- [ ] **Step 1: Format, vet, and test the whole module**

Run:
```bash
gofmt -l internal/ && go vet ./... && go test ./...
```
Expected: `gofmt -l` prints nothing (all formatted); `go vet` is silent; `go test ./...` reports `ok` for `internal/config`, `internal/convert`, `internal/server`.

- [ ] **Step 2: (Optional) run the smoke test if a real fbc is available**

Run: `FBC_BIN=$(which fbc) go test -tags smoke -run TestSmoke ./internal/convert/ -v`
Expected: PASS if `fbc` is installed; skip otherwise (no fbc binary on this machine — this is informational only).

- [ ] **Step 3: Manual UI checklist**

Run the server: `go run . ` (defaults to `:8080`, `FBC_BIN=fbc`), open `http://localhost:8080`, expand **Settings**, and confirm:

1. **Footnotes** dropdown shows `floatRenumbered` selected.
2. **Insert soft hyphens**, **Generate cover**, and **Drop caps** checkboxes are all checked.
3. With a real fbc available, convert a sample `.fb2` with defaults — succeeds.
4. Uncheck **Generate cover** and **Drop caps**, convert again — succeeds (verifies the explicit-`false` path; cross-check against `TestConvertCheckboxFalseDisablesDefault` which already asserts the config bytes).

- [ ] **Step 4: Final no-op commit guard**

Confirm a clean tree:
```bash
git status --porcelain
```
Expected: empty (everything committed across Tasks 1–5).

---

## Self-Review

**1. Spec coverage:**
- Footnotes → `floatRenumbered`: app default (Task 2) + UI pre-select (Task 4). ✓
- Insert soft hyphens → `true`: app default (Task 2) + pre-checked + explicit boolean (Tasks 4, 5). ✓
- Generate cover → `true`: new `CoverGenerate` field + `document.images.cover.generate` (Task 2), handler parse (Task 3), checkbox (Tasks 4, 5). ✓
- Drop caps → `true` (added mid-session): new `DropcapsEnable` field + `document.dropcaps.enable` (Task 2), handler parse (Task 3), checkbox (Tasks 4, 5). ✓
- "Backend + UI" scope choice: backend defaults always applied (no more `(nil,nil)` shortcut) **and** UI mirrors them. ✓
- Overridability preserved: raw YAML beats defaults (`TestBuildConfigRawOverridesDefault`); form beats raw (`TestBuildConfigFormWinsOverRaw`); explicit `false` from a checkbox disables a default (`TestConvertCheckboxFalseDisablesDefault`). ✓

**2. Placeholder scan:** No `TBD`/`TODO`/"handle edge cases"/"similar to". Every code step shows complete code; every run step gives an exact command and expected result.

**3. Type consistency:** `FormOptions.CoverGenerate`/`DropcapsEnable` are `*bool`, set in `formOptions` and read in `BuildConfig` with identical names. YAML keys match the fb2cng guide: `document.images.cover.generate`, `document.dropcaps.enable`, `document.footnotes.mode`, `document.insert_soft_hyphen`. JS keys (`cover_generate`, `dropcaps_enable`, `insert_soft_hyphen`) match the `r.FormValue(...)` keys in `formOptions`. `deepMerge` signature is the same in Task 1's definition and Task 2's use. `Runner.Convert` signature in `capturingRunner` matches the interface (`ctx, inPath, format, cfgPath, destDir`).
