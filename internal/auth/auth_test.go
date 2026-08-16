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
