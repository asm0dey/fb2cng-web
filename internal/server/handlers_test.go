package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/jobs"
	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/web"
)

// testJobsDir is set by newTestServer so loadStatus can read status.json directly.
var testJobsDir string

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
	tpl, err := web.Templates()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JobsDir == "" {
		cfg.JobsDir = t.TempDir()
	}
	testJobsDir = cfg.JobsDir
	store := jobs.NewStore(cfg.JobsDir, time.Hour)
	presetStore := presets.NewStore(t.TempDir())
	return New(cfg, r, web.FS, tpl, store, presetStore).Handler()
}

// multipartConvert builds a POST /convert request with one uploaded file and
// the given form fields.
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

// countingRunner is a convert.Runner used to exercise the retry concurrency
// fix: it counts ConvertLogged calls per input (so tests can assert an input
// was converted exactly once per intended attempt, catching duplicate
// conversions from an unguarded double retry), and mimics fake-fbc.sh's rule
// that any input whose basename contains "corrupt" fails unless the config
// passed via -c contains "use_broken_images: true".
type countingRunner struct {
	mu     sync.Mutex
	counts map[string]int
	delay  time.Duration // artificial per-call delay to widen concurrency windows
}

func newCountingRunner(delay time.Duration) *countingRunner {
	return &countingRunner{counts: map[string]int{}, delay: delay}
}

func (c *countingRunner) callCount(input string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[input]
}

func (c *countingRunner) DumpDefaults(context.Context) ([]byte, error) {
	return []byte("version: 1\n"), nil
}

func (c *countingRunner) Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error) {
	outs, _, err := c.run(inputPath, format, configPath, destDir)
	return outs, err
}

func (c *countingRunner) ConvertLogged(ctx context.Context, inputPath, format, configPath, destDir, logPath string) ([]string, error) {
	outs, logLines, err := c.run(inputPath, format, configPath, destDir)
	_ = os.WriteFile(logPath, []byte(logLines), 0o644)
	return outs, err
}

