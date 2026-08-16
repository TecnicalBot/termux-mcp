package registry

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"termux-mcp/internal/config"
)

func mustHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}

func register(t *testing.T, r *Registry, name string, tier Tier) {
	t.Helper()
	if err := r.Register(mcp.NewTool(name), mustHandler, Meta{Module: "test", Tier: tier}); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := New()
	register(t, r, "dup", TierSafe)
	if err := r.Register(mcp.NewTool("dup"), mustHandler, Meta{Module: "test", Tier: TierSafe}); err == nil {
		t.Fatal("duplicate registration must fail")
	}
}

func TestFilterTiers(t *testing.T) {
	r := New()
	register(t, r, "safe_tool", TierSafe)
	register(t, r, "sens_tool", TierSensitive)
	register(t, r, "danger_tool", TierDangerous)

	tc := config.Default().Tools

	names := r.Names(&tc)
	if len(names) != 1 || names[0] != "safe_tool" {
		t.Fatalf("default filter = %v, want only safe_tool", names)
	}

	tc.EnableSensitive = true
	names = r.Names(&tc)
	if len(names) != 2 {
		t.Fatalf("sensitive filter = %v", names)
	}

	tc.EnableDangerous = true
	names = r.Names(&tc)
	if len(names) != 3 {
		t.Fatalf("dangerous filter = %v", names)
	}
}

func TestAllowDeny(t *testing.T) {
	r := New()
	register(t, r, "a", TierSafe)
	register(t, r, "b", TierSafe)

	tc := config.Default().Tools
	tc.Allow = []string{"a"}
	if names := r.Names(&tc); len(names) != 1 || names[0] != "a" {
		t.Fatalf("allow filter = %v", names)
	}

	tc = config.Default().Tools
	tc.Deny = []string{"b"}
	if names := r.Names(&tc); len(names) != 1 || names[0] != "a" {
		t.Fatalf("deny filter = %v", names)
	}
}

func TestEnabledFn(t *testing.T) {
	tc := config.Default().Tools
	if !Enabled("x", TierSafe, &tc) {
		t.Fatal("safe must be enabled by default")
	}
	if Enabled("x", TierSensitive, &tc) {
		t.Fatal("sensitive must be off by default")
	}
	if Enabled("x", TierDangerous, &tc) {
		t.Fatal("dangerous must be off by default")
	}
}
