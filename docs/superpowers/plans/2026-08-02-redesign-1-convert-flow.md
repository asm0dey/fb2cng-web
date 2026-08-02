# Plan 1: Visual + Convert Flow — Implementation Plan

> For agentic workers: execute this plan with `superpowers:subagent-driven-development`.
> Each task is a self-contained TDD unit — write the failing test, run it (see it fail),
> write the minimal implementation, run it (see it pass), commit. Do the steps in order;
> do not skip the "run and observe" steps. Every code block below is complete — type it
> verbatim, there are no placeholders to fill in.

**Goal:** Deliver the whole redesigned look and the full Convert flow. Replace the Pico/SPA
front end with a server-rendered `html/template` + htmx UI carrying the mockup's 8-token
theme, and stand up a retained job store so a batch conversion produces per-file results,
stored logs, download-all zip, and retry. Presets are represented by the built-in "Defaults"
only (Plan 2 wires real presets); the Settings tab is a stub link (Plan 2 fills it); the
option editor and schema are Plan 3. This plan **owns** the new `Server` struct and `New()`
signature that Plans 2 and 3 extend.

**Architecture:** One Go service that shells out to `fbc`. `POST /convert` writes a job dir,
persists the uploaded inputs, then runs `fbc` per input in a background goroutine (capped by
the existing semaphore), capturing each run's combined stdout+stderr to a `<input>.log` and
recording per-file state/outputs/first-error into `status.json`. The browser shows a
convert-card that htmx polls (`GET /jobs/{id}`) until the batch is terminal, then renders
result rows with download links, a "Download all .zip" link, a full-log `<details>`, and — for
failures — a first-error `<pre>` plus a Retry button. Retry re-runs only the failed inputs
with `use_broken_images: true` merged over the config, reusing the retained inputs.

**Tech Stack:** Go 1.26.3 (`module fb2cng-web`), stdlib `html/template` + `embed`,
`gopkg.in/yaml.v3` (already a dep), htmx via CDN `<script>`, IBM Plex Mono + Helvetica Neue
via a Google Fonts `<link>`. No new Go module dependencies. Tests use `testdata/fake-fbc.sh`.

## Global Constraints

- **Go 1.26** — module `fb2cng-web`, `go 1.26.3`. No new third-party Go deps; only stdlib +
  the existing `gopkg.in/yaml.v3`.
- **Shells to `fbc`** — all conversion goes through the `convert.Runner` interface. Tests use
  `testdata/fake-fbc.sh`; never invoke a real `fbc`.
- **Reuse, do not reinvent** — reuse `convert.BuildConfig` for effective-config layering,
  reuse the `ForwardAuth(enabled, trustedProxies, next)` middleware unchanged, and reuse the
  `Server.sem` semaphore (`chan struct{}`, size `MaxConcurrent`) to cap simultaneous `fbc`
  processes. Do not add a second concurrency mechanism.
- **htmx + Pico-drop** — Pico CSS and the old `index.html` / top-level `app.js` are removed.
  htmx (CDN) drives exactly two swap regions across the whole redesign; Plan 1 uses one:
  `#convert-card` polling `GET /jobs/{id}`.
- **Locked theme toggle** — keep the manual light/dark/system toggle (localStorage, applied
  before first paint via an inline `<head>` script), overriding OS via `data-theme` on
  `<html>`. This decision is LOCKED; do not replace it with pure OS-only theming.
- **Visual source of truth** — `docs/superpowers/specs/mockup/FB2-Converter.dc.html`. The 8
  token values are copied **verbatim** from its dark-theme "Tokens" table (lines 36–68); the
  Convert idle/file-selected/running/done frames (screens 1–2d) and the failure frame
  (screen 3) define the card / result-row / error-card markup that the `.gohtml` templates
  reproduce as `app.css` classes. Ignore the `<x-dc>`/`<helmet>`/`support.js` canvas scaffolding
  and the annotation prose — extract only the app markup inside each frame.
- **Self-hosted, low-hardening posture** — a single shared job store, temp/scratch dirs, sem
  cap, and TTL sweep are sufficient. A single `sync.Mutex` guarding `status.json` updates is
  acceptable; do not build elaborate locking.

## File Structure

| File | Create/Modify | Responsibility |
|------|---------------|----------------|
| `internal/config/config.go` | Modify | Add `PresetsDir`, `JobsDir`, `JobsTTL` fields + env parsing. |
| `internal/config/config_test.go` | Modify | Cover the new env vars and defaults. |
| `internal/convert/runner.go` | Modify | Add `ConvertLogged` to the `FBC` type and the `Runner` interface. |
| `internal/convert/runner_test.go` | Modify | Tests for `ConvertLogged` (fail path, retry-override success, multi-line log). |
| `testdata/fake-fbc.sh` | Modify | Emit a multi-line stdout+stderr log; succeed on a "corrupt" input when the `-c` config contains `use_broken_images: true`; emit an `ERR` line on failure. |
| `internal/jobs/jobs.go` | Create | `Store`, `Status`, `FileResult`, `FileState`; create/paths/load/save + traversal-safe `OutputFile`, atomic `status.json`. |
| `internal/jobs/zip_sweep.go` | Create | `WriteZip` (flat, collision-suffixed) and `Sweep` (TTL). |
| `internal/jobs/jobs_test.go` | Create | Lifecycle, atomic save/load, traversal rejection. |
| `internal/jobs/zip_sweep_test.go` | Create | Zip assembly + collision suffix, TTL sweep. |
| `internal/web/static/app.css` | Create | Ported theme: 8 tokens on `:root`, one dark `@media`, `data-theme` overrides, one 640px breakpoint, component styles. |
| `internal/web/static/app.js` | Create | Slim glue: theme toggle, drag-drop → file input, `/me` badge. |
| `internal/web/templates/base.gohtml` | Create | Layout `{{define "base"}}`: `<head>` (fonts, htmx, css, pre-paint theme script), nav tabs Convert/Settings via `.Tab`, dispatches to page body via `{{content .ContentName .}}`. |
| `internal/web/templates/convert.gohtml` | Create | Convert page body: preset dropdown (Defaults only), format select, drop zone, `#convert-card` mount. |
| `internal/web/templates/_convert_card.gohtml` | Create | The `convert_card` partial: running / done / failed rendering, downloads, zip, logs, retry. |
| `internal/web/embed.go` | Modify | Embed `static` + `templates`; add `Templates()` parser. |
| `internal/web/templates_test.go` | Create | Templates parse; `app.css` carries the 8 tokens + dark media. |
| `internal/server/server.go` | Modify | New `Server` fields (`tpl`, `jobs`; no presets/schema in Plan 1), 5-arg `New()`, `render`/`renderPartial` helpers, `GET /` page, route table. |
| `internal/server/convert_flow.go` | Create | `POST /convert`, `GET /jobs/{id}`, download/zip/log, retry, the background per-file worker, view models. |
| `internal/server/handlers.go` | Modify | Drop the old streaming `handleConvert`/`formOptions`; keep `handleDefaults`, `handleMe`, `writeJSON`, `streamFile`, `contentTypeFor`, `contentDisposition`, `allowedFormats`. |
| `internal/server/handlers_test.go` | Modify | Add `ConvertLogged` to test stubs; rewrite convert tests for the job flow; update `newTestServer` to the new signature. |
| `internal/server/static_test.go` | Modify | `GET /` now renders the Convert page; assert nav + tab. |
| `main.go` | Modify | Parse templates, build the jobs store, start the sweeper goroutine, wire the 5-arg `New()`. |

---

### Task 1: Config — `PRESETS_DIR`, `JOBS_DIR`, `JOBS_TTL`

**Files:** `internal/config/config.go`, `internal/config/config_test.go`

**Interfaces**
- Produces:
  ```go
  type Config struct {
      Addr           string
      FBCBin         string
      MaxConcurrent  int
      ForwardAuth    bool
      TrustedProxies []string
      PresetsDir     string        // PRESETS_DIR, default "/data/presets"
      JobsDir        string        // JOBS_DIR,   default os.TempDir()+"/fb2cng-jobs"
      JobsTTL        time.Duration // JOBS_TTL,   default time.Hour
  }
  func FromEnv() Config
  ```

**Steps**

- [ ] Write the failing test. Replace the body of `internal/config/config_test.go` with:
  ```go
  package config

  import (
  	"os"
  	"path/filepath"
  	"reflect"
  	"testing"
  	"time"
  )

  func TestFromEnvDefaults(t *testing.T) {
  	t.Setenv("PORT", "")
  	t.Setenv("FBC_BIN", "")
  	t.Setenv("MAX_CONCURRENT", "")
  	t.Setenv("AUTH_FORWARD_AUTH", "")
  	t.Setenv("TRUSTED_PROXIES", "")
  	t.Setenv("PRESETS_DIR", "")
  	t.Setenv("JOBS_DIR", "")
  	t.Setenv("JOBS_TTL", "")

  	got := FromEnv()
  	want := Config{
  		Addr:           ":8080",
  		FBCBin:         "fbc",
  		MaxConcurrent:  3,
  		ForwardAuth:    false,
  		TrustedProxies: nil,
  		PresetsDir:     "/data/presets",
  		JobsDir:        filepath.Join(os.TempDir(), "fb2cng-jobs"),
  		JobsTTL:        time.Hour,
  	}
  	if !reflect.DeepEqual(got, want) {
  		t.Fatalf("defaults: got %+v want %+v", got, want)
  	}
  }

  func TestFromEnvOverrides(t *testing.T) {
  	t.Setenv("PORT", "9000")
  	t.Setenv("FBC_BIN", "/opt/fbc")
  	t.Setenv("MAX_CONCURRENT", "5")
  	t.Setenv("AUTH_FORWARD_AUTH", "true")
  	t.Setenv("TRUSTED_PROXIES", "10.0.0.1, 10.0.0.2")
  	t.Setenv("PRESETS_DIR", "/srv/presets")
  	t.Setenv("JOBS_DIR", "/srv/jobs")
  	t.Setenv("JOBS_TTL", "30m")

  	got := FromEnv()
  	if got.Addr != ":9000" || got.FBCBin != "/opt/fbc" || got.MaxConcurrent != 5 || !got.ForwardAuth {
  		t.Fatalf("overrides not applied: %+v", got)
  	}
  	if !reflect.DeepEqual(got.TrustedProxies, []string{"10.0.0.1", "10.0.0.2"}) {
  		t.Fatalf("trusted proxies: %+v", got.TrustedProxies)
  	}
  	if got.PresetsDir != "/srv/presets" || got.JobsDir != "/srv/jobs" || got.JobsTTL != 30*time.Minute {
  		t.Fatalf("new env not applied: %+v", got)
  	}
  }

  func TestJobsTTLInvalidFallsBack(t *testing.T) {
  	t.Setenv("JOBS_TTL", "not-a-duration")
  	if got := FromEnv().JobsTTL; got != time.Hour {
  		t.Fatalf("invalid JOBS_TTL should fall back to 1h, got %v", got)
  	}
  }
  ```
  The expected `JobsDir` default is computed with `filepath.Join(os.TempDir(), "fb2cng-jobs")`,
  exactly as the implementation will build it — no test helper needed.

- [ ] Run it, expect FAIL (missing fields):
  ```
  go test ./internal/config/ -run TestFromEnv
  ```
  Expected: compile error `unknown field PresetsDir in struct literal` (and siblings).

- [ ] Minimal implementation. Replace `internal/config/config.go` with:
  ```go
  // Package config loads app configuration from environment variables.
  package config

  import (
  	"os"
  	"path/filepath"
  	"strconv"
  	"strings"
  	"time"
  )

  // Config holds all runtime configuration.
  type Config struct {
  	Addr           string        // listen address, e.g. ":8080"
  	FBCBin         string        // path to the fbc binary
  	MaxConcurrent  int           // max concurrent fbc processes
  	ForwardAuth    bool          // trust reverse-proxy Remote-* headers
  	TrustedProxies []string      // source IPs allowed to set Remote-* headers (empty = trust any)
  	PresetsDir     string        // persistent preset store dir
  	JobsDir        string        // ephemeral job scratch dir (swept on TTL)
  	JobsTTL        time.Duration // max job age before sweep
  }

  // FromEnv builds a Config from environment variables, applying defaults.
  func FromEnv() Config {
  	c := Config{
  		Addr:          ":" + envOr("PORT", "8080"),
  		FBCBin:        envOr("FBC_BIN", "fbc"),
  		MaxConcurrent: atoiOr(os.Getenv("MAX_CONCURRENT"), 3),
  		ForwardAuth:   os.Getenv("AUTH_FORWARD_AUTH") == "true",
  		PresetsDir:    envOr("PRESETS_DIR", "/data/presets"),
  		JobsDir:       envOr("JOBS_DIR", filepath.Join(os.TempDir(), "fb2cng-jobs")),
  		JobsTTL:       durationOr(os.Getenv("JOBS_TTL"), time.Hour),
  	}
  	for _, p := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
  		if s := strings.TrimSpace(p); s != "" {
  			c.TrustedProxies = append(c.TrustedProxies, s)
  		}
  	}
  	return c
  }

  func envOr(key, def string) string {
  	if v := os.Getenv(key); v != "" {
  		return v
  	}
  	return def
  }

  func atoiOr(s string, def int) int {
  	if n, err := strconv.Atoi(s); err == nil && n > 0 {
  		return n
  	}
  	return def
  }

  func durationOr(s string, def time.Duration) time.Duration {
  	if d, err := time.ParseDuration(s); err == nil && d > 0 {
  		return d
  	}
  	return def
  }
  ```

