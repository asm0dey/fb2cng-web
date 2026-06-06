package config

import (
	"reflect"
	"testing"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("FBC_BIN", "")
	t.Setenv("MAX_CONCURRENT", "")
	t.Setenv("AUTH_FORWARD_AUTH", "")
	t.Setenv("TRUSTED_PROXIES", "")

	got := FromEnv()
	want := Config{Addr: ":8080", FBCBin: "fbc", MaxConcurrent: 3, ForwardAuth: false, TrustedProxies: nil}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults: got %+v want %+v", got, want)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("FBC_BIN", "/opt/fbc")
	t.Setenv("MAX_CONCURRENT", "5")
	t.Setenv("AUTH_FORWARD_AUTH", "true")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1, 10.0.0.2")

	got := FromEnv()
	if got.Addr != ":9000" || got.FBCBin != "/opt/fbc" || got.MaxConcurrent != 5 || !got.ForwardAuth {
		t.Fatalf("overrides not applied: %+v", got)
	}
	if !reflect.DeepEqual(got.TrustedProxies, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Fatalf("trusted proxies: %+v", got.TrustedProxies)
	}
}
