package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func TestMiddlewareDisabledPassthrough(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	NewStub(testKey, false).Middleware(okHandler()).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("disabled must pass, got %d", rec.Code)
	}
}

func TestMiddlewareNoSessionRedirects(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/settings", nil)
	NewStub(testKey, true).Middleware(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/auth/login?return=%2Fsettings" {
		t.Fatalf("redirect target: %q", loc)
	}
}

func TestMiddlewareValidSessionPasses(t *testing.T) {
	a := NewStub(testKey, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName,
		Value: SignSession(Session{Sub: "u1", Name: "Alice", Exp: time.Now().Add(time.Hour).Unix()}, testKey)})
	a.Middleware(okHandler()).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("valid session must pass, got %d", rec.Code)
	}
}

func TestMiddlewareExpiredSessionRedirects(t *testing.T) {
	a := NewStub(testKey, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName,
		Value: SignSession(Session{Sub: "u1", Exp: time.Now().Add(-time.Second).Unix()}, testKey)})
	a.Middleware(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expired session must redirect, got %d", rec.Code)
	}
}

func TestLoginSetsFlowCookieAndRedirects(t *testing.T) {
	a, _ := testIDP(t) // from auth_test.go — a fully wired Authenticator
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/login?return=/settings", nil)
	a.Login(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == flowCookieName {
			found = true
		}
	}
	if !found {
		t.Fatal("login must set the flow cookie")
	}
	if loc := rec.Header().Get("Location"); loc == "" {
		t.Fatal("login must redirect to the IdP")
	}
}

func TestSafeReturn(t *testing.T) {
	cases := map[string]string{"": "/", "/settings": "/settings", "//evil.com": "/", "https://evil": "/", "/\\evil.com": "/"}
	for in, want := range cases {
		if got := safeReturn(in); got != want {
			t.Fatalf("safeReturn(%q)=%q want %q", in, got, want)
		}
	}
}
