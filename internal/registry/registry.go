// Package registry holds the declarative tool catalog: definitions, handlers,
// permission tiers, and config-driven filtering for tools/list.
package registry

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/config"
)

// Tier is a permission class for a tool.
type Tier string

const (
	TierSafe      Tier = "safe"
	TierSensitive Tier = "sensitive"
	TierDangerous Tier = "dangerous"
)

// Meta carries operational metadata for a tool.
type Meta struct {
	Module  string
	Tier    Tier
	Timeout time.Duration
}

// Tool binds an MCP tool definition to its handler and metadata.
type Tool struct {
	Def     mcp.Tool
	Handler server.ToolHandlerFunc
	Meta    Meta
}

// Registry is a concurrency-safe tool catalog.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool; duplicate names are rejected.
func (r *Registry) Register(def mcp.Tool, h server.ToolHandlerFunc, m Meta) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[def.Name]; ok {
		return fmt.Errorf("duplicate tool %q", def.Name)
	}
	if m.Timeout <= 0 {
		m.Timeout = 120 * time.Second
	}
	r.tools[def.Name] = Tool{Def: def, Handler: h, Meta: m}
	return nil
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns every registered tool sorted by name.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Def.Name < out[j].Def.Name })
	return out
}

// Enabled reports whether a tool's tier is enabled plus allow/deny rules.
func Enabled(name string, tier Tier, tc *config.ToolsConfig) bool {
	switch tier {
	case TierSafe:
	case TierSensitive:
		if !tc.EnableSensitive {
			return false
		}
	case TierDangerous:
		if !tc.EnableDangerous {
			return false
		}
	}
	for _, d := range tc.Deny {
		if d == name {
			return false
		}
	}
	if len(tc.Allow) > 0 {
		for _, a := range tc.Allow {
			if a == name {
				return true
			}
		}
		return false
	}
	return true
}

// Filtered returns tools enabled under tc, sorted by name.
func (r *Registry) Filtered(tc *config.ToolsConfig) []Tool {
	all := r.All()
	out := make([]Tool, 0, len(all))
	for _, t := range all {
		if Enabled(t.Def.Name, t.Meta.Tier, tc) {
			out = append(out, t)
		}
	}
	return out
}

// ToServerTools converts the filtered catalog into mcp-go ServerTool entries.
func (r *Registry) ToServerTools(tc *config.ToolsConfig) []server.ServerTool {
	tools := r.Filtered(tc)
	out := make([]server.ServerTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, server.ServerTool{Tool: t.Def, Handler: t.Handler})
	}
	return out
}

// Names returns the sorted names of filtered tools.
func (r *Registry) Names(tc *config.ToolsConfig) []string {
	tools := r.Filtered(tc)
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Def.Name)
	}
	return out
}
