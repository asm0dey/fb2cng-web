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
	tampered := cookie[:len(cookie)-1] + "X" // corrupt the last mac char
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
