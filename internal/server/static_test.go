package server

import (
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