- [ ] Run it, expect PASS:
  ```
  go test ./internal/config/
  ```
  Expected: `ok  fb2cng-web/internal/config`.

- [ ] Commit:
  ```
  git add internal/config/config.go internal/config/config_test.go
  git commit -m "feat(config): add PRESETS_DIR, JOBS_DIR, JOBS_TTL env vars

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 2: `convert.FBC.ConvertLogged` + `Runner` interface + fake-fbc log/retry

**Files:** `internal/convert/runner.go`, `internal/convert/runner_test.go`,
`testdata/fake-fbc.sh`, `internal/server/handlers_test.go`

**Interfaces**
- Consumes: `collectOutputs(destDir string) ([]string, error)` (existing, unchanged).
- Produces:
  ```go
  // Runner gains this method (fake in tests must implement it too):
  func (f *FBC) ConvertLogged(ctx context.Context, inputPath, format, configPath, destDir, logPath string) (outs []string, err error)
  ```
  Behaves like `Convert` but writes combined stdout+stderr to `logPath` (created/truncated),
  returns any partial outputs even on failure, and returns a non-nil error on non-zero exit.

**Steps**

- [ ] Extend the fake first (the tests below depend on its new behavior). Replace
  `testdata/fake-fbc.sh` with:
  ```sh
  #!/bin/sh
  # Fake fbc for tests. Emulates: `dumpconfig --default` and
  # `[-c cfg] convert --to FMT [--overwrite --nd] INPUT DEST`.
  # convert emits a multi-line stdout+stderr log. A "corrupt" input fails with an
  # ERR line unless the -c config contains `use_broken_images: true` (retry path).
  mode=""
  for a in "$@"; do
    case "$a" in
      dumpconfig) mode=dump ;;
      convert) mode=convert ;;
    esac
  done

  if [ "$mode" = "dump" ]; then
    dest_file=""
    for a in "$@"; do
      case "$a" in
        dumpconfig|--default) ;;
        *) dest_file="$a" ;;
      esac
    done
    yaml='version: 1\ndocument:\n    toc_type: normal\n'
    if [ -n "$dest_file" ]; then
      printf "$yaml" > "$dest_file"
    else
      printf "$yaml"
    fi
    exit 0
  fi

  if [ "$mode" = "convert" ]; then
    to=""
    cfg=""
    positionals=""
    while [ $# -gt 0 ]; do
      case "$1" in
        --to) to="$2"; shift 2; continue ;;
        -c) cfg="$2"; shift 2; continue ;;
        convert|--overwrite|--ow|--nd|--nodirs|-d|--debug) shift; continue ;;
        *) positionals="$positionals $1"; shift ;;
      esac
    done
    # positionals = INPUT ... DEST  (last is dest, first is input)
    # shellcheck disable=SC2086
    set -- $positionals
    input=$1
    eval dest=\${$#}
    base=$(basename "$input"); base=${base%.*}
    case "$to" in
      epub2|epub3|"") ext="epub" ;;
      kepub) ext="kepub.epub" ;;
      *) ext="$to" ;;
    esac

    broken=""
    if [ -n "$cfg" ] && grep -q 'use_broken_images: true' "$cfg" 2>/dev/null; then
      broken="yes"
    fi

    echo "INFO: opening $input"
    echo "INFO: target format $to" >&2
    if echo "$input" | grep -q corrupt && [ -z "$broken" ]; then
      echo "ERR: cannot parse $input: broken image" >&2
      echo "INFO: aborted"
      exit 1
    fi
    echo "INFO: converting $base"
    echo "WARN: cosmetic issue in $base" >&2
    mkdir -p "$dest"
    printf 'FAKE-%s' "$to" > "$dest/$base.$ext"
    if echo "$input" | grep -q multi; then
      printf 'FAKE2' > "$dest/${base}-2.$ext"
    fi
    echo "INFO: wrote $base.$ext"
    exit 0
  fi

  echo "fake-fbc: unknown invocation: $*" >&2
  exit 2
  ```
  Keep it executable:
  ```
  chmod +x testdata/fake-fbc.sh
  ```

- [ ] Write the failing test. Append to `internal/convert/runner_test.go`:
  ```go
  func TestConvertLoggedSuccessMultiLineLog(t *testing.T) {
  	dir := t.TempDir()
  	in := filepath.Join(dir, "book.fb2")
  	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
  		t.Fatal(err)
  	}
  	logPath := filepath.Join(dir, "book.log")
  	outs, err := New(fakeBin(t)).ConvertLogged(context.Background(), in, "epub3", "", filepath.Join(dir, "out"), logPath)
  	if err != nil {
  		t.Fatal(err)
  	}
  	if len(outs) != 1 || filepath.Base(outs[0]) != "book.epub" {
  		t.Fatalf("unexpected outputs: %v", outs)
  	}
  	b, err := os.ReadFile(logPath)
  	if err != nil {
  		t.Fatal(err)
  	}
  	if strings.Count(string(b), "\n") < 2 {
  		t.Fatalf("expected a multi-line log, got:\n%s", b)
  	}
  }

  func TestConvertLoggedFailureCapturesErr(t *testing.T) {
  	dir := t.TempDir()
  	in := filepath.Join(dir, "corrupt.fb2")
  	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
  		t.Fatal(err)
  	}
  	logPath := filepath.Join(dir, "corrupt.log")
  	_, err := New(fakeBin(t)).ConvertLogged(context.Background(), in, "epub3", "", filepath.Join(dir, "out"), logPath)
  	if err == nil {
  		t.Fatal("expected failure on corrupt input")
  	}
  	b, _ := os.ReadFile(logPath)
  	if !strings.Contains(string(b), "ERR") {
  		t.Fatalf("log should contain the ERR line, got:\n%s", b)
  	}
  }

  func TestConvertLoggedRetryOverrideSucceeds(t *testing.T) {
  	dir := t.TempDir()
  	in := filepath.Join(dir, "corrupt.fb2")
  	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
  		t.Fatal(err)
  	}
  	cfg := filepath.Join(dir, "cfg.yaml")
  	if err := os.WriteFile(cfg, []byte("use_broken_images: true\n"), 0o644); err != nil {
  		t.Fatal(err)
  	}
  	logPath := filepath.Join(dir, "corrupt.log")
  	outs, err := New(fakeBin(t)).ConvertLogged(context.Background(), in, "epub3", cfg, filepath.Join(dir, "out"), logPath)
  	if err != nil {
  		t.Fatalf("override should make corrupt input succeed: %v", err)
  	}
  	if len(outs) != 1 {
  		t.Fatalf("expected 1 output on retry, got %v", outs)
  	}
  }
  ```

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/convert/ -run TestConvertLogged
  ```
  Expected: `f.ConvertLogged undefined (type *FBC has no field or method ConvertLogged)`.

- [ ] Minimal implementation. In `internal/convert/runner.go`, add `"io"` to imports and add
  `ConvertLogged` to the interface and the `FBC` type. The interface becomes:
  ```go
  // Runner converts fb2 inputs via fbc.
  type Runner interface {
  	// DumpDefaults returns fbc's embedded default configuration as YAML.
  	DumpDefaults(ctx context.Context) ([]byte, error)
  	// Convert runs fbc on inputPath into destDir and returns the produced
  	// output file paths. configPath is optional ("" = use fbc defaults).
  	Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error)
  	// ConvertLogged behaves like Convert but also captures combined stdout+stderr
  	// into logPath (created/truncated). Returns outputs even on fbc failure when
  	// partial output exists; err is non-nil on non-zero exit.
  	ConvertLogged(ctx context.Context, inputPath, format, configPath, destDir, logPath string) ([]string, error)
  }
  ```
  Then add the method (place it just after `Convert`):
  ```go
  func (f *FBC) ConvertLogged(ctx context.Context, inputPath, format, configPath, destDir, logPath string) ([]string, error) {
  	if err := os.MkdirAll(destDir, 0o755); err != nil {
  		return nil, err
  	}
  	logf, err := os.Create(logPath)
  	if err != nil {
  		return nil, err
  	}
  	defer logf.Close()

  	// -c is a GLOBAL flag and must come before the subcommand.
  	args := []string{}
  	if configPath != "" {
  		args = append(args, "-c", configPath)
  	}
  	args = append(args, "convert", "--to", format, "--overwrite", "--nd", inputPath, destDir)

  	cmd := exec.CommandContext(ctx, f.Bin, args...)
  	cmd.Stdout = io.MultiWriter(logf)
  	cmd.Stderr = io.MultiWriter(logf)
  	runErr := cmd.Run()

  	outs, collectErr := collectOutputs(destDir)
  	if runErr != nil {
  		return outs, fmt.Errorf("conversion failed: %w", runErr)
  	}
  	if collectErr != nil {
  		return nil, collectErr
  	}
  	return outs, nil
  }
  ```
  (The `io.MultiWriter` wraps make the intent — both streams share one sink — explicit and
  keep the door open for a tee later; a bare `logf` would also compile.)

- [ ] The `Runner` interface grew, so the fake stubs in `internal/server/handlers_test.go`
  no longer satisfy it. Add a `ConvertLogged` method to each. Add to `stubRunner`:
  ```go
  func (s stubRunner) ConvertLogged(context.Context, string, string, string, string, string) ([]string, error) {
  	return s.outputs, s.err
  }
  ```
  Add to `capturingRunner`:
  ```go
  func (c *capturingRunner) ConvertLogged(_ context.Context, in, format, cfgPath, dest, _ string) ([]string, error) {
  	return c.Convert(context.Background(), in, format, cfgPath, dest)
  }
  ```
  Add to `blockingRunner`:
  ```go
  func (b *blockingRunner) ConvertLogged(ctx context.Context, in, format, cfgPath, dest, _ string) ([]string, error) {
  	return b.Convert(ctx, in, format, cfgPath, dest)
  }
  ```

- [ ] Run the convert tests and the full build, expect PASS / compile clean:
  ```
  go test ./internal/convert/ -run TestConvertLogged
  go build ./...
  ```
  Expected: `ok  fb2cng-web/internal/convert` and a clean build. (The server package still
  uses the old `handleConvert`; that is rewritten in Task 7. Its tests may still pass here
  because only the interface widened.)

