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
