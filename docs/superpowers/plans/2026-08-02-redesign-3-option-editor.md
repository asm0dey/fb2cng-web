# Plan 3: Option Editor + Schema — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a checked-in option schema (`internal/schema` + a `go generate` scaffolder `cmd/schemagen`) and replace Plan 2's minimal raw-YAML preset editor with a full per-option grid editor that has type-aware widgets, changed markers, sparse Save, and a live effective-config pane whose validity is confirmed by having `fbc` parse the merged config.

**Architecture:** `cmd/schemagen` reads `fbc dumpconfig --default` YAML, flattens it to dotted keys, infers a `schema.Kind` from each value's Go type, merges best-effort descriptions parsed from `docs/config.md`, and writes `internal/schema/options.json` (printing a drift report vs the previous file), which is embedded into the `schema` package via `//go:embed`. At runtime `schema.Load()` parses that embedded JSON into an ordered, grouped `*schema.Schema` (no path argument, no `SCHEMA_PATH` env — it ships inside the binary like `internal/web` assets). The editor handler (`GET /settings/preset/{id}`) renders every option grouped by `Group` in schema order using `presets.Store` for the current preset; Save (`POST /settings/preset/{id}`) collects field values, drops any equal to the schema default (sparse), and writes `Preset.Overrides` via `presets.Store.Save`; the effective pane (`POST /settings/preset/{id}/effective`, htmx-swapped into `#effective`) merges the current field values over fbc defaults with `convert.BuildConfig` and validates them with a new `convert.Runner.Validate` probe.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, `html/template`, `net/http` (`ServeMux` path values), htmx (CDN), `httptest`, a fake `fbc` (`testdata/fake-fbc.sh`).

## Global Constraints

- **Go 1.26** (`go.mod` declares `go 1.26.3`). Only dependency is `gopkg.in/yaml.v3`.
- **Builds on Plans 1 & 2.** Plan 1 owns `base.gohtml`, `app.css` (incl. the `--changed` token), the `Server` struct + `New(...)` + `Server.Handler()`, the template `embed.FS`, and the `#effective` htmx swap-region convention. Plan 2 owns `internal/presets` (`presets.Store`, `presets.Preset`) and previously registered a *minimal* editor at `GET/POST /settings/preset/{id}` — **this plan replaces that editor.** When Plans 1 & 2 are not yet merged, rely on the frozen signatures in `docs/superpowers/plans/2026-08-02-shared-interfaces.md`; each affected task lists exactly what it consumes.
- **Schema is a checked-in `internal/schema/options.json`**, scaffolded from `fbc dumpconfig --default`. Because the real `fbc` is unavailable in CI, all tests drive `schemagen` against a checked-in fixture dump (`cmd/schemagen/testdata/dump.yaml`), never the real binary.
- **Descriptions are best-effort / incremental.** `docs/config.md` covers only ~25–30 of ~104 options; the rest get blank descriptions. Options absent from `options.json` still render (label = key, no description) so the grid is complete before prose is filled.
- **Validity is checked by having `fbc` parse the merged config** (a `dumpconfig -c <cfg>` probe), not by client-side validation.
- **Self-hosted, low-hardening posture.** Single shared store, no per-user data.
- **Kind values are the exact strings** `"bool"`, `"int"`, `"string"`, `"enum"`. Enum is never inferred by `schemagen` (annotation only).
- **Every commit** ends with the trailer `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.

---

## File Structure

**Created:**
- `internal/schema/schema.go` — `Kind` consts, `Option`, `Option.IsDefault`, `normalize` helper. (Task 1)
- `internal/schema/schema_test.go` — `IsDefault` tests. (Task 1)
- `internal/schema/load.go` — `Schema`, `Load` (no-arg, `//go:embed options.json`), unexported `parse`, `Get`, `Groups`, `InGroup`. (Task 2)
- `internal/schema/load_test.go` — load/get/groups tests. (Task 2)
- `cmd/schemagen/schemagen.go` — flatten/infer/build + descriptions/drift/write. (Tasks 3, 4)
- `cmd/schemagen/main.go` — CLI `main()`. (Task 4)
- `cmd/schemagen/schemagen_test.go` — schemagen unit tests. (Tasks 3, 4)
- `cmd/schemagen/testdata/dump.yaml` — fixture defaults dump. (Task 3)
- `cmd/schemagen/testdata/config.md` — fixture descriptions doc. (Task 4)
- `internal/schema/options.json` — embedded checked-in scaffold (placeholder `[]` committed in Task 2 so the `//go:embed` compiles; regenerated with real content in Task 4). (Tasks 2, 4)
- `internal/convert/runner_validate_test.go` — `Validate` probe tests. (Task 5)
- `internal/server/editor.go` — VM types, flatten/unflatten, `buildEditorVM`, `collectOverrides`, `handlePresetEditor`, `handlePresetSave`. (Tasks 6, 7)
- `internal/server/effective.go` — `computeEffective`, `handlePresetEffective`, `renderEffective`. (Task 8)
- `internal/server/editor_test.go` — editor GET/Save/effective handler tests. (Tasks 6, 7, 8)
- `internal/web/templates/editor.gohtml` — preset-editor **content template** (`{{define "editor"}}`), rendered through Plan 1's `base` layout. (Task 6)
- `internal/web/templates/_effective.gohtml` — effective-config partial (`{{define "_effective"}}`), htmx-swapped into `#effective`. (Task 6)
- `internal/schema/gen.go` — `//go:generate` directive. (Task 9)

