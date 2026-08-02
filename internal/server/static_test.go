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
