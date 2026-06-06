# fb2cng Web Interface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A self-hosted web app that accepts dropped `fb2` / `fb2.zip` files, converts them to EPUB (and other fbc formats) by shelling out to the bundled `fbc` binary, and auto-downloads each result.

**Architecture:** A single Go HTTP service serves an embedded static frontend (vanilla JS + Pico CSS from CDN) and exposes `GET /defaults`, `GET /me`, and `POST /convert`. Each conversion runs `fbc` in an isolated temp dir, streams the result back, and cleans up. Conversion is stateless; a semaphore caps concurrent `fbc` processes. Optional forward-auth (Authelia) is enforced by middleware that trusts `Remote-*` headers only behind a reverse proxy.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, `embed`, Pico CSS v2 (CDN), Docker. fbc release: `v1.4.5`, asset `fbc-linux-amd64.zip`.

**Spec:** `docs/superpowers/specs/2026-06-06-fb2cng-web-interface-design.md`

---

## File Structure

```
go.mod / go.sum
main.go                              # wire config + runner + server, ListenAndServe
internal/config/config.go           # Config struct + FromEnv()
internal/config/config_test.go
internal/convert/config_yaml.go     # FormOptions + BuildConfig (merge form over raw YAML)
internal/convert/config_yaml_test.go
internal/convert/runner.go          # Runner interface + FBC (shells out to fbc)
internal/convert/runner_test.go     # uses testdata/fake-fbc.sh
internal/server/server.go           # Server struct, mux, semaphore, Handler()
internal/server/handlers.go         # handleDefaults, handleMe, handleConvert
internal/server/handlers_test.go    # stub runner
internal/server/auth.go             # ForwardAuth middleware
internal/server/auth_test.go
internal/server/static_test.go      # embedded frontend is served
internal/web/index.html             # page markup (Pico)
internal/web/app.js                 # drop area, settings, theme, auto-download
internal/web/embed.go               # //go:embed FS
internal/convert/smoke_test.go      # real-fbc end-to-end, gated (build tag)
testdata/sample.fb2
testdata/fake-fbc.sh
Dockerfile
.dockerignore
docker-compose.example.yml
README.md
```

Module path: `fb2cng-web`. Allowed formats everywhere: `epub2, epub3, kepub, kfx, azw8, pdf`; default `epub3`.

---

## Task 1: Project scaffold + config from env

**Files:**
- Create: `go.mod`, `main.go`, `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Init module**

Run:
```bash
cd /home/finkel/work_self/web-converter
go mod init fb2cng-web
```
Expected: creates `go.mod` with `module fb2cng-web` and `go 1.26`.

- [ ] **Step 2: Write the failing config test**

Create `internal/config/config_test.go`:
```go
package config

