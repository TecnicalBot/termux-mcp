# Remote access

Three modes:

1. **Local stdio** — `termux-mcp serve stdio` (default). Used by on-device agents.
2. **LAN HTTP** — `termux-mcp serve http` with `server.bind: 0.0.0.0` and a token.
3. **Remote HTTPS** — `termux-mcp tunnel start` (cloudflared quick tunnel).

## 1. LAN

```bash
termux-mcp token new --write
termux-mcp serve http --config $PREFIX/var/lib/termux-mcp/config.yaml
# => http://192.168.x.x:3000/mcp
```

Client config (`claude_desktop_config.json` / MCP settings):

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

## 2. Remote via cloudflared

```bash
# The tunnel exposes the device publicly, so auth MUST be enforced:
# set auth.require: true (and auth.token) in the config first.
termux-mcp serve http &                    # loopback is enough
termux-mcp tunnel start                    # prints https://xxx.trycloudflare.com
```

`termux-mcp tunnel start` refuses to run unless `auth.token` is set **and**
`auth.require: true` — tunnel traffic arrives from the local cloudflared
proxy (loopback), so without `require: true` the token would not be checked.

Client config:

```json
{
  "mcpServers": {
    "termux": {
      "url": "https://xxx.trycloudflare.com/mcp",
      "headers": { "Authorization": "Bearer YOUR_TOKEN" }
    }
  }
}
```

Any non-loopback bind **requires** `auth.token`; the server refuses to start
without one. With `auth.require: false` (the default), on-device/loopback
clients skip the token — set `auth.require: true` when serving remotely.

## 3. Hosts that only support stdio

Use the `mcp-remote` bridge on the desktop machine:

```bash
npx -y mcp-remote --transport http-first https://xxx.trycloudflare.com/mcp
# then point your stdio host at `npx mcp-remote ...`
```

## Keeping it alive

- `termux-services`: `sv up termux-mcp` (installed by `scripts/install.sh`)
- Termux:Boot: `~/.termux/boot/start-mcp.sh`
- `termux-wake-lock` is taken on start; exempt Termux from battery optimization.
