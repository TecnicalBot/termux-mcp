// Package version holds the server name/version constants.
package version

const (
	// Name is the MCP server name advertised in initialize.
	Name = "termux-mcp"
)

// Version is the semantic version of the server, kept in sync with the git
// tag format (vX.Y.Z). It is a variable so release builds can stamp it via:
//
//	go build -ldflags "-X termux-mcp/internal/version.Version=v1.2.3" ./cmd/termux-mcp
var Version = "v0.1.0"
