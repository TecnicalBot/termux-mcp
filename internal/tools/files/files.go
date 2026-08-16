// Package files implements Module E: filesystem tools (sensitive tier) with
// a path sandbox restricting access to configured roots.
package files

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/config"
	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
)

// All returns the files module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("list_directory",
			mcp.WithDescription("List a directory inside an allowed file root."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Directory path"))),
			Handler: listDir(k),
			Meta:    registry.Meta{Module: "files", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("read_file",
			mcp.WithDescription("Read a text file inside an allowed file root."),
			mcp.WithString("path", mcp.Required(), mcp.Description("File path")),
			mcp.WithNumber("max_chars", mcp.Description("Max characters to return (default 100000)"), mcp.Min(1))),
			Handler: readFile(k),
			Meta:    registry.Meta{Module: "files", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("write_file",
			mcp.WithDescription("Write text to a file inside an allowed file root."),
			mcp.WithString("path", mcp.Required(), mcp.Description("File path")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Content to write"))),
			Handler: writeFile(k),
			Meta:    registry.Meta{Module: "files", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("download_file",
			mcp.WithDescription("Download a URL to a file inside an allowed root via termux-download."),
			mcp.WithString("url", mcp.Required(), mcp.Description("URL to download")),
			mcp.WithString("output", mcp.Required(), mcp.Description("Output file path (inside an allowed root)")),
			mcp.WithString("title", mcp.Description("Download title")),
			mcp.WithString("description", mcp.Description("Download description"))),
			Handler: downloadFile(k),
			Meta:    registry.Meta{Module: "files", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("share_file",
			mcp.WithDescription("Open the Android share sheet for a file via termux-share."),
			mcp.WithString("path", mcp.Required(), mcp.Description("File path"))),
			Handler: shareFile(k),
			Meta:    registry.Meta{Module: "files", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("media_scan",
			mcp.WithDescription("Scan a file or directory so it appears in the media library via termux-media-scan."),
			mcp.WithString("path", mcp.Required(), mcp.Description("File or directory path"))),
			Handler: mediaScan(k),
			Meta:    registry.Meta{Module: "files", Tier: registry.TierSensitive}},
	}
}

// Roots returns the configured sandbox roots, defaulting to the home dir
// and /sdcard when none are configured.
func Roots(tc *config.ToolsConfig) []string {
	if len(tc.FileRoots) > 0 {
		return tc.FileRoots
	}
	var roots []string
	if h, err := os.UserHomeDir(); err == nil {
		roots = append(roots, h)
	}
	roots = append(roots, "/sdcard")
	return roots
}

// Resolve cleans raw, makes it absolute, resolves existing symlinks, and
// verifies the result stays inside one of the allowed roots.
func Resolve(raw string, roots []string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("empty path")
	}
	p, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	for _, r := range roots {
		rr := filepath.Clean(r)
		if !filepath.IsAbs(rr) {
			if abs, err := filepath.Abs(rr); err == nil {
				rr = abs
			}
		}
		if rr == p || strings.HasPrefix(p, rr+string(filepath.Separator)) {
			return p, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed roots %v", raw, roots)
}

type dirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

func listDir(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := kit.RequireStr(req, "path")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := Resolve(raw, Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return kit.ResultError("read dir: %v", err), nil
		}
		out := make([]dirEntry, 0, len(entries))
		for _, e := range entries {
			info, ierr := e.Info()
			sz := int64(0)
			if ierr == nil {
				sz = info.Size()
			}
			out = append(out, dirEntry{Name: e.Name(), Path: filepath.Join(p, e.Name()), IsDir: e.IsDir(), Size: sz})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return kit.ResultJSON(map[string]any{"path": p, "count": len(out), "entries": out}), nil
	}
}

func readFile(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := kit.RequireStr(req, "path")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := Resolve(raw, Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return kit.ResultError("read file: %v", err), nil
		}
		max := int64(100000)
		if m := int64(math.Round(kit.NumArg(req, "max_chars"))); m > 0 {
			max = m
		}
		truncated := false
		if int64(len(data)) > max {
			data = data[:max]
			truncated = true
		}
		return kit.ResultJSON(map[string]any{
			"path":      p,
			"size":      len(data),
			"truncated": truncated,
			"content":   string(data),
		}), nil
	}
}

func writeFile(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := kit.RequireStr(req, "path")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		content, err := kit.RequireStr(req, "content")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := Resolve(raw, Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if int64(len(content)) > k.Cfg.Exec.MaxOutputBytes {
			return kit.ResultError("content too large (%d bytes, cap %d)", len(content), k.Cfg.Exec.MaxOutputBytes), nil
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return kit.ResultError("write file: %v", err), nil
		}
		return kit.ResultJSON(map[string]any{"path": p, "bytes_written": len(content)}), nil
	}
}

func downloadFile(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		url, err := kit.RequireStr(req, "url")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		out, err := kit.RequireStr(req, "output")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := Resolve(out, Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{"-o", p}
		if t := kit.StrArg(req, "title"); t != "" {
			args = append(args, "-t", t)
		}
		if d := kit.StrArg(req, "description"); d != "" {
			args = append(args, "-d", d)
		}
		args = append(args, url)
		res, err := k.Run(ctx, "termux-download", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-download exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("downloaded to %s", p), nil
	}
}

func shareFile(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := kit.RequireStr(req, "path")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := Resolve(raw, Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		res, err := k.Run(ctx, "termux-share", []string{p}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-share exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("share sheet opened for %s", p), nil
	}
}

func mediaScan(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := kit.RequireStr(req, "path")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := Resolve(raw, Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		res, err := k.Run(ctx, "termux-media-scan", []string{p}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-media-scan exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("media scan queued for %s", p), nil
	}
}
