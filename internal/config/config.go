// Package config loads app configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration.
type Config struct {
	Addr           string   // listen address, e.g. ":8080"
	FBCBin         string   // path to the fbc binary
	MaxConcurrent  int      // max concurrent fbc processes
	ForwardAuth    bool     // trust reverse-proxy Remote-* headers
	TrustedProxies []string // source IPs allowed to set Remote-* headers (empty = trust any source)
}

// FromEnv builds a Config from environment variables, applying defaults.
func FromEnv() Config {
	c := Config{
		Addr:          ":" + envOr("PORT", "8080"),
		FBCBin:        envOr("FBC_BIN", "fbc"),
		MaxConcurrent: atoiOr(os.Getenv("MAX_CONCURRENT"), 3),
		ForwardAuth:   os.Getenv("AUTH_FORWARD_AUTH") == "true",
	}
	for _, p := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if s := strings.TrimSpace(p); s != "" {
			c.TrustedProxies = append(c.TrustedProxies, s)
		}
	}
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}
