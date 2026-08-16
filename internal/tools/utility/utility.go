// Package utility implements Module H: liveness, status and introspection
// tools (safe tier).
package utility

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
	"termux-mcp/internal/version"
)

var started = time.Now()

// All returns the utility module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("ping",
			mcp.WithDescription("Liveness check; returns pong.")),
			Handler: ping(k),
			Meta:    registry.Meta{Module: "utility", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("server_status",
			mcp.WithDescription("Server version, uptime, transport mode and enabled tool count.")),
			Handler: serverStatus(k),
			Meta:    registry.Meta{Module: "utility", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_config",
			mcp.WithDescription("Show the sanitized server configuration (secrets removed).")),
			Handler: getConfig(k),
			Meta:    registry.Meta{Module: "utility", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("list_tools",
			mcp.WithDescription("List all registered tools with their module, tier and enabled state.")),
			Handler: listTools(k),
			Meta:    registry.Meta{Module: "utility", Tier: registry.TierSafe}},
	}
}

func ping(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return kit.ResultJSON(map[string]any{
			"pong":  true,
			"time":  time.Now().UTC().Format(time.RFC3339),
			"alive": true,
		}), nil
	}
}

func serverStatus(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tools := k.Reg.Names(&k.Cfg.Tools)
		return kit.ResultJSON(map[string]any{
			"name":           version.Name,
			"version":        version.Version,
			"mode":           k.Cfg.Server.Mode,
			"uptime_seconds": int64(time.Since(started).Seconds()),
			"tools_enabled":  len(tools),
			"tools":          tools,
		}), nil
	}
}

func getConfig(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return kit.ResultJSON(k.Cfg.Sanitized()), nil
	}
}

func listTools(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		type row struct {
			Name    string `json:"name"`
			Module  string `json:"module"`
			Tier    string `json:"tier"`
			Enabled bool   `json:"enabled"`
		}
		var rows []row
		for _, t := range k.Reg.All() {
			rows = append(rows, row{
				Name:    t.Def.Name,
				Module:  t.Meta.Module,
				Tier:    string(t.Meta.Tier),
				Enabled: registry.Enabled(t.Def.Name, t.Meta.Tier, &k.Cfg.Tools),
			})
		}
		return kit.ResultJSON(rows), nil
	}
}
