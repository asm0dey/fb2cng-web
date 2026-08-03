package auth

import (
	"testing"
	"time"
)

func TestSessionRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s := Session{Sub: "u1", Name: "Alice", Exp: time.Now().Add(time.Hour).Unix()}
	got, ok := ParseSession(SignSession(s, key), key, time.Now())
	if !ok || got != s {
		t.Fatalf("roundtrip: ok=%v got=%+v want=%+v", ok, got, s)
	}
}

func TestSessionTampered(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	cookie := SignSession(Session{Sub: "u1", Exp: time.Now().Add(time.Hour).Unix()}, key)
	// Corrupt the first payload char, flipping to a guaranteed-different byte.
	// (Flipping the last mac char is not reliable: RawURLEncoding's final char
	// has unused trailing bits, so a different char can decode to the same mac.
	// The mac is computed over the base64 payload string, so any payload change
	// guarantees a mismatch.)
	repl := byte('A')
	if cookie[0] == repl {
		repl = 'B'
	}
	tampered := string(repl) + cookie[1:]
	if _, ok := ParseSession(tampered, key, time.Now()); ok {
		t.Fatal("tampered cookie must be rejected")
	}
}

func TestSessionWrongKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	other := []byte("ffffffffffffffffffffffffffffffff")
	cookie := SignSession(Session{Sub: "u1", Exp: time.Now().Add(time.Hour).Unix()}, key)
	if _, ok := ParseSession(cookie, other, time.Now()); ok {
		t.Fatal("cookie signed with a different key must be rejected")
	}
}

func TestSessionExpired(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	cookie := SignSession(Session{Sub: "u1", Exp: time.Now().Add(-time.Second).Unix()}, key)
	if _, ok := ParseSession(cookie, key, time.Now()); ok {
		t.Fatal("expired cookie must be rejected")
	}
}
