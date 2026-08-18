package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Server.Mode != "stdio" {
		t.Fatalf("mode = %q, want stdio", c.Server.Mode)
	}
	if c.Exec.DefaultTimeoutSeconds != 30 {
		t.Fatalf("timeout = %d", c.Exec.DefaultTimeoutSeconds)
	}
	if c.Auth.Token != "" {
		t.Fatalf("default must have no token")
	}
}

func TestLoadMissingFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.Mode != "stdio" {
		t.Fatalf("mode = %q", c.Server.Mode)
	}
}

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	content := `
server:
  mode: http
  port: 8080
tools:
  enable_sensitive: true
  file_roots: ["/sdcard", "/data/data/com.termux/files/home"]
exec:
  shell_allowed: true
  shell_allow_patterns: ["*"]
`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.Mode != "http" || c.Server.Port != 8080 {
		t.Fatalf("server = %+v", c.Server)
	}
	if !c.Tools.EnableSensitive {
		t.Fatal("sensitive not enabled")
	}
	if !c.Exec.ShellAllowed {
		t.Fatal("shell not allowed")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("TERMUX_MCP_SERVER_MODE", "http")
	t.Setenv("TERMUX_MCP_AUTH_TOKEN", "sekret")
	t.Setenv("TERMUX_MCP_TOOLS_ENABLE_SENSITIVE", "true")
	t.Setenv("TERMUX_MCP_EXEC_DEFAULT_TIMEOUT_SECONDS", "7")

	c, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.Mode != "http" {
		t.Fatalf("mode = %q", c.Server.Mode)
	}
	if c.Auth.Token != "sekret" {
		t.Fatalf("token = %q", c.Auth.Token)
	}
	if !c.Tools.EnableSensitive {
		t.Fatal("sensitive not enabled")
	}
	if c.Exec.DefaultTimeoutSeconds != 7 {
		t.Fatalf("timeout = %d", c.Exec.DefaultTimeoutSeconds)
	}
}

func TestNonLoopbackRequiresToken(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("server:\n  mode: http\n  bind: 0.0.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected error binding non-loopback without token")
	}

	// With a token it must pass.
	t.Setenv("TERMUX_MCP_AUTH_TOKEN", "x")
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load with token: %v", err)
	}
	if !c.IsNonLoopback() {
		t.Fatal("expected non-loopback")
	}
}

func TestShellRequiresAllowlist(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	// Explicitly clearing patterns while shell_allowed: true must fail.
	if err := os.WriteFile(p, []byte("exec:\n  shell_allowed: true\n  shell_allow_patterns: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("shell_allowed with empty patterns must fail validation")
	}

	if err := os.WriteFile(p, []byte("exec:\n  shell_allowed: true\n  shell_allow_patterns: [\"*\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("shell with allowlist must pass: %v", err)
	}
}

func TestSanitized(t *testing.T) {
	c := Default()
	c.Auth.Token = "hunter2"
	s := c.Sanitized()
	if s.Auth.Token != "" {
		t.Fatal("sanitized config leaked token")
	}
	if c.Auth.Token != "hunter2" {
		t.Fatal("sanitize mutated original")
	}
}

func TestDiscoverPrefersTermuxPrefix(t *testing.T) {
	t.Setenv("PREFIX", t.TempDir())
	t.Setenv("TERMUX_MCP_CONFIG", "")
	dir := filepath.Join(os.Getenv("PREFIX"), "var", "lib", "termux-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("server:\n  mode: http\n  port: 9000\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.Mode != "http" || c.Server.Port != 9000 {
		t.Fatalf("server = %+v, want discovered config", c.Server)
	}
}

func TestDiscoverEnvConfigWins(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "prefix")
	if err := os.MkdirAll(filepath.Join(prefix, "var", "lib", "termux-mcp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "var", "lib", "termux-mcp", "config.yaml"),
		[]byte("server:\n  port: 1111\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envCfg := filepath.Join(dir, "env.yaml")
	if err := os.WriteFile(envCfg, []byte("server:\n  port: 2222\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PREFIX", prefix)
	t.Setenv("TERMUX_MCP_CONFIG", envCfg)

	c, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.Port != 2222 {
		t.Fatalf("port = %d, want TERMUX_MCP_CONFIG to win", c.Server.Port)
	}
}

func TestEmptyFieldsKeepSafeDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("server:\n  bind: \"\"\naudit:\n  dir: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.Bind != "127.0.0.1" {
		t.Fatalf("bind = %q, want loopback default restored", c.Server.Bind)
	}
	want := filepath.Join(os.Getenv("PREFIX"), "var", "lib", "termux-mcp")
	if os.Getenv("PREFIX") == "" {
		hd, _ := os.UserHomeDir()
		want = filepath.Join(hd, ".termux-mcp")
	}
	if c.Audit.Dir != want {
		t.Fatalf("audit dir = %q, want default %q", c.Audit.Dir, want)
	}
}

// TestExampleConfig ensures the shipped config.example.yaml is valid and
// parses with the default policy (no sensitive/dangerous tools enabled).
func TestExampleConfig(t *testing.T) {
	p := filepath.Join("..", "..", "config.example.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	c := Default()
	if err := yaml.Unmarshal(data, c); err != nil {
		t.Fatalf("config.example.yaml is invalid YAML: %v", err)
	}
	if c.Tools.EnableSensitive || c.Tools.EnableDangerous {
		t.Fatal("example config must ship with sensitive/dangerous tiers disabled")
	}
}
