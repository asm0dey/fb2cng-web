# Plan 2: Presets — Implementation Plan

> For agentic workers: execute this with **superpowers:subagent-driven-development**.
> Each `### Task N` is one self-contained TDD unit — write the full failing test, run it and
> confirm it FAILs, write the complete minimal implementation, run it and confirm it PASSes,
> then commit. No placeholders, no `// ...`, no "add validation later": every code block below
> is the whole thing.

**Goal:** Add a named, persisted, single **shared** preset store (`internal/presets`) exactly
per the shared-interfaces contract, expose a Settings/Presets page with CRUD + a default marker,
give presets a minimal usable editor (a name field + raw-YAML overrides textarea), and wire
presets into the Convert flow so a chosen preset's sparse override map is marshaled to YAML and
fed to `convert.BuildConfig` as the raw-YAML layer.

**Architecture:** One Go service (already shelling out to `fbc`). Presets are stored one file per
preset (`PRESETS_DIR/<id>.yaml`) holding `name`, `updated_at`, and a **sparse nested override map**
(only keys that differ from fbc defaults — the same shape `BuildConfig` already merges). A single
`_meta.yaml` holds the `default:` marker. A synthetic read-only Builtin **"Defaults"** preset
(id `defaults`, empty overrides) is never written to disk and is always first in `List()`.
Presets do not touch fbc directly — their `Overrides` marshal to YAML and go through
`convert.BuildConfig(rawYAML, convert.FormOptions{})`.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3` (already a dependency), `html/template` + Go 1.22
`net/http.ServeMux` path patterns (`{id}` / `r.PathValue`), `httptest` for handler tests. No new
third-party dependencies.

## Global Constraints

- **Go 1.26.** Module path `fb2cng-web`. Import the package as `fb2cng-web/internal/presets`.
- **Builds ON Plan 1.** Plan 1 ("Visual + Convert flow") owns and this plan does **not** redefine:
  the `Server` struct's Plan-1 fields, `Server.Handler()`'s existing routes, the
  `internal/web/templates/*.gohtml` embed + `s.tpl *template.Template` + `s.render` helper +
  `base.gohtml`/`layout` (layout + nav tabs), and `main.go`'s `tpl`/`jobs` wiring. Per the contract's
  per-plan field-growth rule, Plan 1 ships `New(cfg, runner, static, tpl, jobs)` with **no** presets
  field; this plan **appends** the field `presets *presets.Store` to `Server` and **widens** the
  constructor to `New(cfg, runner, static, tpl, jobs, presets)` (6 args, presets last), **adds** its
  route registrations to `Handler()`, **adds** content-only page templates into Plan 1's templates
  dir, and updates `main.go` + Plan 1's `newTestServer` to pass the real store.
- Presets = a **sparse override map** fed to `convert.BuildConfig` as its `rawYAML` argument, with
  `convert.FormOptions{}` (no form layer). The web app's `applicationDefaults()` still applies
  underneath, unchanged.
- **Single shared store, no per-user data.** Forward-auth only gates access; there is no per-user
  preset isolation. Self-hosted, low-hardening posture.
- Path stem = preset ID. Every path builder MUST reject unsafe ids (`""`, `_meta`, anything
  containing `/`, `\`, or `..`, anything not matching `^[a-z0-9][a-z0-9_-]*$`).
- `_meta.yaml` is reserved and never treated as a preset.

### Integration with Plan 1 — assumptions (Plan 1 was concurrent/absent when this was written)

Tasks 6–9 consume Plan 1 artifacts. Written against the contract; if Plan 1's actual shapes
differ, adjust the noted call sites (handler logic and data structs are unaffected):

- **`Server` fields + `New` arity (contract §"HTTP / template conventions", per-plan field growth):**
  Plan 1 ships `Server{ cfg, runner, static, tpl *template.Template, jobs *jobs.Store }` with
  `New(cfg, runner, static, tpl, jobs)` — **no `presets` field, no placeholders.** Plan 2 (Task 6)
  **appends** the field `presets *presets.Store` and **widens** the constructor to
  `New(cfg, runner, static, tpl, jobs, presets)` (6 args, presets last), updating `main.go` and Plan
  1's `newTestServer`. Plan 3 later appends `schema`.
- **One layout (contract-mandated).** `base.gohtml` defines `{{define "base"}}` (Google-Fonts links,
  pre-paint theme script, header with `fb2→epub` + Convert/Settings nav whose active tab comes from
  **`.Tab`** ∈ {`"convert"`,`"settings"`}, theme toggle) and then `{{template .ContentName .}}` for
  the body. Every page VM embeds `type layout struct { Tab, ContentName string; User string }` and is
  rendered via Plan 1's helper `func (s *Server) render(w http.ResponseWriter, data any)` (executes
  `"base"`). This plan's pages are **content-only** templates: `{{define "settings"}}` and
  `{{define "preset_edit"}}` — **no `head`/`foot` partials, no `.Active`, no outer `<html>`.**
- **Templates dir + embed**: Plan 1 parses `internal/web/templates/*.gohtml` once at startup into
  `s.tpl` and exposes a parse helper (referred to here as `web.Templates()`); new `.gohtml` files
  added here are picked up by that glob automatically.
- **Because of the above, execute Plan 2 after Plan 1 is merged.** Tasks 1–5 (the `internal/presets`
  package) are fully standalone and can run before Plan 1.
- **`app.css`** (`internal/web/static/app.css`) is owned by Plan 1, which defines the 8 design
  tokens on `:root` (`--bg`, `--bg-sunk`, `--line`, `--fg`, `--fg-muted`, `--fg-dim`, `--accent`,
  `--changed`) + the dark `@media` block. This plan **appends** a preset-specific class block that
  references those tokens; it does not redefine tokens or base rules.

### Target markup (mockup)

The exact target for the Settings/Presets page is **SCREEN 4** of
`docs/superpowers/specs/mockup/FB2-Converter.dc.html` (read the frames at lines ~880–1035: the
375px phone frame and the 1440px desktop frame). Task 6 reproduces that structure — one card with a
grid header row (`Default | Name | Overrides | Edited | actions`), a radio to pick the default
(picking POSTs immediately), per-row `N options changed` + edited date + Edit/Duplicate/Delete, a
read-only **Defaults · Built-in** row (no radio; View/Duplicate only), a `+ New preset` control, and
a collapsed **How presets work** `<details>` — with the mockup's inline styles lifted into app.css
classes. (SCREEN 5, the full preset editor, is Plan 3's; Task 8 here is only the minimal stand-in.)

## File Structure

| File | Action | Purpose |
|------|--------|---------|
| `internal/presets/presets.go` | create (Task 1) | `Preset`, `ChangedCount`, `BuiltinID`, id helpers |
| `internal/presets/presets_test.go` | create (Task 1) | unit tests for the above |
| `internal/presets/store.go` | create (Task 2), extend (Tasks 3–5) | `Store` + all methods |
| `internal/presets/store_test.go` | create (Task 2), extend (Tasks 3–5) | store tests |
| `internal/server/presets_http.go` | create (Task 6), extend (Tasks 7–9) | preset handlers + `buildConfigForPreset` |
| `internal/server/presets_http_test.go` | create (Task 6), extend (Tasks 7–9) | handler tests |
| `internal/web/templates/settings.gohtml` | create (Task 6) | Presets list page (mockup SCREEN 4) |
| `internal/web/static/app.css` | modify (Task 6) | append preset-specific classes (Plan 1 owns the file + tokens) |
| `internal/web/templates/preset_edit.gohtml` | create (Task 8) | minimal preset editor (Plan 3 replaces) |
| `internal/server/server.go` | modify (Tasks 6–8) | widen `New` (+`presets` field/arg), register preset routes in `Handler()` (Plan 1 owns the file) |
| `internal/server/convert_flow.go` | modify (Task 9) | `handleIndex` fills `pageVM.Presets` from the store; `handleConvert` builds config from the chosen preset (Plan 1 owns the file) |
| `main.go` | modify (Task 9) | construct `presets.NewStore(cfg.PresetsDir)`, pass as 6th `Server.New` arg (Plan 1 owns the file) |

---

### Task 1 — `Preset`, `ChangedCount`, and id helpers

**Files:** create `internal/presets/presets.go`, `internal/presets/presets_test.go`

**Interfaces**
- Consumes: none (stdlib + `gopkg.in/yaml.v3` already vendored).
- Produces (exact, from contract):
  ```go
  type Preset struct {
      ID        string         `yaml:"-"`
      Name      string         `yaml:"name"`
      Overrides map[string]any `yaml:"overrides"`
      UpdatedAt time.Time      `yaml:"updated_at"`
      Builtin   bool           `yaml:"-"`
  }
  func (p *Preset) ChangedCount() int   // number of leaf keys in Overrides
  const BuiltinID = "defaults"
  ```
  plus unexported helpers `validID(id string) bool` and `slugify(name string) string`.

**Steps**

- [ ] Write the full failing test `internal/presets/presets_test.go`:
  ```go
  package presets

  import "testing"

  func TestChangedCountLeaves(t *testing.T) {
      p := &Preset{Overrides: map[string]any{
          "document": map[string]any{
              "toc_type": "inline",
              "images":   map[string]any{"optimize": true, "jpeg_quality_level": 82},
          },
          "output": map[string]any{"format": "epub3"},
      }}
      if got := p.ChangedCount(); got != 4 {
          t.Fatalf("ChangedCount = %d, want 4", got)
      }
      if got := (&Preset{Overrides: map[string]any{}}).ChangedCount(); got != 0 {
          t.Fatalf("empty ChangedCount = %d, want 0", got)
      }
      if got := (&Preset{}).ChangedCount(); got != 0 {
          t.Fatalf("nil-overrides ChangedCount = %d, want 0", got)
      }
  }

  func TestValidID(t *testing.T) {
      good := []string{"kindle", "compact-2", "a_b", "x1"}
      for _, id := range good {
          if !validID(id) {
              t.Errorf("validID(%q) = false, want true", id)
          }
      }
      bad := []string{"", "_meta", "../etc", "a/b", `a\b`, "..", "Kindle", "-lead", "a b"}
      for _, id := range bad {
          if validID(id) {
              t.Errorf("validID(%q) = true, want false", id)
          }
      }
  }

  func TestSlugify(t *testing.T) {
      cases := map[string]string{
          "Kindle":            "kindle",
          "My Preset 2":       "my-preset-2",
          "  spaced  out  ":   "spaced-out",
          "!!!":               "preset",
          "Defaults":          "preset", // reserved id must not be produced
      }
      for in, want := range cases {
          if got := slugify(in); got != want {
              t.Errorf("slugify(%q) = %q, want %q", in, got, want)
          }
      }
  }
  ```

- [ ] Run and confirm FAIL: `go test ./internal/presets/ -run 'TestChangedCountLeaves|TestValidID|TestSlugify' -v`
  → expected: `# fb2cng-web/internal/presets [build failed]` (undefined: `Preset`, `validID`, `slugify`), `FAIL`.

- [ ] Write the full implementation `internal/presets/presets.go`:
  ```go
  // Package presets manages named sparse-override presets over fbc defaults.
  package presets

  import (
      "regexp"
      "strings"
      "time"
  )

  // Preset is a named sparse override map over fbc defaults.
  type Preset struct {
      ID        string         `yaml:"-"`         // filename stem
      Name      string         `yaml:"name"`
      Overrides map[string]any `yaml:"overrides"` // nested map, same shape BuildConfig merges
      UpdatedAt time.Time      `yaml:"updated_at"`
      Builtin   bool           `yaml:"-"` // true only for synthetic "Defaults"
  }

  // ChangedCount returns the number of leaf keys in Overrides.
  func (p *Preset) ChangedCount() int { return countLeaves(p.Overrides) }

  func countLeaves(m map[string]any) int {
      n := 0
      for _, v := range m {
          if sub, ok := v.(map[string]any); ok {
              n += countLeaves(sub)
              continue
          }
          n++
      }
      return n
  }

  // BuiltinID is the id of the synthetic read-only "Defaults" preset.
  const BuiltinID = "defaults"

  var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

  // validID reports whether id is a safe filename stem.
  func validID(id string) bool {
      if id == "" || len(id) > 64 || id == "_meta" {
          return false
      }
      if strings.Contains(id, "/") || strings.Contains(id, `\`) || strings.Contains(id, "..") {
          return false
      }
      return idRe.MatchString(id)
  }

  // slugify turns a display name into a safe candidate id.
  func slugify(name string) string {
      var b strings.Builder
      prevDash := false
      for _, r := range strings.ToLower(name) {
          switch {
          case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
              b.WriteRune(r)
              prevDash = false
          default:
              if !prevDash {
                  b.WriteByte('-')
                  prevDash = true
              }
          }
      }
      s := strings.Trim(b.String(), "-")
      if len(s) > 48 {
          s = strings.Trim(s[:48], "-")
      }
      if s == "" || s == BuiltinID || s == "_meta" {
          s = "preset"
      }
      return s
  }
  ```

- [ ] Run and confirm PASS: `go test ./internal/presets/ -run 'TestChangedCountLeaves|TestValidID|TestSlugify' -v`
  → expected: `--- PASS: TestChangedCountLeaves`, `--- PASS: TestValidID`, `--- PASS: TestSlugify`, `ok  fb2cng-web/internal/presets`.

- [ ] Commit:
  ```
  git add internal/presets/presets.go internal/presets/presets_test.go
  git commit -m "feat(presets): add Preset type, ChangedCount, and id helpers

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 2 — `Store.Get` and `Store.List`

**Files:** create `internal/presets/store.go`, `internal/presets/store_test.go`

**Interfaces**
- Consumes: `Preset`, `BuiltinID`, `validID` (Task 1).
- Produces (exact, from contract):
  ```go
  type Store struct { Dir string }
  func NewStore(dir string) *Store
  func (s *Store) Get(id string) (*Preset, error) // "defaults"/"" -> synthetic empty, Builtin=true
  func (s *Store) List() ([]*Preset, error)        // synthetic Builtin "defaults" first, then user presets
  ```
  plus unexported `builtin()` and `(s *Store) path/metaPath`, and the `meta` sidecar type.

**Steps**

- [ ] Write the full failing test `internal/presets/store_test.go`:
  ```go
  package presets

  import (
      "errors"
      "os"
      "path/filepath"
      "testing"
  )

  func writeRaw(t *testing.T, dir, id, body string) {
      t.Helper()
      if err := os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o644); err != nil {
          t.Fatal(err)
      }
  }

  func TestGetBuiltin(t *testing.T) {
      s := NewStore(t.TempDir())
      for _, id := range []string{"", "defaults"} {
          p, err := s.Get(id)
          if err != nil {
              t.Fatalf("Get(%q): %v", id, err)
          }
          if p.ID != "defaults" || p.Name != "Defaults" || !p.Builtin || len(p.Overrides) != 0 {
              t.Fatalf("Get(%q) = %+v, want synthetic Defaults", id, p)
          }
      }
  }

  func TestGetUnsafeID(t *testing.T) {
      s := NewStore(t.TempDir())
      if _, err := s.Get("../etc"); err == nil {
          t.Fatal("Get(unsafe) should error")
      }
  }

  func TestGetMissing(t *testing.T) {
      s := NewStore(t.TempDir())
      _, err := s.Get("nope")
      if !errors.Is(err, os.ErrNotExist) {
          t.Fatalf("Get(missing) err = %v, want os.ErrNotExist", err)
      }
  }

  func TestGetReadsFile(t *testing.T) {
      dir := t.TempDir()
      writeRaw(t, dir, "kindle", "name: Kindle\noverrides:\n  document:\n    toc_type: inline\n")
      p, err := NewStore(dir).Get("kindle")
      if err != nil {
          t.Fatal(err)
      }
      if p.ID != "kindle" || p.Name != "Kindle" || p.Builtin {
          t.Fatalf("got %+v", p)
      }
      doc, _ := p.Overrides["document"].(map[string]any)
      if doc["toc_type"] != "inline" {
          t.Fatalf("overrides not parsed: %+v", p.Overrides)
      }
  }

  func TestListBuiltinFirstAndSorted(t *testing.T) {
      dir := t.TempDir()
      writeRaw(t, dir, "beta", "name: Beta\noverrides: {}\n")
      writeRaw(t, dir, "alpha", "name: Alpha\noverrides: {}\n")
      // _meta.yaml must be ignored, not treated as a preset:
      os.WriteFile(filepath.Join(dir, "_meta.yaml"), []byte("default: alpha\n"), 0o644)

      list, err := NewStore(dir).List()
      if err != nil {
          t.Fatal(err)
      }
      var names []string
      for _, p := range list {
          names = append(names, p.Name)
      }
      want := []string{"Defaults", "Alpha", "Beta"}
      if len(names) != len(want) {
          t.Fatalf("List names = %v, want %v", names, want)
      }
      for i := range want {
          if names[i] != want[i] {
              t.Fatalf("List names = %v, want %v", names, want)
          }
      }
      if !list[0].Builtin {
          t.Fatal("first entry must be the Builtin")
      }
  }

  func TestListEmptyDir(t *testing.T) {
      list, err := NewStore(filepath.Join(t.TempDir(), "does-not-exist")).List()
      if err != nil {
          t.Fatalf("List on missing dir: %v", err)
      }
      if len(list) != 1 || !list[0].Builtin {
          t.Fatalf("List on missing dir = %+v, want [Defaults]", list)
      }
  }
  ```

- [ ] Run and confirm FAIL: `go test ./internal/presets/ -run 'TestGet|TestList' -v`
  → expected: `# fb2cng-web/internal/presets [build failed]` (undefined: `NewStore`, `Store`), `FAIL`.

- [ ] Write the full implementation `internal/presets/store.go`:
  ```go
  package presets

  import (
      "fmt"
      "os"
      "path/filepath"
      "sort"
      "strings"

      "gopkg.in/yaml.v3"
  )

  // Store owns PRESETS_DIR. One file per preset: <id>.yaml. Default marker in _meta.yaml.
  type Store struct {
      Dir string
  }

  // NewStore returns a Store rooted at dir. The directory is created lazily on write.
  func NewStore(dir string) *Store { return &Store{Dir: dir} }

  func (s *Store) path(id string) string  { return filepath.Join(s.Dir, id+".yaml") }
  func (s *Store) metaPath() string       { return filepath.Join(s.Dir, "_meta.yaml") }

  // meta is the sidecar holding the single default marker.
  type meta struct {
      Default string `yaml:"default"`
  }

  // builtin returns a fresh synthetic read-only "Defaults" preset.
  func builtin() *Preset {
      return &Preset{ID: BuiltinID, Name: "Defaults", Overrides: map[string]any{}, Builtin: true}
  }

  // Get returns a preset by id. "" or "defaults" returns the synthetic Builtin.
  func (s *Store) Get(id string) (*Preset, error) {
      if id == "" || id == BuiltinID {
          return builtin(), nil
      }
      if !validID(id) {
          return nil, fmt.Errorf("invalid preset id %q", id)
      }
      b, err := os.ReadFile(s.path(id))
      if err != nil {
          return nil, err
      }
      var p Preset
      if err := yaml.Unmarshal(b, &p); err != nil {
          return nil, fmt.Errorf("parse preset %q: %w", id, err)
      }
      p.ID = id
      p.Builtin = false
      if p.Overrides == nil {
          p.Overrides = map[string]any{}
      }
      return &p, nil
  }

  // List returns all presets, the synthetic Builtin first, then user presets sorted by name.
  func (s *Store) List() ([]*Preset, error) {
      out := []*Preset{builtin()}
      entries, err := os.ReadDir(s.Dir)
      if err != nil {
          if os.IsNotExist(err) {
              return out, nil
          }
          return nil, err
      }
      var users []*Preset
      for _, e := range entries {
          if e.IsDir() {
              continue
          }
          name := e.Name()
          if name == "_meta.yaml" || !strings.HasSuffix(name, ".yaml") {
              continue
          }
          id := strings.TrimSuffix(name, ".yaml")
          if !validID(id) {
              continue
          }
          p, err := s.Get(id)
          if err != nil {
              return nil, err
          }
          users = append(users, p)
      }
      sort.Slice(users, func(i, j int) bool {
          return strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name)
      })
      return append(out, users...), nil
  }
  ```

- [ ] Run and confirm PASS: `go test ./internal/presets/ -run 'TestGet|TestList' -v`
  → expected: `--- PASS` for `TestGetBuiltin`, `TestGetUnsafeID`, `TestGetMissing`, `TestGetReadsFile`, `TestListBuiltinFirstAndSorted`, `TestListEmptyDir`; `ok  fb2cng-web/internal/presets`.

- [ ] Commit:
  ```
  git add internal/presets/store.go internal/presets/store_test.go
  git commit -m "feat(presets): add Store with Get and List (synthetic Defaults first)

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 3 — `Store.Create` and `Store.Save` (sparse round-trip)

**Files:** extend `internal/presets/store.go`, `internal/presets/store_test.go`

**Interfaces**
- Consumes: `Store`, `Get`, `slugify`, `validID`, `BuiltinID`.
- Produces (exact, from contract):
  ```go
  func (s *Store) Create(name string) (*Preset, error) // new empty preset, persisted
  func (s *Store) Save(p *Preset) error                // rejects Builtin; sets UpdatedAt
  ```
  plus unexported `write(p *Preset) error` and `freeID(base string) (string, error)`.

**Steps**

- [ ] Add the full failing tests to `internal/presets/store_test.go`:
  ```go
  func TestCreatePersistsAndRoundTrips(t *testing.T) {
      s := NewStore(t.TempDir())
      p, err := s.Create("Kindle")
      if err != nil {
          t.Fatal(err)
      }
      if p.ID != "kindle" || p.Name != "Kindle" || p.UpdatedAt.IsZero() {
          t.Fatalf("Create = %+v", p)
      }
      got, err := s.Get("kindle")
      if err != nil {
          t.Fatalf("reload: %v", err)
      }
      if got.Name != "Kindle" || len(got.Overrides) != 0 {
          t.Fatalf("reloaded = %+v", got)
      }
  }

  func TestCreateUniqueIDs(t *testing.T) {
      s := NewStore(t.TempDir())
      a, _ := s.Create("Kindle")
      b, _ := s.Create("Kindle")
      if a.ID == b.ID {
          t.Fatalf("duplicate names produced same id %q", a.ID)
      }
      if b.ID != "kindle-2" {
          t.Fatalf("second id = %q, want kindle-2", b.ID)
      }
  }

  func TestSaveRejectsBuiltin(t *testing.T) {
      s := NewStore(t.TempDir())
      if err := s.Save(&Preset{ID: "defaults", Name: "x", Builtin: true}); err == nil {
          t.Fatal("Save(builtin) should error")
      }
      if err := s.Save(&Preset{ID: "defaults", Name: "x"}); err == nil {
          t.Fatal("Save(id=defaults) should error")
      }
  }

  func TestSaveSparseRoundTripAndStampsTime(t *testing.T) {
      s := NewStore(t.TempDir())
      p, _ := s.Create("Compact")
      p.Overrides = map[string]any{
          "document": map[string]any{
              "images": map[string]any{"optimize": true, "jpeg_quality_level": 70},
          },
      }
      if err := s.Save(p); err != nil {
          t.Fatal(err)
      }
      got, err := s.Get(p.ID)
      if err != nil {
          t.Fatal(err)
      }
      if got.ChangedCount() != 2 {
          t.Fatalf("round-tripped ChangedCount = %d, want 2", got.ChangedCount())
      }
      if got.UpdatedAt.IsZero() {
          t.Fatal("Save should stamp UpdatedAt")
      }
      img := got.Overrides["document"].(map[string]any)["images"].(map[string]any)
      if img["jpeg_quality_level"] != 70 || img["optimize"] != true {
          t.Fatalf("overrides not preserved: %+v", got.Overrides)
      }
  }
  ```

- [ ] Run and confirm FAIL: `go test ./internal/presets/ -run 'TestCreate|TestSave' -v`
  → expected: `# fb2cng-web/internal/presets [build failed]` (undefined: `Create`, `Save`), `FAIL`.

- [ ] Add the full implementation to `internal/presets/store.go` (add `"time"` to the import block and append these methods):
  ```go
  // Create makes a new empty preset named name and persists it.
  func (s *Store) Create(name string) (*Preset, error) {
      name = strings.TrimSpace(name)
      if name == "" {
          name = "New preset"
      }
      id, err := s.freeID(slugify(name))
      if err != nil {
          return nil, err
      }
      p := &Preset{ID: id, Name: name, Overrides: map[string]any{}}
      if err := s.write(p); err != nil {
          return nil, err
      }
      return p, nil
  }

  // freeID returns base, or base-2, base-3, … until an unused id is found.
  func (s *Store) freeID(base string) (string, error) {
      if !validID(base) {
          base = "preset"
      }
      candidate := base
      for i := 2; ; i++ {
          _, err := os.Stat(s.path(candidate))
          if os.IsNotExist(err) {
              return candidate, nil
          }
          if err != nil {
              return "", err
          }
          candidate = fmt.Sprintf("%s-%d", base, i)
      }
  }

  // Save persists p, rejecting the Builtin, and stamps UpdatedAt.
  func (s *Store) Save(p *Preset) error {
      if p.Builtin || p.ID == BuiltinID {
          return fmt.Errorf("cannot save built-in preset")
      }
      if !validID(p.ID) {
          return fmt.Errorf("invalid preset id %q", p.ID)
      }
      if p.Overrides == nil {
          p.Overrides = map[string]any{}
      }
      return s.write(p)
  }

  // write stamps UpdatedAt and atomically writes the preset file.
  func (s *Store) write(p *Preset) error {
      if err := os.MkdirAll(s.Dir, 0o755); err != nil {
          return err
      }
      p.UpdatedAt = time.Now().UTC().Truncate(time.Second)
      b, err := yaml.Marshal(p)
      if err != nil {
          return err
      }
      tmp := s.path(p.ID) + ".tmp"
      if err := os.WriteFile(tmp, b, 0o644); err != nil {
          return err
      }
      return os.Rename(tmp, s.path(p.ID))
  }
  ```

- [ ] Run and confirm PASS: `go test ./internal/presets/ -run 'TestCreate|TestSave' -v`
  → expected: `--- PASS` for `TestCreatePersistsAndRoundTrips`, `TestCreateUniqueIDs`, `TestSaveRejectsBuiltin`, `TestSaveSparseRoundTripAndStampsTime`; `ok  fb2cng-web/internal/presets`.

- [ ] Commit:
  ```
  git add internal/presets/store.go internal/presets/store_test.go
  git commit -m "feat(presets): add Create and Save with sparse-override round-trip

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 4 — `Store.DefaultID` and `Store.SetDefault` (`_meta.yaml`)

**Files:** extend `internal/presets/store.go`, `internal/presets/store_test.go`

**Interfaces**
- Consumes: `Store`, `Create`, `BuiltinID`, `validID`, the `meta` type, `metaPath`.
- Produces (exact, from contract):
  ```go
  func (s *Store) DefaultID() string   // "" if unset -> UI treats as "defaults"
  func (s *Store) SetDefault(id string) error
  ```
  plus unexported `writeMeta(m meta) error`.

**Steps**

- [ ] Add the full failing tests to `internal/presets/store_test.go`:
  ```go
  func TestDefaultUnset(t *testing.T) {
      if id := NewStore(t.TempDir()).DefaultID(); id != "" {
          t.Fatalf("DefaultID unset = %q, want \"\"", id)
      }
  }

  func TestSetDefaultRoundTrip(t *testing.T) {
      s := NewStore(t.TempDir())
      p, _ := s.Create("Kindle")
      if err := s.SetDefault(p.ID); err != nil {
          t.Fatal(err)
      }
      if id := s.DefaultID(); id != p.ID {
          t.Fatalf("DefaultID = %q, want %q", id, p.ID)
      }
  }

  func TestSetDefaultBuiltinClears(t *testing.T) {
      s := NewStore(t.TempDir())
      p, _ := s.Create("Kindle")
      s.SetDefault(p.ID)
      if err := s.SetDefault("defaults"); err != nil {
          t.Fatal(err)
      }
      if id := s.DefaultID(); id != "" {
          t.Fatalf("DefaultID after builtin = %q, want \"\"", id)
      }
  }

  func TestSetDefaultUnknown(t *testing.T) {
      s := NewStore(t.TempDir())
      if err := s.SetDefault("ghost"); err == nil {
          t.Fatal("SetDefault(unknown) should error")
      }
  }
  ```

- [ ] Run and confirm FAIL: `go test ./internal/presets/ -run 'TestDefault|TestSetDefault' -v`
  → expected: `# fb2cng-web/internal/presets [build failed]` (undefined: `DefaultID`, `SetDefault`), `FAIL`.

- [ ] Append the full implementation to `internal/presets/store.go`:
  ```go
  // DefaultID returns the id of the default preset, or "" if unset (UI treats "" as "defaults").
  func (s *Store) DefaultID() string {
      b, err := os.ReadFile(s.metaPath())
      if err != nil {
          return ""
      }
      var m meta
      if err := yaml.Unmarshal(b, &m); err != nil {
          return ""
      }
      if m.Default == BuiltinID {
          return ""
      }
      return m.Default
  }

  // SetDefault marks id as the default preset; the Builtin ("" or "defaults") clears the marker.
  func (s *Store) SetDefault(id string) error {
      if id == "" || id == BuiltinID {
          return s.writeMeta(meta{Default: ""})
      }
      if !validID(id) {
          return fmt.Errorf("invalid preset id %q", id)
      }
      if _, err := os.Stat(s.path(id)); err != nil {
          return fmt.Errorf("unknown preset %q", id)
      }
      return s.writeMeta(meta{Default: id})
  }

  // writeMeta atomically writes the _meta.yaml sidecar.
  func (s *Store) writeMeta(m meta) error {
      if err := os.MkdirAll(s.Dir, 0o755); err != nil {
          return err
      }
      b, err := yaml.Marshal(m)
      if err != nil {
          return err
      }
      tmp := s.metaPath() + ".tmp"
      if err := os.WriteFile(tmp, b, 0o644); err != nil {
          return err
      }
      return os.Rename(tmp, s.metaPath())
  }
  ```

- [ ] Run and confirm PASS: `go test ./internal/presets/ -run 'TestDefault|TestSetDefault' -v`
  → expected: `--- PASS` for `TestDefaultUnset`, `TestSetDefaultRoundTrip`, `TestSetDefaultBuiltinClears`, `TestSetDefaultUnknown`; `ok  fb2cng-web/internal/presets`.

- [ ] Commit:
  ```
  git add internal/presets/store.go internal/presets/store_test.go
  git commit -m "feat(presets): add DefaultID and SetDefault via _meta.yaml marker

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 5 — `Store.Delete` and `Store.Duplicate`

**Files:** extend `internal/presets/store.go`, `internal/presets/store_test.go`

**Interfaces**
- Consumes: `Store`, `Get`, `Create`, `write`, `DefaultID`, `BuiltinID`, `validID`.
- Produces (exact, from contract):
  ```go
  func (s *Store) Delete(id string) error                    // rejects Builtin & current default
  func (s *Store) Duplicate(id, newName string) (*Preset, error)
  ```
  plus unexported `deepCopy(m map[string]any) map[string]any`.

**Steps**

- [ ] Add the full failing tests to `internal/presets/store_test.go`:
  ```go
  func TestDeleteRemoves(t *testing.T) {
      s := NewStore(t.TempDir())
      p, _ := s.Create("Kindle")
      if err := s.Delete(p.ID); err != nil {
          t.Fatal(err)
      }
      if _, err := s.Get(p.ID); !errors.Is(err, os.ErrNotExist) {
          t.Fatalf("after delete Get err = %v, want os.ErrNotExist", err)
      }
  }

  func TestDeleteRejectsBuiltin(t *testing.T) {
      s := NewStore(t.TempDir())
      if err := s.Delete("defaults"); err == nil {
          t.Fatal("Delete(defaults) should error")
      }
      if err := s.Delete(""); err == nil {
          t.Fatal("Delete(\"\") should error")
      }
  }

  func TestDeleteRejectsCurrentDefault(t *testing.T) {
      s := NewStore(t.TempDir())
      p, _ := s.Create("Kindle")
      s.SetDefault(p.ID)
      if err := s.Delete(p.ID); err == nil {
          t.Fatal("Delete(default) should error")
      }
  }

  func TestDuplicateDeepCopiesOverrides(t *testing.T) {
      s := NewStore(t.TempDir())
      src, _ := s.Create("Kindle")
      src.Overrides = map[string]any{"document": map[string]any{"toc_type": "inline"}}
      s.Save(src)

      dup, err := s.Duplicate(src.ID, "Kindle copy")
      if err != nil {
          t.Fatal(err)
      }
      if dup.ID == src.ID {
          t.Fatalf("duplicate shares id %q", dup.ID)
      }
      if dup.Name != "Kindle copy" || dup.ChangedCount() != 1 {
          t.Fatalf("dup = %+v", dup)
      }
      // Mutating the duplicate's nested map must not affect the source on disk.
      dup.Overrides["document"].(map[string]any)["toc_type"] = "none"
      reSrc, _ := s.Get(src.ID)
      if reSrc.Overrides["document"].(map[string]any)["toc_type"] != "inline" {
          t.Fatal("Duplicate must deep-copy overrides")
      }
  }
  ```

- [ ] Run and confirm FAIL: `go test ./internal/presets/ -run 'TestDelete|TestDuplicate' -v`
  → expected: `# fb2cng-web/internal/presets [build failed]` (undefined: `Delete`, `Duplicate`), `FAIL`.

- [ ] Append the full implementation to `internal/presets/store.go`:
  ```go
  // Delete removes a preset, rejecting the Builtin and the current default.
  func (s *Store) Delete(id string) error {
      if id == "" || id == BuiltinID {
          return fmt.Errorf("cannot delete built-in preset")
      }
      if !validID(id) {
          return fmt.Errorf("invalid preset id %q", id)
      }
      if s.DefaultID() == id {
          return fmt.Errorf("cannot delete the default preset")
      }
      return os.Remove(s.path(id))
  }

  // Duplicate copies id's overrides into a new preset named newName.
  func (s *Store) Duplicate(id, newName string) (*Preset, error) {
      src, err := s.Get(id)
      if err != nil {
          return nil, err
      }
      dup, err := s.Create(newName)
      if err != nil {
          return nil, err
      }
      dup.Overrides = deepCopy(src.Overrides)
      if err := s.write(dup); err != nil {
          return nil, err
      }
      return dup, nil
  }

  // deepCopy clones a nested override map so callers cannot mutate the source.
  func deepCopy(m map[string]any) map[string]any {
      out := make(map[string]any, len(m))
      for k, v := range m {
          if sub, ok := v.(map[string]any); ok {
              out[k] = deepCopy(sub)
              continue
          }
          out[k] = v
      }
      return out
  }
  ```

- [ ] Run and confirm PASS (whole package regression): `go test ./internal/presets/... -v`
  → expected: all `--- PASS` including `TestDeleteRemoves`, `TestDeleteRejectsBuiltin`, `TestDeleteRejectsCurrentDefault`, `TestDuplicateDeepCopiesOverrides`; `ok  fb2cng-web/internal/presets`.

- [ ] Commit:
  ```
  git add internal/presets/store.go internal/presets/store_test.go
  git commit -m "feat(presets): add Delete and Duplicate (deep-copy overrides)

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 6 — Settings/Presets list page (`GET /settings`)

> **Consumes/extends Plan 1:** the `Server` struct + `New(cfg, runner, static, tpl, jobs)` — this task
> **appends** the `presets *presets.Store` field and **widens** `New` to 6 args (presets last);
> `Handler()`'s existing `mux`; the `layout` header struct + `s.render` helper + `base.gohtml`'s
> `{{define "base"}}` (which renders `{{template .ContentName .}}` and picks the active nav tab from
> `.Tab`); Plan 1's `tpl` parse helper (`web.Templates()`); and `internal/web/static/app.css`
> (8 tokens + dark block). Run after Plan 1. This page is a **content-only** `{{define "settings"}}`
> template — no `head`/`foot`, no `.Active`.
>
> **Target:** reproduce **SCREEN 4** of the mockup (`docs/superpowers/specs/mockup/FB2-Converter.dc.html`,
> lines ~880–1035). Read it before writing the template. The markup below lifts the mockup's inline
> styles into app.css classes.

**Files:** create `internal/server/presets_http.go`, `internal/server/presets_http_test.go`,
`internal/web/templates/settings.gohtml`; modify `internal/web/static/app.css` (append preset
classes) and `internal/server/server.go` (route registration).

**Interfaces**
- Consumes: `presets.Store` (`List`, `DefaultID`, `Create`), `presets.Preset` (`.ChangedCount`,
  `.UpdatedAt`, `.Name`, `.ID`, `.Builtin`).
- Produces:
  ```go
  func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request)
  type settingsData struct { layout; DefaultID string; Presets []*presets.Preset } // layout is Plan 1's header struct
  ```
  and the Task-6 widening of `Server`/`New` (append `presets *presets.Store`).

**Steps**

- [ ] Write the full failing test `internal/server/presets_http_test.go`:
  ```go
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
      tpl := web.Templates()
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
          "Kindle",              // user preset name
          "Defaults",            // built-in row
          "Built-in",            // built-in badge
          "options changed",     // overrides column phrasing (mockup SCREEN 4)
          "How presets work",    // collapsed details
          "+ New preset",        // create control
          `name="default"`,      // default radio
      } {
          if !strings.Contains(body, want) {
              t.Errorf("settings page missing %q", want)
          }
      }
  }
  ```

- [ ] Run and confirm FAIL: `go test ./internal/server/ -run TestSettingsListsPresets -v`
  → expected: this task both widens `New` (append the 6th `presets *presets.Store` arg + field) and
  adds `handleSettings`. Until the widened `New`/field/handler exist, the build fails
  (`too many arguments in call to New` / `undefined: (*Server).handleSettings` / `srv.presets`) →
  `# fb2cng-web/internal/server [build failed]`, `FAIL`.

  > **Widen `New` (Plan 2's field append, per contract §"HTTP / template conventions"):** in
  > `internal/server/server.go` add the field `presets *presets.Store` to the `Server` struct and
  > change the constructor to
  > `func New(cfg config.Config, runner convert.Runner, static fs.FS, tpl *template.Template, jobs *jobs.Store, presets *presets.Store) *Server`,
  > storing `presets` on the returned `Server`. Update Plan 1's existing `newTestServer` helper and
  > `main.go` (Task 9) to pass the new 6th argument.

- [ ] Create the template `internal/web/templates/settings.gohtml` (mockup SCREEN 4 structure —
  header + `+ New preset`, one card with a grid header row and per-preset rows, a read-only
  **Defaults · Built-in** row with no radio, and a **How presets work** `<details>`). The default
  radios live in **one** `<form>` (so they mutually exclude); picking one rewrites the form action to
  the chosen preset's set-default route and submits immediately. Duplicate/Delete are separate POST
  forms referenced by `form=` id (so they can sit inside the row markup without nesting `<form>`s);
  Delete confirms first. Per the contract's **one-layout** rule, `settings.gohtml` defines exactly
  one **content** template named `{{define "settings"}}` (no `head`/`foot` partials, no `.Active`,
  no outer `<html>`); Plan 1's `base.gohtml` supplies the chrome/nav and does `{{template
  .ContentName .}}`, and the handler renders it via Plan 1's `s.render` helper:
  ```gotemplate
  {{define "settings"}}
  <main class="presets-page">
    <div class="presets-head">
      <h1>Presets</h1>
      <form method="post" action="/settings/preset" class="new-preset">
        <input type="text" name="name" placeholder="New preset name" aria-label="New preset name">
        <button type="submit">+ New preset</button>
      </form>
    </div>

    <form id="default-form" method="post" class="presets-table">
      <div class="presets-row presets-row--head">
        <span>Default</span><span>Name</span><span>Overrides</span><span>Edited</span><span></span>
      </div>
      {{range .Presets}}
      <div class="presets-row{{if .Builtin}} presets-row--builtin{{end}}">
        <span class="preset-default">
          {{if not .Builtin}}
          <input type="radio" name="default" value="{{.ID}}"
            {{if eq .ID $.DefaultID}}checked{{end}}
            onchange="this.form.action='/settings/preset/'+this.value+'/default';this.form.submit()">
          {{end}}
        </span>
        <span class="preset-name">{{.Name}}{{if .Builtin}} <span class="badge">Built-in</span>{{end}}</span>
        <span class="preset-overrides">{{.ChangedCount}} options changed{{if .Builtin}} · read-only{{end}}</span>
        <span class="preset-edited">{{if .Builtin}}&mdash;{{else}}{{.UpdatedAt.Format "2 Jan 2006"}}{{end}}</span>
        <span class="preset-actions">
          {{if .Builtin}}<a href="/settings/preset/{{.ID}}">View</a>
          {{else}}<a href="/settings/preset/{{.ID}}">Edit</a>{{end}}
          <button type="submit" form="dup-{{.ID}}">Duplicate</button>
          {{if not .Builtin}}<button type="submit" form="del-{{.ID}}" class="danger">Delete</button>{{end}}
        </span>
      </div>
      {{end}}
    </form>

    {{range .Presets}}
    <form id="dup-{{.ID}}" method="post" action="/settings/preset/{{.ID}}/duplicate" hidden></form>
    {{if not .Builtin}}
    <form id="del-{{.ID}}" method="post" action="/settings/preset/{{.ID}}/delete"
      onsubmit="return confirm('Delete this preset?')" hidden></form>
    {{end}}
    {{end}}

    <details class="presets-help">
      <summary>How presets work</summary>
      <div class="presets-help-body">
        <p>A preset stores only the options you changed; everything else comes from fb2cng's built-in defaults at conversion time, so upgrading the binary picks up new defaults for free.</p>
        <p>The default preset is preselected on the Convert screen — that is the only thing the marker does.</p>
        <p>Presets live in <code>PRESETS_DIR/&lt;id&gt;.yaml</code>. Editing them by hand is equivalent to using this screen; the app re-reads on each request.</p>
      </div>
    </details>
  </main>
  {{end}}
  ```
  > **Note on the built-in "View" link:** it points at `/settings/preset/defaults`, which
  > `handlePresetEdit` (Task 8) redirects back to `/settings` because the Builtin is read-only. Plan
  > 3 replaces the editor with the full grid and makes this a real read-only view of the fbc defaults.

- [ ] Append the preset-specific classes to `internal/web/static/app.css` (Plan 1 owns the file and
  the 8 `:root` tokens this block references; do not redefine tokens). This lifts the mockup SCREEN 4
  inline styles into classes and stacks the grid on phones at the shared 640px breakpoint:
  ```css
  /* ===== Presets list (Plan 2) ===== */
  .presets-page { max-width: 900px; margin: 0 auto; padding: 32px 16px 56px; display: flex; flex-direction: column; gap: 14px; }
  .presets-head { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; }
  .presets-head h1 { margin: 0; font-size: 17px; font-weight: 600; }
  .new-preset { display: flex; gap: 8px; }
  .new-preset input { height: 34px; padding: 0 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--bg); color: var(--fg); font: inherit; }
  .new-preset button { height: 34px; padding: 0 14px; border: 0; border-radius: 6px; background: var(--accent); color: var(--bg); font: inherit; font-weight: 600; cursor: pointer; }
  .presets-table { border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
  .presets-row { display: grid; grid-template-columns: 88px 1fr 190px 150px 210px; gap: 16px; align-items: center; padding: 12px 16px; border-bottom: 1px solid var(--line); }
  .presets-row:last-child { border-bottom: 0; }
  .presets-row--head { background: var(--bg-sunk); font: 500 11px/1 'IBM Plex Mono', monospace; letter-spacing: .08em; text-transform: uppercase; color: var(--fg-muted); }
  .presets-row--builtin { background: var(--bg-sunk); }
  .preset-name { font-size: 15px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
  .preset-overrides, .preset-edited { font-size: 14px; color: var(--fg-muted); }
  .preset-actions { display: flex; gap: 14px; justify-content: flex-end; font-size: 14px; }
  .preset-actions a, .preset-actions button { background: none; border: 0; padding: 0; font: inherit; color: var(--accent); text-decoration: none; cursor: pointer; }
  .preset-actions .danger { color: oklch(0.5 0.17 27); }
  .badge { font: 500 10px/1 'IBM Plex Mono', monospace; letter-spacing: .06em; text-transform: uppercase; padding: 4px 6px; border-radius: 3px; border: 1px solid var(--line); color: var(--fg-muted); }
  .presets-help { border: 1px solid var(--line); border-radius: 8px; background: var(--bg-sunk); }
  .presets-help summary { padding: 11px 16px; font-size: 14px; font-weight: 500; cursor: pointer; }
  .presets-help-body { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; padding: 4px 16px 16px; font-size: 13.5px; line-height: 1.55; color: var(--fg-muted); }
  .presets-help-body p { margin: 0; }
  @media (max-width: 640px) {
    .presets-row--head { display: none; }
    .presets-row { grid-template-columns: auto 1fr; gap: 6px 10px; }
    .preset-name, .preset-overrides, .preset-edited { grid-column: 2; }
    .preset-actions { grid-column: 1 / -1; justify-content: flex-start; }
    .presets-help-body { grid-template-columns: 1fr; }
  }
  ```

- [ ] Create `internal/server/presets_http.go` with the handler and data type. The VM embeds Plan
  1's common header struct `layout{ Tab, ContentName string; User string }` (Tab drives the active
  nav tab; ContentName selects the content template `base` renders), and the handler renders through
  Plan 1's `func (s *Server) render(w http.ResponseWriter, data any)` helper:
  ```go
  package server

  import (
      "net/http"

      "fb2cng-web/internal/presets"
  )

  // settingsData is the Presets list page VM. layout is Plan 1's shared header struct.
  type settingsData struct {
      layout
      DefaultID string
      Presets   []*presets.Preset
  }

  func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
      list, err := s.presets.List()
      if err != nil {
          http.Error(w, "server error", http.StatusInternalServerError)
          return
      }
      def := s.presets.DefaultID()
      if def == "" {
          def = presets.BuiltinID
      }
      s.render(w, settingsData{
          layout:    layout{Tab: "settings", ContentName: "settings"},
          DefaultID: def,
          Presets:   list,
      })
  }
  ```

- [ ] Register the route in `internal/server/server.go`'s `Handler()` (Plan 1 owns this file; add this
  line alongside the other `mux.HandleFunc` calls, before the `mux.Handle("/", ...)` fallback):
  ```go
  mux.HandleFunc("GET /settings", s.handleSettings)
  ```

- [ ] Run and confirm PASS: `go test ./internal/server/ -run TestSettingsListsPresets -v`
  → expected: `--- PASS: TestSettingsListsPresets`; `ok  fb2cng-web/internal/server`.

- [ ] Commit:
  ```
  git add internal/server/presets_http.go internal/server/presets_http_test.go internal/web/templates/settings.gohtml internal/web/static/app.css internal/server/server.go
  git commit -m "feat(server): add GET /settings presets list page (mockup SCREEN 4)

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 7 — Preset CRUD routes (create / duplicate / delete / set-default)

> **Consumes Plan 1:** `Server.presets`, `Handler()`'s `mux`. Go 1.22 `{id}` path params via
> `r.PathValue("id")`.

**Files:** extend `internal/server/presets_http.go`, `internal/server/presets_http_test.go`;
modify `internal/server/server.go` (routes).

**Interfaces**
- Consumes: `presets.Store` (`Create`, `Duplicate`, `Delete`, `SetDefault`, `Get`).
- Produces:
  ```go
  func (s *Server) handlePresetCreate(w http.ResponseWriter, r *http.Request)
  func (s *Server) handlePresetDuplicate(w http.ResponseWriter, r *http.Request)
  func (s *Server) handlePresetDelete(w http.ResponseWriter, r *http.Request)
  func (s *Server) handlePresetDefault(w http.ResponseWriter, r *http.Request)
  ```

**Steps**

- [ ] Add the full failing tests to `internal/server/presets_http_test.go` (add `"net/url"`,
  `"os"`, `"path/filepath"`, `"errors"` imports as needed):
  ```go
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
  ```

- [ ] Run and confirm FAIL: `go test ./internal/server/ -run 'TestCreateRedirects|TestSetDefaultRoute|TestDuplicateRoute|TestDeleteRoute|TestDeleteDefaultBlocked' -v`
  → expected: `undefined: (*Server).handlePresetCreate` (and siblings) → `[build failed]`, `FAIL`.

- [ ] Append the handlers to `internal/server/presets_http.go` (add `"strings"` to its import block):
  ```go
  func (s *Server) handlePresetCreate(w http.ResponseWriter, r *http.Request) {
      if err := r.ParseForm(); err != nil {
          http.Error(w, "bad form", http.StatusBadRequest)
          return
      }
      name := strings.TrimSpace(r.FormValue("name"))
      if name == "" {
          name = "New preset"
      }
      p, err := s.presets.Create(name)
      if err != nil {
          http.Error(w, err.Error(), http.StatusUnprocessableEntity)
          return
      }
      http.Redirect(w, r, "/settings/preset/"+p.ID, http.StatusSeeOther)
  }

  func (s *Server) handlePresetDuplicate(w http.ResponseWriter, r *http.Request) {
      id := r.PathValue("id")
      src, err := s.presets.Get(id)
      if err != nil {
          http.Error(w, "not found", http.StatusNotFound)
          return
      }
      if _, err := s.presets.Duplicate(id, src.Name+" copy"); err != nil {
          http.Error(w, err.Error(), http.StatusUnprocessableEntity)
          return
      }
      http.Redirect(w, r, "/settings", http.StatusSeeOther)
  }

  func (s *Server) handlePresetDelete(w http.ResponseWriter, r *http.Request) {
      if err := s.presets.Delete(r.PathValue("id")); err != nil {
          http.Error(w, err.Error(), http.StatusUnprocessableEntity)
          return
      }
      http.Redirect(w, r, "/settings", http.StatusSeeOther)
  }

  func (s *Server) handlePresetDefault(w http.ResponseWriter, r *http.Request) {
      if err := s.presets.SetDefault(r.PathValue("id")); err != nil {
          http.Error(w, err.Error(), http.StatusUnprocessableEntity)
          return
      }
      http.Redirect(w, r, "/settings", http.StatusSeeOther)
  }
  ```

- [ ] Register the routes in `internal/server/server.go`'s `Handler()` (add next to the `GET /settings`
  line from Task 6):
  ```go
  mux.HandleFunc("POST /settings/preset", s.handlePresetCreate)
  mux.HandleFunc("POST /settings/preset/{id}/duplicate", s.handlePresetDuplicate)
  mux.HandleFunc("POST /settings/preset/{id}/delete", s.handlePresetDelete)
  mux.HandleFunc("POST /settings/preset/{id}/default", s.handlePresetDefault)
  ```