import (
	"reflect"
	"testing"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("FBC_BIN", "")
	t.Setenv("MAX_CONCURRENT", "")
	t.Setenv("AUTH_FORWARD_AUTH", "")
	t.Setenv("TRUSTED_PROXIES", "")

	got := FromEnv()
	want := Config{Addr: ":8080", FBCBin: "fbc", MaxConcurrent: 3, ForwardAuth: false, TrustedProxies: nil}
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

	got := FromEnv()
	if got.Addr != ":9000" || got.FBCBin != "/opt/fbc" || got.MaxConcurrent != 5 || !got.ForwardAuth {
		t.Fatalf("overrides not applied: %+v", got)
	}
	if !reflect.DeepEqual(got.TrustedProxies, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Fatalf("trusted proxies: %+v", got.TrustedProxies)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — `undefined: FromEnv` / `undefined: Config`.

- [ ] **Step 4: Implement config**

Create `internal/config/config.go`:
```go
// Package config loads app configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration.
type Config struct {
	Addr           string   // listen address, e.g. ":8080"
	FBCBin         string   // path to the fbc binary
	MaxConcurrent  int      // max concurrent fbc processes
	ForwardAuth    bool     // trust reverse-proxy Remote-* headers
	TrustedProxies []string // source IPs allowed to set Remote-* headers (empty = trust any source)
}

// FromEnv builds a Config from environment variables, applying defaults.
func FromEnv() Config {
	c := Config{
		Addr:          ":" + envOr("PORT", "8080"),
		FBCBin:        envOr("FBC_BIN", "fbc"),
		MaxConcurrent: atoiOr(os.Getenv("MAX_CONCURRENT"), 3),
		ForwardAuth:   os.Getenv("AUTH_FORWARD_AUTH") == "true",
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
```

- [ ] **Step 5: Add a minimal main so the module builds**

Create `main.go`:
```go
package main

import (
	"log"

	"fb2cng-web/internal/config"
)

func main() {
	cfg := config.FromEnv()
	log.Printf("fb2cng-web starting on %s (fbc=%s)", cfg.Addr, cfg.FBCBin)
	// Server wiring is added in Task 5.
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/config/... && go build ./...`
Expected: PASS, build succeeds.

- [ ] **Step 7: Commit**

```bash
git add go.mod main.go internal/config/
git commit -m "feat: project scaffold and env config"
```

---

## Task 2: YAML config builder (merge form options over raw YAML)

**Files:**
- Create: `internal/convert/config_yaml.go`, `internal/convert/config_yaml_test.go`

- [ ] **Step 1: Add yaml dependency**

Run: `go get gopkg.in/yaml.v3`
Expected: `gopkg.in/yaml.v3` added to `go.mod`.

- [ ] **Step 2: Write the failing test**

Create `internal/convert/config_yaml_test.go`:
```go
package convert

import (
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestBuildConfigEmpty(t *testing.T) {
	out, err := BuildConfig("", FormOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil config when nothing set, got %q", out)
	}
}

func TestBuildConfigFormOnly(t *testing.T) {
	out, err := BuildConfig("", FormOptions{
		TocType:        ptr("flat"),
		ImagesOptimize: ptr(false),
		JpegQuality:    ptr(80),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"version: 1", "toc_type: flat", "optimize: false", "jpeg_quality_level: 80"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestBuildConfigFormWinsOverRaw(t *testing.T) {
	raw := "version: 1\ndocument:\n  toc_type: normal\n  insert_soft_hyphen: false\n"
	out, err := BuildConfig(raw, FormOptions{TocType: ptr("old_kindle")})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "toc_type: old_kindle") {
		t.Errorf("form value should win, got:\n%s", s)
	}
	if !strings.Contains(s, "insert_soft_hyphen: false") {
		t.Errorf("unrelated raw key should be preserved, got:\n%s", s)
	}
}

func TestBuildConfigRawOnlyEnsuresVersion(t *testing.T) {
	out, err := BuildConfig("document:\n  toc_type: flat\n", FormOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "version: 1") {
		t.Errorf("version must be ensured, got:\n%s", out)
	}
}

func TestBuildConfigInvalidYAML(t *testing.T) {
	if _, err := BuildConfig("\tnot: [valid", FormOptions{}); err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/convert/...`
Expected: FAIL — `undefined: BuildConfig` / `undefined: FormOptions`.

- [ ] **Step 4: Implement the builder**

Create `internal/convert/config_yaml.go`:
```go
package convert

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// FormOptions are the common knobs the web form exposes. A nil pointer means
// "not set by the user" and is left to fbc's default / the raw YAML value.
type FormOptions struct {
	TocType          *string // document.toc_type
	ImagesOptimize   *bool   // document.images.optimize
	JpegQuality      *int    // document.images.jpeg_quality_level
	FootnotesMode    *string // document.footnotes.mode
	InsertSoftHyphen *bool   // document.insert_soft_hyphen
}

func (o FormOptions) empty() bool {
	return o.TocType == nil && o.ImagesOptimize == nil && o.JpegQuality == nil &&
		o.FootnotesMode == nil && o.InsertSoftHyphen == nil
}

// BuildConfig merges form options on top of the user's raw YAML and returns the
// effective config to pass to fbc via -c. Form values win on conflict. Returns
// (nil, nil) when neither raw YAML nor any form option is provided, so the
// caller sends no -c and fbc uses its embedded defaults.
func BuildConfig(rawYAML string, o FormOptions) ([]byte, error) {
	if rawYAML == "" && o.empty() {
		return nil, nil
	}

	root := map[string]any{}
	if rawYAML != "" {
		if err := yaml.Unmarshal([]byte(rawYAML), &root); err != nil {
			return nil, fmt.Errorf("parse config YAML: %w", err)
		}
	}
	if _, ok := root["version"]; !ok {
		root["version"] = 1
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

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal config YAML: %w", err)
	}
	return out, nil
}

// child returns the nested map at key, creating it if absent or not a map.
func child(m map[string]any, key string) map[string]any {
	if existing, ok := m[key].(map[string]any); ok {
		return existing
	}
	c := map[string]any{}
	m[key] = c
	return c
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/convert/...`
Expected: PASS (all 5 tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/convert/config_yaml.go internal/convert/config_yaml_test.go
git commit -m "feat: build effective fbc config by merging form over raw YAML"
```

---

## Task 3: fbc runner (shell out) + fake-fbc test harness

**Files:**
- Create: `internal/convert/runner.go`, `internal/convert/runner_test.go`, `testdata/fake-fbc.sh`, `testdata/sample.fb2`

- [ ] **Step 1: Create the fake fbc script**

Create `testdata/fake-fbc.sh` (emulates the real fbc CLI for tests):
```sh
#!/bin/sh
# Fake fbc for tests. Emulates: `dumpconfig --default` and
# `[-c cfg] convert --to FMT [--overwrite --nd] INPUT DEST`.
mode=""
for a in "$@"; do
  case "$a" in
    dumpconfig) mode=dump ;;
    convert) mode=convert ;;
  esac
done

if [ "$mode" = "dump" ]; then
  printf 'version: 1\ndocument:\n    toc_type: normal\n'
  exit 0
fi

if [ "$mode" = "convert" ]; then
  to=""
  positionals=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --to) to="$2"; shift 2; continue ;;
      -c) shift 2; continue ;;
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
  if echo "$input" | grep -q corrupt; then
    echo "fake-fbc: cannot parse $input" >&2
    exit 1
  fi
  mkdir -p "$dest"
  printf 'FAKE-%s' "$to" > "$dest/$base.$ext"
  if echo "$input" | grep -q multi; then
    printf 'FAKE2' > "$dest/${base}-2.$ext"
  fi
  exit 0
fi

echo "fake-fbc: unknown invocation: $*" >&2
exit 2
```

Run: `chmod +x testdata/fake-fbc.sh`

- [ ] **Step 2: Create a sample fb2 fixture**

Create `testdata/sample.fb2`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0">
<description><title-info>
<genre>sf</genre>
<author><first-name>Jane</first-name><last-name>Doe</last-name></author>
<book-title>Hello Book</book-title>
<lang>en</lang>
</title-info></description>
<body><section><title><p>Chapter 1</p></title><p>Hello world.</p></section></body>
</FictionBook>
```

- [ ] **Step 3: Write the failing runner test**

Create `internal/convert/runner_test.go`:
```go
package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeBin(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/fake-fbc.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fake fbc missing: %v", err)
	}
	return abs
}

func TestDumpDefaults(t *testing.T) {
	out, err := New(fakeBin(t)).DumpDefaults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "version: 1") {
		t.Fatalf("unexpected defaults: %s", out)
	}
}

func TestConvertSingleOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "book.fb2")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")

	outs, err := New(fakeBin(t)).Convert(context.Background(), in, "epub3", "", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || filepath.Base(outs[0]) != "book.epub" {
		t.Fatalf("unexpected outputs: %v", outs)
	}
}

func TestConvertMultiOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "multi.fb2")
	os.WriteFile(in, []byte("x"), 0o644)
	outs, err := New(fakeBin(t)).Convert(context.Background(), in, "epub3", "", filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 2 {
		t.Fatalf("expected 2 outputs, got %v", outs)
	}
}

func TestConvertError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "corrupt.fb2")
	os.WriteFile(in, []byte("x"), 0o644)
	_, err := New(fakeBin(t)).Convert(context.Background(), in, "epub3", "", filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Fatalf("expected fbc error with stderr, got %v", err)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/convert/... -run TestConvert`
