// Package auth provides constant-time bearer-token verification and an
// HTTP middleware that enforces it.
package auth

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// ValidToken reports whether the Authorization header value (with or without
// the "Bearer " prefix) matches the expected token, compared in constant time.
func ValidToken(headerValue, want string) bool {
	if want == "" {
		return true
	}
	got := strings.TrimSpace(headerValue)
	if len(got) >= 7 && strings.EqualFold(got[:7], "Bearer ") {
		got = strings.TrimSpace(got[7:])
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// RequireBearer wraps next so that requests must carry a valid bearer token.
// Loopback connections (127.0.0.1/::1 — on-device clients and local proxies)
// are exempt unless require is true. If token is empty and require is false,
// all requests pass through.
func RequireBearer(next http.Handler, token string, require bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if require || (token != "" && !isLoopback(r.RemoteAddr)) {
			if !ValidToken(r.Header.Get("Authorization"), token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="termux-mcp"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopback reports whether remoteAddr is a loopback connection.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
