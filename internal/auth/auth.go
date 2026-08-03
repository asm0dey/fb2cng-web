package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"fb2cng-web/internal/config"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Authenticator runs the OIDC flow and validates session cookies. A zero-OIDC
// authenticator (Enabled() == false) passes every request through.
type Authenticator struct {
	enabled       bool
	key           []byte
	secure        bool
	sessionTTL    time.Duration
	groupsClaim   string
	requiredGroup string
	oauth         *oauth2.Config
	verifier      *oidc.IDTokenVerifier
}

var errForbidden = errors.New("not in required group")

// New builds an Authenticator from config. When AUTH_MODE != "oidc" it returns a
// disabled passthrough. Otherwise it validates required fields, resolves the
// session key, and performs OIDC discovery (network) — a failure here is fatal
// to the caller.
func New(ctx context.Context, cfg config.Config) (*Authenticator, error) {
	if cfg.AuthMode != "oidc" {
		return &Authenticator{enabled: false}, nil
	}

	var missing []string
	if cfg.OIDCIssuer == "" {
		missing = append(missing, "AUTH_OIDC_ISSUER")
	}
	if cfg.OIDCClientID == "" {
		missing = append(missing, "AUTH_OIDC_CLIENT_ID")
	}
	if cfg.OIDCClientSecret == "" {
		missing = append(missing, "AUTH_OIDC_CLIENT_SECRET")
	}
	if cfg.OIDCRedirectURL == "" {
		missing = append(missing, "AUTH_OIDC_REDIRECT_URL")
	}
	if cfg.OIDCRequiredGroup == "" {
		missing = append(missing, "AUTH_OIDC_REQUIRED_GROUP")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("AUTH_MODE=oidc requires: %s", strings.Join(missing, ", "))
	}

	key, err := resolveKey(cfg.SessionKey)
	if err != nil {
		return nil, err
	}

	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery %q: %w", cfg.OIDCIssuer, err)
	}

	claim := cfg.OIDCGroupsClaim
	if claim == "" {
		claim = "groups"
	}
	ttl := cfg.SessionTTL
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}

	return &Authenticator{
		enabled:       true,
		key:           key,
		secure:        strings.HasPrefix(cfg.OIDCRedirectURL, "https://"),
		sessionTTL:    ttl,
		groupsClaim:   claim,
		requiredGroup: cfg.OIDCRequiredGroup,
		oauth: &oauth2.Config{
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.OIDCRedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID}),
	}, nil
}

// NewStub returns a session-only Authenticator (no OIDC provider). Used by the
// disabled path and by tests that exercise session/middleware behaviour without
// standing up an IdP.
// ponytail: session-only seam, avoids an oidctest server in every server test.
func NewStub(key []byte, enabled bool) *Authenticator {
	return &Authenticator{enabled: enabled, key: key, secure: false, sessionTTL: 8 * time.Hour}
}

func resolveKey(b64key string) ([]byte, error) {
	if b64key != "" {
		k, err := base64.StdEncoding.DecodeString(b64key)
		if err != nil || len(k) < 32 {
			return nil, fmt.Errorf("AUTH_SESSION_KEY must be base64 of at least 32 bytes")
		}
		return k, nil
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	log.Printf("WARNING: AUTH_SESSION_KEY unset; generated a random key. Sessions will not survive a restart. Set AUTH_SESSION_KEY (base64 of 32 bytes) in production.")
	return k, nil
}

// Enabled reports whether requests are gated.
func (a *Authenticator) Enabled() bool { return a.enabled }

// verifyAndAuthorize verifies the raw ID token (signature/audience/expiry),
// checks the nonce and required group, and returns the session to persist.
func (a *Authenticator) verifyAndAuthorize(ctx context.Context, rawIDToken, nonce string) (Session, error) {
	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Session{}, fmt.Errorf("verify id token: %w", err)
	}
	if idToken.Nonce != nonce {
		return Session{}, errors.New("nonce mismatch")
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Session{}, fmt.Errorf("parse claims: %w", err)
	}
	if !hasGroup(claims[a.groupsClaim], a.requiredGroup) {
		return Session{}, errForbidden
	}
	return Session{
		Sub:  idToken.Subject,
		Name: displayName(claims, idToken.Subject),
		Exp:  time.Now().Add(a.sessionTTL).Unix(),
	}, nil
}

// hasGroup reports whether required is present in a claim expected to be a JSON
// array of strings.
func hasGroup(claim any, required string) bool {
	arr, ok := claim.([]any)
	if !ok {
		return false
	}
	for _, g := range arr {
		if s, ok := g.(string); ok && s == required {
			return true
		}
	}
	return false
}

func displayName(claims map[string]any, sub string) string {
	for _, k := range []string{"name", "email"} {
		if s, ok := claims[k].(string); ok && s != "" {
			return s
		}
	}
	return sub
}
