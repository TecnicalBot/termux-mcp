// Package shell implements Module G: arbitrary shell execution (dangerous
// tier, disabled by default, allowlist-only).
package shell

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/config"
	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
)

const shellTimeout = 60 * time.Second

// All returns the shell module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("execute_command",
			mcp.WithDescription("Run a shell command. Disabled by default: requires exec.shell_allowed=true and at least one allow pattern in config."),
			mcp.WithString("command", mcp.Required(), mcp.Description("Shell command to run"))),
			Handler: execute(k),
			Meta:    registry.Meta{Module: "shell", Tier: registry.TierDangerous, Timeout: shellTimeout}},
	}
}

func execute(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cmd, err := kit.RequireStr(req, "command")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if !k.Cfg.Exec.ShellAllowed {
			return kit.ResultError("shell tool is disabled: set exec.shell_allowed=true with allow patterns in config.yaml"), nil
		}
		if !Allowed(cmd, &k.Cfg.Exec) {
			return kit.ResultError("command rejected by shell allow/deny policy"), nil
		}
		res, err := k.Run(ctx, "sh", []string{"-c", cmd}, shellTimeout)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("%s", strings.TrimRight(res.Stdout, "\n")), nil
	}
}

// Allowed applies the allow/deny regex policy. With no allow patterns the
// command is rejected (allowlist-only), unless "*" allows everything.
// Exported so the tasks module applies the exact same policy to run_task.
func Allowed(cmd string, ec *config.ExecConfig) bool {
	for _, d := range ec.ShellDenyPatterns {
		if ok, _ := regexp.MatchString(d, cmd); ok {
			return false
		}
	}
	if len(ec.ShellAllowPatterns) == 0 {
		return false
	}
	for _, a := range ec.ShellAllowPatterns {
		if a == "*" {
			return true
		}
		if ok, _ := regexp.MatchString(a, cmd); ok {
			return true
		}
	}
	return false
}