- [ ] Commit:
  ```
  git add internal/convert/runner.go internal/convert/runner_test.go testdata/fake-fbc.sh internal/server/handlers_test.go
  git commit -m "feat(convert): add log-capturing ConvertLogged and retry-aware fake fbc

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 3: `internal/jobs` — Store, Status, paths, atomic save, traversal-safe output

**Files:** `internal/jobs/jobs.go`, `internal/jobs/jobs_test.go`

**Interfaces**
- Produces (exact contract names):
  ```go
  package jobs

  type FileState string
  const (
      StatePending FileState = "pending"
      StateRunning FileState = "running"
      StateDone    FileState = "done"
      StateFailed  FileState = "failed"
  )
  type FileResult struct {
      Input      string           `json:"input"`
      State      FileState        `json:"state"`
      Outputs    []string         `json:"outputs"`
      Sizes      map[string]int64 `json:"sizes"`
      LogLines   int              `json:"log_lines"`
      FirstError string           `json:"first_error"`
      Err        string           `json:"err"`
      Millis     int64            `json:"millis"`
  }
  type Status struct {
      ID     string       `json:"id"`
      Preset string       `json:"preset"`
      Format string       `json:"format"`
      Files  []FileResult `json:"files"`
      Done   bool         `json:"done"`
  }
  type Store struct { Dir string; TTL time.Duration }
  func NewStore(dir string, ttl time.Duration) *Store
  func (s *Store) Create(preset, format string) (id string, err error)
  func (s *Store) InputPath(id, input string) string
  func (s *Store) OutDir(id, input string) string
  func (s *Store) LogPath(id, input string) string
  func (s *Store) Load(id string) (*Status, error)
  func (s *Store) Save(id string, st *Status) error
  func (s *Store) OutputFile(id, input, base string) (string, error)
  ```
  Job id = 16 hex chars. `OutputFile` rejects `input`/`base` containing `/`, `\`, or `..`.
  `Save` writes `status.json` atomically (temp + rename).

**Steps**

- [ ] Write the failing test. Create `internal/jobs/jobs_test.go`:
  ```go
  package jobs

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  	"time"
  )

  func TestCreateBuildsTreeAndInitialStatus(t *testing.T) {
  	s := NewStore(t.TempDir(), time.Hour)
  	id, err := s.Create("defaults", "epub3")
  	if err != nil {
  		t.Fatal(err)
  	}
  	if len(id) != 16 {
  		t.Fatalf("id should be 16 hex chars, got %q", id)
  	}
  	for _, sub := range []string{"in", "out"} {
  		if fi, err := os.Stat(filepath.Join(s.Dir, id, sub)); err != nil || !fi.IsDir() {
  			t.Fatalf("missing dir %s: %v", sub, err)
  		}
  	}
  	st, err := s.Load(id)
  	if err != nil {
  		t.Fatal(err)
  	}
  	if st.ID != id || st.Preset != "defaults" || st.Format != "epub3" || st.Done {
  		t.Fatalf("unexpected initial status: %+v", st)
  	}
  }

  func TestPathBuilders(t *testing.T) {
  	s := NewStore("/base", time.Hour)
  	if got := s.InputPath("abc", "book.fb2"); got != filepath.Join("/base", "abc", "in", "book.fb2") {
  		t.Fatalf("InputPath: %s", got)
  	}
  	if got := s.OutDir("abc", "book.fb2"); got != filepath.Join("/base", "abc", "out", "book.fb2") {
  		t.Fatalf("OutDir: %s", got)
  	}
  	if got := s.LogPath("abc", "book.fb2"); got != filepath.Join("/base", "abc", "book.fb2.log") {
  		t.Fatalf("LogPath: %s", got)
  	}
  }

  func TestSaveLoadRoundtripAtomic(t *testing.T) {
  	s := NewStore(t.TempDir(), time.Hour)
  	id, _ := s.Create("defaults", "epub3")
  	st, _ := s.Load(id)
  	st.Files = []FileResult{{
  		Input:   "book.fb2",
  		State:   StateDone,
  		Outputs: []string{"book.epub"},
  		Sizes:   map[string]int64{"book.epub": 42},
  		Millis:  7,
  	}}
  	st.Done = true
  	if err := s.Save(id, st); err != nil {
  		t.Fatal(err)
  	}
  	if _, err := os.Stat(filepath.Join(s.Dir, id, "status.json.tmp")); !os.IsNotExist(err) {
  		t.Fatalf("temp file should be renamed away, err=%v", err)
  	}
  	got, err := s.Load(id)
  	if err != nil {
  		t.Fatal(err)
  	}
  	if !got.Done || len(got.Files) != 1 || got.Files[0].Sizes["book.epub"] != 42 {
  		t.Fatalf("roundtrip mismatch: %+v", got)
  	}
  }

  func TestOutputFileRejectsTraversal(t *testing.T) {
  	s := NewStore(t.TempDir(), time.Hour)
  	for _, bad := range []string{"../secret", "a/b", `a\b`, "..", ""} {
  		if _, err := s.OutputFile("job", "book.fb2", bad); err == nil {
  			t.Errorf("base %q should be rejected", bad)
  		}
  		if _, err := s.OutputFile("job", bad, "book.epub"); err == nil {
  			t.Errorf("input %q should be rejected", bad)
  		}
  	}
  	got, err := s.OutputFile("job", "book.fb2", "book.epub")
  	if err != nil {
  		t.Fatal(err)
  	}
  	if !strings.HasSuffix(got, filepath.Join("job", "out", "book.fb2", "book.epub")) {
  		t.Fatalf("unexpected safe path: %s", got)
  	}
  }
  ```

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/jobs/ -run 'TestCreate|TestPath|TestSaveLoad|TestOutputFile'
  ```
  Expected: build error — package `jobs` has no Go files yet / undefined symbols.

- [ ] Minimal implementation. Create `internal/jobs/jobs.go`:
  ```go
  // Package jobs owns JOBS_DIR: one subdir per batch conversion holding inputs,
  // outputs, per-input logs, and a status.json. It backs the results view,
  // download-all zip, stored logs, retry, and the TTL sweep.
  package jobs

  import (
  	"crypto/rand"
  	"encoding/hex"
  	"encoding/json"
  	"fmt"
  	"os"
  	"path/filepath"
  	"strings"
  	"time"
  )

  // FileState is the lifecycle state of one input within a batch.
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
  	ID     string       `json:"id"`
  	Preset string       `json:"preset"` // preset id used ("" = Defaults)
  	Format string       `json:"format"`
  	Files  []FileResult `json:"files"`
  	Done   bool         `json:"done"` // all files terminal
  }

  // Store owns JOBS_DIR. One subdir per job:
  // <id>/in/, <id>/out/<input>/, <id>/<input>.log, <id>/status.json
  type Store struct {
  	Dir string
  	TTL time.Duration
  }

  // NewStore returns a Store rooted at dir with the given sweep TTL.
  func NewStore(dir string, ttl time.Duration) *Store { return &Store{Dir: dir, TTL: ttl} }

  func newID() string {
  	b := make([]byte, 8)
  	_, _ = rand.Read(b)
  	return hex.EncodeToString(b)
  }

  func (s *Store) root(id string) string        { return filepath.Join(s.Dir, id) }
  func (s *Store) statusPath(id string) string  { return filepath.Join(s.root(id), "status.json") }

  // Create makes the job tree and writes the initial status.
  func (s *Store) Create(preset, format string) (string, error) {
  	id := newID()
  	for _, d := range []string{filepath.Join(s.root(id), "in"), filepath.Join(s.root(id), "out")} {
  		if err := os.MkdirAll(d, 0o755); err != nil {
  			return "", err
  		}
  	}
  	if err := s.Save(id, &Status{ID: id, Preset: preset, Format: format}); err != nil {
  		return "", err
  	}
  	return id, nil
  }

  // InputPath is <id>/in/<input>. filepath.Base neutralizes any path components.
  func (s *Store) InputPath(id, input string) string {
  	return filepath.Join(s.root(id), "in", filepath.Base(input))
  }

  // OutDir is <id>/out/<input>.
  func (s *Store) OutDir(id, input string) string {
  	return filepath.Join(s.root(id), "out", filepath.Base(input))
  }

  // LogPath is <id>/<input>.log.
  func (s *Store) LogPath(id, input string) string {
  	return filepath.Join(s.root(id), filepath.Base(input)+".log")
  }

  // Load reads and decodes status.json.
  func (s *Store) Load(id string) (*Status, error) {
  	b, err := os.ReadFile(s.statusPath(id))
  	if err != nil {
  		return nil, err
  	}
  	var st Status
  	if err := json.Unmarshal(b, &st); err != nil {
  		return nil, err
  	}
  	return &st, nil
  }

  // Save writes status.json atomically (temp file + rename).
  func (s *Store) Save(id string, st *Status) error {
  	b, err := json.MarshalIndent(st, "", "  ")
  	if err != nil {
  		return err
  	}
  	final := s.statusPath(id)
  	tmp := final + ".tmp"
  	if err := os.WriteFile(tmp, b, 0o644); err != nil {
  		return err
  	}
  	return os.Rename(tmp, final)
  }

  // OutputFile returns the safe-joined path to one output, rejecting names that
  // contain a path separator or "..".
  func (s *Store) OutputFile(id, input, base string) (string, error) {
  	if err := safeName(input); err != nil {
  		return "", err
  	}
  	if err := safeName(base); err != nil {
  		return "", err
  	}
  	return filepath.Join(s.root(id), "out", input, base), nil
  }

  func safeName(name string) error {
  	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
  		return fmt.Errorf("invalid name %q", name)
  	}
  	return nil
  }
  ```

- [ ] Run it, expect PASS:
  ```
  go test ./internal/jobs/ -run 'TestCreate|TestPath|TestSaveLoad|TestOutputFile'
  ```
  Expected: `ok  fb2cng-web/internal/jobs`.

- [ ] Commit:
  ```
  git add internal/jobs/jobs.go internal/jobs/jobs_test.go
  git commit -m "feat(jobs): job store with atomic status and traversal-safe output paths

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 4: `internal/jobs` — `WriteZip` (collision-suffixed) and `Sweep` (TTL)

**Files:** `internal/jobs/zip_sweep.go`, `internal/jobs/zip_sweep_test.go`

**Interfaces**
- Consumes: `Store` (Task 3).
- Produces:
  ```go
  func (s *Store) WriteZip(id string, w io.Writer) error // all outputs, flat, name-collision-suffixed
  func (s *Store) Sweep(now time.Time) (removed int)     // delete job dirs older than TTL
  ```

**Steps**

- [ ] Write the failing test. Create `internal/jobs/zip_sweep_test.go`:
  ```go
  package jobs

  import (
  	"archive/zip"
  	"bytes"
  	"os"
  	"path/filepath"
  	"testing"
  	"time"
  )

  func TestWriteZipFlatWithCollisionSuffix(t *testing.T) {
  	s := NewStore(t.TempDir(), time.Hour)
  	id, _ := s.Create("defaults", "epub3")
  	// Two inputs each produce a "book.epub" — the zip must not collide.
  	for _, in := range []string{"a.fb2", "b.fb2"} {
  		dir := s.OutDir(id, in)
  		if err := os.MkdirAll(dir, 0o755); err != nil {
  			t.Fatal(err)
  		}
  		if err := os.WriteFile(filepath.Join(dir, "book.epub"), []byte("DATA"), 0o644); err != nil {
  			t.Fatal(err)
  		}
  	}
  	var buf bytes.Buffer
  	if err := s.WriteZip(id, &buf); err != nil {
  		t.Fatal(err)
  	}
  	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
  	if err != nil {
  		t.Fatal(err)
  	}
  	names := map[string]bool{}
  	for _, f := range zr.File {
  		if names[f.Name] {
  			t.Fatalf("duplicate zip entry %q", f.Name)
  		}
  		names[f.Name] = true
  	}
  	if len(zr.File) != 2 {
  		t.Fatalf("expected 2 entries, got %d: %v", len(zr.File), names)
  	}
  	if !names["book.epub"] || (!names["book-1.epub"]) {
  		t.Fatalf("expected book.epub + book-1.epub, got %v", names)
  	}
  }

  func TestSweepRemovesOldJobs(t *testing.T) {
  	s := NewStore(t.TempDir(), time.Hour)
  	oldID, _ := s.Create("defaults", "epub3")
  	newID, _ := s.Create("defaults", "epub3")

  	// Age the old job's dir past the TTL.
  	past := time.Now().Add(-2 * time.Hour)
  	if err := os.Chtimes(filepath.Join(s.Dir, oldID), past, past); err != nil {
  		t.Fatal(err)
  	}

  	removed := s.Sweep(time.Now())
  	if removed != 1 {
  		t.Fatalf("expected 1 removed, got %d", removed)
  	}
  	if _, err := os.Stat(filepath.Join(s.Dir, oldID)); !os.IsNotExist(err) {
  		t.Fatalf("old job should be gone, err=%v", err)
  	}
  	if _, err := os.Stat(filepath.Join(s.Dir, newID)); err != nil {
  		t.Fatalf("new job should survive: %v", err)
  	}
  }
  ```

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/jobs/ -run 'TestWriteZip|TestSweep'
  ```
  Expected: `s.WriteZip undefined` / `s.Sweep undefined`.

- [ ] Minimal implementation. Create `internal/jobs/zip_sweep.go`:
  ```go
  package jobs

  import (
  	"archive/zip"
  	"io"
  	"io/fs"
  	"os"
  	"path/filepath"
  	"strconv"
  	"strings"
  	"time"
  )

  // WriteZip streams every output file under <id>/out/ into w as one flat zip,
  // suffixing basename collisions (book.epub, book-1.epub, ...).
  func (s *Store) WriteZip(id string, w io.Writer) error {
  	outRoot := filepath.Join(s.root(id), "out")
  	zw := zip.NewWriter(w)
  	defer zw.Close()

  	seen := map[string]int{}
  	return filepath.WalkDir(outRoot, func(path string, d fs.DirEntry, err error) error {
  		if err != nil {
  			if os.IsNotExist(err) {
  				return nil
  			}
  			return err
  		}
  		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
  			return nil
  		}
  		name := d.Name()
  		if n := seen[d.Name()]; n > 0 {
  			ext := filepath.Ext(name)
  			name = strings.TrimSuffix(name, ext) + "-" + strconv.Itoa(n) + ext
  		}
  		seen[d.Name()]++

  		f, err := os.Open(path)
  		if err != nil {
  			return err
  		}
  		defer f.Close()
  		hw, err := zw.Create(name)
  		if err != nil {
  			return err
  		}
  		_, err = io.Copy(hw, f)
  		return err
  	})
  }

  // Sweep deletes job dirs whose mtime is older than the TTL and returns the count.
  func (s *Store) Sweep(now time.Time) (removed int) {
  	entries, err := os.ReadDir(s.Dir)
  	if err != nil {
  		return 0
  	}
  	for _, e := range entries {
  		if !e.IsDir() {
  			continue
  		}
  		info, err := e.Info()
  		if err != nil {
  			continue
  		}
  		if now.Sub(info.ModTime()) > s.TTL {
  			if os.RemoveAll(filepath.Join(s.Dir, e.Name())) == nil {
  				removed++
  			}
  		}
  	}
  	return removed
  }
  ```

- [ ] Run it, expect PASS:
  ```
  go test ./internal/jobs/
  ```
  Expected: `ok  fb2cng-web/internal/jobs`.