- [ ] Run and confirm PASS: `go test ./internal/server/ -run 'TestCreateRedirects|TestSetDefaultRoute|TestDuplicateRoute|TestDeleteRoute|TestDeleteDefaultBlocked' -v`
  → expected: `--- PASS` for all five; `ok  fb2cng-web/internal/server`.

- [ ] Commit:
  ```
  git add internal/server/presets_http.go internal/server/presets_http_test.go internal/server/server.go
  git commit -m "feat(server): add preset create/duplicate/delete/set-default routes

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 8 — Minimal preset editor (`GET`/`POST /settings/preset/{id}`)

> **This editor is intentionally minimal — a name field plus a raw-YAML overrides textarea.**
> **Plan 3 (Option editor + schema) REPLACES this minimal editor with the full grouped option
> grid, per-option reset/change markers, search, and the live effective-config pane.** The route
> paths and the `handlePresetSave` parse-back-into-`Overrides` contract stay; only the rendered
> form and the effective-config wiring change in Plan 3.

**Files:** extend `internal/server/presets_http.go`, `internal/server/presets_http_test.go`;
create `internal/web/templates/preset_edit.gohtml`; modify `internal/server/server.go` (routes).

**Interfaces**
- Consumes: `presets.Store` (`Get`, `Save`), `presets.Preset`, Plan 1's `layout` + `s.render`,
  `gopkg.in/yaml.v3`.
- Produces:
  ```go
  func (s *Server) handlePresetEdit(w http.ResponseWriter, r *http.Request) // GET
  func (s *Server) handlePresetSave(w http.ResponseWriter, r *http.Request) // POST
  type presetEditData struct { layout; Preset *presets.Preset; YAML, Error string } // content template "preset_edit"
  ```

**Steps**

- [ ] Add the full failing tests to `internal/server/presets_http_test.go`:
  ```go
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
  ```

- [ ] Run and confirm FAIL: `go test ./internal/server/ -run 'TestEditor|TestEditBuiltin|TestSaveParses|TestSaveInvalid' -v`
  → expected: `undefined: (*Server).handlePresetEdit` / `handlePresetSave` → `[build failed]`, `FAIL`.

- [ ] Create the template `internal/web/templates/preset_edit.gohtml`. Same one-layout rule: a single
  **content** template `{{define "preset_edit"}}` (no `head`/`foot`, no `.Active`, no outer `<html>`),
  rendered through Plan 1's `base` by the `s.render` helper. (Plan 3 replaces this whole file with the
  full option-grid editor, which the contract names `{{define "editor"}}`.)
  ```gotemplate
  {{define "preset_edit"}}
  <main class="preset-editor">
    <h1>Edit preset</h1>
    {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
    <form method="post" action="/settings/preset/{{.Preset.ID}}">
      <label>Name
        <input type="text" name="name" value="{{.Preset.Name}}">
      </label>
      <label>Overrides (YAML)
        <textarea name="overrides_yaml" rows="20" spellcheck="false">{{.YAML}}</textarea>
      </label>
      <p class="hint">Minimal editor &mdash; Plan 3 replaces this with the full option grid.</p>
      <button type="submit">Save</button>
      <a href="/settings">Cancel</a>
    </form>
  </main>
  {{end}}
  ```

- [ ] Append the handlers to `internal/server/presets_http.go` (add `"gopkg.in/yaml.v3"` to its import
  block):
  ```go
  func (s *Server) handlePresetEdit(w http.ResponseWriter, r *http.Request) {
      id := r.PathValue("id")
      p, err := s.presets.Get(id)
      if err != nil {
          http.Error(w, "not found", http.StatusNotFound)
          return
      }
      if p.Builtin {
          http.Redirect(w, r, "/settings", http.StatusSeeOther)
          return
      }
      yamlText := ""
      if len(p.Overrides) > 0 {
          b, err := yaml.Marshal(p.Overrides)
          if err != nil {
              http.Error(w, "server error", http.StatusInternalServerError)
              return
          }
          yamlText = string(b)
      }
      s.render(w, presetEditData{
          layout: layout{Tab: "settings", ContentName: "preset_edit"},
          Preset: p,
          YAML:   yamlText,
      })
  }

  func (s *Server) handlePresetSave(w http.ResponseWriter, r *http.Request) {
      id := r.PathValue("id")
      if err := r.ParseForm(); err != nil {
          http.Error(w, "bad form", http.StatusBadRequest)
          return
      }
      name := strings.TrimSpace(r.FormValue("name"))
      if name == "" {
          name = id
      }
      yamlText := r.FormValue("overrides_yaml")
      overrides := map[string]any{}
      if strings.TrimSpace(yamlText) != "" {
          if err := yaml.Unmarshal([]byte(yamlText), &overrides); err != nil {
              w.WriteHeader(http.StatusUnprocessableEntity)
              s.render(w, presetEditData{
                  layout: layout{Tab: "settings", ContentName: "preset_edit"},
                  Preset: &presets.Preset{ID: id, Name: name},
                  YAML:   yamlText,
                  Error:  "invalid YAML: " + err.Error(),
              })
              return
          }
      }
      p := &presets.Preset{ID: id, Name: name, Overrides: overrides}
      if err := s.presets.Save(p); err != nil {
          http.Error(w, err.Error(), http.StatusUnprocessableEntity)
          return
      }
      http.Redirect(w, r, "/settings", http.StatusSeeOther)
  }
  ```
  And add the data type near `settingsData` (embedding Plan 1's `layout` header struct):
  ```go
  type presetEditData struct {
      layout
      Preset *presets.Preset
      YAML   string
      Error  string
  }
  ```

- [ ] Register the routes in `internal/server/server.go`'s `Handler()`:
  ```go
  mux.HandleFunc("GET /settings/preset/{id}", s.handlePresetEdit)
  mux.HandleFunc("POST /settings/preset/{id}", s.handlePresetSave)
  ```

- [ ] Run and confirm PASS: `go test ./internal/server/ -run 'TestEditor|TestEditBuiltin|TestSaveParses|TestSaveInvalid' -v`
  → expected: `--- PASS` for `TestEditorSeedsYAML`, `TestEditBuiltinRedirects`, `TestSaveParsesYAMLBackIntoOverrides`, `TestSaveInvalidYAML`; `ok  fb2cng-web/internal/server`.

- [ ] Commit:
  ```
  git add internal/server/presets_http.go internal/server/presets_http_test.go internal/web/templates/preset_edit.gohtml internal/server/server.go
  git commit -m "feat(server): add minimal preset editor (name + raw-YAML overrides)

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 9 — Wire presets into the Convert flow + `main.go`

> **Consumes/modifies Plan 1's REAL code (not `handlers.go`):** Plan 1 puts the convert page/handler
> in `internal/server/convert_flow.go` as `handleIndex` (page) and `handleConvert` (POST). The page
> VM is `pageVM`, which **already has** a `Presets []presetOption` field, and `convert.gohtml`
> **already** does `{{range .Presets}}` — Plan 1 seeds it with a Defaults-only stub and builds config
> via `BuildConfig("", convert.FormOptions{})`. So this task does **not** add `Presets`/`PresetDefault`
> fields or replace a hardcoded `<option>`. It: (1) populates `pageVM.Presets` from `presets.Store.List()`
> (mapping each `*presets.Preset` → `presetOption{ID,Name}`), marking the `DefaultID()` entry selected;
> (2) makes `handleConvert` read `r.FormValue("preset")`, `presets.Store.Get(id)`, and build config via
> the `buildConfigForPreset` seam; (3) wires `main.go` to construct `presets.NewStore(cfg.PresetsDir)`
> and pass it as the 6th `New(...)` argument.

**Files:** extend `internal/server/presets_http.go`, `internal/server/presets_http_test.go`;
modify `internal/server/convert_flow.go` (Plan 1's `handleIndex`/`handleConvert`/`pageVM`), and
`main.go`. (`convert.gohtml` needs no change — its `{{range .Presets}}` already renders the list;
confirm its `<option>` marks selection from the `presetOption` selected flag, see below.)

**Interfaces**
- Consumes: `presets.Store` (`Get`, `List`, `DefaultID`), `convert.BuildConfig(rawYAML string, o
  convert.FormOptions) ([]byte, error)`, `convert.FormOptions{}`, `gopkg.in/yaml.v3`.
- Produces (the testable seam the convert handler and dropdown consume):
  ```go
  func (s *Server) buildConfigForPreset(id string) ([]byte, error) // preset overrides -> YAML -> BuildConfig
  ```

**Steps**

- [ ] Add the full failing test to `internal/server/presets_http_test.go`:
  ```go
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
  ```

- [ ] Run and confirm FAIL: `go test ./internal/server/ -run 'TestBuildConfigFor' -v`
  → expected: `undefined: (*Server).buildConfigForPreset` → `[build failed]`, `FAIL`.

- [ ] Append the seam to `internal/server/presets_http.go` (add `"fb2cng-web/internal/convert"` to its
  import block):
  ```go
  // buildConfigForPreset marshals a preset's sparse overrides to YAML and feeds them to
  // convert.BuildConfig as the raw-YAML layer (no form options). The Builtin "defaults" preset
  // has empty overrides, so this yields just the application defaults.
  func (s *Server) buildConfigForPreset(id string) ([]byte, error) {
      p, err := s.presets.Get(id)
      if err != nil {
          return nil, err
      }
      raw := ""
      if len(p.Overrides) > 0 {
          b, err := yaml.Marshal(p.Overrides)
          if err != nil {
              return nil, err
          }
          raw = string(b)
      }
      return convert.BuildConfig(raw, convert.FormOptions{})
  }
  ```

- [ ] Run and confirm PASS: `go test ./internal/server/ -run 'TestBuildConfigFor' -v`
  → expected: `--- PASS: TestBuildConfigForPreset`, `--- PASS: TestBuildConfigForDefaults`; `ok  fb2cng-web/internal/server`.

- [ ] **Populate `pageVM.Presets` from the real store (Plan 1's `handleIndex` in
  `internal/server/convert_flow.go`).** Do **not** add fields — `pageVM.Presets []presetOption` already
  exists. Add a helper next to `buildConfigForPreset` that maps the store's presets to Plan 1's
  `presetOption`, marking the default selected, and call it from `handleIndex` where Plan 1 currently
  assigns the Defaults-only stub:
  ```go
  // presetOptions returns Plan 1's convert-page dropdown options from the real preset store,
  // with the default preset marked selected. presetOption is Plan 1's type (fields ID, Name,
  // Selected — align the field names below if Plan 1 differs).
  func (s *Server) presetOptions() []presetOption {
      list, err := s.presets.List()
      if err != nil {
          return []presetOption{{ID: presets.BuiltinID, Name: "Defaults", Selected: true}}
      }
      def := s.presets.DefaultID()
      if def == "" {
          def = presets.BuiltinID
      }
      opts := make([]presetOption, 0, len(list))
      for _, p := range list {
          opts = append(opts, presetOption{ID: p.ID, Name: p.Name, Selected: p.ID == def})
      }
      return opts
  }
  ```
  In `handleIndex`, replace Plan 1's stub assignment with `vm.Presets = s.presetOptions()` (keeping the
  rest of `pageVM` construction as Plan 1 wrote it). If Plan 1's `presetOption` has no `Selected` field,
  drop that field here and instead mark selection however `convert.gohtml` expects (e.g. a
  `pageVM.PresetDefault` compared in the `<option>`); the `{{range .Presets}}` loop itself is unchanged.

- [ ] **Wire the convert POST (Plan 1's `handleConvert` in `convert_flow.go`).** Plan 1 builds config
  via `BuildConfig("", convert.FormOptions{})`. Replace that single config-building line with a lookup
  keyed on Plan 1's existing `preset` form field, through the seam:
  ```go
  cfgBytes, err := s.buildConfigForPreset(r.FormValue("preset"))
  if err != nil {
      http.Error(w, "invalid preset: "+err.Error(), http.StatusUnprocessableEntity)
      return
  }
  ```
  (`buildConfigForPreset` treats `""`/`"defaults"` as the Builtin, so an absent field is safe; it still
  layers `convert.applicationDefaults()` underneath. Import `fb2cng-web/internal/presets` in
  `convert_flow.go` if not already present.)

- [ ] **Wire `main.go`** (Plan 1 owns the file; add the real construction and pass it as the 6th
  `New(...)` argument, leaving Plan 1's `tpl`/`jobs` wiring intact). Plan 1 added `PRESETS_DIR` to
  `internal/config` as `cfg.PresetsDir`:
  ```go
  presetStore := presets.NewStore(cfg.PresetsDir)
  srv := server.New(cfg, convert.New(cfg.FBCBin), web.FS, tpl, jobStore, presetStore)
  ```
  where `tpl` and `jobStore` are Plan 1's existing locals in `main.go` (the parsed templates and the
  `jobs.NewStore(cfg.JobsDir, cfg.JobsTTL)` value). Add the import `"fb2cng-web/internal/presets"`.

- [ ] Run the full suite and confirm PASS: `go build ./... && go test ./...`
  → expected: `ok  fb2cng-web/internal/presets`, `ok  fb2cng-web/internal/server`, and the rest of the
  module green (no compile errors from the `convert_flow.go` / `main.go` edits).

- [ ] Commit:
  ```
  git add internal/server/presets_http.go internal/server/presets_http_test.go internal/server/convert_flow.go main.go
  git commit -m "feat(convert): drive conversion config from the selected preset

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

## Done criteria

- `internal/presets` implements the contract exactly: `Preset` + `ChangedCount`, `Store` with
  `NewStore`, `Get`, `List`, `Create`, `Save`, `Delete`, `Duplicate`, `DefaultID`, `SetDefault`;
  one `<id>.yaml` per preset (sparse nested overrides + `name` + `updated_at`); `_meta.yaml` default
  marker; synthetic Builtin "Defaults" first in `List()` and returned by `Get("defaults")`; `Save`
  rejects the Builtin; `Delete` rejects the Builtin and the current default; unsafe ids rejected.
- `GET /settings` reproduces the mockup **SCREEN 4** structure using Plan 1's `base.gohtml` + nav:
  the grid card (Default radio / Name / `N options changed` / Edited date / Edit·Duplicate·Delete),
  the read-only **Defaults · Built-in** row (no radio; View/Duplicate), `+ New preset`, and the
  **How presets work** `<details>` — with the mockup's inline styles lifted into `app.css` classes
  appended under Plan 1's tokens.
- `POST /settings/preset`, `.../{id}/duplicate`, `.../{id}/delete`, `.../{id}/default` work and
  redirect to the appropriate page.
- Minimal editor (`GET`/`POST /settings/preset/{id}`) round-trips name + raw-YAML overrides;
  **explicitly flagged for replacement by Plan 3's full option grid.**
- `Server` grows a `presets *presets.Store` field and `New` is widened to
  `New(cfg, runner, static, tpl, jobs, presets)` (6 args); Plan 1's `newTestServer` and `main.go`
  updated to pass it.
- The Convert tab (`handleIndex`/`handleConvert` in `convert_flow.go`): `pageVM.Presets` is filled
  from `presets.Store.List()` with the default marked selected (existing `{{range .Presets}}` in
  `convert.gohtml` unchanged); `handleConvert` builds its fbc config from `r.FormValue("preset")` via
  `buildConfigForPreset`.
- `main.go` constructs `presets.NewStore(cfg.PresetsDir)` and passes it as the 6th `Server.New` arg.
- `go build ./... && go test ./...` is green.
