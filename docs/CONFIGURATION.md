# Configuration

The server reads `config.yaml` (default locations, in order):
`TERMUX_MCP_CONFIG`, `$PREFIX/var/lib/termux-mcp/config.yaml`, or the current
directory. Copy `config.example.yaml` and `chmod 600`.

## Reference

| Key | Default | Description |
|---|---|---|
| `server.mode` | `stdio` | `stdio` or `http` |
| `server.bind` | `127.0.0.1` | Bind address (non-loopback requires a token) |
| `server.port` | `3000` | HTTP port |
| `server.max_body_bytes` | `1048576` | Request body cap |
| `server.max_sessions` | `32` | Concurrent MCP sessions |
| `auth.token` | *(empty)* | Bearer token; generate with `termux-mcp token new --write` |
| `auth.require` | `false` | Enforce the token even on loopback |
| `tools.enable_sensitive` | `false` | Enable modules B, C, E |
| `tools.enable_dangerous` | `false` | Enable modules F, G |
| `tools.allow` / `tools.deny` | `[]` | Per-tool overrides |
| `tools.file_roots` | `$HOME`, `/sdcard` | Filesystem sandbox roots |
| `exec.default_timeout_seconds` | `30` | Per-command timeout |
| `exec.max_output_bytes` | `1048576` | Per-command output cap |
| `exec.shell_allowed` | `true` | Enable `execute_command` |
| `exec.shell_allow_patterns` | `[".*"]` | Allowlist regexes (wildcard by default) |
| `exec.shell_deny_patterns` | `[]` | Denylist regexes |
| `audit.enabled` | `true` | Write `audit.jsonl` |
| `audit.dir` | `$PREFIX/var/lib/termux-mcp` | Audit log directory |
| `logging.level` | `info` | debug/info/warn/error |

## Environment overrides

Every key maps to `TERMUX_MCP_*`:

```bash
export TERMUX_MCP_SERVER_MODE=http
export TERMUX_MCP_SERVER_BIND=0.0.0.0
export TERMUX_MCP_AUTH_TOKEN="$(termux-mcp token new)"
export TERMUX_MCP_TOOLS_ENABLE_SENSITIVE=true
```

Environment beats file values. See `internal/config/config.go` for the full list.

## Filesystem sandbox (Module E)

File tools only operate inside `tools.file_roots` (defaults: `$HOME` and
`/sdcard`). Paths are made absolute, symlinks are resolved, and escape
attempts are rejected.