- [ ] Commit:
  ```
  git add internal/jobs/zip_sweep.go internal/jobs/zip_sweep_test.go
  git commit -m "feat(jobs): WriteZip with collision suffixes and TTL Sweep

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 5: Front-end assets — CSS theme, templates, slim JS, embed + parser

**Files:** `internal/web/static/app.css`, `internal/web/static/app.js`,
`internal/web/templates/base.gohtml`, `internal/web/templates/convert.gohtml`,
`internal/web/templates/_convert_card.gohtml`, `internal/web/embed.go`,
`internal/web/templates_test.go`

> This task adds the new assets and the `Templates()` parser **without removing** the old
> `index.html` / top-level `app.js` yet, so the build and the existing `static_test.go`
> stay green. Task 6 removes the old files and switches `GET /` to render the template.

**Interfaces**
- Produces:
  ```go
  // internal/web/embed.go
  func Templates() (*template.Template, error) // parses templates/*.gohtml; expose "base" + "convert_card"
  ```
  Templates are keyed by `{{define}}` name: `base`, `content` (Convert page body), and
  `convert_card` (the htmx-polled partial).

**Steps**

- [ ] Write the failing test. Create `internal/web/templates_test.go`:
  ```go
  package web

  import (
  	"strings"
  	"testing"
  )

  func TestTemplatesParse(t *testing.T) {
  	tpl, err := Templates()
  	if err != nil {
  		t.Fatal(err)
  	}
  	for _, name := range []string{"base", "convert", "convert_card"} {
  		if tpl.Lookup(name) == nil {
  			t.Fatalf("template %q not defined", name)
  		}
  	}
  }

  func TestAppCSSCarriesTokens(t *testing.T) {
  	b, err := FS.ReadFile("static/app.css")
  	if err != nil {
  		t.Fatal(err)
  	}
  	css := string(b)
  	for _, tok := range []string{
  		// the 8 tokens
  		"--bg", "--bg-sunk", "--line", "--fg", "--fg-muted", "--fg-dim",
  		"--accent", "--changed",
  		// exact light values from the mockup Tokens table
  		"oklch(.99 .002 255)", "oklch(.52 .14 255)", "oklch(.72 .14 75)",
  		// exact dark values
  		"@media (prefers-color-scheme: dark)", "oklch(.19 .012 255)", "oklch(.66 .13 255)",
  		// the two extra dark rules: log pre near-black + success/error tints
  		"oklch(.15 .01 255)",
  		"[data-theme=",
  	} {
  		if !strings.Contains(css, tok) {
  			t.Errorf("app.css missing %q", tok)
  		}
  	}
  }
  ```

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/web/ -run 'TestTemplates|TestAppCSS'
  ```
  Expected: `undefined: Templates` and `open static/app.css: file does not exist`.

- [ ] Create `internal/web/static/app.css`. The 8 tokens carry the **exact `oklch()` values
  from the mockup's dark-theme "Tokens" table** (`docs/superpowers/specs/mockup/FB2-Converter.dc.html`,
  lines 36–68) — do not substitute your own colors. `:root` holds the light column, the
  `@media (prefers-color-scheme: dark)` block re-declares them with the dark column, and
  `html[data-theme=…]` mirrors each so the manual toggle wins over the OS. The two extra rules
  the mockup calls out (line 70) are encoded as `--log-bg` (log `<pre>` keeps its own near-black
  `oklch(.15 .01 255)` in both themes) and the success/error tint variables, which drop to
  ~0.28 lightness in dark rather than inverting. Component classes below convert the mockup's
  inline styles (header, dashed drop `<label>`, preset select, Convert button, running/success/
  error cards, file rows, status dots, "Download all" button, first-error `<pre>`, logs
  `<details>`, Retry/Skip) into reusable classes.
  ```css
  /* ===== 8 theme tokens — exact oklch() from the mockup Tokens table ===== */
  :root {
    --bg:        oklch(.99 .002 255);   /* page + card surface */
    --bg-sunk:   oklch(.98 .003 255);   /* header, group rows */
    --line:      oklch(.89 .006 255);   /* every hairline */
    --fg:        oklch(.28 .012 255);   /* never pure white */
    --fg-muted:  oklch(.46 .012 255);   /* descriptions */
    --fg-dim:    oklch(.68 .01 255);    /* "default …" — recedes */
    --accent:    oklch(.52 .14 255);
    --changed:   oklch(.72 .14 75);     /* amber marker + rule */

    /* extra rules the mockup calls out */
    --log-bg:    oklch(.15 .01 255);    /* log <pre> keeps its own near-black */
    --ok-border: oklch(.84 .03 155);
    --ok-bg:     oklch(.975 .02 155);
    --ok-dot:    oklch(.6 .13 155);
    --err-border: oklch(.84 .06 27);
    --err-bg:    oklch(.975 .02 27);
    --err-dot:   oklch(.55 .19 27);
    --err-line:  oklch(.55 .19 27);
    --err-fg:    oklch(.45 .17 27);
    --on-accent: #fff;
  }

  @media (prefers-color-scheme: dark) {
    :root {
      --bg:        oklch(.19 .012 255);
      --bg-sunk:   oklch(.235 .012 255); /* lighter in dark, not darker */
      --line:      oklch(.33 .012 255);
      --fg:        oklch(.92 .008 255);
      --fg-muted:  oklch(.7 .01 255);
      --fg-dim:    oklch(.55 .01 255);
      --accent:    oklch(.66 .13 255);   /* lifted so it stays visible on dark */
      --changed:   oklch(.78 .13 75);

      /* log pre keeps near-black; tints drop to ~0.28 lightness, not inverted */
      --log-bg:    oklch(.15 .01 255);
      --ok-border: oklch(.4 .06 155);
      --ok-bg:     oklch(.28 .04 155);
      --ok-dot:    oklch(.72 .14 155);
      --err-border: oklch(.6 .15 27);
      --err-bg:    oklch(.28 .04 27);
      --err-dot:   oklch(.72 .16 27);
      --err-line:  oklch(.6 .15 27);
      --err-fg:    oklch(.88 .08 27);
      --on-accent: oklch(.16 .02 255);
    }
  }

  /* Manual toggle wins over OS via data-theme on <html> (LOCKED decision). */
  html[data-theme="light"] {
    --bg: oklch(.99 .002 255); --bg-sunk: oklch(.98 .003 255); --line: oklch(.89 .006 255);
    --fg: oklch(.28 .012 255); --fg-muted: oklch(.46 .012 255); --fg-dim: oklch(.68 .01 255);
    --accent: oklch(.52 .14 255); --changed: oklch(.72 .14 75);
    --log-bg: oklch(.15 .01 255);
    --ok-border: oklch(.84 .03 155); --ok-bg: oklch(.975 .02 155); --ok-dot: oklch(.6 .13 155);
    --err-border: oklch(.84 .06 27); --err-bg: oklch(.975 .02 27); --err-dot: oklch(.55 .19 27);
    --err-line: oklch(.55 .19 27); --err-fg: oklch(.45 .17 27); --on-accent: #fff;
  }
  html[data-theme="dark"] {
    --bg: oklch(.19 .012 255); --bg-sunk: oklch(.235 .012 255); --line: oklch(.33 .012 255);
    --fg: oklch(.92 .008 255); --fg-muted: oklch(.7 .01 255); --fg-dim: oklch(.55 .01 255);
    --accent: oklch(.66 .13 255); --changed: oklch(.78 .13 75);
    --log-bg: oklch(.15 .01 255);
    --ok-border: oklch(.4 .06 155); --ok-bg: oklch(.28 .04 155); --ok-dot: oklch(.72 .14 155);
    --err-border: oklch(.6 .15 27); --err-bg: oklch(.28 .04 27); --err-dot: oklch(.72 .16 27);
    --err-line: oklch(.6 .15 27); --err-fg: oklch(.88 .08 27); --on-accent: oklch(.16 .02 255);
  }

  /* ===== base ===== */
  * { box-sizing: border-box; }
  body {
    margin: 0; background: var(--bg); color: var(--fg);
    font-family: "Helvetica Neue", Helvetica, Arial, sans-serif; line-height: 1.5;
  }
  .mono, code, pre, input, select, button { font-family: "IBM Plex Mono", ui-monospace, Menlo, monospace; }
  a { color: var(--accent); text-decoration: none; }
  a:hover { text-decoration: underline; }

  /* ===== header + tabs (mockup .topbar) ===== */
  .topbar {
    display: flex; align-items: center; justify-content: space-between; gap: 12px;
    padding: 11px 24px; border-bottom: 1px solid var(--line); background: var(--bg-sunk);
  }
  .brand { font: 600 14px/1 "IBM Plex Mono", monospace; letter-spacing: -0.02em; color: var(--fg); }
  .topbar .right { display: flex; align-items: center; gap: 12px; color: var(--fg-dim); font-size: 13px; }
  .topbar .right select { padding: 4px 6px; font-size: 12px; width: auto; background: var(--bg); color: var(--fg); border: 1px solid var(--line); border-radius: 6px; }
  nav.tabs { display: flex; gap: 18px; font-size: 14px; }
  nav.tabs a { color: var(--fg-muted); padding-bottom: 2px; border-bottom: 2px solid transparent; }
  nav.tabs a.active { color: var(--fg); font-weight: 600; border-bottom-color: var(--accent); }

  /* ===== convert page: centered column, max 560px ===== */
  .page { padding: 40px 24px 56px; display: flex; justify-content: center; }
  .convert-form { width: 100%; max-width: 560px; display: flex; flex-direction: column; gap: 16px; }

  .dropzone {
    display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px;
    min-height: 170px; padding: 20px; text-align: center; cursor: pointer;
    border: 1.5px dashed color-mix(in oklch, var(--fg-dim), var(--line)); border-radius: 8px;
    background: var(--bg-sunk);
  }
  .dropzone.drag { border-color: var(--accent); }
  .dz-title { font-size: 16px; font-weight: 600; color: var(--fg); }
  .dz-hint { font-size: 13px; color: var(--fg-muted); }

  .field { display: flex; flex-direction: column; gap: 6px; }
  .field > label { font-size: 13px; color: var(--fg-muted); }
  .select {
    height: 44px; padding: 0 12px; font-size: 16px; width: 100%;
    background: var(--bg); color: var(--fg); border: 1px solid var(--line); border-radius: 6px;
  }
  .btn-primary {
    height: 50px; border: 0; border-radius: 6px; background: var(--accent); color: var(--on-accent);
    font: 600 16px/1 "IBM Plex Mono", monospace; cursor: pointer;
  }
  .btn-primary:hover { filter: brightness(1.05); }
  .btn-primary[disabled] { opacity: .45; cursor: default; }

  /* ===== cards ===== */
  .card { display: flex; flex-direction: column; gap: 14px; padding: 16px; border-radius: 8px; }
  .card.running { border: 1px solid var(--line); background: var(--bg-sunk); }
  .card.success { border: 1px solid var(--ok-border); background: var(--ok-bg); }
  .card.error   { border: 1px solid var(--err-border); background: var(--err-bg); gap: 10px; }

  .card-head { display: flex; align-items: center; gap: 8px; font-size: 15px; font-weight: 600; color: var(--fg); }
  .dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
  .dot.done { background: var(--ok-dot); }
  .dot.running { background: var(--accent); }
  .dot.queued { background: var(--line); }
  .dot.failed { background: var(--err-dot); }

  /* indeterminate progress bar (mockup 2c) */
  .bar { position: relative; height: 4px; border-radius: 2px; background: var(--line); overflow: hidden; }
  .bar > i { position: absolute; inset: 0 auto 0 0; width: 30%; border-radius: 2px; background: var(--accent); animation: indet 1.15s ease-in-out infinite; }
  @keyframes indet { 0% { left: -30%; } 50% { left: 45%; } 100% { left: 100%; } }

  /* file lists inside cards */
  .file-list { display: flex; flex-direction: column; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; overflow: hidden; }
  .file-row { display: flex; align-items: center; gap: 12px; padding: 10px 12px; border-bottom: 1px solid var(--line); min-height: 48px; }
  .file-row:last-child { border-bottom: none; }
  .file-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .file-name { font: 500 13.5px/1.35 "IBM Plex Mono", monospace; word-break: break-all; color: var(--fg); }
  .file-size { font-size: 12px; color: var(--fg-muted); }
  .file-state { font-size: 12px; color: var(--fg-muted); }
  .dl { font-size: 14px; padding: 6px 2px; }

  /* progress rows (no inner card) */
  .prog-list { display: flex; flex-direction: column; border-top: 1px solid var(--line); }
  .prog-row { display: flex; align-items: center; gap: 10px; padding: 9px 0; border-bottom: 1px solid var(--line); }
  .prog-row:last-child { border-bottom: none; }
  .prog-name { flex: 1; min-width: 0; font: 500 13px/1.3 "IBM Plex Mono", monospace; word-break: break-all; color: var(--fg); }

  .zip-btn {
    display: flex; align-items: center; justify-content: center; height: 50px; border-radius: 6px;
    background: var(--accent); color: var(--on-accent); font-size: 16px; font-weight: 600;
  }
  .zip-btn:hover { text-decoration: none; filter: brightness(1.05); }

  /* failure card extras */
  .fail-file { font: 500 13px/1.35 "IBM Plex Mono", monospace; word-break: break-all; color: var(--fg); }
  .fail-msg { margin: 0; font-size: 14px; line-height: 1.5; color: var(--fg); }
  .fail-hint { margin: 0; font-size: 13px; line-height: 1.5; color: var(--err-fg); }
  .fail-actions { display: flex; flex-direction: column; gap: 8px; padding-top: 2px; }
  .btn-retry {
    display: flex; align-items: center; justify-content: center; height: 46px; border-radius: 6px;
    border: 1px solid var(--err-line); color: var(--err-fg); background: var(--bg);
    font-size: 15px; font-weight: 600; font-family: "IBM Plex Mono", monospace; cursor: pointer;
  }
  .btn-skip {
    display: flex; align-items: center; justify-content: center; height: 46px; border-radius: 6px;
    border: 1px solid var(--line); color: var(--fg-muted); background: var(--bg); font-size: 15px;
  }
  .btn-skip:hover { text-decoration: none; }

  .first-error-label { font: 500 12px/1 "IBM Plex Mono", monospace; color: var(--fg-muted); }
  /* Log / first-error <pre> keep their own near-black in BOTH themes (--log-bg). */
  pre.log, pre.err {
    margin: 0; padding: 10px 12px; border-radius: 6px; background: var(--log-bg);
    font: 400 12px/1.55 "IBM Plex Mono", monospace; overflow: auto; max-height: 260px; white-space: pre;
  }
  pre.err { color: oklch(.9 .03 27); }
  pre.log { color: oklch(.88 .01 255); }

  details.logs { border: 1px solid var(--line); border-radius: 6px; overflow: hidden; }
  details.logs > summary { padding: 12px 14px; font-size: 14px; color: var(--fg-muted); cursor: pointer; list-style: none; }
  details.logs > summary::-webkit-details-marker { display: none; }

  .card-foot { align-self: center; font-size: 15px; padding: 10px; }

  /* ===== one breakpoint ===== */
  @media (max-width: 640px) {
    .topbar { padding: 11px 14px; }
    .page { padding: 16px; }
    .file-row { flex-wrap: wrap; }
  }
  ```