Expected: FAIL — `undefined: New` / `DumpDefaults` / `Convert`.

- [ ] **Step 5: Implement the runner**

Create `internal/convert/runner.go`:
```go
package convert

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Runner converts fb2 inputs via fbc.
type Runner interface {
	// DumpDefaults returns fbc's embedded default configuration as YAML.
	DumpDefaults(ctx context.Context) ([]byte, error)
	// Convert runs fbc on inputPath into destDir and returns the produced
	// output file paths. configPath is optional ("" = use fbc defaults).
	Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error)
}

// FBC shells out to the fbc binary.
type FBC struct{ Bin string }

// New returns an FBC runner using the given binary path.
func New(bin string) *FBC { return &FBC{Bin: bin} }

func (f *FBC) DumpDefaults(ctx context.Context) ([]byte, error) {
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, f.Bin, "dumpconfig", "--default")
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("fbc dumpconfig: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func (f *FBC) Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	// -c is a GLOBAL flag and must come before the subcommand.
	args := []string{}
	if configPath != "" {
		args = append(args, "-c", configPath)
	}
	args = append(args, "convert", "--to", format, "--overwrite", "--nd", inputPath, destDir)

	var errb bytes.Buffer
	cmd := exec.CommandContext(ctx, f.Bin, args...)
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("conversion failed: %s", msg)
	}
	return collectOutputs(destDir)
}

// collectOutputs returns regular, non-hidden files under destDir.
func collectOutputs(destDir string) ([]string, error) {
	var outs []string
	err := filepath.WalkDir(destDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		outs = append(outs, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(outs) == 0 {
		return nil, fmt.Errorf("conversion produced no output")
	}
	sort.Strings(outs)
	return outs, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/convert/...`
Expected: PASS (runner + config_yaml tests).

- [ ] **Step 7: Commit**

```bash
git add internal/convert/runner.go internal/convert/runner_test.go testdata/
git commit -m "feat: fbc runner with fake-fbc test harness"
```

---

## Task 4: Forward-auth middleware

**Files:**
- Create: `internal/server/auth.go`, `internal/server/auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/auth_test.go`:
```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func TestAuthDisabledPassthrough(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	ForwardAuth(false, nil, okHandler()).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("disabled auth should pass, got %d", rec.Code)
	}
}

func TestAuthEnabledMissingHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	ForwardAuth(true, nil, okHandler()).ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("missing Remote-User should be 401, got %d", rec.Code)
	}
}

func TestAuthEnabledWithHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Remote-User", "alice")
	ForwardAuth(true, nil, okHandler()).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("valid user should pass, got %d", rec.Code)
	}
}

func TestAuthUntrustedProxyRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.9:5555" // not in trusted list
	req.Header.Set("Remote-User", "alice")
	ForwardAuth(true, []string{"10.0.0.1"}, okHandler()).ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("untrusted source must be 401 even with header, got %d", rec.Code)
	}
}

func TestAuthTrustedProxyAccepted(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("Remote-User", "alice")
	ForwardAuth(true, []string{"10.0.0.1"}, okHandler()).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("trusted source with header should pass, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/... -run TestAuth`
Expected: FAIL — `undefined: ForwardAuth`.

- [ ] **Step 3: Implement the middleware**

Create `internal/server/auth.go`:
```go
package server

import (
	"net"
	"net/http"
)

// remoteUserHeader is the identity header Authelia sets via the reverse proxy.
const remoteUserHeader = "Remote-User"

// ForwardAuth enforces reverse-proxy forward authentication.
//
// When enabled is false, requests pass through unchanged. When enabled:
//   - if trusted is non-empty, the request's source IP must be in it; otherwise
//     the request is rejected (so Remote-* headers can never be spoofed by a
//     client reaching the app directly).
//   - a non-empty Remote-User header is required; missing -> 401.
func ForwardAuth(enabled bool, trusted []string, next http.Handler) http.Handler {
	trustSet := map[string]bool{}
	for _, ip := range trusted {
		trustSet[ip] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			next.ServeHTTP(w, r)
			return
		}
		if len(trustSet) > 0 {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if !trustSet[host] {
				http.Error(w, "forbidden: untrusted source", http.StatusUnauthorized)
				return
			}
		}
		if r.Header.Get(remoteUserHeader) == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/... -run TestAuth`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/server/auth.go internal/server/auth_test.go
git commit -m "feat: optional forward-auth middleware with trusted-proxy gate"
```

---

## Task 5: Server skeleton, embedded frontend, /defaults and /me

**Files:**
- Create: `internal/web/embed.go`, `internal/web/index.html` (placeholder), `internal/web/app.js` (placeholder), `internal/server/server.go`, `internal/server/handlers.go`, `internal/server/handlers_test.go`, `internal/server/static_test.go`
- Modify: `main.go`

> The real frontend markup is written in Task 7. Here we create minimal placeholder files so `embed` compiles and the static route is testable.

- [ ] **Step 1: Create placeholder frontend + embed**

Create `internal/web/index.html`:
```html
<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>fb2 to epub</title></head>
<body><main><h1>fb2 to epub</h1></main></body></html>
```

Create `internal/web/app.js`:
```js
// Replaced with the real frontend in Task 7.
```

Create `internal/web/embed.go`:
```go
// Package web holds the embedded static frontend.
package web

import "embed"

//go:embed index.html app.js
var FS embed.FS
```

- [ ] **Step 2: Write failing handler + static tests**

Create `internal/server/handlers_test.go`:
```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/web"
)

// stubRunner implements convert.Runner for handler tests.
type stubRunner struct {
	defaults []byte
	outputs  []string
	err      error
}

func (s stubRunner) DumpDefaults(context.Context) ([]byte, error) { return s.defaults, s.err }
func (s stubRunner) Convert(context.Context, string, string, string, string) ([]string, error) {
	return s.outputs, s.err
}

func newTestServer(t *testing.T, cfg config.Config, r convert.Runner) http.Handler {
	t.Helper()
	return New(cfg, r, web.FS).Handler()
}

