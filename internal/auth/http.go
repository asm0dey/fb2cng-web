package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// flowState is the CSRF/PKCE state stashed in the signed flow cookie between
// /auth/login and /auth/callback.
type flowState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	ReturnTo string `json:"r"`
}

// SessionFromRequest returns the valid session on the request, if any.
func (a *Authenticator) SessionFromRequest(r *http.Request) (Session, bool) {
	if !a.enabled {
		return Session{}, false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return Session{}, false
	}
	return ParseSession(c.Value, a.key, time.Now())
}

// Middleware gates the wrapped handler: no valid session -> redirect to login.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	if !a.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.SessionFromRequest(r); !ok {
			http.Redirect(w, r, "/auth/login?return="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Login starts the auth-code flow: mint state/nonce/PKCE, stash them, redirect.
func (a *Authenticator) Login(w http.ResponseWriter, r *http.Request) {
	verifier := oauth2.GenerateVerifier()
	fs := flowState{
		State:    randToken(),
		Nonce:    randToken(),
		Verifier: verifier,
		ReturnTo: safeReturn(r.URL.Query().Get("return")),
	}
	payload, _ := json.Marshal(fs)
	setSignedCookie(w, flowCookieName, payload, a.key, a.secure, 600)
	authURL := a.oauth.AuthCodeURL(fs.State, oidc.Nonce(fs.Nonce), oauth2.S256ChallengeOption(verifier))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback completes the flow: verify state, exchange code, verify+authorize the
// ID token, set the session cookie, redirect to the original path.
func (a *Authenticator) Callback(w http.ResponseWriter, r *http.Request) {
	raw, ok := readSignedCookie(r, flowCookieName, a.key)
	if !ok {
		http.Error(w, "auth: missing flow state", http.StatusBadRequest)
		return
	}
	clearCookie(w, flowCookieName, a.secure)
	var fs flowState
	if err := json.Unmarshal(raw, &fs); err != nil {
		http.Error(w, "auth: bad flow state", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != fs.State {
		http.Error(w, "auth: state mismatch", http.StatusBadRequest)
		return
	}
	tok, err := a.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(fs.Verifier))
	if err != nil {
		log.Printf("auth: code exchange: %v", err)
		http.Error(w, "auth failed", http.StatusBadGateway)
		return
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "auth: no id_token in response", http.StatusBadGateway)
		return
	}
	sess, err := a.verifyAndAuthorize(r.Context(), rawID, fs.Nonce)
	if err != nil {
		if errors.Is(err, errForbidden) {
			http.Error(w, "forbidden: not in the required group", http.StatusForbidden)
			return
		}
		log.Printf("auth: %v", err)
		http.Error(w, "auth failed", http.StatusUnauthorized)
		return
	}
	setSessionCookie(w, sess, a.key, a.secure, a.sessionTTL)
	http.Redirect(w, r, fs.ReturnTo, http.StatusFound)
}

// Logout clears the session cookie.
func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, sessionCookieName, a.secure)
	http.Redirect(w, r, "/", http.StatusFound)
}

// safeReturn keeps redirects local: only a path beginning with a single "/".
func safeReturn(p string) string {
	if p == "" || p[0] != '/' || (len(p) > 1 && (p[1] == '/' || p[1] == '\\')) {
		return "/"
	}
	return p
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
