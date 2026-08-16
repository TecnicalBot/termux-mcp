// Package config loads, validates and sanitizes the YAML server config
// with environment-variable overrides (TERMUX_MCP_*).
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the full server configuration tree.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Auth    AuthConfig    `yaml:"auth"`
	Tools   ToolsConfig   `yaml:"tools"`
	Exec    ExecConfig    `yaml:"exec"`
	Audit   AuditConfig   `yaml:"audit"`
	Logging LoggingConfig `yaml:"logging"`
}

type ServerConfig struct {
	Mode         string `yaml:"mode"` // stdio | http
	Bind         string `yaml:"bind"` // 127.0.0.1 (loopback default)
	Port         int    `yaml:"port"`
	MaxBodyBytes int64  `yaml:"max_body_bytes"`
	MaxSessions  int    `yaml:"max_sessions"`
}

type AuthConfig struct {
	Token   string `yaml:"token"`
	Require bool   `yaml:"require"` // enforce token even on loopback
}

type ToolsConfig struct {
	EnableSensitive bool     `yaml:"enable_sensitive"`
	EnableDangerous bool     `yaml:"enable_dangerous"`
	Allow           []string `yaml:"allow"`
	Deny            []string `yaml:"deny"`
	FileRoots       []string `yaml:"file_roots"`
}

type ExecConfig struct {
	DefaultTimeoutSeconds int      `yaml:"default_timeout_seconds"`
	MaxOutputBytes        int64    `yaml:"max_output_bytes"`
	MaxTaskLogBytes       int64    `yaml:"max_task_log_bytes"` // per-stream cap for background task logs
	ShellAllowed          bool     `yaml:"shell_allowed"`
	ShellAllowPatterns    []string `yaml:"shell_allow_patterns"`
	ShellDenyPatterns     []string `yaml:"shell_deny_patterns"`
}