func TestDefaultsEndpoint(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 1}, stubRunner{defaults: []byte("version: 1\n")})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/defaults", nil))
	if rec.Code != 200 || rec.Body.String() != "version: 1\n" {
		t.Fatalf("defaults: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMeDisabled(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 1, ForwardAuth: false}, stubRunner{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", body)
	}
}

func TestMeEnabled(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 1, ForwardAuth: true}, stubRunner{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Remote-User", "alice")
	req.Header.Set("Remote-Name", "Alice Liddell")
	h.ServeHTTP(rec, req)
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["enabled"] != true || body["user"] != "alice" || body["name"] != "Alice Liddell" {
		t.Fatalf("unexpected /me body: %v", body)
	}
}
```

Create `internal/server/static_test.go`:
```go
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/web"
)

func TestServesIndex(t *testing.T) {
	h := New(config.Config{MaxConcurrent: 1}, stubRunner{}, web.FS).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "fb2 to epub") {
		t.Fatalf("index not served: code=%d", rec.Code)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/server/... -run 'TestDefaults|TestMe|TestServesIndex'`
Expected: FAIL — `undefined: New` / `Handler`.

- [ ] **Step 4: Implement server + defaults/me handlers**

Create `internal/server/server.go`:
```go
package server

import (
	"io/fs"
	"net/http"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
)

// Server wires configuration, the fbc runner, and the static frontend.
type Server struct {
	cfg    config.Config
	runner convert.Runner
	static fs.FS
	sem    chan struct{}
}

// New constructs a Server. static is the embedded frontend filesystem.
func New(cfg config.Config, runner convert.Runner, static fs.FS) *Server {
	n := cfg.MaxConcurrent
	if n < 1 {
		n = 1
	}
	return &Server{cfg: cfg, runner: runner, static: static, sem: make(chan struct{}, n)}
}

// Handler returns the full HTTP handler with routes and auth applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /defaults", s.handleDefaults)
	mux.HandleFunc("GET /me", s.handleMe)
	mux.HandleFunc("POST /convert", s.handleConvert)
	mux.Handle("/", http.FileServer(http.FS(s.static)))
	return ForwardAuth(s.cfg.ForwardAuth, s.cfg.TrustedProxies, mux)
}
```

Create `internal/server/handlers.go`:
```go
package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleDefaults(w http.ResponseWriter, r *http.Request) {
	out, err := s.runner.DumpDefaults(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Write(out)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"enabled": s.cfg.ForwardAuth}
	if s.cfg.ForwardAuth {
		resp["user"] = r.Header.Get("Remote-User")
		resp["name"] = r.Header.Get("Remote-Name")
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/...`
Expected: PASS (auth + defaults + me + static). `handleConvert` is referenced but implemented in Task 6 — add a temporary stub so it compiles:

Append to `internal/server/handlers.go`:
```go
func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
```

- [ ] **Step 6: Wire main.go**

Replace `main.go`:
```go
package main

import (
	"log"
	"net/http"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/server"
	"fb2cng-web/internal/web"
)

func main() {
	cfg := config.FromEnv()
	srv := server.New(cfg, convert.New(cfg.FBCBin), web.FS)
	log.Printf("fb2cng-web listening on %s (fbc=%s, auth=%v)", cfg.Addr, cfg.FBCBin, cfg.ForwardAuth)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 7: Verify build + tests**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/web/ internal/server/server.go internal/server/handlers.go internal/server/handlers_test.go internal/server/static_test.go main.go
git commit -m "feat: server skeleton with /defaults, /me, static frontend"
```

---

## Task 6: /convert handler (multipart, config merge, concurrency, streaming)

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: Write failing convert tests**

Add to `internal/server/handlers_test.go`:
```go
func multipartConvert(t *testing.T, filename string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write([]byte("<FictionBook/>"))
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/convert", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestConvertSingleStreamsFile(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "Book.epub")
	os.WriteFile(outFile, []byte("EPUBDATA"), 0o644)

	h := newTestServer(t, config.Config{MaxConcurrent: 2}, stubRunner{outputs: []string{outFile}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3"}))

	if rec.Code != 200 || rec.Body.String() != "EPUBDATA" {
		t.Fatalf("expected streamed file, code=%d body=%q", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "Book.epub") {
		t.Fatalf("missing filename in Content-Disposition: %q", cd)
	}
}

func TestConvertMultiZips(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "A.epub")
	b := filepath.Join(dir, "B.epub")
	os.WriteFile(a, []byte("AAAA"), 0o644)
	os.WriteFile(b, []byte("BBBB"), 0o644)

	h := newTestServer(t, config.Config{MaxConcurrent: 2}, stubRunner{outputs: []string{a, b}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "archive.fb2.zip", map[string]string{"format": "epub3"}))

	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("expected zip, got %q", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil || len(zr.File) != 2 {
		t.Fatalf("expected 2-entry zip, err=%v files=%d", err, len(zr.File))
	}
}

func TestConvertBadFormat(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 1}, stubRunner{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "mobi"}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad format should be 422, got %d", rec.Code)
	}
}

func TestConvertRunnerError(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 1}, stubRunner{err: errors.New("conversion failed: boom")})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3"}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("runner error should be 422, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("error message not surfaced: %s", rec.Body.String())
	}
}
```

Add these imports to the top `import` block of `internal/server/handlers_test.go`:
```go
	"archive/zip"
	"bytes"
	"errors"
	"mime/multipart"
	"os"
	"path/filepath"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/... -run TestConvert`
Expected: FAIL — handler returns 501 (not implemented).

- [ ] **Step 3: Implement the convert handler**

Replace the stub `handleConvert` in `internal/server/handlers.go` with the following, and add the imports listed after it:
```go
// allowedFormats are the output types fbc supports.
var allowedFormats = map[string]bool{
	"epub2": true, "epub3": true, "kepub": true, "kfx": true, "azw8": true, "pdf": true,
}

func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(128 << 20); err != nil { // 128 MiB in memory/disk
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

	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	inName := filepath.Base(hdr.Filename)
	lower := strings.ToLower(inName)
	if !strings.HasSuffix(lower, ".fb2") && !strings.HasSuffix(lower, ".zip") {
		http.Error(w, "only .fb2 and .zip files are accepted", http.StatusUnprocessableEntity)
		return
	}

	// Build effective config from form fields (form wins over raw YAML).
	cfgBytes, err := convert.BuildConfig(r.FormValue("raw_yaml"), formOptions(r))
	if err != nil {
		http.Error(w, "invalid config: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	work, err := os.MkdirTemp("", "fb2conv-*")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(work)

	inPath := filepath.Join(work, inName)
	dst, err := os.Create(inPath)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	dst.Close()

	cfgPath := ""
	if cfgBytes != nil {
		cfgPath = filepath.Join(work, "config.yaml")
		if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
	}

	destDir := filepath.Join(work, "out")

	// Concurrency guard: cap simultaneous fbc processes.
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-r.Context().Done():
		http.Error(w, "client gone", http.StatusRequestTimeout)
		return
	}

	outs, err := s.runner.Convert(r.Context(), inPath, format, cfgPath, destDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	if len(outs) == 1 {
		streamFile(w, outs[0])
		return
	}
	streamZip(w, strings.TrimSuffix(inName, filepath.Ext(inName))+".zip", outs)
}

// formOptions reads the common-knob form fields. Absent/empty fields stay nil.
func formOptions(r *http.Request) convert.FormOptions {
	var o convert.FormOptions
	if v := r.FormValue("toc_type"); v != "" {
		o.TocType = &v
	}
	if v := r.FormValue("footnotes_mode"); v != "" {
		o.FootnotesMode = &v
	}
	if v := r.FormValue("images_optimize"); v != "" {
		b := v == "true"
		o.ImagesOptimize = &b
	}
	if v := r.FormValue("insert_soft_hyphen"); v != "" {
		b := v == "true"
		o.InsertSoftHyphen = &b
	}
	if v := r.FormValue("jpeg_quality"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			o.JpegQuality = &n
		}
	}
	return o
}

func streamFile(w http.ResponseWriter, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	name := filepath.Base(path)
	w.Header().Set("Content-Type", contentTypeFor(name))
	w.Header().Set("Content-Disposition", contentDisposition(name))
	io.Copy(w, f)
}

func streamZip(w http.ResponseWriter, zipName string, paths []string) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(zipName))
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return
		}
		hw, err := zw.Create(filepath.Base(p))
		if err != nil {
			f.Close()
			return
		}
		io.Copy(hw, f)
		f.Close()
	}
}

