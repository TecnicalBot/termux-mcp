// Package server wires the registry into an mcp-go MCPServer and exposes
// stdio and Streamable HTTP entrypoints with audit + auth middleware.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpgo "github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/audit"
	"termux-mcp/internal/auth"
	"termux-mcp/internal/config"
	"termux-mcp/internal/registry"
	"termux-mcp/internal/tasks"
	"termux-mcp/internal/version"
)

var started = time.Now()

// BuildMCP constructs the MCPServer from the filtered registry, wrapping
// every handler with audit logging.
func BuildMCP(reg *registry.Registry, cfg *config.Config, al *audit.Logger) *mcpgo.MCPServer {
	srv := mcpgo.NewMCPServer(version.Name, version.Version,
		mcpgo.WithInstructions("Termux MCP server. Grants access to device, SMS, media, files, clipboard and (opt-in) shell/UI tools."),
	)
	var tools []mcpgo.ServerTool
	for _, t := range reg.Filtered(&cfg.Tools) {
		h := t.Handler
		wrapped := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			res, err := h(ctx, req)
			args, _ := req.Params.Arguments.(map[string]any)
			e := audit.Entry{
				Time:       time.Now(),
				Caller:     "mcp",
				Tool:       req.Params.Name,
				Args:       args,
				DurationMS: time.Since(start).Milliseconds(),
			}
			if err != nil {
				e.Error = err.Error()
			} else if res != nil && res.IsError {
				e.Error = "tool_error"
			}
			_ = al.Log(e)
			return res, err
		}
		tools = append(tools, mcpgo.ServerTool{Tool: t.Def, Handler: wrapped})
	}
	srv.AddTools(tools...)
	return srv
}

// RunStdio serves MCP over stdin/stdout (local mode).
func RunStdio(s *mcpgo.MCPServer) error {
	return mcpgo.ServeStdio(s)
}

// RunHTTP serves MCP over Streamable HTTP behind auth and logging middleware.
func RunHTTP(s *mcpgo.MCPServer, cfg *config.Config, mgr *tasks.Manager) error {
	mcpHandler := mcpgo.NewStreamableHTTPServer(s)

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/terminal/stream", TerminalStreamHandler(mgr, mgr.Dir()))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":         "ok",
			"name":           version.Name,
			"version":        version.Version,
			"uptime_seconds": int64(time.Since(started).Seconds()),
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	var h http.Handler = mux
	h = withMaxBody(h, cfg.Server.MaxBodyBytes)
	h = auth.RequireBearer(h, cfg.Auth.Token, cfg.Auth.Require)
	h = withLogging(h)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr(),
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("http server listening", "addr", cfg.HTTPAddr())
	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

func withMaxBody(next http.Handler, max int64) http.Handler {
	if max <= 0 {
		max = 1 << 20
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, max)
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr, "ms", time.Since(start).Milliseconds())
	})
}
