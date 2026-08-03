// Package config loads app configuration from environment variables.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// DefaultFBCTimeout is the fallback per-file fbc conversion timeout when
// FBCTimeout is unset (zero value) or the FBC_TIMEOUT env var is invalid.
// Big books can legitimately take minutes; ops can tune via FBC_TIMEOUT.
const DefaultFBCTimeout = 10 * time.Minute

// Config holds all runtime configuration.
type Config struct {
	Addr              string        // listen address, e.g. ":8080"
	FBCBin            string        // path to the fbc binary
	MaxConcurrent     int           // max concurrent fbc processes
	AuthMode          string        // "off" (default) or "oidc"
	OIDCIssuer        string        // issuer / discovery base URL
	OIDCClientID      string        // OIDC client id
	OIDCClientSecret  string        // OIDC client secret (plaintext)
	OIDCRedirectURL   string        // absolute callback URL
	OIDCGroupsClaim   string        // ID-token claim holding group membership
	OIDCRequiredGroup string        // group value required for access
	SessionKey        string        // base64-encoded HMAC key (empty = random at startup)
	SessionTTL        time.Duration // session lifetime / revalidation interval
	PresetsDir        string        // persistent preset store dir
	JobsDir           string        // ephemeral job scratch dir (swept on TTL)
	JobsTTL           time.Duration // max job age before sweep
	FBCTimeout        time.Duration // max time a single fbc invocation (convert/validate) may run before being killed
}

// FromEnv builds a Config from environment variables, applying defaults.
func FromEnv() Config {
	c := Config{
		Addr:              ":" + envOr("PORT", "8080"),
		FBCBin:            envOr("FBC_BIN", "fbc"),
		MaxConcurrent:     atoiOr(os.Getenv("MAX_CONCURRENT"), 3),
		AuthMode:          envOr("AUTH_MODE", "off"),
		OIDCIssuer:        os.Getenv("AUTH_OIDC_ISSUER"),
		OIDCClientID:      os.Getenv("AUTH_OIDC_CLIENT_ID"),
		OIDCClientSecret:  os.Getenv("AUTH_OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:   os.Getenv("AUTH_OIDC_REDIRECT_URL"),
		OIDCGroupsClaim:   envOr("AUTH_OIDC_GROUPS_CLAIM", "groups"),
		OIDCRequiredGroup: os.Getenv("AUTH_OIDC_REQUIRED_GROUP"),
		SessionKey:        os.Getenv("AUTH_SESSION_KEY"),
		SessionTTL:        durationOr(os.Getenv("AUTH_SESSION_TTL"), 8*time.Hour),
		PresetsDir:        envOrFunc("PRESETS_DIR", defaultPresetsDir),
		JobsDir:           envOrFunc("JOBS_DIR", defaultJobsDir),
		JobsTTL:           durationOr(os.Getenv("JOBS_TTL"), time.Hour),
		FBCTimeout:        durationOr(os.Getenv("FBC_TIMEOUT"), DefaultFBCTimeout),
	}
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envOrFunc returns the env value for key, or def() when unset. def is evaluated
// only on a miss, so a default helper's side effects (e.g. creating a fallback
// dir) never run when the env var is set — the container sets JOBS_DIR/PRESETS_DIR
// explicitly but has no HOME.
func envOrFunc(key string, def func() string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def()
}

// defaultPresetsDir picks a writable per-user location for the preset store when
// PRESETS_DIR is unset (a bare local run). The container image sets
// PRESETS_DIR=/data/presets explicitly, so this default only affects env-less
// runs — for which /data is neither writable nor creatable by a normal user.
func defaultPresetsDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "fb2cng", "presets")
	}
	return privateFallbackDir("fb2cng-presets")
}

// defaultJobsDir picks a writable per-user cache location for the ephemeral job
// store when JOBS_DIR is unset (a bare local run). The container image sets
// JOBS_DIR explicitly, so this default only affects env-less runs.
func defaultJobsDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "fb2cng", "jobs")
	}
	return privateFallbackDir("fb2cng-jobs")
}

// privateFallbackDir returns a private directory for the rare case where no
// per-user config/cache dir is available (no HOME). os.MkdirTemp creates it 0700
// with a random name, avoiding the predictable, publicly writable path that a
// fixed name under the system temp dir would produce (Sonar go:S5445).
func privateFallbackDir(name string) string {
	if dir, err := os.MkdirTemp("", name+"-"); err == nil {
		return dir
	}
	return filepath.Join(".", name)
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}

func durationOr(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return def
}