func contentTypeFor(name string) string {
	if strings.HasSuffix(name, ".epub") {
		return "application/epub+zip"
	}
	if strings.HasSuffix(name, ".pdf") {
		return "application/pdf"
	}
	return "application/octet-stream"
}

// contentDisposition sets a safe attachment filename (RFC 5987 for UTF-8).
func contentDisposition(name string) string {
	return "attachment; filename*=UTF-8''" + url.PathEscape(name)
}
```

Update the `import` block at the top of `internal/server/handlers.go` to:
```go
import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fb2cng-web/internal/convert"
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/...`
Expected: PASS (all server tests).

- [ ] **Step 5: Verify the concurrency cap (add a focused test)**

Add to `internal/server/handlers_test.go`:
```go
// blockingRunner blocks in Convert until released, recording peak concurrency.
type blockingRunner struct {
	mu      sync.Mutex
	cur     int
	peak    int
	release chan struct{}
}

func (b *blockingRunner) DumpDefaults(context.Context) ([]byte, error) { return nil, nil }
func (b *blockingRunner) Convert(ctx context.Context, _, _, _, dest string) ([]string, error) {
	b.mu.Lock()
	b.cur++
	if b.cur > b.peak {
		b.peak = b.cur
	}
	b.mu.Unlock()
	<-b.release
	b.mu.Lock()
	b.cur--
	b.mu.Unlock()
	out := filepath.Join(dest, "x.epub")
	os.MkdirAll(dest, 0o755)
	os.WriteFile(out, []byte("x"), 0o644)
	return []string{out}, nil
}

func TestConvertConcurrencyCap(t *testing.T) {
	br := &blockingRunner{release: make(chan struct{})}
	h := newTestServer(t, config.Config{MaxConcurrent: 2}, br)

	const n = 5
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3"}))
		}()
	}
	time.Sleep(100 * time.Millisecond) // let goroutines reach the semaphore
	close(br.release)
	wg.Wait()

	if br.peak > 2 {
		t.Fatalf("peak concurrency %d exceeded cap 2", br.peak)
	}
}
```
Add `"sync"` and `"time"` to the test imports.

- [ ] **Step 6: Run the concurrency test**

Run: `go test ./internal/server/... -run TestConvertConcurrencyCap -race`
Expected: PASS, peak ≤ 2.

- [ ] **Step 7: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go
git commit -m "feat: /convert handler with config merge, concurrency cap, zip-on-multi"
```

---

## Task 7: Real frontend (drop area, settings, theme, auto-download)

**Files:**
- Modify: `internal/web/index.html`, `internal/web/app.js`

> Pure browser code. No automated unit tests (the served-page test from Task 5 still covers embedding). Verify via the manual checklist in Step 4.

- [ ] **Step 1: Write index.html**

