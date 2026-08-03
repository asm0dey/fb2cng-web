package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"fb2cng-web/internal/config"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
)

// testIDP spins up an oidctest server and returns an Authenticator wired to it,
// plus a function that mints a signed ID token with the given nonce and groups.
func testIDP(t *testing.T) (*Authenticator, func(nonce string, groups []string) string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &oidctest.Server{PublicKeys: []oidctest.PublicKey{
		{PublicKey: priv.Public(), KeyID: "key-1", Algorithm: oidc.RS256},
	}}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	s.SetIssuer(srv.URL)

	a, err := New(context.Background(), config.Config{
		AuthMode:          "oidc",
		OIDCIssuer:        srv.URL,
		OIDCClientID:      "test-client",
		OIDCClientSecret:  "secret",
		OIDCRedirectURL:   "http://app.local/auth/callback",
		OIDCGroupsClaim:   "groups",
		OIDCRequiredGroup: "fb2cng-users",
		SessionKey:        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", // 32 bytes
		SessionTTL:        time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	mint := func(nonce string, groups []string) string {
		gj := "["
		for i, g := range groups {
			if i > 0 {
				gj += ","
			}
			gj += fmt.Sprintf("%q", g)
		}
		gj += "]"
		claims := fmt.Sprintf(`{"iss":%q,"aud":"test-client","sub":"user-123","name":"Alice","nonce":%q,"groups":%s,"exp":%d,"iat":%d}`,
			srv.URL, nonce, gj, time.Now().Add(time.Hour).Unix(), time.Now().Unix())
		return oidctest.SignIDToken(priv, "key-1", oidc.RS256, claims)
	}
	return a, mint
}

func TestVerifyAndAuthorizeHappy(t *testing.T) {
	a, mint := testIDP(t)
	sess, err := a.verifyAndAuthorize(context.Background(), mint("n1", []string{"fb2cng-users"}), "n1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sess.Sub != "user-123" || sess.Name != "Alice" {
		t.Fatalf("session: %+v", sess)
	}
}

func TestVerifyAndAuthorizeWrongGroup(t *testing.T) {
	a, mint := testIDP(t)
	_, err := a.verifyAndAuthorize(context.Background(), mint("n1", []string{"other"}), "n1")
	if err != errForbidden {
		t.Fatalf("want errForbidden, got %v", err)
	}
}

func TestVerifyAndAuthorizeNonceMismatch(t *testing.T) {
	a, mint := testIDP(t)
	_, err := a.verifyAndAuthorize(context.Background(), mint("n1", []string{"fb2cng-users"}), "n2")
	if err == nil {
		t.Fatal("want nonce mismatch error")
	}
}

func TestNewValidationMissingFields(t *testing.T) {
	_, err := New(context.Background(), config.Config{AuthMode: "oidc"})
	if err == nil {
		t.Fatal("want validation error for missing OIDC fields")
	}
}

func TestNewDisabled(t *testing.T) {
	a, err := New(context.Background(), config.Config{AuthMode: "off"})
	if err != nil || a.Enabled() {
		t.Fatalf("off mode: err=%v enabled=%v", err, a.Enabled())
	}
}

func TestNewInvalidMode(t *testing.T) {
	_, err := New(context.Background(), config.Config{AuthMode: "oidic"})
	if err == nil {
		t.Fatal("want error for invalid AUTH_MODE, got nil")
	}
}
