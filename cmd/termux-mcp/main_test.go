package main

import (
	"strings"
	"testing"

	vpkg "termux-mcp/internal/version"
)

func TestVersionConst(t *testing.T) {
	if vpkg.Version == "" {
		t.Fatal("version constant must not be empty")
	}
	if vpkg.Name != "termux-mcp" {
		t.Fatalf("unexpected name %q", vpkg.Name)
	}
}

func TestNewToken(t *testing.T) {
	a := newToken()
	b := newToken()
	if len(a) != 64 {
		t.Fatalf("token length %d, want 64 hex chars", len(a))
	}
	if a == b {
		t.Fatal("two tokens must differ")
	}
	if strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("token %q is not hex", a)
	}
}

func TestParseTokenArgs(t *testing.T) {
	t.Setenv("TERMUX_MCP_CONFIG", "")
	cases := []struct {
		args    []string
		sub     string
		write   bool
		cfgPath string
	}{
		{args: nil, sub: "new", write: false, cfgPath: ""},
		{args: []string{"new"}, sub: "new", write: false, cfgPath: ""},
		{args: []string{"new", "--write", "--config", "/a/b.yaml"}, sub: "new", write: true, cfgPath: "/a/b.yaml"},
		{args: []string{"--write", "new", "--config=/a/b.yaml"}, sub: "new", write: true, cfgPath: "/a/b.yaml"},
		{args: []string{"rotate"}, sub: "rotate", write: false, cfgPath: ""},
		{args: []string{"new", "-c", "/c.yaml"}, sub: "new", write: false, cfgPath: "/c.yaml"},
	}
	for _, c := range cases {
		sub, write, cfgPath := parseTokenArgs(c.args)
		if sub != c.sub || write != c.write || cfgPath != c.cfgPath {
			t.Errorf("parseTokenArgs(%v) = (%q, %v, %q), want (%q, %v, %q)",
				c.args, sub, write, cfgPath, c.sub, c.write, c.cfgPath)
		}
	}
}

func TestParseTokenArgsEnvConfig(t *testing.T) {
	t.Setenv("TERMUX_MCP_CONFIG", "/env/config.yaml")
	sub, write, cfgPath := parseTokenArgs([]string{"new", "--write"})
	if sub != "new" || !write || cfgPath != "/env/config.yaml" {
		t.Fatalf("got (%q, %v, %q), want env config fallback", sub, write, cfgPath)
	}
}

func TestRepoSlug(t *testing.T) {
	cases := map[string]string{
		"TecnicalBot/termux-mcp":                    "TecnicalBot/termux-mcp",
		"https://github.com/TecnicalBot/termux-mcp": "TecnicalBot/termux-mcp",
		"git@github.com:TecnicalBot/termux-mcp.git": "TecnicalBot/termux-mcp",
		"not-a-repository":                          "",
	}
	for input, expected := range cases {
		if actual := repoSlug(input); actual != expected {
			t.Errorf("repoSlug(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestReleaseArch(t *testing.T) {
	if arch, err := releaseArch("arm64"); err != nil || arch != "arm64" {
		t.Fatalf("releaseArch(arm64) = (%q, %v)", arch, err)
	}
	if _, err := releaseArch("amd64"); err == nil {
		t.Fatal("releaseArch(amd64) should reject an unavailable release")
	}
}