Replace `internal/web/index.html`:
```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>fb2 → epub</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.min.css">
  <script>
    // Apply theme before first paint to avoid flash.
    (function () {
      var t = localStorage.getItem('theme') || 'system';
      if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
    })();
  </script>
  <style>
    #drop { border: 2px dashed var(--pico-muted-border-color); border-radius: var(--pico-border-radius);
      padding: 2rem; text-align: center; cursor: pointer; }
    #drop.drag { border-color: var(--pico-primary); background: var(--pico-card-background-color); }
    .row { display: flex; gap: 1rem; align-items: center; justify-content: space-between; flex-wrap: wrap; }
    .status-ok { color: var(--pico-ins-color); }
    .status-err { color: var(--pico-del-color); }
    #results li { list-style: none; }
    #results { padding-left: 0; }
  </style>
</head>
<body>
  <main class="container">
    <div class="row">
      <h1>fb2 → epub</h1>
      <div class="row">
        <span id="user" class="secondary"></span>
        <select id="theme" aria-label="Theme" style="width:auto">
          <option value="system">System</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </div>
    </div>

    <div id="drop">
      <p><strong>Drop fb2 / fb2.zip files here</strong></p>
      <p class="secondary">or tap to choose files</p>
      <input id="file" type="file" accept=".fb2,.zip" multiple hidden>
    </div>

    <details>
      <summary>Settings <span id="fmt-label" class="secondary"></span></summary>

      <label>Output format
        <select id="format">
          <option value="epub3" selected>EPUB3</option>
          <option value="epub2">EPUB2</option>
          <option value="kepub">KEPUB</option>
          <option value="kfx">KFX</option>
          <option value="azw8">AZW8</option>
          <option value="pdf">PDF</option>
        </select>
      </label>

      <label>Table of contents
        <select id="toc_type">
          <option value="">(default)</option>
          <option value="normal">normal</option>
          <option value="old_kindle">old_kindle</option>
          <option value="flat">flat</option>
        </select>
      </label>

      <label>Footnotes
        <select id="footnotes_mode">
          <option value="">(default)</option>
          <option value="default">default</option>
          <option value="float">float</option>
          <option value="floatRenumbered">floatRenumbered</option>
        </select>
      </label>

      <label><input id="images_optimize" type="checkbox"> Optimize images</label>
      <label>JPEG quality <input id="jpeg_quality" type="number" min="1" max="100" placeholder="default"></label>
      <label><input id="insert_soft_hyphen" type="checkbox"> Insert soft hyphens</label>

      <hr>
      <label>Upload settings file (.yaml)
        <input id="yaml_upload" type="file" accept=".yaml,.yml">
      </label>
      <label>Raw configuration (YAML) — overrides above where keys overlap
        <textarea id="raw_yaml" rows="10" spellcheck="false" placeholder="Loading defaults…"></textarea>
      </label>
    </details>

    <h2>Results</h2>
    <ul id="results"></ul>
  </main>
  <script src="app.js"></script>
</body>
</html>
```

- [ ] **Step 2: Write app.js**

Replace `internal/web/app.js`:
```js
'use strict';

const $ = (id) => document.getElementById(id);

// ---- Theme ----
const themeSel = $('theme');
themeSel.value = localStorage.getItem('theme') || 'system';
themeSel.addEventListener('change', () => {
  const v = themeSel.value;
  localStorage.setItem('theme', v);
  if (v === 'system') document.documentElement.removeAttribute('data-theme');
  else document.documentElement.setAttribute('data-theme', v);
});

// ---- Identity ----
fetch('/me')
  .then((r) => r.json())
  .then((m) => { if (m.enabled && (m.name || m.user)) $('user').textContent = '👤 ' + (m.name || m.user); })
  .catch(() => {});

// ---- Defaults ----
fetch('/defaults')
  .then((r) => (r.ok ? r.text() : Promise.reject()))
  .then((yaml) => { $('raw_yaml').value = yaml; })
  .catch(() => { $('raw_yaml').placeholder = 'Could not load defaults'; });

// Keep the settings summary showing the chosen format.
const fmt = $('format');
const fmtLabel = $('fmt-label');
const syncFmt = () => { fmtLabel.textContent = '(' + fmt.value + ')'; };
fmt.addEventListener('change', syncFmt);
syncFmt();

// Upload a settings YAML into the raw editor.
$('yaml_upload').addEventListener('change', (e) => {
  const f = e.target.files[0];
  if (!f) return;
  f.text().then((t) => { $('raw_yaml').value = t; });
});

// ---- Drop area ----
const drop = $('drop');
const fileInput = $('file');
drop.addEventListener('click', () => fileInput.click());
fileInput.addEventListener('change', () => handleFiles(fileInput.files));
['dragenter', 'dragover'].forEach((ev) =>
  drop.addEventListener(ev, (e) => { e.preventDefault(); drop.classList.add('drag'); }));
['dragleave', 'drop'].forEach((ev) =>
  drop.addEventListener(ev, (e) => { e.preventDefault(); drop.classList.remove('drag'); }));
drop.addEventListener('drop', (e) => handleFiles(e.dataTransfer.files));

function optionalField(form, key, el, kind) {
  let v = '';
  if (kind === 'check') v = el.checked ? 'true' : '';
  else v = el.value;
  if (v !== '') form.append(key, v);
}

function buildForm(file) {
  const form = new FormData();
  form.append('file', file);
  form.append('format', fmt.value);
  optionalField(form, 'toc_type', $('toc_type'));
  optionalField(form, 'footnotes_mode', $('footnotes_mode'));
  optionalField(form, 'jpeg_quality', $('jpeg_quality'));
  optionalField(form, 'images_optimize', $('images_optimize'), 'check');
  optionalField(form, 'insert_soft_hyphen', $('insert_soft_hyphen'), 'check');
  const raw = $('raw_yaml').value.trim();
  if (raw) form.append('raw_yaml', raw);
  return form;
}

function handleFiles(fileList) {
  for (const file of fileList) convertOne(file);
}

async function convertOne(file) {
  const li = document.createElement('li');
  li.textContent = `${file.name} — converting…`;
  $('results').prepend(li);
  try {
    const res = await fetch('/convert', { method: 'POST', body: buildForm(file) });
    if (!res.ok) {
      const msg = await res.text();
      li.innerHTML = `${file.name} — <span class="status-err">failed</span>: ${escapeHtml(msg.trim())}`;
      return;
    }
    const blob = await res.blob();
    const name = filenameFromDisposition(res.headers.get('Content-Disposition')) || (file.name + '.out');
    triggerDownload(blob, name);
    li.innerHTML = `${file.name} — <span class="status-ok">✓ downloaded ${escapeHtml(name)}</span>`;
  } catch (err) {
    li.innerHTML = `${file.name} — <span class="status-err">error</span>: ${escapeHtml(String(err))}`;
  }
}

function triggerDownload(blob, name) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}

function filenameFromDisposition(cd) {
  if (!cd) return null;
  let m = /filename\*=UTF-8''([^;]+)/i.exec(cd);
  if (m) return decodeURIComponent(m[1]);
  m = /filename="?([^"]+)"?/i.exec(cd);
  return m ? m[1] : null;
}

function escapeHtml(s) {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
```

