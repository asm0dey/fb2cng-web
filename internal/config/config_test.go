package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("FBC_BIN", "")
	t.Setenv("MAX_CONCURRENT", "")
	t.Setenv("AUTH_FORWARD_AUTH", "")
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("PRESETS_DIR", "")
	t.Setenv("JOBS_DIR", "")
	t.Setenv("JOBS_TTL", "")
	t.Setenv("FBC_TIMEOUT", "")

	got := FromEnv()
	want := Config{
		Addr:           ":8080",
		FBCBin:         "fbc",
		MaxConcurrent:  3,
		ForwardAuth:    false,
		TrustedProxies: nil,
		PresetsDir:     "/data/presets",
		JobsDir:        filepath.Join(os.TempDir(), "fb2cng-jobs"),
		JobsTTL:        time.Hour,
		FBCTimeout:     DefaultFBCTimeout,
	}
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
	t.Setenv("PRESETS_DIR", "/srv/presets")
	t.Setenv("JOBS_DIR", "/srv/jobs")
	t.Setenv("JOBS_TTL", "30m")
	t.Setenv("FBC_TIMEOUT", "2m")

	got := FromEnv()
	if got.Addr != ":9000" || got.FBCBin != "/opt/fbc" || got.MaxConcurrent != 5 || !got.ForwardAuth {
		t.Fatalf("overrides not applied: %+v", got)
	}
	if !reflect.DeepEqual(got.TrustedProxies, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Fatalf("trusted proxies: %+v", got.TrustedProxies)
	}
	if got.PresetsDir != "/srv/presets" || got.JobsDir != "/srv/jobs" || got.JobsTTL != 30*time.Minute {
		t.Fatalf("new env not applied: %+v", got)
	}
	if got.FBCTimeout != 2*time.Minute {
		t.Fatalf("FBC_TIMEOUT not applied: got %v", got.FBCTimeout)
	}
}

func TestJobsTTLInvalidFallsBack(t *testing.T) {
	t.Setenv("JOBS_TTL", "not-a-duration")
	if got := FromEnv().JobsTTL; got != time.Hour {
		t.Fatalf("invalid JOBS_TTL should fall back to 1h, got %v", got)
	}
}

func TestFBCTimeoutInvalidFallsBack(t *testing.T) {
	t.Setenv("FBC_TIMEOUT", "not-a-duration")
	if got := FromEnv().FBCTimeout; got != DefaultFBCTimeout {
		t.Fatalf("invalid FBC_TIMEOUT should fall back to %v, got %v", DefaultFBCTimeout, got)
	}
}