func (c *countingRunner) run(inputPath, format, configPath, destDir string) ([]string, string, error) {
	base := filepath.Base(inputPath)
	c.mu.Lock()
	c.counts[base]++
	n := c.counts[base]
	c.mu.Unlock()

	if c.delay > 0 {
		time.Sleep(c.delay)
	}

	broken := false
	if configPath != "" {
		b, _ := os.ReadFile(configPath)
		if strings.Contains(string(b), "use_broken_images: true") {
			broken = true
		}
	}

	log := fmt.Sprintf("INFO: opening %s\n", inputPath)
	if strings.Contains(base, "corrupt") && !broken {
		log += fmt.Sprintf("ERR: cannot parse %s: broken image\n", inputPath)
		return nil, log, fmt.Errorf("fake failure #%d for %s", n, base)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, log, err
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	outPath := filepath.Join(destDir, stem+"."+format)
	if err := os.WriteFile(outPath, []byte("FAKE-"+format), 0o644); err != nil {
		return nil, log, err
	}
	log += fmt.Sprintf("INFO: wrote %s\n", filepath.Base(outPath))
	return []string{outPath}, log, nil
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

// TestConvertDedupesCollidingUploadBasenames is the regression test for bean
// slb6: two uploaded files whose filepath.Base collides (e.g. two "book.fb2")
// must not overwrite each other's persisted input nor share a status.json
// row. Pre-fix, the second os.Create(InputPath) clobbers the first upload's
// bytes and both StatePending rows share Input "book.fb2"; updateFile then
// matches the FIRST row on every write and never touches the second, so it
// stays StatePending forever and the batch never goes Done.
func TestConvertDedupesCollidingUploadBasenames(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for i := 0; i < 2; i++ {
		fw, _ := mw.CreateFormFile("file", "book.fb2")
		fw.Write([]byte("<FictionBook/>"))
	}
	mw.WriteField("format", "epub3")
	mw.WriteField("preset", "defaults")
	mw.Close()
	req := httptest.NewRequest("POST", "/convert", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("convert POST code=%d body=%q", rec.Code, rec.Body.String())
	}
	id := extractJobID(t, rec.Body.String())

	st := waitDone(t, h, id)
	if len(st.Files) != 2 {
		t.Fatalf("expected 2 distinct file rows for colliding basenames, got %d: %+v", len(st.Files), st.Files)
	}
	seen := map[string]bool{}
	for _, f := range st.Files {
		if seen[f.Input] {
			t.Fatalf("duplicate FileResult.Input %q — colliding uploads must be deduped at upload time", f.Input)
		}
		seen[f.Input] = true
		if f.State != jobs.StateDone {
			t.Errorf("input %q: expected done, got %s (err=%q)", f.Input, f.State, f.Err)
		}
		if len(f.Outputs) != 1 {
			t.Errorf("input %q: expected 1 output, got %+v", f.Input, f.Outputs)
		}
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

// TestRetryConcurrentDedupesAndSurvivesInFlightWorker is the regression test
// for the lost-update / double-conversion race in handleRetry: the failed->
// pending reset must happen under s.mu (like updateFile) so that (a) a
// concurrent in-flight background worker writing status.json for other files
// never gets its update clobbered, and (b) firing retry multiple times
// concurrently converts each failed input exactly once, not once per request.
//
// The runner's artificial delay keeps the original job's background worker
// busy (still writing status.json for later files) while a burst of
// concurrent retry requests race in for the already-failed inputs. Run with
// `-race -count=10`: pre-fix, handleRetry's unlocked Load/Save of status.json
// races with updateFile's s.mu-guarded Load/Save on the same file.
func TestRetryConcurrentDedupesAndSurvivesInFlightWorker(t *testing.T) {
	runner := newCountingRunner(15 * time.Millisecond)
	h := newTestServer(t, config.Config{MaxConcurrent: 4}, runner)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, name := range []string{"corrupt1.fb2", "corrupt2.fb2", "ok1.fb2", "ok2.fb2", "ok3.fb2"} {
		fw, _ := mw.CreateFormFile("file", name)
		fw.Write([]byte("<FictionBook/>"))
	}
	mw.WriteField("format", "epub3")
	mw.WriteField("preset", "defaults")
	mw.Close()
	req := httptest.NewRequest("POST", "/convert", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("convert POST code=%d body=%q", rec.Code, rec.Body.String())
	}
	id := extractJobID(t, rec.Body.String())

	// Wait until BOTH corrupt inputs are marked failed (every input this test
	// expects retry to catch), but don't wait for the whole batch: the ok*
	// inputs' background worker is still running (thanks to the artificial
	// delay), so this is the in-flight window the fix must protect. Waiting
	// for only one corrupt file would let the retry burst miss whichever
	// corrupt file hadn't failed yet, permanently stranding it (a test bug,
	// not a fix bug) since nothing retries it again afterward.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for both corrupt files to fail")
		}
		st := loadStatus(t, h, id)
		if st != nil {
			failedCorrupt := 0
			for _, f := range st.Files {
				if strings.HasPrefix(f.Input, "corrupt") && f.State == jobs.StateFailed {
					failedCorrupt++
				}
			}
			if failedCorrupt == 2 {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Fire a burst of concurrent retry requests while the original worker may
	// still be converting the remaining inputs.
	const concurrentRetries = 6
	var wg sync.WaitGroup
	for i := 0; i < concurrentRetries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("POST", "/jobs/"+id+"/retry", nil))
			if rec.Code != 200 {
				t.Errorf("retry POST code=%d body=%q", rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()

	st := waitDone(t, h, id)
	if len(st.Files) != 5 {
		t.Fatalf("lost update: expected 5 files in final status, got %d: %+v", len(st.Files), st.Files)
	}
	for _, f := range st.Files {
		if f.State != jobs.StateDone {
			t.Errorf("file %q: expected done (no lost update), got state=%s err=%q", f.Input, f.State, f.Err)
		}
	}

	// Each corrupt input must have been converted exactly twice: once by the
	// original (failing) attempt, once by whichever single retry request won
	// the race. An unguarded/undeduped retry would convert it 1 + N times.
	for _, name := range []string{"corrupt1.fb2", "corrupt2.fb2"} {
		if got := runner.callCount(name); got != 2 {
			t.Errorf("%s: expected exactly 2 conversion attempts (1 fail + 1 retry), got %d — duplicate conversion from unguarded/undeduped retry", name, got)
		}
	}
	for _, name := range []string{"ok1.fb2", "ok2.fb2", "ok3.fb2"} {
		if got := runner.callCount(name); got != 1 {
			t.Errorf("%s: expected exactly 1 conversion attempt, got %d", name, got)
		}
	}
}

// TestRetryConfigWriteFailureLeavesFilesFailed is the regression test for
// bean wx5d: handleRetry must build and write the retry config BEFORE
// flipping failed files to StatePending. Pre-fix, the flip+Save happens
// first; if writing retry-config.yaml then fails, the files are stranded
// StatePending with no worker launched, and a later retry finds no
// StateFailed rows to re-catch them.
//
// To force a genuine, isolated config-write failure, a directory is
// pre-created at the exact path handleRetry writes retry-config.yaml to, so
// os.WriteFile fails with EISDIR — without touching status.json's own write
// path (a different filename in the same dir), so the test can tell the two
// apart. Assert status.json is byte-for-byte unchanged and the file stays
// StateFailed.
func TestRetryConfigWriteFailureLeavesFilesFailed(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "corrupt.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
	id := extractJobID(t, rec.Body.String())
	st := waitDone(t, h, id)
	if st.Files[0].State != jobs.StateFailed {
		t.Fatalf("precondition: expected failed, got %s", st.Files[0].State)
	}

	jobDir := filepath.Join(testJobsDir, id)
	before, err := os.ReadFile(filepath.Join(jobDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobDir, "retry-config.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/jobs/"+id+"/retry", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on retry config write failure, got %d body=%q", rec.Code, rec.Body.String())
	}

	after, err := os.ReadFile(filepath.Join(jobDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("status.json changed on config write failure:\nbefore=%s\nafter=%s", before, after)
	}

	st = loadStatus(t, h, id)
	if st.Files[0].State != jobs.StateFailed {
		t.Fatalf("config write failure must leave file StateFailed (not orphaned Pending), got %s", st.Files[0].State)
	}
}

// TestRetryBadIDNotFound is the traversal regression test for POST
// /jobs/{id}/retry: the existing traversal suite only covers the GET routes.
// net/http's ServeMux matches an encoded slash inside {id} as a literal
// single path segment and r.PathValue("id") returns it unescaped, so retry
// must reject it the same way every other {id}-keyed route does.
func TestRetryBadIDNotFound(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 1}, fakeFbcRunner(t))

	badID := "..%2f..%2fetc%2fpasswd"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/jobs/"+badID+"/retry", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("retry on bad id: expected 404, got %d body=%q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/jobs/deadbeefdeadbeef/retry", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("retry on unknown id: expected 404, got %d body=%q", rec.Code, rec.Body.String())
	}
}

// TestDownloadTraversalRejected exercises encoded path-traversal payloads and
// unknown job ids against all three download routes: these reach our handler
// code directly, so each must 404 there (never stream a file).
func TestDownloadTraversalRejected(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "multi.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
	id := extractJobID(t, rec.Body.String())
	waitDone(t, h, id)

	targets := []string{
		"/jobs/" + id + "/download/..%2f..%2fetc%2fpasswd",
		"/jobs/" + id + "/log/..%2f..%2fetc%2fpasswd",
		"/jobs/deadbeefdeadbeef/download/multi.epub",
		"/jobs/deadbeefdeadbeef/zip",
		"/jobs/deadbeefdeadbeef/log/multi.fb2",
	}
	for _, target := range targets {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("target %q: expected 404, got %d", target, rec.Code)
		}
	}
}

// TestJobIDTraversalRejected is the trust-boundary regression test for the
// job {id} path segment itself (as opposed to the {file} segment, already
// covered above). net/http's ServeMux only canonicalizes literal dot-segments
// before matching; an encoded slash (%2f) inside the {id} segment still
// matches the single-segment {id} pattern, and r.PathValue("id") returns it
// *unescaped* — so "..%2f..%2fetc" arrives at the handler as the literal id
// "../../etc". Every route keyed on {id} must reject that id, not just the
// {file} segment.
func TestJobIDTraversalRejected(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "multi.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
	id := extractJobID(t, rec.Body.String())
	waitDone(t, h, id)

	badID := "..%2f..%2fetc%2fpasswd"
	targets := []string{
		"/jobs/" + badID,
		"/jobs/" + badID + "/download/multi.epub",
		"/jobs/" + badID + "/zip",
		"/jobs/" + badID + "/log/multi.fb2",
	}
	for _, target := range targets {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("target %q: expected 404, got %d body=%q", target, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "FAKE-") || strings.Contains(rec.Body.String(), "INFO:") {
			t.Errorf("target %q: leaked job content: %q", target, rec.Body.String())
		}
	}
}

// TestDownloadLiteralDotSegmentNeverServesFile covers the "../../x" form.
// net/http's ServeMux canonicalizes literal dot-segments before any pattern
// match, 307-redirecting to the resolved path — which lands outside every
// /jobs/{id}/... route (proven below) rather than in our handler at all. That
// redirect is the actual security boundary here; assert it never resolves to
// a job file being streamed.
func TestDownloadLiteralDotSegmentNeverServesFile(t *testing.T) {
	h := newTestServer(t, config.Config{MaxConcurrent: 2}, fakeFbcRunner(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartConvert(t, "multi.fb2", map[string]string{"format": "epub3", "preset": "defaults"}))
	id := extractJobID(t, rec.Body.String())
	waitDone(t, h, id)

	targets := []string{
		"/jobs/" + id + "/download/../../etc/passwd",
		"/jobs/" + id + "/log/../../etc/passwd",
	}
	for _, target := range targets {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", target, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("target %q: expected mux to redirect the unclean path, got %d", target, rec.Code)
		}
		loc := rec.Header().Get("Location")
		if strings.Contains(loc, "/jobs/"+id+"/download/") || strings.Contains(loc, "/jobs/"+id+"/log/") {
			t.Fatalf("target %q: redirect %q still targets a download/log route", target, loc)
		}
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", loc, nil))
		if rec.Header().Get("Content-Disposition") != "" {
			t.Fatalf("target %q: redirected request %q streamed a file (Content-Disposition set)", target, loc)
		}
		if strings.Contains(rec.Body.String(), "FAKE-") || strings.Contains(rec.Body.String(), "INFO:") {
			t.Fatalf("target %q: redirected request %q leaked job content: %q", target, loc, rec.Body.String())
		}
	}
}
