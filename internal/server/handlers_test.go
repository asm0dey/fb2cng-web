package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
func (s stubRunner) ConvertLogged(context.Context, string, string, string, string, string) ([]string, error) {
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

func (c *capturingRunner) ConvertLogged(_ context.Context, in, format, cfgPath, dest, _ string) ([]string, error) {
	return c.Convert(context.Background(), in, format, cfgPath, dest)
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

// blockingRunner blocks in Convert until released, recording peak concurrency.
type blockingRunner struct {
	mu      sync.Mutex
	cur     int
	peak    int
	release chan struct{}
	entered chan struct{}
}

func (b *blockingRunner) DumpDefaults(context.Context) ([]byte, error) { return nil, nil }
func (b *blockingRunner) Convert(ctx context.Context, _, _, _, dest string) ([]string, error) {
	b.mu.Lock()
	b.cur++
	if b.cur > b.peak {
		b.peak = b.cur
	}
	b.mu.Unlock()
	b.entered <- struct{}{}
	<-b.release
	b.mu.Lock()
	b.cur--
	b.mu.Unlock()
	out := filepath.Join(dest, "x.epub")
	os.MkdirAll(dest, 0o755)
	os.WriteFile(out, []byte("x"), 0o644)
	return []string{out}, nil
}

func (b *blockingRunner) ConvertLogged(ctx context.Context, in, format, cfgPath, dest, _ string) ([]string, error) {
	return b.Convert(ctx, in, format, cfgPath, dest)
}

func TestConvertConcurrencyCap(t *testing.T) {
	const n = 5
	br := &blockingRunner{
		release: make(chan struct{}),
		entered: make(chan struct{}, n),
	}
	h := newTestServer(t, config.Config{MaxConcurrent: 2}, br)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, multipartConvert(t, "book.fb2", map[string]string{"format": "epub3"}))
		}()
	}
	// Wait until exactly MaxConcurrent (2) goroutines have entered Convert,
	// confirming the semaphore cap is saturated — deterministic, no sleep.
	for i := 0; i < 2; i++ {
		<-br.entered
	}
	close(br.release)
	wg.Wait()

	if br.peak > 2 {
		t.Fatalf("peak concurrency %d exceeded cap 2", br.peak)
	}
}
