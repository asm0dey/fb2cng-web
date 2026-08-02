package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/jobs"
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
	return New(cfg, r, web.FS, tpl, store).Handler()
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
