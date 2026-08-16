package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"abc", "abc", true},
		{"  Bearer abc  ", "abc", true},
		{"Bearer xyz", "abc", false},
		{"", "abc", false},
		{"", "", true},
	}
	for _, c := range cases {
		if got := ValidToken(c.header, c.want); got != c.ok {
			t.Errorf("ValidToken(%q, %q) = %v, want %v", c.header, c.want, got, c.ok)
		}
	}
}

func TestRequireBearer(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := RequireBearer(next, "tok123", true)

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no token: code = %d, want 401", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer tok123")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("with token: code = %d, want 200", rr.Code)
	}
}

func TestRequireBearerDisabled(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := RequireBearer(next, "", false)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disabled auth: code = %d, want 200", rr.Code)
	}
}

// TestRequireBearerLoopback verifies the documented contract: with
// require=false the token is enforced for non-loopback clients but loopback
// (on-device) clients are exempt; with require=true it is enforced everywhere.
func TestRequireBearerLoopback(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name    string
		require bool
		remote  string
		header  string
		want    int
	}{
		{"loopback exempt, no token", false, "127.0.0.1:54321", "", http.StatusOK},
		{"ipv6 loopback exempt, no token", false, "[::1]:54321", "", http.StatusOK},
		{"loopback with token", false, "127.0.0.1:54321", "Bearer tok123", http.StatusOK},
		{"remote, no token", false, "192.168.1.20:54321", "", http.StatusUnauthorized},
		{"remote, wrong token", false, "192.168.1.20:54321", "Bearer nope", http.StatusUnauthorized},
		{"remote, good token", false, "192.168.1.20:54321", "Bearer tok123", http.StatusOK},
		{"require: loopback, no token", true, "127.0.0.1:54321", "", http.StatusUnauthorized},
		{"require: loopback, good token", true, "127.0.0.1:54321", "Bearer tok123", http.StatusOK},
		{"require: remote, no token", true, "192.168.1.20:54321", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := RequireBearer(next, "tok123", c.require)
			req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			req.RemoteAddr = c.remote
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != c.want {
				t.Fatalf("code = %d, want %d", rr.Code, c.want)
			}
		})
	}
}