**Modified:**
- `internal/convert/runner.go` — add `Validate` to `Runner` interface + `FBC.Validate`. (Task 5)
- `testdata/fake-fbc.sh` — `dumpconfig` branch handles `-c` and rejects a `__invalid` marker. (Task 5)
- `internal/server/handlers_test.go` — existing `stubRunner` gains a `Validate` method. (Task 5)
- `internal/server/server.go` — append concrete field `schema *schema.Schema` to `Server` (Task 6); widen `New(...)` to 7 args + assign, and register the three editor routes in `Handler()` (Task 9).
- `internal/server/editor.go` (Plan 2's minimal editor) — delete `handlePresetEdit`, `handlePresetSave`, `presetEditData`; remove Plan 2's `GET`/`POST /settings/preset/{id}` route registrations and `preset_edit.gohtml`. (Task 9)
- `main.go` — `schema.Load()` wired as the 7th `New(...)` arg (real code). (Task 9)

---

### Task 1: schema types + `IsDefault`

**Files:**
- Create: `internal/schema/schema.go`
- Test: `internal/schema/schema_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  ```go
  type Kind string
  const ( KindBool Kind = "bool"; KindInt Kind = "int"; KindString Kind = "string"; KindEnum Kind = "enum" )
  type Option struct {
      Key         string   `json:"key"`
      Group       string   `json:"group"`
      Label       string   `json:"label"`
      Kind        Kind     `json:"kind"`
      Default     any      `json:"default"`
      Enum        []string `json:"enum,omitempty"`
      Description string   `json:"description,omitempty"`
  }
  func (o Option) IsDefault(val any) bool
  ```

- [ ] **Step 1: Write the failing test**

Create `internal/schema/schema_test.go`:

```go
package schema

import "testing"

func TestIsDefaultBool(t *testing.T) {
	o := Option{Kind: KindBool, Default: false}
	if !o.IsDefault(false) {
		t.Fatal("false should equal default false")
	}
	if o.IsDefault(true) {
		t.Fatal("true should not equal default false")
	}
}

func TestIsDefaultIntAcrossFloat64(t *testing.T) {
	// JSON numbers decode to float64; the editor supplies an int.
	o := Option{Kind: KindInt, Default: float64(75)}
	if !o.IsDefault(75) {
		t.Fatal("int 75 should equal default float64(75)")
	}
	if o.IsDefault(80) {
		t.Fatal("80 should not equal default 75")
	}
}

func TestIsDefaultString(t *testing.T) {
	o := Option{Kind: KindString, Default: "normal"}
	if !o.IsDefault("normal") {
		t.Fatal("string should equal identical default")
	}
	if o.IsDefault("flat") {
		t.Fatal("different string should not equal default")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/schema/ -run TestIsDefault -v`
Expected: FAIL — build error `undefined: Option` / `undefined: KindBool`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/schema/schema.go`:

```go
// Package schema loads the checked-in option schema (options.json) that drives
// the preset editor: which options exist, their type, group, label, default,
// and (best-effort) description.
package schema

import (
	"fmt"
	"strconv"
)

// Kind is the value type of a config option.
type Kind string

const (
	KindBool   Kind = "bool"
	KindInt    Kind = "int"
	KindString Kind = "string"
	KindEnum   Kind = "enum"
)

// Option is one config key.
type Option struct {
	Key         string   `json:"key"`         // dotted path, e.g. "document.images.jpeg_quality_level"
	Group       string   `json:"group"`       // first path segment, e.g. "document"
	Label       string   `json:"label"`       // last path segment
	Kind        Kind     `json:"kind"`
	Default     any      `json:"default"`
	Enum        []string `json:"enum,omitempty"`
	Description string   `json:"description,omitempty"`
}

// IsDefault reports whether val equals the option's default value. It normalizes
// across the numeric types JSON (float64) and form parsing (int) produce, so
// int(75) and float64(75) compare equal.
func (o Option) IsDefault(val any) bool {
	return normalize(o.Default) == normalize(val)
}

func normalize(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/schema/ -run TestIsDefault -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/schema/schema.go internal/schema/schema_test.go
git commit -m "feat(schema): add Option type and IsDefault change-detection

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `Schema` + `Load`/`Get`/`Groups`/`InGroup`

**Files:**
- Create: `internal/schema/load.go`
- Test: `internal/schema/load_test.go`

**Interfaces:**
- Consumes: `Option`, `Kind` (Task 1).
- Produces:
  ```go
  type Schema struct { Options []Option }
  func Load() (*Schema, error)                 // parses the //go:embed-ed options.json (no path/env)
  func parse(data []byte) (*Schema, error)     // unexported; Load = parse(embedded); tests drive this
  func (s *Schema) Get(key string) (Option, bool)
  func (s *Schema) Groups() []string           // group names in first-seen order
  func (s *Schema) InGroup(g string) []Option  // options in group g, schema order
  ```
  `options.json` is a **top-level JSON array** of `Option`, embedded into the binary via `//go:embed options.json`. Because embedding requires the file to exist at compile time, this task also commits a placeholder `internal/schema/options.json` containing `[]`; Task 4 regenerates it with the real scaffold.

- [ ] **Step 1: Write the failing test**

Create `internal/schema/load_test.go`:

```go
package schema

import "testing"

const fixtureJSON = `[
  {"key":"version","group":"version","label":"version","kind":"int","default":1},
  {"key":"document.toc_type","group":"document","label":"toc_type","kind":"string","default":"normal"},
  {"key":"document.images.optimize","group":"document","label":"optimize","kind":"bool","default":true}
]`

func TestParseAndGet(t *testing.T) {
	s, err := parse([]byte(fixtureJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Options) != 3 {
		t.Fatalf("want 3 options, got %d", len(s.Options))
	}
	o, ok := s.Get("document.toc_type")
	if !ok || o.Label != "toc_type" || o.Kind != KindString {
		t.Fatalf("Get(document.toc_type) = %+v ok=%v", o, ok)
	}
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get(nope) should be false")
	}
}

func TestGroupsAndInGroup(t *testing.T) {
	s, err := parse([]byte(fixtureJSON))
	if err != nil {
		t.Fatal(err)
	}
	groups := s.Groups()
	if len(groups) != 2 || groups[0] != "version" || groups[1] != "document" {
		t.Fatalf("groups first-seen order wrong: %v", groups)
	}
	doc := s.InGroup("document")
	if len(doc) != 2 || doc[0].Key != "document.toc_type" {
		t.Fatalf("InGroup(document) = %+v", doc)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := parse([]byte("{not json")); err == nil {
		t.Fatal("parse of invalid JSON should error")
	}
}

// TestLoadEmbedded confirms the //go:embed-ed options.json parses. In Task 2 the
// embedded file is the placeholder `[]` (0 options); after Task 4 regenerates it
// this still passes with the real option count.
func TestLoadEmbedded(t *testing.T) {
	if _, err := Load(); err != nil {
		t.Fatalf("Load() embedded options.json: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/schema/ -run 'TestParse|TestGroups|TestLoadEmbedded' -v`
Expected: FAIL — build error `undefined: parse` / `undefined: Load` / `undefined: Schema`.

- [ ] **Step 3: Write minimal implementation**

First commit the embed placeholder so `//go:embed options.json` compiles (Task 4 regenerates it with real content):

```bash
printf '[]\n' > internal/schema/options.json
```

Create `internal/schema/load.go`:

```go
package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// optionsJSON is the checked-in option schema, compiled into the binary. It is
// regenerated by cmd/schemagen (see gen.go); no runtime file or SCHEMA_PATH env.
//
//go:embed options.json
var optionsJSON []byte

// Schema is the ordered, grouped list of option descriptors.
type Schema struct {
	Options []Option
}

// Load parses the embedded options.json into a Schema.
func Load() (*Schema, error) {
	return parse(optionsJSON)
}

// parse decodes a JSON array of Option into a Schema.
func parse(data []byte) (*Schema, error) {
	var opts []Option
	if err := json.Unmarshal(data, &opts); err != nil {
		return nil, fmt.Errorf("parse options.json: %w", err)
	}
	return &Schema{Options: opts}, nil
}

// Get returns the option with the given dotted key.
func (s *Schema) Get(key string) (Option, bool) {
	for _, o := range s.Options {
		if o.Key == key {
			return o, true
		}
	}
	return Option{}, false
}

// Groups returns group names in first-seen (file) order.
func (s *Schema) Groups() []string {
	var gs []string
	seen := map[string]bool{}
	for _, o := range s.Options {
		if !seen[o.Group] {
			seen[o.Group] = true
			gs = append(gs, o.Group)
		}
	}
	return gs
}

// InGroup returns the options in group g, in schema order.
func (s *Schema) InGroup(g string) []Option {
	var out []Option
	for _, o := range s.Options {
		if o.Group == g {
			out = append(out, o)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/schema/ -v`
Expected: PASS (all Task 1 + Task 2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/schema/load.go internal/schema/load_test.go internal/schema/options.json
git commit -m "feat(schema): add Schema.Load/Get/Groups/InGroup

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: schemagen — flatten, infer Kind, build options

**Files:**
- Create: `cmd/schemagen/schemagen.go`
- Create: `cmd/schemagen/testdata/dump.yaml`
- Test: `cmd/schemagen/schemagen_test.go`

**Interfaces:**
- Consumes: `schema.Option`, `schema.Kind`, `schema.KindBool/KindInt/KindString` (Tasks 1–2); `gopkg.in/yaml.v3`.
- Produces (package `main`, testable helpers):
  ```go
  type kv struct { Key string; Val any }
  func flattenNode(prefix string, n *yaml.Node, out *[]kv) // ordered scalar-leaf walk
  func inferKind(v any) schema.Kind                          // bool→KindBool, int→KindInt, else KindString
  func buildOptions(root *yaml.Node) []schema.Option         // ordered; Group=seg[0], Label=last seg
  ```
  Note: **Group = first path segment** (per this plan's directive). `go test` compiles the package even though `main()` is added in Task 4.

- [ ] **Step 1: Write the fixture dump**

Create `cmd/schemagen/testdata/dump.yaml`:

```yaml
version: 1
document:
    toc_type: normal
    insert_soft_hyphen: false
    images:
        optimize: true
        jpeg_quality_level: 75
    footnotes:
        mode: block
    output_name_template: "{title}"
```

- [ ] **Step 2: Write the failing test**

Create `cmd/schemagen/schemagen_test.go`:

```go
package main

import (
	"os"
	"testing"

	"fb2cng-web/internal/schema"
	"gopkg.in/yaml.v3"
)

func loadDump(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		t.Fatal(err)
	}
	return &n
}

func TestBuildOptionsOrderAndKinds(t *testing.T) {
	opts := buildOptions(loadDump(t, "testdata/dump.yaml"))
	want := []struct {
		key, group, label string
		kind              schema.Kind
	}{
		{"version", "version", "version", schema.KindInt},
		{"document.toc_type", "document", "toc_type", schema.KindString},
		{"document.insert_soft_hyphen", "document", "insert_soft_hyphen", schema.KindBool},
		{"document.images.optimize", "document", "optimize", schema.KindBool},
		{"document.images.jpeg_quality_level", "document", "jpeg_quality_level", schema.KindInt},
		{"document.footnotes.mode", "document", "mode", schema.KindString},
		{"document.output_name_template", "document", "output_name_template", schema.KindString},
	}
	if len(opts) != len(want) {
		t.Fatalf("got %d opts, want %d: %+v", len(opts), len(want), opts)
	}
	for i, w := range want {
		o := opts[i]
		if o.Key != w.key || o.Group != w.group || o.Label != w.label || o.Kind != w.kind {
			t.Fatalf("opt[%d]=%+v want key=%s group=%s label=%s kind=%s",
				i, o, w.key, w.group, w.label, w.kind)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/schemagen/ -run TestBuildOptions -v`
Expected: FAIL — build error `undefined: buildOptions`.

- [ ] **Step 4: Write minimal implementation**

Create `cmd/schemagen/schemagen.go`:

```go
package main

import (
	"strings"

	"fb2cng-web/internal/schema"
	"gopkg.in/yaml.v3"
)

type kv struct {
	Key string
	Val any
}

// flattenNode walks a YAML node in document order, emitting one kv per scalar
// leaf with a dotted key path. Nested mappings recurse; sequences/other kinds
// are treated as leaf scalars (decoded as-is).
func flattenNode(prefix string, n *yaml.Node, out *[]kv) {
	if n.Kind == yaml.DocumentNode {
		for _, c := range n.Content {
			flattenNode(prefix, c, out)
		}
		return
	}
	if n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		name := n.Content[i].Value
		val := n.Content[i+1]
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		if val.Kind == yaml.MappingNode {
			flattenNode(key, val, out)
			continue
		}
		var v any
		_ = val.Decode(&v)
		*out = append(*out, kv{Key: key, Val: v})
	}
}

// inferKind maps a Go scalar to a schema.Kind. Enums are never inferred (they
// require annotation); anything non-bool/non-int is a string.
func inferKind(v any) schema.Kind {
	switch v.(type) {
	case bool:
		return schema.KindBool
	case int, int64:
		return schema.KindInt
	default:
		return schema.KindString
	}
}

// buildOptions flattens the dump and builds ordered Option descriptors. Group is
// the first path segment; Label is the last.
func buildOptions(root *yaml.Node) []schema.Option {
	var kvs []kv
	flattenNode("", root, &kvs)
	opts := make([]schema.Option, 0, len(kvs))
	for _, p := range kvs {
		seg := strings.Split(p.Key, ".")
		opts = append(opts, schema.Option{
			Key:     p.Key,
			Group:   seg[0],
			Label:   seg[len(seg)-1],
			Kind:    inferKind(p.Val),
			Default: p.Val,
		})
	}
	return opts
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/schemagen/ -run TestBuildOptions -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/schemagen/schemagen.go cmd/schemagen/schemagen_test.go cmd/schemagen/testdata/dump.yaml
git commit -m "feat(schemagen): flatten dump to ordered typed options

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: schemagen — descriptions, drift report, write, `main()`, checked-in options.json

**Files:**
- Modify: `cmd/schemagen/schemagen.go` (add `parseDescriptions`, `applyDescriptions`, `computeDrift`, `writeOptions`, `loadExisting`)
- Create: `cmd/schemagen/main.go`
- Create: `cmd/schemagen/testdata/config.md`
- Test: `cmd/schemagen/schemagen_test.go` (append)
- Create: `internal/schema/options.json` (generated, committed)

**Interfaces:**
- Consumes: `buildOptions`, `kv` (Task 3); `schema.Option`, `schema.Load` (Tasks 1–2).
- Produces:
  ```go
  func parseDescriptions(md string) map[string]string        // dotted key -> description
  func applyDescriptions(opts []schema.Option, desc map[string]string)
  type driftReport struct { Added, Removed, Retyped []string }
  func computeDrift(old, new []schema.Option) driftReport
  func writeOptions(path string, opts []schema.Option) error // JSON array, indented
  func loadExisting(path string) []schema.Option             // nil if absent/unparsable
  func main()
  ```

- [ ] **Step 1: Write the fixture descriptions doc**

Create `cmd/schemagen/testdata/config.md`:

```markdown
# fbc configuration

Partial reference. Only some options are documented.

- `document.images.jpeg_quality_level` — JPEG quality (1-100); lower is smaller.
- `document.toc_type`: Table-of-contents style.
```

- [ ] **Step 2: Write the failing tests**

Append to `cmd/schemagen/schemagen_test.go`:

```go
func TestParseAndApplyDescriptions(t *testing.T) {
	md, err := os.ReadFile("testdata/config.md")
	if err != nil {
		t.Fatal(err)
	}
	desc := parseDescriptions(string(md))
	if desc["document.images.jpeg_quality_level"] == "" {
		t.Fatalf("jpeg description not parsed: %v", desc)
	}
	if desc["document.toc_type"] == "" {
		t.Fatalf("toc_type description (colon form) not parsed: %v", desc)
	}
	opts := buildOptions(loadDump(t, "testdata/dump.yaml"))
	applyDescriptions(opts, desc)
	var got string
	for _, o := range opts {
		if o.Key == "document.toc_type" {
			got = o.Description
		}
	}
	if got == "" {
		t.Fatal("description not applied to document.toc_type")
	}
}

func TestComputeDrift(t *testing.T) {
	old := []schema.Option{
		{Key: "gone", Kind: schema.KindString},
		{Key: "document.toc_type", Kind: schema.KindInt}, // was int
	}
	next := []schema.Option{
		{Key: "document.toc_type", Kind: schema.KindString}, // now string -> retyped
		{Key: "fresh", Kind: schema.KindBool},               // added
	}
	d := computeDrift(old, next)
	if len(d.Added) != 1 || d.Added[0] != "fresh" {
		t.Fatalf("added: %v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "gone" {
		t.Fatalf("removed: %v", d.Removed)
	}
	if len(d.Retyped) != 1 || d.Retyped[0] != "document.toc_type" {
		t.Fatalf("retyped: %v", d.Retyped)
	}
}

func TestWriteAndReloadRoundTrip(t *testing.T) {
	opts := buildOptions(loadDump(t, "testdata/dump.yaml"))
	out := filepath.Join(t.TempDir(), "options.json")
	if err := writeOptions(out, opts); err != nil {
		t.Fatal(err)
	}
	// schema.Load reads the embedded file, so reload the written array directly to
	// confirm writeOptions emits valid, re-parseable JSON.
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded []schema.Option
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("re-parse written options.json: %v", err)
	}
	if len(reloaded) != len(opts) {
		t.Fatalf("round-trip count %d vs %d", len(reloaded), len(opts))
	}
	s := &schema.Schema{Options: reloaded}
	o, ok := s.Get("document.images.jpeg_quality_level")
	if !ok || o.Kind != schema.KindInt {
		t.Fatalf("reloaded jpeg option: %+v ok=%v", o, ok)
	}
}
```

Add `"encoding/json"` and `"path/filepath"` to the test file's import block (alongside `os`, `testing`, `schema`, `yaml`).

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./cmd/schemagen/ -run 'TestParse|TestComputeDrift|TestWrite' -v`
Expected: FAIL — build error `undefined: parseDescriptions` etc.

- [ ] **Step 4: Add the implementation helpers**

Append to `cmd/schemagen/schemagen.go` and add imports. Change the top import block to:

```go
import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"

	"fb2cng-web/internal/schema"
	"gopkg.in/yaml.v3"
)
```

Append these functions:

```go
// descRe matches a markdown line like:  - `dotted.key` — description
// or:  `dotted.key`: description   (separator may be em dash, hyphen, or colon).
var descRe = regexp.MustCompile("^\\s*[-*]?\\s*`([A-Za-z0-9_.]+)`\\s*[-—:]+\\s*(.+?)\\s*$")

// parseDescriptions best-effort parses key->description pairs from config.md.
func parseDescriptions(md string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(md, "\n") {
		if m := descRe.FindStringSubmatch(line); m != nil {
			out[m[1]] = m[2]
		}
	}
	return out
}

// applyDescriptions fills Description on options that have a documented key.
func applyDescriptions(opts []schema.Option, desc map[string]string) {
	for i := range opts {
		if d, ok := desc[opts[i].Key]; ok {
			opts[i].Description = d
		}
	}
}

type driftReport struct {
	Added   []string
	Removed []string
	Retyped []string
}

// computeDrift reports keys added/removed and keys whose Kind changed.
func computeDrift(old, next []schema.Option) driftReport {
	oldByKey := map[string]schema.Option{}
	for _, o := range old {
		oldByKey[o.Key] = o
	}
	nextByKey := map[string]schema.Option{}
	for _, o := range next {
		nextByKey[o.Key] = o
	}
	var r driftReport
	for _, o := range next {
		prev, ok := oldByKey[o.Key]
		if !ok {
			r.Added = append(r.Added, o.Key)
			continue
		}
		if prev.Kind != o.Kind {
			r.Retyped = append(r.Retyped, o.Key)
		}
	}
	for _, o := range old {
		if _, ok := nextByKey[o.Key]; !ok {
			r.Removed = append(r.Removed, o.Key)
		}
	}
	sort.Strings(r.Added)
	sort.Strings(r.Removed)
	sort.Strings(r.Retyped)
	return r
}

// writeOptions writes opts as an indented JSON array.
func writeOptions(path string, opts []schema.Option) error {
	data, err := json.MarshalIndent(opts, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// loadExisting reads a prior options.json for the drift comparison; a missing or
// unparsable file yields nil (treated as an empty prior schema).
func loadExisting(path string) []schema.Option {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var opts []schema.Option
	if json.Unmarshal(data, &opts) != nil {
		return nil
	}
	return opts
}
```

- [ ] **Step 5: Add `main()`**

Create `cmd/schemagen/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func main() {
	dumpPath := flag.String("dump", "", "path to an `fbc dumpconfig --default` YAML; empty runs -fbc")
	docsPath := flag.String("docs", "docs/config.md", "path to config.md for descriptions")
	outPath := flag.String("out", "internal/schema/options.json", "output options.json path")
	fbcBin := flag.String("fbc", os.Getenv("FBC_BIN"), "fbc binary (used when -dump is empty)")
	flag.Parse()

	var dumpBytes []byte
	var err error
	if *dumpPath != "" {
		dumpBytes, err = os.ReadFile(*dumpPath)
	} else {
		dumpBytes, err = runDump(*fbcBin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read dump:", err)
		os.Exit(1)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(dumpBytes, &root); err != nil {
		fmt.Fprintln(os.Stderr, "parse dump:", err)
		os.Exit(1)
	}
	opts := buildOptions(&root)

	if md, derr := os.ReadFile(*docsPath); derr == nil {
		applyDescriptions(opts, parseDescriptions(string(md)))
	} else {
		fmt.Fprintf(os.Stderr, "note: %s not read (%v); descriptions left blank\n", *docsPath, derr)
	}

	d := computeDrift(loadExisting(*outPath), opts)
	fmt.Fprintf(os.Stderr, "DRIFT added=%d removed=%d retyped=%d\n", len(d.Added), len(d.Removed), len(d.Retyped))
	for _, k := range d.Added {
		fmt.Fprintln(os.Stderr, "  + "+k)
	}
	for _, k := range d.Removed {
		fmt.Fprintln(os.Stderr, "  - "+k)
	}
	for _, k := range d.Retyped {
		fmt.Fprintln(os.Stderr, "  ~ "+k)
	}

	if err := writeOptions(*outPath, opts); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %d options to %s\n", len(opts), *outPath)
}

// runDump invokes `fbc dumpconfig --default <tmp>` and returns the YAML bytes.
func runDump(bin string) ([]byte, error) {
	if bin == "" {
		return nil, fmt.Errorf("no -dump path and no -fbc/FBC_BIN binary")
	}
	dir, err := os.MkdirTemp("", "schemagen-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "defaults.yaml")
	if err := exec.Command(bin, "dumpconfig", "--default", out).Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./cmd/schemagen/ -v`
Expected: PASS (all schemagen tests). Also confirm the command builds: `go build ./cmd/schemagen/` (expected: no output).

- [ ] **Step 7: Generate the checked-in scaffold**

Run: `go run ./cmd/schemagen -dump cmd/schemagen/testdata/dump.yaml -docs cmd/schemagen/testdata/config.md -out internal/schema/options.json`
Expected on stderr: a `DRIFT added=7 removed=0 retyped=0` block (the prior file is the Task 2 placeholder `[]`, so every key reads as added) and `wrote 7 options to internal/schema/options.json`. This overwrites the placeholder with the real scaffold that the `//go:embed` in Task 2 already references.

Verify it loads: `go test ./internal/schema/ -v` (still PASS — `TestLoadEmbedded` now parses the real 7-option file instead of the placeholder).

- [ ] **Step 8: Commit**

```bash
git add cmd/schemagen/schemagen.go cmd/schemagen/main.go cmd/schemagen/schemagen_test.go cmd/schemagen/testdata/config.md internal/schema/options.json
git commit -m "feat(schemagen): descriptions, drift report, writer, and scaffold options.json

The checked-in options.json is scaffolded from the fixture dump; the real
~104-option file is regenerated via 'go generate' with a real fbc binary.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: `convert.Runner.Validate` probe + fake + stub

**Files:**
- Modify: `internal/convert/runner.go` (add `Validate` to `Runner`, add `FBC.Validate`)
- Modify: `testdata/fake-fbc.sh` (`dumpconfig` branch: handle `-c`, reject `__invalid`)
- Modify: `internal/server/handlers_test.go` (add `Validate` to `stubRunner`)
- Test: `internal/convert/runner_validate_test.go`

**Interfaces:**
- Consumes: existing `FBC` runner, `fakeBin(t)` helper (from `runner_test.go`).
- Produces:
  ```go
  // Validate probes whether fbc can parse configPath. Returns nil if valid,
  // a non-nil error (with fbc's stderr) if fbc rejects it.
  func (f *FBC) Validate(ctx context.Context, configPath string) error
  ```
  and adds `Validate(ctx context.Context, configPath string) error` to the `Runner` interface.
- Coordination: Plan 1 also extends `Runner` with `ConvertLogged(ctx, inputPath, format, configPath, destDir, logPath string) ([]string, error)`. Merge both additions; a fake/stub must implement whichever methods the merged interface declares.

- [ ] **Step 1: Write the failing test**

Create `internal/convert/runner_validate_test.go`:

```go
package convert

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAcceptsGoodConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfg, []byte("version: 1\ndocument:\n  toc_type: normal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(fakeBin(t)).Validate(context.Background(), cfg); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfg, []byte("__invalid: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(fakeBin(t)).Validate(context.Background(), cfg); err == nil {
		t.Fatal("invalid config accepted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/convert/ -run TestValidate -v`
Expected: FAIL — build error `f.Validate undefined`.

- [ ] **Step 3: Add `Validate` to the interface and `FBC`**

In `internal/convert/runner.go`, add one line to the `Runner` interface (leave Plan 1's `ConvertLogged` line if already present):

```go
type Runner interface {
	// DumpDefaults returns fbc's embedded default configuration as YAML.
	DumpDefaults(ctx context.Context) ([]byte, error)
	// Convert runs fbc on inputPath into destDir and returns the produced
	// output file paths. configPath is optional ("" = use fbc defaults).
	Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error)
	// Validate probes whether fbc can parse configPath. Returns nil when valid.
	Validate(ctx context.Context, configPath string) error
}
```

Append the method to `internal/convert/runner.go` (imports `bytes`, `context`, `fmt`, `os`, `os/exec`, `path/filepath`, `strings` are already present):

```go
// Validate probes whether fbc can parse configPath by running a dumpconfig with
// -c pointed at it. A non-zero exit (parse error) is surfaced as an error whose
// message includes fbc's stderr. -c is a GLOBAL flag and precedes the subcommand.
func (f *FBC) Validate(ctx context.Context, configPath string) error {
	dir, err := os.MkdirTemp("", "fbc-validate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	out := filepath.Join(dir, "merged.yaml")
	var errb bytes.Buffer
	cmd := exec.CommandContext(ctx, f.Bin, "-c", configPath, "dumpconfig", out)
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return fmt.Errorf("invalid config: %s", msg)
		}
		return fmt.Errorf("invalid config: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Teach the fake fbc to validate `-c`**

In `testdata/fake-fbc.sh`, replace the entire `if [ "$mode" = "dump" ]; then ... fi` block with:

```sh
if [ "$mode" = "dump" ]; then
  # Handle both `dumpconfig --default <out>` and `-c <cfg> dumpconfig <out>`.
  cfg=""
  dest_file=""
  while [ $# -gt 0 ]; do
    case "$1" in
      -c) cfg="$2"; shift 2; continue ;;
      dumpconfig|--default) shift; continue ;;
      *) dest_file="$1"; shift ;;
    esac
  done
  # Validation probe: reject a config carrying the __invalid marker.
  if [ -n "$cfg" ] && grep -q '__invalid' "$cfg" 2>/dev/null; then
    echo "fake-fbc: invalid config: $cfg" >&2
    exit 1
  fi
  yaml='version: 1\ndocument:\n    toc_type: normal\n'
  if [ -n "$dest_file" ]; then
    printf "$yaml" > "$dest_file"
  else
    printf "$yaml"
  fi
  exit 0
fi
```

- [ ] **Step 5: Add `Validate` to the server test stub**

In `internal/server/handlers_test.go`, add this method next to the other `stubRunner` methods (keeps the package compiling now that `Runner` requires `Validate`):

```go
func (s stubRunner) Validate(context.Context, string) error { return nil }
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/convert/ -run TestValidate -v`
Expected: PASS (2 tests).
Run: `go test ./internal/convert/ ./internal/server/`
Expected: PASS — existing `DumpDefaults`/`Convert`/handler tests still green (the fake's `dumpconfig --default <out>` path is preserved).

- [ ] **Step 7: Commit**

```bash
git add internal/convert/runner.go internal/convert/runner_validate_test.go testdata/fake-fbc.sh internal/server/handlers_test.go
git commit -m "feat(convert): add Runner.Validate config-parse probe

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Editor page — `GET /settings/preset/{id}` full option grid

**Files:**
- Modify: `internal/server/server.go` (append `schema *schema.Schema` field + `"fb2cng-web/internal/schema"` import)
- Create: `internal/server/editor.go`
- Create: `internal/web/templates/editor.gohtml`
- Create: `internal/web/templates/_effective.gohtml`
- Test: `internal/server/editor_test.go`

**Interfaces:**
- Consumes:
  - Plan 1: `Server` struct fields `runner convert.Runner`, `presets *presets.Store`, `tpl *template.Template`; the base-layout contract — `base.gohtml`'s `{{define "base"}}` renders shared chrome (Google Fonts, pre-paint theme script, `fb2→epub` + Convert/Settings nav keyed off `.Tab`, theme toggle, htmx CDN) then `{{template .ContentName .}}`; the header struct `type layout struct { Tab, ContentName string; User string }`; the render helper `func (s *Server) render(w http.ResponseWriter, data any)` which executes `"base"`; `app.css` (defines the `--changed` amber token and editor classes); the template `embed.FS` (glob includes `editor.gohtml`/`_effective.gohtml` at startup).
  - Plan 2: `presets.Store` — `Get(id) (*presets.Preset, error)`, `DefaultID() string`; `presets.Preset{ID, Name string; Overrides map[string]any}`.
  - Task 2: `schema.Schema.Groups`/`InGroup`/`Get`; `schema.Option`; Task 1 `Option.IsDefault`.
  - This handler **replaces** Plan 2's minimal preset editor (`handlePresetEdit`) at `GET /settings/preset/{id}` — Plan 2's version is deleted in Task 9.
- Produces:
  ```go
  // Server gains one concrete field (per shared contract, Plan 3 appends it):
  //   schema *schema.Schema
  type rowVM struct { Key, Label, Kind string; Enum []string; StrValue, DefaultStr, Description string; Checked, Changed, Known, IsTemplate bool }
  type groupVM struct { Name string; Total, Changed int; Rows []rowVM }
  type effectiveVM struct { YAML, Error string; Valid, Invalid bool; OverrideCount int }
  // editorVM embeds Plan 1's layout header so base can read .Tab/.ContentName/.User.
  type editorVM struct { layout; Preset *presets.Preset; IsDefaultPreset bool; TotalOptions, ChangedCount int; Groups []groupVM; Effective effectiveVM }
  func flattenOverrides(prefix string, m map[string]any, out map[string]any)
  func (s *Server) buildEditorVM(p *presets.Preset, flat map[string]any) editorVM
  func (s *Server) handlePresetEditor(w http.ResponseWriter, r *http.Request)
  ```
  Note: the editor page is rendered via `s.render(w, vm)` through Plan 1's `base` (content template `"editor"`), NOT as a standalone document. The effective pane is left empty on GET here; Task 8 populates it via `computeEffective`.

- [ ] **Step 1: Write the templates**

Create `internal/web/templates/editor.gohtml` as a **content template** (`{{define "editor"}}`), NOT a standalone document. Plan 1's `base.gohtml` supplies `<html>`/`<head>`, Google Fonts, the pre-paint theme script, the Convert/Settings nav, the theme toggle, and the htmx CDN `<script>`; `base` renders this via `{{template .ContentName .}}`. Styling comes from Plan 1's `app.css` via the listed classes — this task only asserts structure. Markup mirrors mockup SCREEN 5 (`docs/superpowers/specs/mockup/FB2-Converter.dc.html`, dark 1440 frame): toolbar (← Presets, name, "N of M options changed", Default-preset checkbox, search, Changed-only, Save preset), two-column grid (`option-list` + `#effective` aside), group headers with `N options · M changed`, per-row `option-key` (+ amber `changed-dot`) / `option-control` / `option-meta` (description · default X · reset), amber left border on `.option-row.changed`, and the `output_name_template` textarea + `filename-preview`. The editor-specific glue `<script>` (search filter, changed-only, reset, live filename preview) lives at the end of the content template:

```html
{{define "editor"}}
<form id="preset-form" class="editor" method="post" action="/settings/preset/{{.Preset.ID}}"
      hx-post="/settings/preset/{{.Preset.ID}}/effective"
      hx-trigger="change delay:300ms"
      hx-target="#effective" hx-swap="innerHTML">
  <div class="editor-toolbar">
    <a class="back-link" href="/settings">← Presets</a>
    <input class="preset-name" type="text" name="name" value="{{.Preset.Name}}" aria-label="Preset name">
    <span class="changed-summary">{{.ChangedCount}} of {{.TotalOptions}} options changed</span>
    <label class="tb-check"><input type="checkbox" name="is_default" value="true" {{if .IsDefaultPreset}}checked{{end}}> Default preset</label>
    <input class="option-search" type="search" id="option-search" placeholder="Search options…" aria-label="Search options">
    <label class="tb-check"><input type="checkbox" id="changed-only"> Changed only</label>
    <span class="tb-spacer"></span>
    <button class="save-btn" type="submit">Save preset</button>
  </div>

  <div class="editor-grid">
    <div class="option-list">
      {{range .Groups}}
      <div class="group-header" data-group="{{.Name}}">{{.Name}} <span class="group-count">{{.Total}} options · {{.Changed}} changed</span></div>
      {{range .Rows}}
      {{$row := .}}
      <div class="option-row{{if .Changed}} changed{{end}}{{if .IsTemplate}} template-row{{end}}" data-path="{{.Key}}">
        <span class="option-key">{{.Label}}{{if .Changed}} <span class="changed-dot">•</span>{{end}}</span>
        <div class="option-control">
          {{if eq .Kind "bool"}}
            <input type="hidden" name="{{.Key}}" value="false">
            <input type="checkbox" id="opt-{{.Key}}" name="{{.Key}}" value="true" data-default="{{.DefaultStr}}" {{if .Checked}}checked{{end}}>
          {{else if eq .Kind "int"}}
            <input type="number" id="opt-{{.Key}}" name="{{.Key}}" value="{{.StrValue}}" data-default="{{.DefaultStr}}">
          {{else if eq .Kind "enum"}}
            <select id="opt-{{.Key}}" name="{{.Key}}" data-default="{{.DefaultStr}}">
              {{range .Enum}}<option value="{{.}}"{{if eq $row.StrValue .}} selected{{end}}>{{.}}</option>{{end}}
            </select>
          {{else if .IsTemplate}}
            <div class="template-wrap">
              <textarea id="opt-{{.Key}}" name="{{.Key}}" rows="3" class="template-input" data-default="{{.DefaultStr}}">{{.StrValue}}</textarea>
              <div class="filename-preview" data-for="opt-{{.Key}}"></div>
            </div>
          {{else}}
            <input type="text" id="opt-{{.Key}}" name="{{.Key}}" value="{{.StrValue}}" data-default="{{.DefaultStr}}">
          {{end}}
        </div>
        <span class="option-meta">{{if .Description}}{{.Description}} · {{end}}{{if .Known}}<span class="option-default">default {{.DefaultStr}}</span>{{end}}{{if .Changed}} · <a class="reset" href="#" data-target="opt-{{.Key}}">reset</a>{{end}}</span>
      </div>
      {{end}}
      {{end}}
    </div>

    <aside id="effective" class="effective">
      {{template "_effective" .Effective}}
    </aside>
  </div>
</form>

<script>
(function () {
  var form = document.getElementById('preset-form');
  if (!form) return;
  var search = document.getElementById('option-search');
  var changedOnly = document.getElementById('changed-only');
  function apply() {
    var q = (search && search.value || '').toLowerCase();
    var only = changedOnly && changedOnly.checked;
    form.querySelectorAll('.option-list .group-header').forEach(function (h) {
      var shown = 0, row = h.nextElementSibling;
      while (row && row.classList.contains('option-row')) {
        var path = (row.getAttribute('data-path') || '').toLowerCase();
        var match = path.indexOf(q) !== -1;
        if (only && !row.classList.contains('changed')) match = false;
        row.style.display = match ? '' : 'none';
        if (match) shown++;
        row = row.nextElementSibling;
      }
      h.style.display = shown ? '' : 'none';
    });
  }
  if (search) search.addEventListener('input', apply);
  if (changedOnly) changedOnly.addEventListener('change', apply);
  form.addEventListener('click', function (e) {
    var t = e.target;
    if (!t.classList.contains('reset')) return;
    e.preventDefault();
    var el = document.getElementById(t.getAttribute('data-target'));
    if (!el) return;
    var d = el.getAttribute('data-default') || '';
    if (el.type === 'checkbox') el.checked = (d === 'true');
    else el.value = d;
    el.dispatchEvent(new Event('change', { bubbles: true }));
  });
  form.addEventListener('input', function (e) {
    var t = e.target;
    if (!t.classList.contains('template-input')) return;
    var prev = form.querySelector('.filename-preview[data-for="' + t.id + '"]');
    if (prev) prev.textContent = '→ ' + t.value.replace(/\{\{[^}]*\}\}/g, 'sample') + '.epub';
  });
})();
</script>
{{end}}
```

Create `internal/web/templates/_effective.gohtml` as a partial (`{{define "_effective"}}`) rendering `effectiveVM`; the validity chip is hidden until Task 8 sets `Valid`/`Invalid`. It is included by the editor content and htmx-swapped directly into `#effective`:

```html
{{define "_effective"}}
<div class="effective-head">
  <span class="effective-title">Effective config</span>
  <span class="effective-note">refreshed on save</span>
</div>
{{if or .Valid .Invalid}}
<div class="validity{{if .Invalid}} invalid{{else}} valid{{end}}">
  <span class="validity-dot"></span>
  {{if .Invalid}}Invalid · {{.OverrideCount}} override{{if ne .OverrideCount 1}}s{{end}}{{else}}Valid · {{.OverrideCount}} override{{if ne .OverrideCount 1}}s{{end}} merged over defaults{{end}}
</div>
{{end}}
{{if .Error}}<p class="effective-error">{{.Error}}</p>{{end}}
<pre class="effective-yaml">{{.YAML}}</pre>
{{end}}
```

- [ ] **Step 2: Write the failing test**

Create `internal/server/editor_test.go`:

```go
package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
func editorTemplates(t *testing.T) *template.Template {
	t.Helper()
	tpl := template.Must(template.New("").Parse(
		`{{define "base"}}<!doctype html><html data-tab="{{.Tab}}">{{template .ContentName .}}</html>{{end}}`))
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestPresetEditorRendersGrid -v`
Expected: FAIL — build error `s.handlePresetEditor undefined` (and VM types undefined). (Requires Plan 1's `layout` type and `render` helper to exist; this plan executes after Plan 1.)

- [ ] **Step 4: Add the concrete `schema` field to `Server`**

In `internal/server/server.go`, add the import `"fb2cng-web/internal/schema"` and append one field to the `Server` struct (per the shared contract, Plan 3 appends its own concretely-typed field — no `interface{}` placeholder). `New(...)` is widened to set it in Task 9; the editor tests set it directly:

```go
type Server struct {
	cfg    config.Config
	runner convert.Runner
	static fs.FS
	// ... Plan 1 fields (tpl, jobs) and Plan 2 field (presets) ...
	schema *schema.Schema
}
```

- [ ] **Step 5: Write the handler + VM builder**

Create `internal/server/editor.go`:

```go
package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/schema"
)

type rowVM struct {
	Key         string
	Label       string
	Kind        string // "bool","int","string","enum"
	Enum        []string
	StrValue    string
	DefaultStr  string
	Description string
	Checked     bool // for bool controls
	Changed     bool
	Known       bool // present in the schema
	IsTemplate  bool // output_name_template -> textarea + preview
}

type groupVM struct {
	Name    string
	Total   int
	Changed int
	Rows    []rowVM
}

type effectiveVM struct {
	YAML          string
	Error         string
	Valid         bool
	Invalid       bool
	OverrideCount int
}

type editorVM struct {
	layout // Plan 1's header struct: Tab, ContentName, User — so base can render chrome + nav
	Preset          *presets.Preset
	IsDefaultPreset bool
	TotalOptions    int
	ChangedCount    int
	Groups          []groupVM
	Effective       effectiveVM
}

// normalizeStr renders a value the way the editor displays it, matching
// schema.Option.IsDefault's normalization of numeric types.
func normalizeStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// flattenOverrides flattens a nested preset override map into dotted keys.
func flattenOverrides(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flattenOverrides(key, sub, out)
			continue
		}
		out[key] = v
	}
}

// buildEditorVM builds the grid: every schema option in schema order (grouped by
// Group), plus any override keys not covered by the schema as synthetic rows.
func (s *Server) buildEditorVM(p *presets.Preset, flat map[string]any) editorVM {
	order := []string{}
	byName := map[string]*groupVM{}
	ensure := func(name string) *groupVM {
		g, ok := byName[name]
		if !ok {
			g = &groupVM{Name: name}
			byName[name] = g
			order = append(order, name)
		}
		return g
	}

	used := map[string]bool{}
	for _, gname := range s.schema.Groups() {
		g := ensure(gname)
		for _, opt := range s.schema.InGroup(gname) {
			val, present := flat[opt.Key]
			if !present {
				val = opt.Default
			}
			used[opt.Key] = true
			g.Rows = append(g.Rows, s.rowFor(opt, val, present))
		}
	}

	// Synthetic rows for override keys the schema does not describe.
	var extra []string
	for k := range flat {
		if !used[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		g := ensure(strings.Split(k, ".")[0])
		g.Rows = append(g.Rows, rowVM{
			Key:      k,
			Label:    k,
			Kind:     string(schema.KindString),
			StrValue: normalizeStr(flat[k]),
			Changed:  true,
			Known:    false,
		})
	}

	vm := editorVM{Preset: p, TotalOptions: len(s.schema.Options)}
	for _, name := range order {
		g := byName[name]
		g.Total = len(g.Rows)
		for _, row := range g.Rows {
			if row.Changed {
				g.Changed++
				vm.ChangedCount++
			}
		}
		vm.Groups = append(vm.Groups, *g)
	}
	return vm
}

func (s *Server) rowFor(opt schema.Option, val any, present bool) rowVM {
	r := rowVM{
		Key:         opt.Key,
		Label:       opt.Label,
		Kind:        string(opt.Kind),
		Enum:        opt.Enum,
		StrValue:    normalizeStr(val),
		DefaultStr:  normalizeStr(opt.Default),
		Description: opt.Description,
		Known:       true,
		IsTemplate:  strings.Contains(opt.Key, "output_name_template"),
	}
	if opt.Kind == schema.KindBool {
		r.Checked = r.StrValue == "true"
	}
	r.Changed = present && !opt.IsDefault(val)
	return r
}

func (s *Server) handlePresetEditor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.presets.Get(id)
	if err != nil {
		http.Error(w, "preset not found", http.StatusNotFound)
		return
	}
	flat := map[string]any{}
	flattenOverrides("", p.Overrides, flat)

	vm := s.buildEditorVM(p, flat)
	vm.layout = layout{Tab: "settings", ContentName: "editor", User: r.Header.Get("Remote-User")}
	vm.IsDefaultPreset = s.presets.DefaultID() == id ||
		(s.presets.DefaultID() == "" && id == "defaults")

	// Render the "editor" content template through Plan 1's base layout.
	s.render(w, vm)
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestPresetEditorRendersGrid -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/server/server.go internal/server/editor.go internal/server/editor_test.go internal/web/templates/editor.gohtml internal/web/templates/_effective.gohtml
git commit -m "feat(server): full option-grid preset editor (GET)

Replaces Plan 2's minimal raw-YAML editor; renders every schema option
grouped by Group, plus synthetic rows for unschematized override keys.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Save — `POST /settings/preset/{id}` (sparse overrides)

**Files:**
- Modify: `internal/server/editor.go` (add `collectOverrides`, `parseValue`, `unflatten`, `handlePresetSave`)
- Test: `internal/server/editor_test.go` (append)

**Interfaces:**
- Consumes: `schema.Schema.Get`, `schema.Option.IsDefault` (Tasks 1–2); Plan 2 `presets.Store.Get`, `presets.Store.Save`, `presets.Store.SetDefault`; `rowVM`/VM types (Task 6).
- Produces:
  ```go
  func (s *Server) collectOverrides(r *http.Request) map[string]any // dotted key -> typed value, defaults dropped
  func parseValue(k schema.Kind, v string) any
  func unflatten(flat map[string]any) map[string]any
  func (s *Server) handlePresetSave(w http.ResponseWriter, r *http.Request)
  ```
  Bool fields use the hidden-`false` + checkbox-`true` trick; `collectOverrides` reads the **last** posted value per key so an unchecked box yields `false`.

- [ ] **Step 1: Write the failing test**

Append to `internal/server/editor_test.go` (add `"fmt"` and `"net/url"` to the import block):

```go
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
	form.Set("document.toc_type", "normal")                  // == default -> dropped
	form.Set("document.images.jpeg_quality_level", "40")     // != default 75 -> kept
	form["document.images.optimize"] = []string{"false"}     // unchecked bool -> false, != default true -> kept

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestPresetSaveSparse -v`
Expected: FAIL — build error `s.handlePresetSave undefined`.

- [ ] **Step 3: Add the save handler**

Append to `internal/server/editor.go`:

```go
// collectOverrides reads posted field values and returns only the sparse set
// that differs from the schema default (dropping equal-to-default keys). Bool
// controls post a hidden "false" plus (when checked) "true"; the last value wins.
// Unknown keys containing a dot are kept as raw strings (synthetic overrides).
func (s *Server) collectOverrides(r *http.Request) map[string]any {
	flat := map[string]any{}
	for key, vals := range r.PostForm {
		if key == "name" || key == "is_default" || key == "" || len(vals) == 0 {
			continue
		}
		v := vals[len(vals)-1]
		if opt, ok := s.schema.Get(key); ok {
			typed := parseValue(opt.Kind, v)
			if opt.IsDefault(typed) {
				continue
			}
			flat[key] = typed
			continue
		}
		if !strings.Contains(key, ".") || strings.TrimSpace(v) == "" {
			continue
		}
		flat[key] = v
	}
	return flat
}

func parseValue(k schema.Kind, v string) any {
	switch k {
	case schema.KindBool:
		return v == "true"
	case schema.KindInt:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		return v
	default:
		return v
	}
}

// unflatten turns dotted keys back into a nested map (the shape presets store).
func unflatten(flat map[string]any) map[string]any {
	root := map[string]any{}
	for key, v := range flat {
		segs := strings.Split(key, ".")
		m := root
		for _, seg := range segs[:len(segs)-1] {
			next, ok := m[seg].(map[string]any)
			if !ok {
				next = map[string]any{}
				m[seg] = next
			}
			m = next
		}
		m[segs[len(segs)-1]] = v
	}
	return root
}

func (s *Server) handlePresetSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	p, err := s.presets.Get(id)
	if err != nil {
		http.Error(w, "preset not found", http.StatusNotFound)
		return
	}
	if name := strings.TrimSpace(r.PostFormValue("name")); name != "" {
		p.Name = name
	}
	p.Overrides = unflatten(s.collectOverrides(r))
	if err := s.presets.Save(p); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if r.PostFormValue("is_default") == "true" {
		if err := s.presets.SetDefault(id); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
	}
	http.Redirect(w, r, "/settings/preset/"+id, http.StatusSeeOther)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run 'TestPresetSaveSparse|TestPresetEditorRendersGrid' -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/server/editor.go internal/server/editor_test.go
git commit -m "feat(server): sparse preset Save collecting non-default fields

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Effective-config pane — `POST /settings/preset/{id}/effective`

**Files:**
- Create: `internal/server/effective.go`
- Modify: `internal/server/editor.go` (`handlePresetEditor` populates `vm.Effective`)
- Test: `internal/server/editor_test.go` (append)

**Interfaces:**
- Consumes: `collectOverrides`, `unflatten`, `effectiveVM` (Tasks 6–7); `convert.BuildConfig(rawYAML string, o convert.FormOptions) ([]byte, error)`; `convert.Runner.Validate(ctx, configPath) error` (Task 5); `gopkg.in/yaml.v3`.
- Produces:
  ```go
  func (s *Server) computeEffective(ctx context.Context, flat map[string]any) effectiveVM
  func (s *Server) handlePresetEffective(w http.ResponseWriter, r *http.Request)
  func (s *Server) renderEffective(w http.ResponseWriter, vm effectiveVM)
  ```
  Preset overrides feed `BuildConfig` as `rawYAML` (form options nil), per the shared contract.

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/editor_test.go` (add `"context"` and `"errors"` to the import block):

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run TestEffective -v`
Expected: FAIL — build error `s.handlePresetEffective undefined`.

- [ ] **Step 3: Add the effective handler**

Create `internal/server/effective.go`:

```go
package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"fb2cng-web/internal/convert"
	"gopkg.in/yaml.v3"
)

// computeEffective merges the given sparse overrides over fbc defaults via
// convert.BuildConfig (overrides as rawYAML, form options nil), then asks fbc to
// parse the result to decide validity.
func (s *Server) computeEffective(ctx context.Context, flat map[string]any) effectiveVM {
	vm := effectiveVM{OverrideCount: len(flat)}

	rawYAML, err := yaml.Marshal(unflatten(flat))
	if err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	cfg, err := convert.BuildConfig(string(rawYAML), convert.FormOptions{})
	if err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	vm.YAML = string(cfg)

	dir, err := os.MkdirTemp("", "effective-*")
	if err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	defer os.RemoveAll(dir)
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, cfg, 0o644); err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	if err := s.runner.Validate(ctx, cfgPath); err != nil {
		vm.Invalid, vm.Error = true, err.Error()
	} else {
		vm.Valid = true
	}
	return vm
}

func (s *Server) handlePresetEffective(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.renderEffective(w, s.computeEffective(r.Context(), s.collectOverrides(r)))
}

func (s *Server) renderEffective(w http.ResponseWriter, vm effectiveVM) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "_effective", vm); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 4: Populate the effective pane on editor load**

In `internal/server/editor.go`, in `handlePresetEditor`, add the effective computation right before rendering. Change the tail of the function from:

```go
	vm := s.buildEditorVM(p, flat)
	vm.layout = layout{Tab: "settings", ContentName: "editor", User: r.Header.Get("Remote-User")}
	vm.IsDefaultPreset = s.presets.DefaultID() == id ||
		(s.presets.DefaultID() == "" && id == "defaults")

	// Render the "editor" content template through Plan 1's base layout.
	s.render(w, vm)
```

to:

```go
	vm := s.buildEditorVM(p, flat)
	vm.layout = layout{Tab: "settings", ContentName: "editor", User: r.Header.Get("Remote-User")}
	vm.IsDefaultPreset = s.presets.DefaultID() == id ||
		(s.presets.DefaultID() == "" && id == "defaults")
	vm.Effective = s.computeEffective(r.Context(), flat)

	// Render the "editor" content template through Plan 1's base layout.
	s.render(w, vm)
```

Note: `TestPresetEditorRendersGrid` (Task 6) constructs a `Server` without a `runner`. Because that server now calls `computeEffective`, which calls `s.runner.Validate`, update that test's server construction to include `runner: effRunner{}`:

```go
	s := &Server{schema: testSchema(t), presets: store, tpl: editorTemplates(t), runner: effRunner{}}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/ -run 'TestEffective|TestPresetEditor|TestPresetSave' -v`
Expected: PASS (all editor tests).

- [ ] **Step 6: Commit**

```bash
git add internal/server/effective.go internal/server/editor.go internal/server/editor_test.go
git commit -m "feat(server): live effective-config pane with fbc-parse validity

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Integration — delete Plan 2's minimal editor, widen `New`, routes, go:generate, Docker

**Files:**
- Modify: `internal/server/editor.go` (Plan 2's file) — **delete** `handlePresetEdit`, `handlePresetSave`, `presetEditData`
- Delete: `internal/web/templates/preset_edit.gohtml` (Plan 2's minimal editor template)
- Modify: `internal/server/server.go` — remove Plan 2's `GET`/`POST /settings/preset/{id}` registrations, register Plan 3's three routes, and widen `New(...)` to 7 args assigning `schema`
- Create: `internal/schema/gen.go` (`//go:generate` directive)
- Modify: `main.go` — `schema.Load()` as the 7th `New(...)` arg (real code)
- Modify: `Dockerfile` — declare `PRESETS_DIR` + `JOBS_DIR` env (no schema COPY — `options.json` is embedded in the binary)
- Modify: `docker-compose.example.yml` — declare `PRESETS_DIR` volume + `JOBS_DIR`

**Interfaces:**
- Consumes: `schema.Load` (Task 2); handler methods (Tasks 6–8); Plan 1's `New(cfg, runner, static, tpl, jobs)` widened by Plan 2 to `+presets`; `Server.Handler()`.
- Produces: final `func New(cfg config.Config, runner convert.Runner, static fs.FS, tpl *template.Template, jobs *jobs.Store, presets *presets.Store, schema *schema.Schema) *Server` (schema **last**).
- Note: `options.json` is **embedded into the `schema` package via `//go:embed`** (Task 2), so `schema.Load()` takes no path, there is no `SCHEMA_PATH` env, and the runtime image ships nothing extra — the schema is inside the compiled binary.

- [ ] **Step 1: Delete Plan 2's minimal editor (required — avoids a duplicate-function compile error and a double-registration panic)**

Plan 3's `internal/server/editor.go` already defines `handlePresetEditor`, `handlePresetSave`, and the VM types. Plan 2 shipped a *minimal* raw-YAML editor whose symbols collide with Plan 3's and whose routes register the same mux patterns. Two Go functions named `handlePresetSave` in package `server` is a compile error; registering `GET /settings/preset/{id}` twice on one `ServeMux` **panics at startup**. So remove Plan 2's versions:

- In `internal/server/editor.go` (Plan 2's file), delete the functions `func (s *Server) handlePresetEdit(...)` and `func (s *Server) handlePresetSave(...)` and the `type presetEditData struct { ... }`. If Plan 2 placed these in a differently named file (e.g. `presets_handlers.go`), delete them there instead — grep first: `grep -rn "handlePresetEdit\|handlePresetSave\|presetEditData" internal/server/`.
- Delete the template file: `git rm internal/web/templates/preset_edit.gohtml`.
- In `internal/server/server.go`, remove Plan 2's two registrations:
  ```go
  mux.HandleFunc("GET /settings/preset/{id}", s.handlePresetEdit)
  mux.HandleFunc("POST /settings/preset/{id}", s.handlePresetSave)
  ```

- [ ] **Step 2: Register Plan 3's routes and widen `New(...)`**

In `internal/server/server.go`, inside `Handler()`, register Plan 3's three routes before the catch-all `mux.Handle("/", ...)` (these are the ONLY `/settings/preset/{id}` registrations now):

```go
	mux.HandleFunc("GET /settings/preset/{id}", s.handlePresetEditor)
	mux.HandleFunc("POST /settings/preset/{id}", s.handlePresetSave)
	mux.HandleFunc("POST /settings/preset/{id}/effective", s.handlePresetEffective)
```

Widen the constructor to its final 7-arg form (schema **last**, per the shared contract) and assign the field added in Task 6. Plan 2 left `New` at `New(cfg, runner, static, tpl, jobs, presets)`; append `schema`:

```go
func New(cfg config.Config, runner convert.Runner, static fs.FS, tpl *template.Template, jobs *jobs.Store, presets *presets.Store, schema *schema.Schema) *Server {
	n := cfg.MaxConcurrent
	if n < 1 {
		n = 1
	}
	return &Server{
		cfg: cfg, runner: runner, static: static, tpl: tpl,
		jobs: jobs, presets: presets, schema: schema,
		sem: make(chan struct{}, n),
	}
}
```

- [ ] **Step 3: Add the go:generate directive**

Create `internal/schema/gen.go`:

```go
package schema

// Regenerate options.json from a real fbc binary (run from this directory):
//   FBC_BIN=/path/to/fbc go generate ./...
// schemagen prints a DRIFT report (keys added/removed/retyped) so the maintainer
// knows what to re-annotate after an fbc version bump.
//go:generate go run ../../cmd/schemagen -out options.json
```

- [ ] **Step 4: Wire schema into main.go (real code)**

Plan 2 already updated `main.go` to build `tpl`, `jobStore`, and `presetStore` and call the 6-arg `New`. Plan 3 adds the schema import, loads the embedded schema, and appends `sch` as the 7th argument. After Plan 2, `main()` looks like the left column; change it to the right column (add the two blocks):

```go
package main

import (
	"log"
	"net/http"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/schema"
	"fb2cng-web/internal/server"
	"fb2cng-web/internal/web"
	// ... plus Plan 1/2 imports for jobs + presets stores ...
)

func main() {
	cfg := config.FromEnv()

	sch, err := schema.Load()
	if err != nil {
		log.Fatalf("load embedded schema: %v", err)
	}

	// tpl, jobStore, presetStore are constructed by Plan 1/2 (templates parsed
	// from web.FS; stores from cfg.JobsDir/cfg.PresetsDir). Reuse those exactly.
	srv := server.New(cfg, convert.New(cfg.FBCBin), web.FS, tpl, jobStore, presetStore, sch)

	log.Printf("fb2cng-web listening on %s (fbc=%s, auth=%v)", cfg.Addr, cfg.FBCBin, cfg.ForwardAuth)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 5: Declare storage volumes**

`options.json` is embedded in the binary (`//go:embed`, Task 2), so the runtime image needs no schema COPY and no `SCHEMA_PATH` — nothing to ship. Only extend the runtime stage's `ENV` with the store locations. In `Dockerfile`:

```dockerfile
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080 TMPDIR=/tmp \
    PRESETS_DIR=/data/presets JOBS_DIR=/tmp/fb2cng-jobs
```

In `docker-compose.example.yml`, add the persistent presets volume and (tmpfs) jobs dir to the `fb2cng-web` service and declare the named volume:

```yaml
  fb2cng-web:
    build: .
    environment:
      AUTH_FORWARD_AUTH: "true"
      TRUSTED_PROXIES: ""
      PRESETS_DIR: /data/presets
      JOBS_DIR: /tmp/fb2cng-jobs
    volumes:
      - presets:/data/presets      # persistent named preset store
    tmpfs:
      - /tmp:rw,mode=1777          # jobs scratch (swept on TTL)
    expose:
      - "8080"

volumes:
  presets:
```

- [ ] **Step 6: Verify the build and full suite**

Run: `go build ./...`
Expected: success (Plan 3 executes after Plans 1 & 2, so `New`'s first six args, `tpl`, `jobStore`, `presetStore`, `layout`, and `render` all exist).

Run: `go test ./...`
Expected: PASS across `internal/schema`, `cmd/schemagen`, `internal/convert`, `internal/server`, `internal/config`.

Sanity-check the container builds: `docker build -t fb2cng-web:plan3 .` (expected: success; the schema is embedded in the binary, so no extra COPY layer is needed).

- [ ] **Step 7: Commit**

```bash
git add internal/server/server.go internal/server/editor.go internal/schema/gen.go main.go Dockerfile docker-compose.example.yml
git rm internal/web/templates/preset_edit.gohtml
git commit -m "feat: wire option schema + editor routes; drop Plan 2's minimal editor

Widens server.New to (cfg, runner, static, tpl, jobs, presets, schema) and
loads the //go:embed-ed options.json via schema.Load().

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage (Plan 3 scope):**
- `cmd/schemagen` reads dump YAML, flattens to dotted keys, infers Kind, Group=first segment, Label=last, merges best-effort descriptions from `docs/config.md`, writes `options.json`, prints DRIFT — Tasks 3, 4. Tests drive a fixture dump, not real fbc — Tasks 3, 4.
- `internal/schema` exactly per contract (Kind consts, Option+IsDefault, Schema no-arg `Load()` over the `//go:embed`-ed options.json + `Get`/`Groups`/`InGroup`) — Tasks 1, 2.
- Full option-grid editor replacing Plan 2's minimal editor; widgets by Kind (checkbox/number/select/textarea); `output_name_template` textarea + live filename preview; per-row description + `default X` + reset; amber changed marker (`.changed`, `changed-dot`); toolbar search / Changed-only / Default-preset / Save — Task 6 (markup mirrors mockup SCREEN 5).
- Save collects field values, drops equal-to-default (sparse), writes `Preset.Overrides` via `presets.Store.Save` — Task 7.
- Effective pane (`#effective`, htmx) merges via `convert.BuildConfig` + validity chip via `fbc` parse; `Runner.Validate` probe added to interface + FBC + fake — Tasks 5, 8.
- Options absent from `options.json` still render (synthetic rows) — Task 6.
- Editor uses the shared `base.gohtml` layout (`{{define "editor"}}` content template via `s.render`, embedding Plan 1's `layout` header for nav/theme/pre-paint) — NOT a standalone document — Task 6.
- `Server` gains one concrete field `schema *schema.Schema` (Task 6); `New(...)` widened to 7 args `(cfg, runner, static, tpl, jobs, presets, schema)` with schema last (Task 9); Plan 2's minimal editor (`handlePresetEdit`/`handlePresetSave`/`presetEditData`/`preset_edit.gohtml` + its routes) deleted to prevent duplicate-symbol/double-registration failures (Task 9); `main.go` loads schema and passes it in as real code — Task 9.
- `options.json` is embedded into the binary via `//go:embed` (no Dockerfile COPY, no `SCHEMA_PATH`); Dockerfile + docker-compose declare `PRESETS_DIR` volume + `JOBS_DIR` — Tasks 2, 9.

**Placeholder scan:** No `TBD`/`...`/"handle errors" — every code step is complete. `main.go` (Task 9 Step 4) is real code: `schema.Load()` passed as the 7th `New(...)` arg, no `_ = sch` fallback. The only cross-plan seams are the `tpl`/`jobStore`/`presetStore` construction lines in `main.go` and the `layout`/`render` symbols, all owned by Plans 1 & 2, which land before Plan 3.

**Type consistency:** `schema.Option`/`schema.Kind`/`schema.Schema`, `presets.Store`/`presets.Preset`, `convert.BuildConfig`/`convert.FormOptions`/`convert.Runner.Validate`, and the internal `rowVM`/`groupVM`/`effectiveVM`/`editorVM` + `flattenOverrides`/`unflatten`/`collectOverrides`/`computeEffective` names are used identically across Tasks 6–9. Route strings match across Tasks 6–9 (`GET`/`POST /settings/preset/{id}`, `POST /settings/preset/{id}/effective`).

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-02-redesign-3-option-editor.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks (REQUIRED SUB-SKILL: superpowers:subagent-driven-development).
2. **Inline Execution** — execute tasks in this session with checkpoints (REQUIRED SUB-SKILL: superpowers:executing-plans).

Plan 3 executes after Plans 1 & 2. Before starting Tasks 5–9, verify the consumed symbols exist: `Server.tpl`/`Server.presets`, the `layout` header struct and `func (s *Server) render(w, data)` helper, `base.gohtml`'s `{{template .ContentName .}}` dispatch, `presets.Store`, `Runner.ConvertLogged`, and the Plan 2 `New(cfg, runner, static, tpl, jobs, presets)` signature that Task 9 widens with `schema` last.
