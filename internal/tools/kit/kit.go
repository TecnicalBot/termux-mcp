// Package kit provides shared dependencies and helpers for tool handlers.
package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"termux-mcp/internal/audit"
	"termux-mcp/internal/config"
	"termux-mcp/internal/exec"
	"termux-mcp/internal/registry"
	"termux-mcp/internal/tasks"
)

// Kit bundles what every tool handler needs.
type Kit struct {
	Cfg   *config.Config
	Audit *audit.Logger
	Reg   *registry.Registry
	Tasks *tasks.Manager // background task manager (tasks module)
}

// Run executes a command with the configured defaults (timeout, output cap).
func (k *Kit) Run(ctx context.Context, name string, args []string, timeout time.Duration) (*exec.Result, error) {
	if timeout <= 0 {
		timeout = time.Duration(k.Cfg.Exec.DefaultTimeoutSeconds) * time.Second
	}
	return exec.Run(ctx, name, args, exec.Config{
		Timeout:   timeout,
		MaxOutput: k.Cfg.Exec.MaxOutputBytes,
	})
}

// RunJSON runs a termux-api-style command, validates its JSON stdout, and
// retries once on transient garbage (placeholder output).
func (k *Kit) RunJSON(ctx context.Context, name string, args []string, timeout time.Duration, retry bool) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = time.Duration(k.Cfg.Exec.DefaultTimeoutSeconds) * time.Second
	}
	raw, _, err := exec.RunJSON(ctx, name, args, exec.Config{
		Timeout:   timeout,
		MaxOutput: k.Cfg.Exec.MaxOutputBytes,
	}, retry)
	return raw, err
}

// --- argument helpers ---

// Args returns the call's arguments as a map (empty when absent). In mcp-go
// v0.57+ Params.Arguments is typed any, so it must be asserted here.
func Args(r mcp.CallToolRequest) map[string]any {
	m, _ := r.Params.Arguments.(map[string]any)
	return m
}

// StrArg returns a string argument (empty when absent).
func StrArg(r mcp.CallToolRequest, name string) string {
	s, _ := Args(r)[name].(string)
	return s
}

// NumArg returns a number argument (0 when absent).
func NumArg(r mcp.CallToolRequest, name string) float64 {
	n, _ := Args(r)[name].(float64)
	return n
}

// BoolArg returns a boolean argument (false when absent).
func BoolArg(r mcp.CallToolRequest, name string) bool {
	b, _ := Args(r)[name].(bool)
	return b
}

// RequireStr returns a non-empty string argument or an error.
func RequireStr(r mcp.CallToolRequest, name string) (string, error) {
	s := StrArg(r, name)
	if s == "" {
		return "", fmt.Errorf("missing required argument %q", name)
	}
	return s, nil
}

// --- result helpers ---

// ResultText formats a plain-text tool result.
func ResultText(format string, a ...any) *mcp.CallToolResult {
	return mcp.NewToolResultText(fmt.Sprintf(format, a...))
}

// ResultJSON marshals v as indented JSON into a tool result.
func ResultJSON(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ResultError("marshal result: %v", err)
	}
	return mcp.NewToolResultText(string(b))
}

// ResultError builds an error result (shown to the client as a failure).
func ResultError(format string, a ...any) *mcp.CallToolResult {
	return mcp.NewToolResultErrorf(format, a...)
}
