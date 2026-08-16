// Package tasks implements Module I's MCP tools: run_task, task_status,
// task_log, stop_task and task_delete for long-running background jobs that
// outlive a single tool call (dangerous tier, disabled by default).
package tasks

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
	"termux-mcp/internal/tools/shell"
)

const moduleTimeout = 15 * time.Second

// All returns the tasks module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("run_task",
			mcp.WithDescription("Run a shell command as a detached background task that keeps running after this call returns. Poll it with task_status and read output with task_log. Follows the same policy as execute_command: requires exec.shell_allowed=true and an allow pattern."),
			mcp.WithString("command", mcp.Required(), mcp.Description("Shell command to run in the background")),
			mcp.WithString("workdir", mcp.Description("Working directory for the task (default: the server's home)"))),
			Handler: runTask(k),
			Meta:    registry.Meta{Module: "tasks", Tier: registry.TierDangerous, Timeout: moduleTimeout}},

		{Def: mcp.NewTool("task_status",
			mcp.WithDescription("Get the current state of a background task (running, finished, failed, stopped, interrupted), its PID, exit code and log paths."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID, e.g. t000001"))),
			Handler: taskStatus(k),
			Meta:    registry.Meta{Module: "tasks", Tier: registry.TierDangerous, Timeout: moduleTimeout}},

		{Def: mcp.NewTool("task_log",
			mcp.WithDescription("Read the output of a background task (tail). stream: stdout, stderr or all (default all). tail_bytes bounds each stream (default 64 KiB, max is the configured per-stream log cap)."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID, e.g. t000001")),
			mcp.WithString("stream", mcp.Description("stdout | stderr | all (default all)")),
			mcp.WithNumber("tail_bytes", mcp.Description("Max bytes to return per stream (default 65536)"))),
			Handler: taskLog(k),
			Meta:    registry.Meta{Module: "tasks", Tier: registry.TierDangerous, Timeout: moduleTimeout}},

		{Def: mcp.NewTool("stop_task",
			mcp.WithDescription("Terminate a running background task: SIGTERM to its process group, escalated to SIGKILL after a grace period."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID, e.g. t000001"))),
			Handler: stopTask(k),
			Meta:    registry.Meta{Module: "tasks", Tier: registry.TierDangerous, Timeout: moduleTimeout}},

		{Def: mcp.NewTool("task_delete",
			mcp.WithDescription("Delete a finished task and its logs. Running tasks must be stopped first."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID, e.g. t000001"))),
			Handler: taskDelete(k),
			Meta:    registry.Meta{Module: "tasks", Tier: registry.TierDangerous, Timeout: moduleTimeout}},
	}
}

func runTask(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cmd, err := kit.RequireStr(req, "command")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if k.Tasks == nil {
			return kit.ResultError("task manager unavailable"), nil
		}
		if !k.Cfg.Exec.ShellAllowed {
			return kit.ResultError("run_task is disabled: set exec.shell_allowed=true with allow patterns in config.yaml"), nil
		}
		if !shell.Allowed(cmd, &k.Cfg.Exec) {
			return kit.ResultError("command rejected by shell allow/deny policy"), nil
		}
		info, err := k.Tasks.Start(cmd, kit.StrArg(req, "workdir"))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultJSON(info), nil
	}
}

func taskStatus(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := kit.RequireStr(req, "id")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		info, err := k.Tasks.Status(id)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultJSON(info), nil
	}
}

func taskLog(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := kit.RequireStr(req, "id")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		stream := kit.StrArg(req, "stream")
		tail := int64(kit.NumArg(req, "tail_bytes"))
		if tail < 0 {
			tail = 0
		}
		out, err := k.Tasks.Log(id, stream, tail)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultText("%s", out), nil
	}
}

func stopTask(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := kit.RequireStr(req, "id")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		info, err := k.Tasks.Stop(id)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultJSON(info), nil
	}
}

func taskDelete(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := kit.RequireStr(req, "id")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if err := k.Tasks.Delete(id); err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultText("task %s deleted", id), nil
	}
}
