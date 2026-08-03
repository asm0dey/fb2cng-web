package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName = "fb2cng_session"
	flowCookieName    = "fb2cng_oidc_flow"
)

var b64 = base64.RawURLEncoding

// Session is the authenticated identity carried in the signed session cookie.
type Session struct {
	Sub  string `json:"sub"`
	Name string `json:"name"`
	Exp  int64  `json:"exp"` // unix seconds
}

func mac(payloadB64 string, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(payloadB64))
	return h.Sum(nil)
}

// signValue encodes payload as "<b64(payload)>.<b64(hmac)>".
func signValue(payload, key []byte) string {
	p := b64.EncodeToString(payload)
	return p + "." + b64.EncodeToString(mac(p, key))
}

// verifyValue checks the mac and returns the raw payload.
func verifyValue(value string, key []byte) ([]byte, bool) {
	p, sig, ok := strings.Cut(value, ".")
	if !ok {
		return nil, false
	}
	got, err := b64.DecodeString(sig)
	if err != nil || !hmac.Equal(mac(p, key), got) {
		return nil, false
	}
	payload, err := b64.DecodeString(p)
	if err != nil {
		return nil, false
	}
	return payload, true
}

// SignSession returns the signed cookie value for s.
func SignSession(s Session, key []byte) string {
	payload, _ := json.Marshal(s)
	return signValue(payload, key)
}

// ParseSession verifies the signature and expiry, returning the session.
func ParseSession(cookie string, key []byte, now time.Time) (Session, bool) {
	payload, ok := verifyValue(cookie, key)
	if !ok {
		return Session{}, false
	}
	var s Session
	if err := json.Unmarshal(payload, &s); err != nil {
		return Session{}, false
	}
	if now.Unix() >= s.Exp {
		return Session{}, false
	}
	return s, true
}

func setSignedCookie(w http.ResponseWriter, name string, payload, key []byte, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    signValue(payload, key),
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func readSignedCookie(r *http.Request, name string, key []byte) ([]byte, bool) {
	c, err := r.Cookie(name)
	if err != nil {
		return nil, false
	}
	return verifyValue(c.Value, key)
}

func setSessionCookie(w http.ResponseWriter, s Session, key []byte, ttl time.Duration) {
	setSignedCookie(w, sessionCookieName, mustJSON(s), key, int(ttl.Seconds()))
}

func mustJSON(s Session) []byte { b, _ := json.Marshal(s); return b }

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}
