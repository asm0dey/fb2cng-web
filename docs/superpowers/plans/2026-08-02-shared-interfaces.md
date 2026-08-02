# Redesign — Shared Interfaces Contract

Frozen signatures the three redesign plans agree on. Any plan that consumes or
produces one of these MUST use these exact names/types. Spec:
`docs/superpowers/specs/2026-08-02-fb2cng-web-redesign-design.md`.

## Existing (reused, do not change signatures)

```go
// internal/convert/config_yaml.go
type FormOptions struct { /* existing */ }
func BuildConfig(rawYAML string, o FormOptions) ([]byte, error)

// internal/convert/runner.go
type Runner interface {
    DumpDefaults(ctx context.Context) ([]byte, error)
    Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error)
}
```

Plan 1 extends `Runner` with a log-capturing convert (adds a method rather than
changing the existing one):

```go
// internal/convert/runner.go — ADD
// ConvertLogged behaves like Convert but also captures combined stdout+stderr
// into logPath (created/truncated). Returns outputs even on fbc failure when
// partial output exists; err is non-nil on non-zero exit.
func (f *FBC) ConvertLogged(ctx context.Context, inputPath, format, configPath, destDir, logPath string) (outs []string, err error)
```
The `Runner` interface gains `ConvertLogged` with the same signature so the
fake in tests implements it.

## internal/jobs (Plan 1 produces; Plans 2 & 3 do NOT depend on it)

```go
package jobs

type FileState string
const (
    StatePending FileState = "pending"
    StateRunning FileState = "running"
    StateDone    FileState = "done"
    StateFailed  FileState = "failed"
)

// FileResult is one input's outcome within a batch.
type FileResult struct {
    Input      string           `json:"input"`       // original filename
    State      FileState        `json:"state"`
    Outputs    []string         `json:"outputs"`     // output basenames under <job>/out/<input>/
    Sizes      map[string]int64 `json:"sizes"`       // basename -> bytes
    LogLines   int              `json:"log_lines"`
    FirstError string           `json:"first_error"` // first line starting "ERR", else ""
    Err        string           `json:"err"`         // short message, "" on success
    Millis     int64            `json:"millis"`
}

// Status is the whole batch, persisted as <job>/status.json.
type Status struct {
    ID       string       `json:"id"`
    Preset   string       `json:"preset"`   // preset id used ("" = Defaults)
    Format   string       `json:"format"`
    Files    []FileResult `json:"files"`
    Done     bool         `json:"done"`     // all files terminal
}

// Store owns JOBS_DIR. One subdir per job: <id>/in/, <id>/out/<input>/, <id>/<input>.log, <id>/status.json
type Store struct { /* Dir string; TTL time.Duration */ }

func NewStore(dir string, ttl time.Duration) *Store
func (s *Store) Create(preset, format string) (id string, err error) // mkdir tree, write initial status
func (s *Store) InputPath(id, input string) string                   // <id>/in/<input>
func (s *Store) OutDir(id, input string) string                      // <id>/out/<input>
func (s *Store) LogPath(id, input string) string                     // <id>/<input>.log
func (s *Store) Load(id string) (*Status, error)                     // read status.json
func (s *Store) Save(id string, st *Status) error                    // write status.json atomically
func (s *Store) OutputFile(id, input, base string) (string, error)   // safe-joined path, error if escapes
func (s *Store) WriteZip(id string, w io.Writer) error               // all outputs, flat, name-collision-suffixed
func (s *Store) Sweep(now time.Time) (removed int)                   // delete jobs older than TTL
```

Job id: URL-safe random token (e.g. 16 hex chars). All path builders MUST reject
`input`/`base` containing `/`, `\`, or `..` (return error / treat as not-found).

## internal/presets (Plan 2 produces; Plan 3 consumes)

```go
package presets

// Preset is a named sparse override map over fbc defaults.
type Preset struct {
    ID        string         `yaml:"-"`          // filename stem
    Name      string         `yaml:"name"`
    Overrides map[string]any `yaml:"overrides"`  // nested map, same shape BuildConfig merges
    UpdatedAt time.Time      `yaml:"updated_at"`
    Builtin   bool           `yaml:"-"`          // true only for synthetic "Defaults"
}