- [ ] **Step 3: Manual verification with fake fbc**

Run:
```bash
FBC_BIN="$PWD/testdata/fake-fbc.sh" PORT=8080 go run .
```
Then in a browser at `http://localhost:8080`:
- [ ] Page loads with Pico styling; theme selector switches light/dark and persists on reload.
- [ ] Settings is collapsed by default; raw YAML textarea is pre-filled with `version: 1` defaults.
- [ ] Dropping `testdata/sample.fb2` triggers an automatic download of a file named `sample.epub` and a green ✓ row.
- [ ] Dropping a file whose name contains `corrupt` (rename a copy) shows a red failure row with the stderr message.
- [ ] Resize to a narrow viewport: layout stays single-column and usable.

Stop the server with Ctrl-C.

- [ ] **Step 4: Update and run the served-page test**

In `internal/server/static_test.go`, change the asserted substring from `"fb2 to epub"` to `"fb2 → epub"` to match the real page.

Run: `go test ./internal/server/... -run TestServesIndex`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/web/index.html internal/web/app.js internal/server/static_test.go
git commit -m "feat: frontend with drop area, settings, theme toggle, auto-download"
```

---

## Task 8: Docker image, compose example, README with proxy recipes

**Files:**
- Create: `Dockerfile`, `.dockerignore`, `docker-compose.example.yml`, `README.md`

- [ ] **Step 1: Write the Dockerfile**

Create `Dockerfile`:
```dockerfile
# syntax=docker/dockerfile:1

# --- Stage 1: fetch the fbc binary ---
FROM alpine:3.20 AS fbc
ARG FBC_VERSION=v1.4.5
ARG FBC_ASSET=fbc-linux-amd64.zip
RUN apk add --no-cache curl unzip \
 && curl -fsSL -o /tmp/fbc.zip \
      "https://github.com/rupor-github/fb2cng/releases/download/${FBC_VERSION}/${FBC_ASSET}" \
 && unzip -o /tmp/fbc.zip -d /opt \
 && chmod +x /opt/fbc

# --- Stage 2: build the Go server ---
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fb2cng-web .

# --- Stage 3: runtime ---
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080
EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
```

Create `.dockerignore`:
```
.git
.superpowers
docs
*.md
testdata
```

- [ ] **Step 2: Build the image**

Run: `docker build -t fb2cng-web .`
Expected: build succeeds; final image contains `/usr/local/bin/fbc` and the server.

- [ ] **Step 3: Smoke-run the container**

Run:
```bash
docker run --rm -p 8080:8080 fb2cng-web &
sleep 2
curl -fsS http://localhost:8080/defaults | head -1
docker stop $(docker ps -q --filter ancestor=fb2cng-web)
```
Expected: `/defaults` returns `version: 1` (this exercises the real bundled fbc).

- [ ] **Step 4: Write docker-compose example (app + Authelia + Caddy)**

Create `docker-compose.example.yml`:
```yaml
# Optional turnkey deployment: fb2cng-web behind Caddy + Authelia forward-auth.
# The app is NOT published directly — only Caddy is. fb2cng-web trusts Remote-*
# headers; AUTH_FORWARD_AUTH gates access. Adjust domains/secrets before use.
services:
  fb2cng-web:
    build: .
    environment:
      AUTH_FORWARD_AUTH: "true"
      TRUSTED_PROXIES: ""   # optional: set to caddy's container IP for defense-in-depth
    expose:
      - "8080"
    # No "ports:" — never expose the app directly when auth is on.

  authelia:
    image: authelia/authelia:latest
    volumes:
      - ./authelia:/config
    expose:
      - "9091"

  caddy:
    image: caddy:2
    ports:
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
    depends_on:
      - fb2cng-web
      - authelia
```

- [ ] **Step 5: Write README with proxy recipes**

Create `README.md` covering:
```markdown
# fb2cng-web

