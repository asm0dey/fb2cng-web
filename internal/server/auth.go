package server

import (
	"net"
	"net/http"
)

// remoteUserHeader is the identity header Authelia sets via the reverse proxy.
const remoteUserHeader = "Remote-User"

// ForwardAuth enforces reverse-proxy forward authentication.
//
// When enabled is false, requests pass through unchanged. When enabled:
//   - if trusted is non-empty, the request's source IP must be in it; otherwise
//     the request is rejected (so Remote-* headers can never be spoofed by a
//     client reaching the app directly).
//   - a non-empty Remote-User header is required; missing -> 401.
func ForwardAuth(enabled bool, trusted []string, next http.Handler) http.Handler {
	trustSet := map[string]bool{}
	for _, ip := range trusted {
		trustSet[ip] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			next.ServeHTTP(w, r)
			return
		}
		if len(trustSet) > 0 {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if !trustSet[host] {
				http.Error(w, "forbidden: untrusted source", http.StatusUnauthorized)
				return
			}
		}
		if r.Header.Get(remoteUserHeader) == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
