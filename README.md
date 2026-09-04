# termux-mcp

A MCP to connect Termux to any AI apps/agents of your choice like Mobile Agent ChatGPT, Claude, or any other

## Demo

[![Termux MCP demo](https://img.youtube.com/vi/nLKPv9LPL1Y/maxresdefault.jpg)](https://youtu.be/nLKPv9LPL1Y?si=myEdb6Akhl9OaUEH)


## Features

- **9 tool modules** — battery, SMS, camera, clipboard, files, UI automation,
  shell (allowlist-only), and more (see the table below)
- **Default-deny security** — sensitive and dangerous tools are opt-in;
  the shell tool is allowlist-only; the server refuses to bind a non-loopback
  address without an auth token
- **Sandboxed filesystem** — file tools resolve symlinks and reject anything
  outside `tools.file_roots`
- **Audit trail** — JSONL log of every tool call with argument redaction
- **Detached background tasks** — long-running jobs survive server restarts
- **Supervised service** — integrates with `termux-services` and Termux:Boot

## Requirements

- An Android device with [Termux](https://f-droid.org/packages/com.termux/)
  installed (from F-Droid)
- The **Termux:API** app from the same source (F-Droid)
- Go ≥ 1.25 only when building from source (the prebuilt install needs no Go)

## One-command install

On the device, inside Termux:

```bash
curl -fsSL https://github.com/TecnicalBot/termux-mcp/raw/main/scripts/install.sh | bash
```

## Manual quick start

```bash
# In Termux:
pkg install -y golang termux-api
go mod tidy
go build -o "$PREFIX/bin/termux-mcp" ./cmd/termux-mcp

termux-mcp doctor        # check environment
termux-mcp               # serve over stdio (default)
```

After installing a released build, update it in place with:

```bash
termux-mcp update
# Or install a specific release:
termux-mcp update --version v0.2.0
```

The updater downloads the Android arm64 release atomically and restarts the
supervised `termux-mcp` service when `termux-services` is available.

## Connecting a client

### stdio (local, on-device agents)

```json
{
  "mcpServers": {
    "termux": {
      "command": "termux-mcp",
      "args": ["serve", "stdio"]
    }
  }
}
```

### LAN (Streamable HTTP)

```bash
termux-mcp token new --write
termux-mcp serve http --config $PREFIX/var/lib/termux-mcp/config.yaml
# => http://192.168.x.x:3000/mcp
```

```json
{
  "mcpServers": {
    "termux": {
      "url": "http://192.168.1.20:3000/mcp",
      "headers": { "Authorization": "Bearer YOUR_TOKEN" }
    }
  }
}
```

### Remote (local OR cloudflared quick tunnel)

```bash
termux-mcp serve http &          # loopback is enough
termux-mcp tunnel start          # prints https://xxx.trycloudflare.com
```

See [Remote access](docs/REMOTE_ACCESS.md) for full client configs, including
stdio-only hosts via `mcp-remote`.

## Tool modules

| Module               | Tier          | Tools                                                                                                         |
| -------------------- | ------------- | ------------------------------------------------------------------------------------------------------------- |
| A. Device & System   | safe          | battery, device info, storage, screen size, brightness, volume, vibrate, flashlight, wifi, telephony, sensors |
| B. Communication     | sensitive\*   | sms list/send, contacts, call log, calls, notifications, toasts                                               |
| C. Media             | sensitive\*   | camera photo/info, audio record, media player, tts/stt                                                        |
| D. Clipboard & Input | safe          | clipboard get/set, open url, open app                                                                         |
| E. Files             | sensitive\*   | list/read/write/download/share/media-scan (sandboxed)                                                         |
| F. UI Automation     | dangerous\*\* | screenshot, ui dump, tap, swipe, text, keyevents                                                              |
| G. Shell & Tasks     | safe          | execute_command (launch detached task, optional workdir), task_status, task_log                                          |
| H. Utility           | safe          | ping, server_status, get_config, list_tools                                                                   |
| I. Task control      | dangerous\*\* | stop_task, task_delete (terminate/delete running tasks)                                                        |

\* opt-in via `tools.enable_sensitive: true` \
\*\* opt-in via `tools.enable_dangerous: true` (and allowlists)

## Configuration

Copy `config.example.yaml` to `$PREFIX/var/lib/termux-mcp/config.yaml` and
`chmod 600`. Every key can be overridden with a `TERMUX_MCP_*` environment
variable. Full reference: [Configuration](docs/CONFIGURATION.md).

```yaml
server:
  mode: stdio # stdio | http
  bind: 127.0.0.1 # non-loopback binds REQUIRE auth.token
  port: 3000

auth:
  token: "" # generate: termux-mcp token new --write
  require: false

tools:
  enable_sensitive: false # modules B, C, E
  enable_dangerous: false # modules F, I

exec:
  default_timeout_seconds: 30
  shell_allowed: false # allowlist-only when enabled
  shell_allow_patterns: [] # e.g. ["^ls ", "^df "]
```

## Security

- Default-deny tool tiers; nothing sensitive is exposed unless you opt in
- Non-loopback binds and tunnels refuse to start without an auth token
- Bearer tokens compared in constant time; config files written `0600`
- File access is sandboxed to `tools.file_roots` with symlink resolution
- Every tool call lands in a JSONL audit log with secrets redacted

Read the full model: [Security](docs/SECURITY.md).

## Development

```bash
go build ./...
go vet ./...
go test ./...        # unit tests use sh-based command fixtures; no device required
```

## Repository layout

```
cmd/termux-mcp        CLI entrypoint
internal/config       YAML config + env overrides + validation + discovery
internal/registry     tool catalog, tiers, tools/list filtering
internal/exec         safe exec wrapper (timeout, caps, retry-once)
internal/audit        JSONL audit log with arg redaction
internal/auth         constant-time bearer token middleware
internal/server       stdio + Streamable HTTP wiring
internal/tools/*      the 9 tool modules
internal/tunnel       cloudflared quick tunnel
scripts/              install.sh (one-command installer), boot.sh (boot entry)
docs/                 configuration, remote access, security
```

## Docs

- [Configuration](docs/CONFIGURATION.md)
- [Remote access (tunnels + auth)](docs/REMOTE_ACCESS.md)
- [Security model](docs/SECURITY.md)

## License

MIT
