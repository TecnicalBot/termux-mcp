// Package auth provides constant-time bearer-token verification and an
// HTTP middleware that enforces it.
package auth

import (
	"crypto/subtle"
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

// RequireBearer wraps next so that every request must carry a valid bearer
// token. If token is empty and require is false, requests pass through.
func RequireBearer(next http.Handler, token string, require bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if require || token != "" {
			if !ValidToken(r.Header.Get("Authorization"), token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="termux-mcp"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
