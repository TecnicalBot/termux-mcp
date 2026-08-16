# Security model

## Principles

- **Default-deny.** Sensitive and dangerous tool tiers are off until explicitly
  enabled. The shell tool is allowlist-only and disabled by default.
- **Never expose without a token.** Binding a non-loopback address without
  `auth.token` is a startup error, not a warning. Tunnels additionally require
  `auth.require: true` (tunnel traffic arrives from loopback, which is exempt
  by default); `termux-mcp tunnel start` refuses to run otherwise.
- **Least privilege per path.** File tools run inside a resolved-path sandbox;
  every command runs with a timeout and an output cap.

## Mechanisms

| Concern | Control |
|---|---|
| Transport auth | Bearer token, constant-time comparison (`internal/auth`) |
| Tool gating | Permission tiers + per-tool allow/deny (`internal/registry`) |
| Shell | Regex allow/deny; must be explicitly enabled (`exec.shell_allowed`) |
| Files | Path sandbox under `tools.file_roots`, symlink resolution |
| Command safety | Timeouts (default 30s), output caps (1 MiB), retry-once JSON validation |
| Observability | JSONL audit log with argument redaction (`internal/audit`) |
| Secrets | Never logged; config `chmod 600`; `termux-mcp get_config` is sanitized |
| DoS | Max body size, max sessions, rate-limit-ready middleware |

## Audit redaction

Arguments named like `text`, `content`, `message`, `token`, `password`,
`secret`, `key`, `pin`, `otp`, `authorization`, `body` are replaced with
`[REDACTED]` in the audit trail; values over 200 chars are truncated.

## Remote exposure checklist

1. `termux-mcp token new --write` and keep the config file private.
2. Keep `tools.enable_sensitive` and `tools.enable_dangerous` **off** unless
   needed; prefer narrow `tools.allow` lists.
3. Restart the server after any config change.
4. Check `audit.jsonl` periodically.
5. OAuth 2.1 + PKCE is a planned future upgrade for public deployments.

## Reporting

Any exposed device on a public tunnel without a token is a misconfiguration —
the server refuses to start in that state. If you find a bug, open an issue in
the repository.
