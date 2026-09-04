// Package shell implements Module G: shell execution (safe tier, enabled by
// default; runtime gating via exec.shell_allowed and allow/deny patterns).
package shell

import (
	"context"
	"regexp"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/config"
	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
)

const shellTimeout = 15 * time.Second

// All returns the shell module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("execute_command",
			mcp.WithDescription("Start a shell command as a detached background task and return its task ID immediately. Stream its live output with the terminal stream endpoint or inspect it with task_status and task_log."),
			mcp.WithString("command", mcp.Required(), mcp.Description("Shell command to run")),
			mcp.WithString("workdir", mcp.Description("Working directory for the command (default: the server's home)"))),
			Handler: execute(k),
			Meta:    registry.Meta{Module: "shell", Tier: registry.TierSafe, Timeout: shellTimeout}},
	}
}

func execute(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cmd, err := kit.RequireStr(req, "command")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if k.Tasks == nil {
			return kit.ResultError("task manager unavailable"), nil
		}
		if !k.Cfg.Exec.ShellAllowed {
			return kit.ResultError("shell tool is disabled: set exec.shell_allowed=true with allow patterns in config.yaml"), nil
		}
		if !Allowed(cmd, &k.Cfg.Exec) {
			return kit.ResultError("command rejected by shell allow/deny policy"), nil
		}
		info, err := k.Tasks.Start(cmd, kit.StrArg(req, "workdir"))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultJSON(info), nil
	}
}

// Allowed applies the allow/deny regex policy. With no allow patterns the
// command is rejected (allowlist-only), unless "*" allows everything.
// Exported so other modules can apply the exact same shell policy.
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