type AuditConfig struct {
	Enabled bool   `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

type LoggingConfig struct {
	Level string `yaml:"level"` // debug | info | warn | error
	JSON  bool   `yaml:"json"`
}

// Default returns a safe, conservative configuration.
func Default() *Config {
	c := &Config{}
	c.Server.Mode = "stdio"
	c.Server.Bind = "127.0.0.1"
	c.Server.Port = 3000
	c.Server.MaxBodyBytes = 1 << 20 // 1 MiB
	c.Server.MaxSessions = 32
	c.Exec.DefaultTimeoutSeconds = 30
	c.Exec.MaxOutputBytes = 1 << 20  // 1 MiB
	c.Exec.MaxTaskLogBytes = 8 << 20 // 8 MiB per stream
	c.Audit.Enabled = true
	c.Audit.Dir = defaultDataDir()
	c.Logging.Level = "info"
	return c
}

// defaultDataDir prefers the Termux prefix so state survives app upgrades.
func defaultDataDir() string {
	if p := os.Getenv("PREFIX"); p != "" {
		return filepath.Join(p, "var", "lib", "termux-mcp")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".termux-mcp")
	}
	return "."
}

// Load reads the YAML file at path (missing file is OK: defaults + env win),
// applies TERMUX_MCP_* environment overrides, then validates.
//
// When path is empty, the config is discovered in order:
// $TERMUX_MCP_CONFIG, $PREFIX/var/lib/termux-mcp/config.yaml, ./config.yaml.
func Load(path string) (*Config, error) {
	c := Default()
	if path == "" {
		path = discover()
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config %s: %w", path, err)
			}
		} else if err := yaml.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	// Empty fields in a user file must not wipe out safe defaults.
	if c.Audit.Dir == "" {
		c.Audit.Dir = defaultDataDir()
	}
	if c.Server.Bind == "" {
		c.Server.Bind = "127.0.0.1"
	}
	applyEnv(c)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// discover locates a config.yaml in the documented default locations.
// It returns "" when none exists (pure defaults + env then apply).
func discover() string {
	if p := os.Getenv("TERMUX_MCP_CONFIG"); p != "" && fileExists(p) {
		return p
	}
	if p := os.Getenv("PREFIX"); p != "" {
		if c := filepath.Join(p, "var", "lib", "termux-mcp", "config.yaml"); fileExists(c) {
			return c
		}
	}
	if fileExists("config.yaml") {
		return "config.yaml"
	}
	return ""
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// applyEnv overrides fields from TERMUX_MCP_* environment variables.
func applyEnv(c *Config) {
	setStr(&c.Auth.Token, os.Getenv("TERMUX_MCP_AUTH_TOKEN"))
	setBool(&c.Auth.Require, os.Getenv("TERMUX_MCP_AUTH_REQUIRE"))
	setStr(&c.Server.Mode, os.Getenv("TERMUX_MCP_SERVER_MODE"))
	setStr(&c.Server.Bind, os.Getenv("TERMUX_MCP_SERVER_BIND"))
	setInt(&c.Server.Port, os.Getenv("TERMUX_MCP_SERVER_PORT"))
	setBool(&c.Tools.EnableSensitive, os.Getenv("TERMUX_MCP_TOOLS_ENABLE_SENSITIVE"))
	setBool(&c.Tools.EnableDangerous, os.Getenv("TERMUX_MCP_TOOLS_ENABLE_DANGEROUS"))
	setList(&c.Tools.Allow, os.Getenv("TERMUX_MCP_TOOLS_ALLOW"))
	setList(&c.Tools.Deny, os.Getenv("TERMUX_MCP_TOOLS_DENY"))
	setList(&c.Tools.FileRoots, os.Getenv("TERMUX_MCP_FILE_ROOTS"))
	setBool(&c.Exec.ShellAllowed, os.Getenv("TERMUX_MCP_EXEC_SHELL_ALLOWED"))
	setInt(&c.Exec.DefaultTimeoutSeconds, os.Getenv("TERMUX_MCP_EXEC_DEFAULT_TIMEOUT_SECONDS"))
	setInt64(&c.Exec.MaxOutputBytes, os.Getenv("TERMUX_MCP_EXEC_MAX_OUTPUT_BYTES"))
	setInt64(&c.Exec.MaxTaskLogBytes, os.Getenv("TERMUX_MCP_EXEC_MAX_TASK_LOG_BYTES"))
	setBool(&c.Audit.Enabled, os.Getenv("TERMUX_MCP_AUDIT_ENABLED"))
	setStr(&c.Audit.Dir, os.Getenv("TERMUX_MCP_AUDIT_DIR"))
	setStr(&c.Logging.Level, os.Getenv("TERMUX_MCP_LOG_LEVEL"))
}

func setStr(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

func setBool(dst *bool, v string) {
	if v == "" {
		return
	}
	b, err := strconv.ParseBool(v)
	if err == nil {
		*dst = b
	}
}

func setInt(dst *int, v string) {
	if v == "" {
		return
	}
	n, err := strconv.Atoi(v)
	if err == nil {
		*dst = n
	}
}

func setInt64(dst *int64, v string) {
	if v == "" {
		return
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err == nil {
		*dst = n
	}
}

func setList(dst *[]string, v string) {
	if v == "" {
		return
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		*dst = out
	}
}

// Validate checks the config for unsafe or invalid combinations.
func (c *Config) Validate() error {
	switch c.Server.Mode {
	case "", "stdio":
		c.Server.Mode = "stdio"
	case "http":
	default:
		return fmt.Errorf("server.mode must be \"stdio\" or \"http\", got %q", c.Server.Mode)
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d out of range 1-65535", c.Server.Port)
	}
	if c.Exec.DefaultTimeoutSeconds <= 0 {
		return fmt.Errorf("exec.default_timeout_seconds must be > 0")
	}
	if c.Exec.MaxOutputBytes <= 0 {
		return fmt.Errorf("exec.max_output_bytes must be > 0")
	}
	if c.Exec.MaxTaskLogBytes <= 0 {
		return fmt.Errorf("exec.max_task_log_bytes must be > 0")
	}

	// Refuse to expose the device without a token on non-loopback binds.
	if c.Server.Mode == "http" && c.IsNonLoopback() && !c.Auth.Require && c.Auth.Token == "" {
		return fmt.Errorf("refusing to bind %s without auth.token: set TERMUX_MCP_AUTH_TOKEN", c.HTTPAddr())
	}

	// Shell tool is allowlist-only: enabling it requires at least one pattern.
	if c.Exec.ShellAllowed && len(c.Exec.ShellAllowPatterns) == 0 {
		return fmt.Errorf("exec.shell_allowed=true requires at least one exec.shell_allow_pattern (use [\"*\"] to allow all)")
	}

	return nil
}

// HTTPAddr returns host:port for the HTTP mode.
func (c *Config) HTTPAddr() string {
	return net.JoinHostPort(c.Server.Bind, strconv.Itoa(c.Server.Port))
}

// IsNonLoopback reports whether the configured bind address is not loopback.
func (c *Config) IsNonLoopback() bool {
	h := c.Server.Bind
	return h != "" && h != "127.0.0.1" && h != "::1" && h != "localhost"
}

// Sanitized returns a copy with secrets removed, safe for logging/status.
func (c *Config) Sanitized() *Config {
	cp := *c
	cp.Auth.Token = ""
	return &cp
}