Web interface for [fb2cng](https://github.com/rupor-github/fb2cng): drop `fb2` / `fb2.zip`
files, get EPUB (or kepub/kfx/azw8/pdf) back automatically.

## Run

    docker build -t fb2cng-web .
    docker run --rm -p 8080:8080 fb2cng-web

Open http://localhost:8080. Defaults work out of the box; expand **Settings** to change
format, ToC, images, footnotes, or paste/upload a full fbc YAML config.

## Configuration (env)

| Var | Default | Meaning |
|-----|---------|---------|
| `PORT` | `8080` | listen port |
| `FBC_BIN` | `fbc` | path to the fbc binary |
| `MAX_CONCURRENT` | `3` | max simultaneous conversions |
| `AUTH_FORWARD_AUTH` | `false` | trust reverse-proxy `Remote-*` headers |
| `TRUSTED_PROXIES` | (empty) | comma-separated source IPs allowed to set `Remote-*` |

## Optional authentication (Authelia forward-auth)

The app has no built-in login. To require auth, run it behind a reverse proxy that
delegates to **Authelia**, with `AUTH_FORWARD_AUTH=true`.

> **Security:** when auth is on, never expose the app port directly. Publish only the
> proxy and keep the app on an internal network. Optionally set `TRUSTED_PROXIES` so the
> app ignores `Remote-*` headers from any other source.

Authelia forward-auth endpoint: `/api/authz/forward-auth`. Copy headers
`Remote-User`, `Remote-Groups`, `Remote-Email`, `Remote-Name` to the app.
```

Also include, in the README, these three proxy snippets verbatim:

Caddy (`Caddyfile`):
```caddyfile
fb2.example.com {
    forward_auth authelia:9091 {
        uri /api/authz/forward-auth
        copy_headers Remote-User Remote-Groups Remote-Email Remote-Name
    }
    reverse_proxy fb2cng-web:8080
}
```

Traefik (dynamic config):
```yaml
http:
  middlewares:
    authelia:
      forwardAuth:
        address: "http://authelia:9091/api/authz/forward-auth"
        authResponseHeaders:
          - "Remote-User"
          - "Remote-Groups"
          - "Remote-Email"
          - "Remote-Name"
  routers:
    fb2:
      rule: "Host(`fb2.example.com`)"
      middlewares: ["authelia"]
      service: fb2cng-web
  services:
    fb2cng-web:
      loadBalancer:
        servers:
          - url: "http://fb2cng-web:8080"
```

Nginx Proxy Manager (Proxy Host → Advanced tab):
```nginx
location /authelia {
    internal;
    proxy_pass http://authelia:9091/api/authz/auth-request;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URL $scheme://$http_host$request_uri;
    proxy_set_header X-Original-Method $request_method;
    proxy_set_header X-Forwarded-For $remote_addr;
}
location / {
    auth_request /authelia;
    auth_request_set $user  $upstream_http_remote_user;
    auth_request_set $groups $upstream_http_remote_groups;
    auth_request_set $name  $upstream_http_remote_name;
    auth_request_set $email $upstream_http_remote_email;
    proxy_set_header Remote-User   $user;
    proxy_set_header Remote-Groups $groups;
    proxy_set_header Remote-Name   $name;
    proxy_set_header Remote-Email  $email;
    error_page 401 =302 https://auth.example.com/?rd=$scheme://$http_host$request_uri;
    proxy_pass http://fb2cng-web:8080;
}
```

- [ ] **Step 6: Commit**

```bash
git add Dockerfile .dockerignore docker-compose.example.yml README.md
git commit -m "feat: Docker image, compose example, README with Authelia proxy recipes"
```

---

## Task 9: Real-fbc end-to-end smoke test (gated)

**Files:**
- Create: `internal/convert/smoke_test.go`

> This test runs the real `fbc`. It is gated by a build tag so the normal suite
> (which has no fbc) stays green. Run it explicitly after downloading fbc.

- [ ] **Step 1: Write the gated smoke test**

Create `internal/convert/smoke_test.go`:
```go
//go:build smoke

package convert

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Requires the real fbc binary. Set FBC_BIN, e.g.:
//   FBC_BIN=$PWD/bin/fbc go test -tags smoke ./internal/convert/ -run TestSmoke
func TestSmokeRealConversion(t *testing.T) {
	bin := os.Getenv("FBC_BIN")
	if bin == "" {
		t.Skip("set FBC_BIN to the real fbc binary to run the smoke test")
	}
	dir := t.TempDir()
	src, err := filepath.Abs("../../testdata/sample.fb2")
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "sample.fb2")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(in, data, 0o644)

	cfg, err := BuildConfig("", FormOptions{TocType: strptr("flat")})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, cfg, 0o644)

	outs, err := New(bin).Convert(context.Background(), in, "epub3", cfgPath, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 {
		t.Fatalf("expected 1 output, got %v", outs)
	}
	// A valid epub is a zip whose first entry is "mimetype".
	zr, err := zip.OpenReader(outs[0])
	if err != nil {
		t.Fatalf("output is not a valid zip/epub: %v", err)
	}
	defer zr.Close()
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatalf("epub missing mimetype entry")
	}
	rc, _ := zr.File[0].Open()
	mt, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(mt, []byte("application/epub+zip")) {
		t.Fatalf("unexpected mimetype: %q", mt)
	}
}

func strptr(s string) *string { return &s }
```

Add `"io"` to the import block.

- [ ] **Step 2: Download fbc and run the smoke test**

Run:
```bash
mkdir -p bin && cd bin \
 && curl -fsSL -o fbc.zip https://github.com/rupor-github/fb2cng/releases/download/v1.4.5/fbc-linux-amd64.zip \
 && unzip -o fbc.zip && chmod +x fbc && cd ..
FBC_BIN="$PWD/bin/fbc" go test -tags smoke ./internal/convert/ -run TestSmoke -v
```
Expected: PASS — output is a valid epub with a `mimetype` entry of `application/epub+zip`.

- [ ] **Step 3: Ignore the local fbc binary**

Append to `.gitignore`:
```
/bin/
```

- [ ] **Step 4: Commit**

```bash
git add internal/convert/smoke_test.go .gitignore
git commit -m "test: gated real-fbc end-to-end smoke test"
```

---

## Task 10: Final verification

- [ ] **Step 1: Full unit suite (no fbc needed)**

Run: `go test ./... -race`
Expected: PASS across config, convert, server.

- [ ] **Step 2: Vet and build**

Run: `go vet ./... && go build ./...`
Expected: no issues.

- [ ] **Step 3: Container end-to-end**

Run:
```bash
docker build -t fb2cng-web . \
 && docker run --rm -d -p 8080:8080 --name fb2test fb2cng-web \
 && sleep 2 \
 && curl -fsS -F "file=@testdata/sample.fb2" -F "format=epub3" http://localhost:8080/convert -o /tmp/out.epub \
 && unzip -l /tmp/out.epub \
 && docker stop fb2test
```
Expected: `/tmp/out.epub` is a valid epub listing (contains `mimetype`).

- [ ] **Step 4: Commit any fixes and finish**

```bash
git add -A && git commit -m "chore: final verification fixes" || true
```

---

## Notes & assumptions

- **`-c` is global, before the subcommand** — confirmed against fbc v1.4.5.
- **Partial configs need `version: 1`** — the builder always ensures it; confirmed fbc merges partials over its embedded defaults.
- **Output filename is template-derived** — handler reads the dest dir rather than assuming a name; a multi-book archive yields multiple files, returned as a zip.
- **Browser multi-download prompt** — dropping many files triggers many automatic downloads; browsers may ask to "allow multiple downloads." Acceptable for self-hosted use (per spec).
- **fbc logs to stderr**; non-zero exit + stderr is surfaced as the per-file error.
- **Image platform** — Dockerfile pins linux/amd64 fbc. For arm64 hosts, override `--build-arg FBC_ASSET=fbc-linux-arm64.zip`.
