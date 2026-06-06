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