- [ ] Create `internal/web/static/app.js` (slim glue only):
  ```js
  'use strict';

  const $ = (id) => document.getElementById(id);

  // ---- Theme toggle (manual override of OS; applied before paint in <head>). ----
  const themeSel = $('theme');
  if (themeSel) {
    themeSel.value = localStorage.getItem('theme') || 'system';
    themeSel.addEventListener('change', () => {
      const v = themeSel.value;
      localStorage.setItem('theme', v);
      if (v === 'system') document.documentElement.removeAttribute('data-theme');
      else document.documentElement.setAttribute('data-theme', v);
    });
  }

  // ---- Identity badge ----
  fetch('/me')
    .then((r) => r.json())
    .then((m) => {
      const el = $('user');
      if (el && m.enabled && (m.name || m.user)) el.textContent = '👤 ' + (m.name || m.user);
    })
    .catch(() => {});

  // ---- Drag-drop assigns files to the real file input inside the form. ----
  const drop = $('drop');
  const fileInput = $('file');
  if (drop && fileInput) {
    drop.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', updateDropLabel);
    ['dragenter', 'dragover'].forEach((ev) =>
      drop.addEventListener(ev, (e) => { e.preventDefault(); drop.classList.add('drag'); }));
    drop.addEventListener('dragleave', (e) => { e.preventDefault(); drop.classList.remove('drag'); });
    drop.addEventListener('drop', (e) => {
      e.preventDefault();
      drop.classList.remove('drag');
      fileInput.files = e.dataTransfer.files;
      updateDropLabel();
    });
  }

  function updateDropLabel() {
    const n = fileInput.files ? fileInput.files.length : 0;
    const label = $('drop-label');
    if (!label) return;
    label.textContent = n === 0 ? 'Drop books here, or choose files'
      : n === 1 ? fileInput.files[0].name
      : n + ' files selected';
  }
  ```

- [ ] Create `internal/web/templates/base.gohtml`:
  ```gotemplate
  {{define "base"}}<!doctype html>
  <html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>fb2 converter</title>
    <script>
      // Apply theme before first paint to avoid a flash.
      (function () {
        var t = localStorage.getItem('theme') || 'system';
        if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
      })();
    </script>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/static/app.css">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  </head>
  <body>
    <header class="topbar">
      <span class="brand">fb2→epub</span>
      <nav class="tabs">
        <a href="/" class="{{if eq .Tab "convert"}}active{{end}}">Convert</a>
        <a href="/settings" class="{{if eq .Tab "settings"}}active{{end}}">Settings</a>
      </nav>
      <div class="right">
        <span id="user">{{if .User}}👤 {{.User}}{{end}}</span>
        <select id="theme" aria-label="Theme">
          <option value="system">System</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </div>
    </header>
    <main class="page">
      {{content .ContentName .}}
    </main>
    <script src="/static/app.js"></script>
  </body>
  </html>{{end}}
  ```
  > The mockup's header is `fb2→epub` + a Convert/Settings `<nav>` on a `--bg-sunk` bar with a
  > hairline bottom border. The theme `<select>` and `#user` badge sit at the right — the LOCKED
  > manual toggle the mockup itself omits (it is pure `@media`), added back per our decision.
  > **How Plans 2/3 conform:** they add one file each defining `{{define "settings"}}` /
  > `{{define "editor"}}` and render via the same `render(w, data)` with a page VM whose embedded
  > `layout` sets `Tab`/`ContentName` accordingly. `base.gohtml` needs no edit — `{{content
  > .ContentName .}}` dispatches to whichever page body name the VM carries.

- [ ] Create `internal/web/templates/convert.gohtml`. Defines the uniquely-named `"convert"`
  page body (base dispatches to it via `.ContentName == "convert"`). Mirrors the mockup's idle
  screen (lines 396–412): dashed `<label for="file">` drop zone, Preset + format selects,
  Convert button, then the card mount.
  ```gotemplate
  {{define "convert"}}
  <form class="convert-form" hx-post="/convert" hx-target="#convert-card" hx-swap="outerHTML"
        hx-encoding="multipart/form-data" hx-disabled-elt="find button">
    <label class="dropzone" for="file" id="drop">
      <span class="dz-title" id="drop-label">Drop books here, or choose files</span>
      <span class="dz-hint">.fb2 · .zip · .fb2.zip — one or several</span>
      <input id="file" type="file" name="file" accept=".fb2,.zip" multiple hidden>
    </label>

    <div class="field">
      <label for="preset">Preset</label>
      <select class="select" id="preset" name="preset">
        {{range .Presets}}<option value="{{.ID}}">{{.Name}}</option>{{end}}
      </select>
    </div>

    <div class="field">
      <label for="format">Output format</label>
      <select class="select" id="format" name="format">
        {{range .Formats}}<option value="{{.}}"{{if eq . $.Format}} selected{{end}}>{{.}}</option>{{end}}
      </select>
    </div>

    <button type="submit" class="btn-primary">Convert</button>
  </form>

  {{template "convert_card" .Card}}
  {{end}}
  ```

- [ ] Create `internal/web/templates/_convert_card.gohtml`. This reproduces the mockup's three
  card states as one partial keyed off `.Status.Done` and the per-file `.State`:
  - **running** (mockup 2c, lines 542–568): a `.card.running` with "Converting N of M…", the
    indeterminate `.bar`, and one `.prog-row` per file with a status `.dot` (done / running /
    queued) and a state word. The whole card keeps the htmx poll trigger.
  - **done, no failures** (mockup 2d, lines 632–666): a `.card.success` with a green `.dot` +
    "N of M converted", a `.file-list` of outputs (name · size · Download), a "Download all · …
    .zip" `.zip-btn` when there is more than one output, and a collapsed logs `<details>`.
  - **done, some failures** (mockup screen 3, lines 736–800): one `.card.error` per failed file
    with the failed filename, a plain-language summary, the `use_broken_images` hint, Retry +
    Skip buttons, the "First error ·" label + `pre.err`, and the full-log `<details>` (lazy
    htmx-loaded). Then a `.card.success` for the files that did convert, exactly like the
    all-success case.
  ```gotemplate
  {{define "convert_card"}}
  {{if .Status}}
  <div id="convert-card"
       {{if not .Status.Done}}hx-get="/jobs/{{.Status.ID}}" hx-trigger="load delay:1s" hx-swap="outerHTML"{{end}}>

    {{if not .Status.Done}}
    {{/* ---------- RUNNING ---------- */}}
    <div class="card running">
      <div>
        <div class="card-head">Converting {{.DoneCount}} of {{.Total}}…</div>
        <div class="file-state mono">preset {{.Status.Preset}} · one at a time</div>
      </div>
      <div class="bar"><i></i></div>
      <div class="prog-list">
        {{range .Status.Files}}
        <div class="prog-row">
          <span class="dot {{if eq .State "done"}}done{{else if eq .State "running"}}running{{else if eq .State "failed"}}failed{{else}}queued{{end}}"></span>
          <span class="prog-name">{{.Input}}</span>
          <span class="file-state">{{if eq .State "pending"}}queued{{else if eq .State "running"}}converting{{else}}{{.State}}{{end}}</span>
        </div>
        {{end}}
      </div>
    </div>

    {{else}}
    {{/* ---------- FAILURES FIRST (one error card per failed file) ---------- */}}
    {{range .Status.Files}}
    {{if eq .State "failed"}}
    <div class="card error">
      <div class="card-head"><span class="dot failed"></span>{{$.FailedCount}} of {{$.Total}} book{{if gt $.Total 1}}s{{end}} failed</div>
      <div class="fail-file">{{.Input}}</div>
      <p class="fail-msg">fb2cng could not finish this book. {{if .FirstError}}The log's first error is shown below.{{end}} The other books in the batch are unaffected.</p>
      <p class="fail-hint">Try again with <b>use_broken_images</b> on — it substitutes a blank image and keeps going.</p>
      <div class="fail-actions">
        <form hx-post="/jobs/{{$.Status.ID}}/retry" hx-target="#convert-card" hx-swap="outerHTML">
          <button type="submit" class="btn-retry">Retry with use_broken_images</button>
        </form>
        <a class="btn-skip" href="/jobs/{{$.Status.ID}}">Skip it</a>
      </div>
      {{if .FirstError}}
      <div class="first-error-label">First error · {{.Input}}</div>
      <pre class="err">{{.FirstError}}</pre>
      {{end}}
      <details class="logs">
        <summary>Full log · {{.Input}} · {{.LogLines}} lines</summary>
        <pre class="log" hx-get="/jobs/{{$.Status.ID}}/log/{{.Input}}" hx-trigger="revealed" hx-swap="innerHTML">…</pre>
      </details>
    </div>
    {{end}}
    {{end}}

    {{/* ---------- SUCCESSFUL FILES ---------- */}}
    {{if gt .DoneCount 0}}
    <div class="card success">
      <div class="card-head"><span class="dot done"></span>{{.DoneCount}} of {{.Total}} converted</div>
      <div class="file-list">
        {{range .Status.Files}}
        {{if eq .State "done"}}
        {{$input := .Input}}
        {{range .Outputs}}
        <div class="file-row">
          <div class="file-info">
            <span class="file-name">{{.}}</span>
          </div>
          <a class="dl" href="/jobs/{{$.Status.ID}}/download/{{.}}">Download</a>
        </div>
        {{end}}
        {{end}}
        {{end}}
      </div>
      {{if gt .TotalOutputs 1}}
      <a class="zip-btn" href="/jobs/{{.Status.ID}}/zip">Download all · {{.TotalOutputs}} files .zip</a>
      {{end}}
    </div>
    {{end}}
    {{end}}

  </div>
  {{else}}
  <div id="convert-card"></div>
  {{end}}
  {{end}}
  ```
  > Note the VM fields the template reads: `.Total`, `.DoneCount`, `.FailedCount` (added to
  > `convertCardVM` in Task 6) and `.TotalOutputs`. `cardFor` (Task 7) computes them.

- [ ] Replace `internal/web/embed.go` with:
  ```go
  // Package web holds the embedded static frontend and templates.
  package web

  import (
  	"embed"
  	"html/template"
  	"strings"
  )

  //go:embed index.html app.js static templates
  var FS embed.FS

  // Templates parses the layout, pages, and partials once. The result exposes named
  // templates: "base" (shared chrome), one per page ("convert" here; Plans 2/3 add
  // "settings"/"editor"), and partials ("convert_card", …).
  //
  // base.gohtml dispatches to the page body with {{content .ContentName .}}. Go's
  // {{template}} action requires a *constant* name, so dynamic dispatch by field goes
  // through this "content" func, which renders the named sub-template into safe HTML.
  func Templates() (*template.Template, error) {
  	root := template.New("root")
  	root.Funcs(template.FuncMap{
  		"content": func(name string, data any) (template.HTML, error) {
  			var b strings.Builder
  			if err := root.ExecuteTemplate(&b, name, data); err != nil {
  				return "", err
  			}
  			return template.HTML(b.String()), nil
  		},
  	})
  	return root.ParseFS(FS, "templates/*.gohtml")
  }
  ```
  > `(*Template).ParseFS` returns the same `root` pointer, so the `content` closure that
  > captures `root` sees every parsed page/partial at execution time. `template.HTML` is safe
  > here because the sub-templates are our own trusted `.gohtml`, still auto-escaping their data.
  >
  > (`index.html` and `app.js` remain embedded for now so `static_test.go` keeps passing until
  > Task 6 removes them.)

- [ ] Run it, expect PASS:
  ```
  go test ./internal/web/
  ```
  Expected: `ok  fb2cng-web/internal/web`.

