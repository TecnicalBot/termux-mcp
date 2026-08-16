// Package clipboard implements Module D: clipboard, URL and app launching
// tools (safe tier).
package clipboard

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
)

// All returns the clipboard module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("get_clipboard",
			mcp.WithDescription("Read the clipboard via termux-clipboard-get.")),
			Handler: getClipboard(k),
			Meta:    registry.Meta{Module: "clipboard", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("set_clipboard",
			mcp.WithDescription("Write text to the clipboard via termux-clipboard-set."),
			mcp.WithString("text", mcp.Required(), mcp.Description("Text to copy"))),
			Handler: setClipboard(k),
			Meta:    registry.Meta{Module: "clipboard", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("open_url",
			mcp.WithDescription("Open a URL in the default browser via termux-open-url."),
			mcp.WithString("url", mcp.Required(), mcp.Description("http(s) URL to open"))),
			Handler: openURL(k),
			Meta:    registry.Meta{Module: "clipboard", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("open_app",
			mcp.WithDescription("Launch an installed app by package name (am start). Requires android-tools."),
			mcp.WithString("package", mcp.Required(), mcp.Description("Android package name, e.g. com.whatsapp"))),
			Handler: openApp(k),
			Meta:    registry.Meta{Module: "clipboard", Tier: registry.TierSafe}},
	}
}

func getClipboard(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := k.Run(ctx, "termux-clipboard-get", nil, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-clipboard-get exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("%s", strings.TrimRight(res.Stdout, "\n")), nil
	}
}

func setClipboard(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := kit.RequireStr(req, "text")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		res, err := k.Run(ctx, "termux-clipboard-set", []string{text}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-clipboard-set exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("clipboard set (%d chars)", len(text)), nil
	}
}

func openURL(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		url, err := kit.RequireStr(req, "url")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return kit.ResultError("url must start with http:// or https://"), nil
		}
		res, err := k.Run(ctx, "termux-open-url", []string{url}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-open-url exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("opened %s", url), nil
	}
}

func openApp(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkg, err := kit.RequireStr(req, "package")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{
			"start", "-a", "android.intent.action.MAIN",
			"-c", "android.intent.category.LAUNCHER",
			"-p", pkg,
		}
		res, err := k.Run(ctx, "am", args, 0)
		if err != nil {
			return kit.ResultError("%v (install android-tools)", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("am exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("launched %s", pkg), nil
	}
}
