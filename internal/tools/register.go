// Package tools wires every tool module into the registry.
package tools

import (
	"termux-mcp/internal/audit"
	"termux-mcp/internal/config"
	"termux-mcp/internal/registry"
	"termux-mcp/internal/tasks"
	"termux-mcp/internal/tools/clipboard"
	"termux-mcp/internal/tools/communication"
	"termux-mcp/internal/tools/device"
	"termux-mcp/internal/tools/files"
	"termux-mcp/internal/tools/kit"
	"termux-mcp/internal/tools/media"
	"termux-mcp/internal/tools/shell"
	tasksmod "termux-mcp/internal/tools/tasks"
	"termux-mcp/internal/tools/ui"
	"termux-mcp/internal/tools/utility"
)

// RegisterAll registers every tool from all modules. Duplicate tool names
// across modules are treated as a startup error.
func RegisterAll(r *registry.Registry, cfg *config.Config, al *audit.Logger, mgr *tasks.Manager) error {
	k := &kit.Kit{
		Cfg:   cfg,
		Audit: al,
		Reg:   r,
		Tasks: mgr,
	}
	sets := [][]registry.Tool{
		device.All(k),
		communication.All(k),
		media.All(k),
		files.All(k),
		clipboard.All(k),
		utility.All(k),
		shell.All(k),
		tasksmod.All(k),
		ui.All(k),
	}
	for _, set := range sets {
		for _, t := range set {
			if err := r.Register(t.Def, t.Handler, t.Meta); err != nil {
				return err
			}
		}
	}
	return nil
}