- [ ] Commit:
  ```
  git add internal/web/static internal/web/templates internal/web/embed.go internal/web/templates_test.go
  git commit -m "feat(web): port 8-token theme, htmx templates, slim app.js

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 6: Server struct + `New()` signature + `GET /` Convert page

**Files:** `internal/server/server.go`, `internal/server/handlers.go`,
`internal/server/handlers_test.go`, `internal/server/static_test.go`,
plus delete `internal/web/index.html` and `internal/web/app.js` and update `embed.go`.

> This task **establishes the new `Server` struct and `New()` signature that Plan 1 owns**, per
> the shared-interfaces contract ("Server fields grow PER-PLAN"). Plan 1's `Server` carries
> **only** `cfg`, `runner`, `static`, `sem`, `tpl`, `jobs` (plus a `mu` for status writes).
> **There are NO `presets`/`schema` fields and NO `interface{}` placeholders** — a package
> can't be imported before it exists. Plan 2 appends `presets *presets.Store` and widens
> `New(...)` to add that parameter; Plan 3 appends `schema *schema.Schema` and widens again.
> Each of them updates `main.go` and this plan's test helper at that time. Plan 1's `New` takes
> exactly five params.
>
> This task also owns the **canonical single layout** (contract §"One layout"): `base.gohtml`
> defines `"base"`, renders the shared chrome, and dispatches to the page body via
> `{{template .ContentName .}}`. The Convert page file defines exactly one uniquely-named
> content template, `{{define "convert"}}`. Every page VM embeds a `layout` header struct
> (`Tab`, `ContentName`, `User`). A `render(w, data)` helper executes `"base"`; htmx partials
> (`_convert_card`, …) render directly by name, never through `base`. This is what lets Plans 2
> and 3 add their own `{{define "settings"}}` / `{{define "editor"}}` pages against the same
> `base` with zero changes here.

**Interfaces**
- Produces (Plan 1 owns; Plans 2/3 append fields + params, never rename):
  ```go
  type Server struct {
      cfg    config.Config
      runner convert.Runner
      static fs.FS
      sem    chan struct{}
      tpl    *template.Template
      jobs   *jobs.Store
      mu     sync.Mutex // guards status.json read-modify-write (low-hardening posture)
  }

  func New(cfg config.Config, runner convert.Runner, static fs.FS,
      tpl *template.Template, jobs *jobs.Store) *Server

  // layout is the shared header VM embedded by every page VM.
  type layout struct {
      Tab         string // "convert" | "settings" — selects the active nav tab
      ContentName string // name of the {{define}} page body base dispatches to
      User        string // forward-auth display name, "" when disabled
  }

  func (s *Server) render(w http.ResponseWriter, data any)             // executes "base"
  func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) // htmx fragment
  ```

**Steps**

- [ ] Update the test harness first (the signature changes, so every `New(...)` call and the
  page assertion move together). In `internal/server/handlers_test.go`, replace `newTestServer`
  with:
  ```go
  func newTestServer(t *testing.T, cfg config.Config, r convert.Runner) http.Handler {
  	t.Helper()
  	tpl, err := web.Templates()
  	if err != nil {
  		t.Fatal(err)
  	}
  	if cfg.JobsDir == "" {
  		cfg.JobsDir = t.TempDir()
  	}
  	store := jobs.NewStore(cfg.JobsDir, time.Hour)
  	return New(cfg, r, web.FS, tpl, store).Handler()
  }
  ```
  Add the needed imports to that file's import block: `"time"` and
  `"fb2cng-web/internal/jobs"`.

- [ ] Update `internal/server/static_test.go` to assert the rendered page:
  ```go
  package server

  import (
  	"net/http/httptest"
  	"strings"
  	"testing"

  	"fb2cng-web/internal/config"
  )

  func TestIndexRendersConvertTab(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 1}, stubRunner{})
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
  	body := rec.Body.String()
  	if rec.Code != 200 {
  		t.Fatalf("index not served: code=%d", rec.Code)
  	}
  	for _, want := range []string{`class="tabs"`, ">Convert<", ">Settings<", `class="convert-form"`, `class="dropzone"`} {
  		if !strings.Contains(body, want) {
  			t.Errorf("rendered page missing %q", want)
  		}
  	}
  }

  func TestStaticCSSServed(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 1}, stubRunner{})
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/static/app.css", nil))
  	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "--accent") {
  		t.Fatalf("app.css not served: code=%d", rec.Code)
  	}
  }
  ```

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/server/ -run 'TestIndexRenders|TestStaticCSS'
  ```
  Expected: compile error — `New` signature mismatch / `web.Templates` used but page handler
  not yet rendering (`GET /` still the old FileServer, and `index.html` still present). Build
  fails first on the `New` arity change.

- [ ] Minimal implementation — rewrite `internal/server/server.go`:
  ```go
  package server

  import (
  	"html/template"
  	"io/fs"
  	"net/http"
  	"sync"

  	"fb2cng-web/internal/config"
  	"fb2cng-web/internal/convert"
  	"fb2cng-web/internal/jobs"
  )

  // Server wires configuration, the fbc runner, the job store, and the templated frontend.
  // Plan 2 appends `presets *presets.Store`; Plan 3 appends `schema *schema.Schema`.
  type Server struct {
  	cfg    config.Config
  	runner convert.Runner
  	static fs.FS
  	sem    chan struct{}
  	tpl    *template.Template
  	jobs   *jobs.Store
  	mu     sync.Mutex // guards status.json read-modify-write (self-hosted, low-hardening)
  }

  // New constructs a Server. Plans 2 and 3 widen this signature (adding presets, then
  // schema) when those packages land; Plan 1 takes exactly five parameters.
  func New(cfg config.Config, runner convert.Runner, static fs.FS,
  	tpl *template.Template, jobStore *jobs.Store) *Server {
  	n := cfg.MaxConcurrent
  	if n < 1 {
  		n = 1
  	}
  	return &Server{
  		cfg:    cfg,
  		runner: runner,
  		static: static,
  		sem:    make(chan struct{}, n),
  		tpl:    tpl,
  		jobs:   jobStore,
  	}
  }

  // Handler returns the full HTTP handler with routes and auth applied.
  func (s *Server) Handler() http.Handler {
  	mux := http.NewServeMux()
  	mux.HandleFunc("GET /", s.handleIndex)
  	mux.HandleFunc("GET /defaults", s.handleDefaults)
  	mux.HandleFunc("GET /me", s.handleMe)
  	mux.HandleFunc("POST /convert", s.handleConvert)
  	mux.HandleFunc("GET /jobs/{id}", s.handleJobStatus)
  	mux.HandleFunc("GET /jobs/{id}/download/{file}", s.handleDownload)
  	mux.HandleFunc("GET /jobs/{id}/zip", s.handleZip)
  	mux.HandleFunc("GET /jobs/{id}/log/{file}", s.handleLog)
  	mux.HandleFunc("POST /jobs/{id}/retry", s.handleRetry)
  	mux.Handle("GET /static/", http.FileServer(http.FS(s.static)))
  	return ForwardAuth(s.cfg.ForwardAuth, s.cfg.TrustedProxies, mux)
  }

  // render executes the shared "base" layout, which dispatches to the page body named by
  // the embedded layout.ContentName. Use it for full-page GETs.
  func (s *Server) render(w http.ResponseWriter, data any) {
  	w.Header().Set("Content-Type", "text/html; charset=utf-8")
  	if err := s.tpl.ExecuteTemplate(w, "base", data); err != nil {
  		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
  	}
  }

  // renderPartial executes a single named template (an htmx fragment) directly, bypassing
  // the base layout. Use it for the convert-card and other swap regions.
  func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) {
  	w.Header().Set("Content-Type", "text/html; charset=utf-8")
  	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
  		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
  	}
  }
  ```
  > The `handleConvert`, `handleJobStatus`, `handleDownload`, `handleZip`, `handleLog`, and
  > `handleRetry` methods are added in Tasks 7–10. To keep this task's build green, the
  > `GET /` handler and the page view models are added now (below); the job handlers get
  > **temporary stubs** here and their real bodies in the later tasks. Add the stubs and the
  > index handler in `internal/server/convert_flow.go` (created new in this task, fleshed out
  > later):
  ```go
  package server

  import (
  	"net/http"
  )

  // layout is the shared header VM embedded by every page VM. base.gohtml reads its
  // promoted fields (.Tab, .ContentName, .User) directly.
  type layout struct {
  	Tab         string // "convert" | "settings"
  	ContentName string // {{define}} name of the page body base dispatches to
  	User        string // forward-auth display name, "" when disabled
  }

  // Page/card view models rendered by the Convert templates.
  type presetOption struct {
  	ID   string
  	Name string
  }

  type convertCardVM struct {
  	Status       *jobsStatus
  	TotalOutputs int // total output files across the batch
  	Total        int // number of input files
  	DoneCount    int // files in state "done"
  	FailedCount  int // files in state "failed"
  }

  type pageVM struct {
  	layout            // embedded: promotes .Tab, .ContentName, .User
  	Presets []presetOption
  	Formats []string
  	Format  string
  	Card    *convertCardVM
  }

  // jobsStatus is an alias so templates can reference status fields; kept in one
  // place so Tasks 7–10 build the same shape. It is exactly *jobs.Status.
  // (Declared via a type alias in convert_flow.go's real version.)

  var uiFormats = []string{"epub3", "epub2", "kepub", "kfx", "azw8", "pdf"}

  func defaultPresetOptions() []presetOption {
  	return []presetOption{{ID: "defaults", Name: "Defaults"}}
  }

  func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
  	s.render(w, pageVM{
  		layout:  layout{Tab: "convert", ContentName: "convert", User: r.Header.Get("Remote-Name")},
  		Presets: defaultPresetOptions(),
  		Formats: uiFormats,
  		Format:  "epub3",
  		Card:    &convertCardVM{},
  	})
  }

  // --- temporary stubs; real bodies land in Tasks 7–10 ---
  func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request)   { http.Error(w, "not yet", http.StatusNotImplemented) }
  func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) { http.Error(w, "not yet", http.StatusNotImplemented) }
  func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request)  { http.Error(w, "not yet", http.StatusNotImplemented) }
  func (s *Server) handleZip(w http.ResponseWriter, r *http.Request)       { http.Error(w, "not yet", http.StatusNotImplemented) }
  func (s *Server) handleLog(w http.ResponseWriter, r *http.Request)       { http.Error(w, "not yet", http.StatusNotImplemented) }
  func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request)     { http.Error(w, "not yet", http.StatusNotImplemented) }
  ```
  > NOTE on `jobsStatus`: the `convertCardVM.Status` field must be `*jobs.Status` so the
  > template can read `.Status.ID`, `.Status.Files`, `.Status.Done`. Replace the placeholder
  > comment/alias by importing `jobs` and typing the field directly:
  ```go
  // In convert_flow.go, replace the pseudo-alias with the real type:
  //   import "fb2cng-web/internal/jobs"
  //   type convertCardVM struct {
  //       Status *jobs.Status; TotalOutputs, Total, DoneCount, FailedCount int
  //   }
  ```
  Apply that now: add `"fb2cng-web/internal/jobs"` to `convert_flow.go` imports and change the
  field to `Status *jobs.Status`. Delete the `jobsStatus` comment block.

- [ ] Remove the old front-end and old convert handler. Delete the dead code from
  `internal/server/handlers.go`: remove `handleConvert`, `formOptions`, `streamZip` (the job
  flow uses `WriteZip`), and the now-unused imports (`archive/zip`, `strconv`). Keep
  `handleDefaults`, `handleMe`, `writeJSON`, `allowedFormats`, `streamFile`, `contentTypeFor`,
  `contentDisposition`. The trimmed import block becomes:
  ```go
  import (
  	"encoding/json"
  	"io"
  	"log"
  	"net/http"
  	"net/url"
  	"os"
  	"path/filepath"
  	"strings"
  )
  ```
  Then delete the old embedded front end and update the embed directive:
  ```
  git rm internal/web/index.html internal/web/app.js
  ```
  In `internal/web/embed.go` change the directive to:
  ```go
  //go:embed static templates
  var FS embed.FS
  ```

- [ ] Run it, expect PASS:
  ```
  go build ./...
  go test ./internal/server/ ./internal/web/
  ```
  Expected: clean build; `ok` for both packages. (The `TestConvert*` job-flow tests are added
  in Task 7; existing bad-format/etc. convert tests that referenced the old streaming behavior
  are rewritten in Task 7 — if any remain here referencing removed behavior, delete them now so
  this task is green, and re-add the job-flow versions in Task 7.)

