// Package tasks implements Module I's MCP tools for long-running background
// jobs that outlive a single tool call: task_status and task_log (safe tier,
// enabled by default) plus stop_task and task_delete (dangerous tier, disabled
// by default). Tasks are launched via the shell module's execute_command.
package tasks

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
)

const moduleTimeout = 15 * time.Second

// All returns the tasks module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("task_status",
			mcp.WithDescription("Get the current state of a background task (running, finished, failed, stopped, interrupted), its PID, exit code and log paths."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID, e.g. t000001"))),
			Handler: taskStatus(k),
			Meta:    registry.Meta{Module: "tasks", Tier: registry.TierSafe, Timeout: moduleTimeout}},

		{Def: mcp.NewTool("task_log",
			mcp.WithDescription("Read the output of a background task (tail). stream: stdout, stderr or all (default all). tail_bytes bounds each stream (default 64 KiB, max is the configured per-stream log cap)."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID, e.g. t000001")),
			mcp.WithString("stream", mcp.Description("stdout | stderr | all (default all)")),
			mcp.WithNumber("tail_bytes", mcp.Description("Max bytes to return per stream (default 65536)"))),
			Handler: taskLog(k),
			Meta:    registry.Meta{Module: "tasks", Tier: registry.TierSafe, Timeout: moduleTimeout}},

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