// ChangedCount = number of leaf keys in Overrides.
func (p *Preset) ChangedCount() int

// Store owns PRESETS_DIR. One file per preset: <id>.yaml. Default marker in _meta.yaml.
type Store struct { /* Dir string */ }

func NewStore(dir string) *Store
func (s *Store) List() ([]*Preset, error)          // includes synthetic Builtin "defaults" first
func (s *Store) Get(id string) (*Preset, error)    // "defaults" -> synthetic empty, Builtin=true
func (s *Store) Create(name string) (*Preset, error)
func (s *Store) Save(p *Preset) error              // rejects Builtin; sets UpdatedAt
func (s *Store) Delete(id string) error            // rejects Builtin & default
func (s *Store) Duplicate(id, newName string) (*Preset, error)
func (s *Store) DefaultID() string                 // "" if unset -> UI treats as "defaults"
func (s *Store) SetDefault(id string) error
```

`Get("defaults")` returns `&Preset{ID:"defaults", Name:"Defaults", Overrides:map[string]any{}, Builtin:true}`.
Overrides feed `convert.BuildConfig` by marshaling to YAML and passing as `rawYAML`
(form options nil) — presets are the raw-YAML layer.

## internal/schema (Plan 3 produces)

```go
package schema

type Kind string
const ( KindBool Kind="bool"; KindInt Kind="int"; KindString Kind="string"; KindEnum Kind="enum" )

// Option is one config key.
type Option struct {
    Key         string   `json:"key"`          // dotted path, e.g. "document.images.jpeg_quality_level"
    Group       string   `json:"group"`        // top-level section, e.g. "images"
    Label       string   `json:"label"`        // defaults to last path segment
    Kind        Kind     `json:"kind"`
    Default     any      `json:"default"`
    Enum        []string `json:"enum,omitempty"`
    Description string   `json:"description,omitempty"`
}

type Schema struct { Options []Option /* ordered; grouped by Group in file order */ }

func Load(path string) (*Schema, error)           // parse options.json
func (s *Schema) Get(key string) (Option, bool)
func (s *Schema) Groups() []string                // group names in first-seen order
func (s *Schema) InGroup(g string) []Option

// IsDefault reports whether val equals the option's default (for change markers).
func (o Option) IsDefault(val any) bool
```

`cmd/schemagen`: reads `fbc dumpconfig --default` YAML → flattens to dotted keys →
infers Kind from value type → Group = first path segment → merges descriptions
parsed from `docs/config.md` (best-effort) → writes `internal/schema/options.json`.
Prints a drift report vs the existing options.json (added/removed/retyped keys).

## HTTP / template conventions (all plans)

- Templates: `internal/web/templates/*.gohtml`, parsed once at startup via
  `embed.FS`, executed with `html/template`. Layout `base.gohtml` + page templates
  + partials (`_convert_card.gohtml`, `_effective.gohtml`, etc.).
- htmx (CDN) drives two swap regions only: the convert-card (`#convert-card`,
  polls `GET /jobs/{id}`) and the effective-config pane (`#effective`).
- Routes are added to `Server.Handler()` in `internal/server/server.go`.
- `Server` gains fields: `jobs *jobs.Store`, `presets *presets.Store`,
  `schema *schema.Schema`, `tpl *template.Template`. Constructor `New(...)`
  signature is extended; `main.go` wires the new stores from new env vars.
- New env (in `internal/config`): `PRESETS_DIR`, `JOBS_DIR`, `JOBS_TTL` (duration).
- CSS: single `internal/web/static/app.css`, 8 tokens on `:root` + one
  `@media (prefers-color-scheme: dark)`; `data-theme` attribute overrides. Served
  static. Fonts via Google Fonts `<link>` in `base.gohtml`.
- Testing: Go tests per package; handler tests via `httptest`; `testdata/fake-fbc.sh`
  extended to emit a multi-line log and to succeed on a "corrupt" input when a
  known override (`use_broken_images: true`) is present (exercises retry).