- [ ] Commit:
  ```
  git add -A
  git commit -m "feat(server): new Server struct, New() signature, and server-rendered GET /

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 7: `POST /convert` — create job, persist inputs, background per-file convert

**Files:** `internal/server/convert_flow.go`, `internal/server/handlers_test.go`

**Interfaces**
- Consumes: `s.jobs *jobs.Store`, `s.runner.ConvertLogged`, `s.sem`,
  `convert.BuildConfig`, `allowedFormats`.
- Produces:
  ```go
  func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) // creates job, returns convert_card
  func (s *Server) processFiles(id, cfgPath, format string, inputs []string) // background worker
  func (s *Server) updateFile(id string, fr jobs.FileResult) // mutex-guarded status merge
  func scanLog(path string) (lines int, firstError string)
  func cardFor(st *jobs.Status) *convertCardVM
  ```

**Steps**

- [ ] Write the failing test. Replace the convert-related tests in
  `internal/server/handlers_test.go` (remove `TestConvertSingleStreamsFile`,
  `TestConvertMultiZips`, `TestConvertRunnerError`, `TestConvertWritesDefaultConfig`,
  `TestConvertCheckboxFalseDisablesDefault`, and the `capturingRunner` type if unused) and add
  a runner backed by the fake `fbc` plus a polling helper:
  ```go
  // fakeFbcRunner drives handler tests through the real fake-fbc.sh so the whole
  // job flow (log capture, retry override) is exercised end-to-end.
  func fakeFbcRunner(t *testing.T) convert.Runner {
  	t.Helper()
  	abs, err := filepath.Abs("../../testdata/fake-fbc.sh")
  	if err != nil {
  		t.Fatal(err)
  	}
  	return convert.New(abs)
  }

  // waitDone polls the job's status.json until the batch is terminal or the deadline hits.
  func waitDone(t *testing.T, h http.Handler, id string) *jobs.Status {
  	t.Helper()
  	deadline := time.Now().Add(5 * time.Second)
  	for time.Now().Before(deadline) {
  		if st := loadStatus(t, h, id); st != nil && st.Done {
  			return st
  		}
  		time.Sleep(20 * time.Millisecond)
  	}
  	t.Fatalf("job %s did not finish in time", id)
  	return nil
  }

  // loadStatus reads the job's status.json directly from the store dir the test
  // configured, avoiding races on partially-written responses.
  func loadStatus(t *testing.T, h http.Handler, id string) *jobs.Status {
  	t.Helper()
  	b, err := os.ReadFile(filepath.Join(testJobsDir, id, "status.json"))
  	if err != nil {
  		return nil
  	}
  	var st jobs.Status
  	if err := json.Unmarshal(b, &st); err != nil {
  		return nil
  	}
  	return &st
  }
  ```
  Because the store dir must be knowable by `loadStatus`, pin it. Add a package-level test var
  and set it in `newTestServer`:
  ```go
  var testJobsDir string
  ```
  and in `newTestServer` replace the `store` lines with:
  ```go
  	if cfg.JobsDir == "" {
  		cfg.JobsDir = t.TempDir()
  	}
  	testJobsDir = cfg.JobsDir
  	store := jobs.NewStore(cfg.JobsDir, time.Hour)
  ```
  Now add the flow tests:
  ```go
  // extractJobID pulls the job id out of the convert_card the POST returns.
  func extractJobID(t *testing.T, body string) string {
  	t.Helper()
  	m := regexp.MustCompile(`/jobs/([0-9a-f]{16})`).FindStringSubmatch(body)
  	if m == nil {
  		t.Fatalf("no job id in response:\n%s", body)
  	}
  	return m[1]
  }

  func TestConvertCreatesJobAndSucceeds(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
  	if rec.Code != 200 {
  		t.Fatalf("convert POST code=%d body=%q", rec.Code, rec.Body.String())
  	}
  	id := extractJobID(t, rec.Body.String())
  	st := waitDone(t, h, id)
  	if len(st.Files) != 1 || st.Files[0].State != jobs.StateDone {
  		t.Fatalf("expected 1 done file, got %+v", st.Files)
  	}
  	if len(st.Files[0].Outputs) != 1 || st.Files[0].LogLines < 2 {
  		t.Fatalf("expected output + multi-line log, got %+v", st.Files[0])
  	}
  }

  func TestConvertPerFileFailureCapturesFirstError(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, multipartConvert(t, "corrupt.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
  	id := extractJobID(t, rec.Body.String())
  	st := waitDone(t, h, id)
  	if st.Files[0].State != jobs.StateFailed {
  		t.Fatalf("expected failed, got %s", st.Files[0].State)
  	}
  	if !strings.HasPrefix(st.Files[0].FirstError, "ERR") {
  		t.Fatalf("expected captured ERR first line, got %q", st.Files[0].FirstError)
  	}
  }

  func TestConvertBadFormat(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 1}, fakeFbcRunner(t))
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "mobi"}))
  	if rec.Code != http.StatusUnprocessableEntity {
  		t.Fatalf("bad format should be 422, got %d", rec.Code)
  	}
  }
  ```
  Update the imports in `handlers_test.go`: add `"regexp"`; keep `os`, `time`, `json`,
  `filepath`, `strings`, `jobs`, `convert`, `config`, `web`, `httptest`, `http`, `bytes`,
  `mime/multipart`, `testing`, `sync`, `context`. Remove `archive/zip`, `errors` if no longer
  referenced.

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/server/ -run TestConvert
  ```
  Expected: `501 Not Implemented` from the stub `handleConvert`, so `extractJobID` fails to
  find a job id.

- [ ] Minimal implementation — replace the stub `handleConvert` in `convert_flow.go` and add
  the worker + helpers. The file's imports become:
  ```go
  import (
  	"bufio"
  	"context"
  	"net/http"
  	"os"
  	"path/filepath"
  	"strings"
  	"time"

  	"fb2cng-web/internal/convert"
  	"fb2cng-web/internal/jobs"
  )
  ```
  Replace the `handleConvert` stub with:
  ```go
  func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
  	if err := r.ParseMultipartForm(128 << 20); err != nil {
  		http.Error(w, "invalid upload: "+err.Error(), http.StatusBadRequest)
  		return
  	}
  	format := r.FormValue("format")
  	if format == "" {
  		format = "epub3"
  	}
  	if !allowedFormats[format] {
  		http.Error(w, "unsupported format: "+format, http.StatusUnprocessableEntity)
  		return
  	}
  	preset := r.FormValue("preset") // Plan 1: only "defaults" is offered
  	if preset == "" {
  		preset = "defaults"
  	}

  	files := r.MultipartForm.File["file"]
  	if len(files) == 0 {
  		http.Error(w, "missing file", http.StatusBadRequest)
  		return
  	}

  	id, err := s.jobs.Create(preset, format)
  	if err != nil {
  		http.Error(w, "server error", http.StatusInternalServerError)
  		return
  	}

  	// Persist inputs synchronously (the request body will not survive the goroutine).
  	var names []string
  	for _, fh := range files {
  		name := filepath.Base(fh.Filename)
  		lower := strings.ToLower(name)
  		if !strings.HasSuffix(lower, ".fb2") && !strings.HasSuffix(lower, ".zip") {
  			http.Error(w, "only .fb2 and .zip files are accepted", http.StatusUnprocessableEntity)
  			return
  		}
  		src, err := fh.Open()
  		if err != nil {
  			http.Error(w, "server error", http.StatusInternalServerError)
  			return
  		}
  		dst, err := os.Create(s.jobs.InputPath(id, name))
  		if err != nil {
  			src.Close()
  			http.Error(w, "server error", http.StatusInternalServerError)
  			return
  		}
  		if _, err := copyAndClose(dst, src); err != nil {
  			http.Error(w, "server error", http.StatusInternalServerError)
  			return
  		}
  		names = append(names, name)
  	}

  	// Seed pending file rows.
  	st, err := s.jobs.Load(id)
  	if err != nil {
  		http.Error(w, "server error", http.StatusInternalServerError)
  		return
  	}
  	for _, n := range names {
  		st.Files = append(st.Files, jobs.FileResult{Input: n, State: jobs.StatePending})
  	}
  	if err := s.jobs.Save(id, st); err != nil {
  		http.Error(w, "server error", http.StatusInternalServerError)
  		return
  	}

  	// Write the effective config once for the whole batch (Defaults => app defaults).
  	cfgPath := filepath.Join(s.jobs.Dir, id, "config.yaml")
  	cfgBytes, err := convert.BuildConfig("", convert.FormOptions{})
  	if err != nil {
  		http.Error(w, "invalid config: "+err.Error(), http.StatusUnprocessableEntity)
  		return
  	}
  	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
  		http.Error(w, "server error", http.StatusInternalServerError)
  		return
  	}

  	go s.processFiles(id, cfgPath, format, names)

  	st, _ = s.jobs.Load(id)
  	s.renderPartial(w, "convert_card", cardFor(st))
  }

  // copyAndClose copies src into dst and closes both, returning the first error.
  func copyAndClose(dst *os.File, src interface{ Read([]byte) (int, error) }) (int64, error) {
  	n, err := ioCopy(dst, src)
  	if cerr := dst.Close(); err == nil {
  		err = cerr
  	}
  	if c, ok := src.(interface{ Close() error }); ok {
  		if cerr := c.Close(); err == nil {
  			err = cerr
  		}
  	}
  	return n, err
  }
  ```
  Add the small copy shim and worker/helpers to the same file:
  ```go
  func ioCopy(dst *os.File, src interface{ Read([]byte) (int, error) }) (int64, error) {
  	buf := make([]byte, 32*1024)
  	var total int64
  	for {
  		n, rerr := src.Read(buf)
  		if n > 0 {
  			if _, werr := dst.Write(buf[:n]); werr != nil {
  				return total, werr
  			}
  			total += int64(n)
  		}
  		if rerr != nil {
  			if rerr.Error() == "EOF" {
  				return total, nil
  			}
  			return total, rerr
  		}
  	}
  }

  // processFiles runs fbc per input under the sem cap, capturing logs and updating status.
  func (s *Server) processFiles(id, cfgPath, format string, inputs []string) {
  	for _, in := range inputs {
  		s.sem <- struct{}{}
  		start := time.Now()
  		outs, cerr := s.runner.ConvertLogged(
  			context.Background(), s.jobs.InputPath(id, in), format, cfgPath,
  			s.jobs.OutDir(id, in), s.jobs.LogPath(id, in))
  		<-s.sem

  		lines, firstErr := scanLog(s.jobs.LogPath(id, in))
  		fr := jobs.FileResult{
  			Input:      in,
  			LogLines:   lines,
  			FirstError: firstErr,
  			Millis:     time.Since(start).Milliseconds(),
  			Sizes:      map[string]int64{},
  		}
  		for _, o := range outs {
  			base := filepath.Base(o)
  			fr.Outputs = append(fr.Outputs, base)
  			if fi, e := os.Stat(o); e == nil {
  				fr.Sizes[base] = fi.Size()
  			}
  		}
  		if cerr != nil {
  			fr.State = jobs.StateFailed
  			fr.Err = cerr.Error()
  		} else {
  			fr.State = jobs.StateDone
  		}
  		s.updateFile(id, fr)
  	}
  }

  // updateFile merges one file's result into status.json and recomputes Done.
  func (s *Server) updateFile(id string, fr jobs.FileResult) {
  	s.mu.Lock()
  	defer s.mu.Unlock()
  	st, err := s.jobs.Load(id)
  	if err != nil {
  		return
  	}
  	found := false
  	for i := range st.Files {
  		if st.Files[i].Input == fr.Input {
  			st.Files[i] = fr
  			found = true
  			break
  		}
  	}
  	if !found {
  		st.Files = append(st.Files, fr)
  	}
  	st.Done = allTerminal(st.Files)
  	_ = s.jobs.Save(id, st)
  }

  func allTerminal(files []jobs.FileResult) bool {
  	for _, f := range files {
  		if f.State != jobs.StateDone && f.State != jobs.StateFailed {
  			return false
  		}
  	}
  	return len(files) > 0
  }

  // scanLog counts lines and returns the first line starting with "ERR".
  func scanLog(path string) (lines int, firstError string) {
  	f, err := os.Open(path)
  	if err != nil {
  		return 0, ""
  	}
  	defer f.Close()
  	sc := bufio.NewScanner(f)
  	for sc.Scan() {
  		lines++
  		if firstError == "" && strings.HasPrefix(sc.Text(), "ERR") {
  			firstError = sc.Text()
  		}
  	}
  	return lines, firstError
  }

  // cardFor builds the convert-card view model from a status.
  func cardFor(st *jobs.Status) *convertCardVM {
  	vm := &convertCardVM{Status: st, Total: len(st.Files)}
  	for _, f := range st.Files {
  		vm.TotalOutputs += len(f.Outputs)
  		switch f.State {
  		case jobs.StateDone:
  			vm.DoneCount++
  		case jobs.StateFailed:
  			vm.FailedCount++
  		}
  	}
  	return vm
  }
  ```
  (The `bufio`, `context`, `time` imports are now used.)

- [ ] Run it, expect PASS:
  ```
  go test ./internal/server/ -run TestConvert
  ```
  Expected: `ok`. If `TestConvertPerFileFailure` flakes, confirm `waitDone` polls the store
  file (it does) rather than the response body.

- [ ] Commit:
  ```
  git add internal/server/convert_flow.go internal/server/handlers_test.go
  git commit -m "feat(server): POST /convert creates a job and runs fbc per file with log capture

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 8: `GET /jobs/{id}` — status partial (htmx poll)

**Files:** `internal/server/convert_flow.go`, `internal/server/handlers_test.go`

**Interfaces**
- Produces:
  ```go
  func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) // renders convert_card for the job
  ```

**Steps**

- [ ] Write the failing test. Append to `internal/server/handlers_test.go`:
  ```go
  func TestJobStatusRendersCard(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
  	id := extractJobID(t, rec.Body.String())
  	waitDone(t, h, id)

  	rec = httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/jobs/"+id, nil))
  	if rec.Code != 200 {
  		t.Fatalf("job status code=%d", rec.Code)
  	}
  	body := rec.Body.String()
  	if !strings.Contains(body, `id="convert-card"`) {
  		t.Fatalf("missing convert-card:\n%s", body)
  	}
  	if !strings.Contains(body, "/jobs/"+id+"/download/book.epub") {
  		t.Fatalf("missing download link:\n%s", body)
  	}
  	// Done card must not carry the polling trigger.
  	if strings.Contains(body, `hx-trigger="load`) {
  		t.Fatalf("done card should not keep polling:\n%s", body)
  	}
  }

  func TestJobStatusUnknownID(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 1}, fakeFbcRunner(t))
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/jobs/deadbeefdeadbeef", nil))
  	if rec.Code != http.StatusNotFound {
  		t.Fatalf("unknown job should 404, got %d", rec.Code)
  	}
  }
  ```

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/server/ -run TestJobStatus
  ```
  Expected: `501 Not Implemented` from the stub.

- [ ] Minimal implementation — replace the `handleJobStatus` stub in `convert_flow.go`:
  ```go
  func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
  	id := r.PathValue("id")
  	st, err := s.jobs.Load(id)
  	if err != nil {
  		http.NotFound(w, r)
  		return
  	}
  	s.renderPartial(w, "convert_card", cardFor(st))
  }
  ```

- [ ] Run it, expect PASS:
  ```
  go test ./internal/server/ -run TestJobStatus
  ```
  Expected: `ok`.

- [ ] Commit:
  ```
  git add internal/server/convert_flow.go internal/server/handlers_test.go
  git commit -m "feat(server): GET /jobs/{id} renders the polled convert-card partial

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 9: Downloads — `GET /jobs/{id}/download/{file}`, `/zip`, `/log/{file}`

**Files:** `internal/server/convert_flow.go`, `internal/server/handlers_test.go`

**Interfaces**
- Consumes: `s.jobs.OutputFile`, `s.jobs.WriteZip`, `s.jobs.LogPath`, `streamFile`.
- Produces:
  ```go
  func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) // one output by basename
  func (s *Server) handleZip(w http.ResponseWriter, r *http.Request)      // all outputs as a zip
  func (s *Server) handleLog(w http.ResponseWriter, r *http.Request)      // full captured log (text/plain)
  ```

**Steps**

- [ ] Write the failing test. Append to `internal/server/handlers_test.go`:
  ```go
  func TestDownloadZipAndLog(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
  	rec := httptest.NewRecorder()
  	// "multi" input produces two outputs.
  	h.ServeHTTP(rec, multipartConvert(t, "multi.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
  	id := extractJobID(t, rec.Body.String())
  	waitDone(t, h, id)

  	// Single download.
  	rec = httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/jobs/"+id+"/download/multi.epub", nil))
  	if rec.Code != 200 || rec.Body.String() != "FAKE-epub3" {
  		t.Fatalf("download code=%d body=%q", rec.Code, rec.Body.String())
  	}

  	// Traversal is rejected.
  	rec = httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/jobs/"+id+"/download/nope.epub", nil))
  	if rec.Code != http.StatusNotFound {
  		t.Fatalf("unknown output should 404, got %d", rec.Code)
  	}

  	// Zip of all outputs.
  	rec = httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/jobs/"+id+"/zip", nil))
  	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" {
  		t.Fatalf("zip code=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
  	}
  	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
  	if err != nil || len(zr.File) != 2 {
  		t.Fatalf("expected 2-entry zip, err=%v files=%d", err, len(zr.File))
  	}

  	// Log stream.
  	rec = httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("GET", "/jobs/"+id+"/log/multi.fb2", nil))
  	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "INFO") {
  		t.Fatalf("log code=%d body=%q", rec.Code, rec.Body.String())
  	}
  }
  ```
  Ensure `archive/zip` and `bytes` are imported in the test file (re-add if Task 7 removed
  them).

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/server/ -run TestDownloadZipAndLog
  ```
  Expected: `501 Not Implemented` from the stubs.

- [ ] Minimal implementation — replace the three stubs in `convert_flow.go`:
  ```go
  func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
  	id := r.PathValue("id")
  	file := r.PathValue("file")
  	st, err := s.jobs.Load(id)
  	if err != nil {
  		http.NotFound(w, r)
  		return
  	}
  	for _, fr := range st.Files {
  		for _, o := range fr.Outputs {
  			if o == file {
  				p, err := s.jobs.OutputFile(id, fr.Input, o)
  				if err != nil {
  					http.NotFound(w, r)
  					return
  				}
  				streamFile(w, p)
  				return
  			}
  		}
  	}
  	http.NotFound(w, r)
  }

  func (s *Server) handleZip(w http.ResponseWriter, r *http.Request) {
  	id := r.PathValue("id")
  	if _, err := s.jobs.Load(id); err != nil {
  		http.NotFound(w, r)
  		return
  	}
  	w.Header().Set("Content-Type", "application/zip")
  	w.Header().Set("Content-Disposition", contentDisposition(id+".zip"))
  	if err := s.jobs.WriteZip(id, w); err != nil {
  		// Header already sent; log-only. (self-hosted, low-hardening)
  		return
  	}
  }

  func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
  	id := r.PathValue("id")
  	input := r.PathValue("file")
  	if strings.ContainsAny(input, `/\`) || strings.Contains(input, "..") {
  		http.NotFound(w, r)
  		return
  	}
  	if _, err := s.jobs.Load(id); err != nil {
  		http.NotFound(w, r)
  		return
  	}
  	f, err := os.Open(s.jobs.LogPath(id, input))
  	if err != nil {
  		http.NotFound(w, r)
  		return
  	}
  	defer f.Close()
  	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
  	_, _ = ioCopyWriter(w, f)
  }

  func ioCopyWriter(w http.ResponseWriter, f *os.File) (int64, error) {
  	buf := make([]byte, 32*1024)
  	var total int64
  	for {
  		n, rerr := f.Read(buf)
  		if n > 0 {
  			if _, werr := w.Write(buf[:n]); werr != nil {
  				return total, werr
  			}
  			total += int64(n)
  		}
  		if rerr != nil {
  			return total, nil
  		}
  	}
  }
  ```

- [ ] Run it, expect PASS:
  ```
  go test ./internal/server/ -run TestDownloadZipAndLog
  ```
  Expected: `ok`.

- [ ] Commit:
  ```
  git add internal/server/convert_flow.go internal/server/handlers_test.go
  git commit -m "feat(server): download, zip, and log routes for retained jobs

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 10: `POST /jobs/{id}/retry` — re-run failed inputs with `use_broken_images`

**Files:** `internal/server/convert_flow.go`, `internal/server/handlers_test.go`

**Interfaces**
- Consumes: `s.jobs`, `s.processFiles`, `convert.BuildConfig`.
- Produces:
  ```go
  func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) // re-runs failed files, returns convert_card
  ```

**Steps**

- [ ] Write the failing test. Append to `internal/server/handlers_test.go`:
  ```go
  func TestRetryReRunsFailedWithOverride(t *testing.T) {
  	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, multipartConvert(t, "corrupt.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
  	id := extractJobID(t, rec.Body.String())
  	st := waitDone(t, h, id)
  	if st.Files[0].State != jobs.StateFailed {
  		t.Fatalf("precondition: expected failed, got %s", st.Files[0].State)
  	}

  	rec = httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest("POST", "/jobs/"+id+"/retry", nil))
  	if rec.Code != 200 {
  		t.Fatalf("retry code=%d body=%q", rec.Code, rec.Body.String())
  	}
  	st = waitDone(t, h, id)
  	if st.Files[0].State != jobs.StateDone {
  		t.Fatalf("retry with use_broken_images should succeed, got %s (err=%q)", st.Files[0].State, st.Files[0].Err)
  	}
  	if len(st.Files[0].Outputs) != 1 {
  		t.Fatalf("expected an output after retry, got %+v", st.Files[0])
  	}
  }
  ```

- [ ] Run it, expect FAIL:
  ```
  go test ./internal/server/ -run TestRetry
  ```
  Expected: `501 Not Implemented` from the stub.

- [ ] Minimal implementation — replace the `handleRetry` stub in `convert_flow.go`:
  ```go
  func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
  	id := r.PathValue("id")
  	st, err := s.jobs.Load(id)
  	if err != nil {
  		http.NotFound(w, r)
  		return
  	}

  	var failed []string
  	for i := range st.Files {
  		if st.Files[i].State == jobs.StateFailed {
  			failed = append(failed, st.Files[i].Input)
  			st.Files[i].State = jobs.StatePending
  			st.Files[i].Err = ""
  			st.Files[i].FirstError = ""
  		}
  	}
  	if len(failed) == 0 {
  		s.renderPartial(w, "convert_card", cardFor(st))
  		return
  	}
  	st.Done = false
  	if err := s.jobs.Save(id, st); err != nil {
  		http.Error(w, "server error", http.StatusInternalServerError)
  		return
  	}

  	// Retry config = batch defaults + the broken-image tolerance flip.
  	cfgBytes, err := convert.BuildConfig("use_broken_images: true", convert.FormOptions{})
  	if err != nil {
  		http.Error(w, "invalid retry config: "+err.Error(), http.StatusUnprocessableEntity)
  		return
  	}
  	cfgPath := filepath.Join(s.jobs.Dir, id, "retry-config.yaml")
  	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
  		http.Error(w, "server error", http.StatusInternalServerError)
  		return
  	}

  	go s.processFiles(id, cfgPath, st.Format, failed)

  	st, _ = s.jobs.Load(id)
  	s.renderPartial(w, "convert_card", cardFor(st))
  }
  ```

- [ ] Run it, expect PASS:
  ```
  go test ./internal/server/
  ```
  Expected: `ok fb2cng-web/internal/server` (whole package).

- [ ] Commit:
  ```
  git add internal/server/convert_flow.go internal/server/handlers_test.go
  git commit -m "feat(server): POST /jobs/{id}/retry re-runs failed inputs with use_broken_images

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 11: `main.go` — wire the store, parse templates, start the TTL sweeper

**Files:** `main.go`

**Interfaces**
- Consumes: `config.FromEnv`, `web.Templates`, `jobs.NewStore`, `server.New`,
  `(*jobs.Store).Sweep`.

**Steps**

- [ ] Write the failing check. `main` has no unit test; guard it with a build + vet gate.
  First confirm it currently fails to compile against the new `New()` signature:
  ```
  go build ./...
  ```
  Expected: FAIL — `not enough arguments in call to server.New`.

- [ ] Minimal implementation — replace `main.go` with:
  ```go
  package main

  import (
  	"log"
  	"net/http"
  	"os"
  	"time"

  	"fb2cng-web/internal/config"
  	"fb2cng-web/internal/convert"
  	"fb2cng-web/internal/jobs"
  	"fb2cng-web/internal/server"
  	"fb2cng-web/internal/web"
  )

  func main() {
  	cfg := config.FromEnv()

  	if err := os.MkdirAll(cfg.JobsDir, 0o755); err != nil {
  		log.Fatalf("jobs dir %s: %v", cfg.JobsDir, err)
  	}
  	tpl, err := web.Templates()
  	if err != nil {
  		log.Fatalf("parse templates: %v", err)
  	}
  	jobStore := jobs.NewStore(cfg.JobsDir, cfg.JobsTTL)

  	// Background sweeper: drop job dirs past their TTL.
  	go func() {
  		t := time.NewTicker(time.Minute)
  		defer t.Stop()
  		for range t.C {
  			if n := jobStore.Sweep(time.Now()); n > 0 {
  				log.Printf("swept %d expired job(s)", n)
  			}
  		}
  	}()

  	// Plans 2 and 3 will extend New with presets, then schema.
  	srv := server.New(cfg, convert.New(cfg.FBCBin), web.FS, tpl, jobStore)
  	log.Printf("fb2cng-web listening on %s (fbc=%s, auth=%v, jobs=%s, ttl=%s)",
  		cfg.Addr, cfg.FBCBin, cfg.ForwardAuth, cfg.JobsDir, cfg.JobsTTL)
  	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
  		log.Fatal(err)
  	}
  }
  ```

- [ ] Run the full gate, expect PASS:
  ```
  go build ./...
  go vet ./...
  go test ./...
  ```
  Expected: clean build, clean vet, and `ok` for every package
  (`config`, `convert`, `jobs`, `web`, `server`).

- [ ] Commit:
  ```
  git add main.go
  git commit -m "feat(main): wire job store, parse templates, start TTL sweeper

  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

## Done criteria (Plan 1)

- `go test ./...` green across all packages.
- `GET /` renders the themed Convert page (nav tabs, preset=Defaults, format select, drop zone).
- A `POST /convert` batch creates a job, runs `fbc` per file under the sem cap, captures a
  multi-line log per input, and the `#convert-card` polls to a terminal state.
- Per-file failure is isolated: the failed file shows the first `ERR` line + full-log
  `<details>` + Retry; sibling files still succeed and offer downloads + Download-all zip.
- Retry re-runs only failed inputs with `use_broken_images: true` and reuses retained inputs.
- Job dirs are swept after `JOBS_TTL`.
- The `Server` struct and the 5-arg `New(cfg, runner, static, tpl, jobs)` are established with
  no presets/schema fields; Plans 2 and 3 append their field + `New` parameter when their
  package lands. `base.gohtml` + the `layout`/`ContentName` dispatch let them add
  `{{define "settings"}}`/`{{define "editor"}}` pages with no change here.
