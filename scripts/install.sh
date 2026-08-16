#!/data/data/com.termux/files/usr/bin/bash
# install.sh — one-command install of termux-mcp on Termux.
#
# Usage (on the device, inside Termux):
#
#   # 1) From a local checkout:
#   bash scripts/install.sh
#
#   # 2) Remote one-liner (replace URL with your repo):
#   curl -fsSL https://github.com/<OWNER>/termux-mcp/raw/main/scripts/install.sh | bash
#
# What it does (all idempotent):
#   1. Installs termux-api (optional: android-tools, cloudflared)
#   2. Grants storage access and checks the Termux:API bridge
#   3. Clones the repo if not already in a checkout (TERMUX_MCP_REPO)
#   4. Installs the binary to $PREFIX/bin/termux-mcp:
#        - preferred: download the prebuilt release matching your device's
#          architecture from GitHub Releases (no Go toolchain needed)
#        - fallback:  build from source (installs golang if needed)
#   5. Creates $PREFIX/var/lib/termux-mcp/config.yaml from the example
#   6. Generates and persists an auth token (printed once at the end)
#   7. Registers the supervised service + boot entry when termux-services
#      and/or Termux:Boot are present
#
# Env knobs:
#   TERMUX_MCP_REPO      git URL of the repo (defaults to the canonical
#                        GitHub repo; forks should export their own URL)
#   TERMUX_MCP_VERSION   release tag to install (default: latest release)
#   TERMUX_MCP_SOURCE    "yes" to always build from source
#   TERMUX_MCP_SERVICE   "yes" (default) to register the supervised service
#   TERMUX_MCP_BOOT      "yes" (default) to install the Termux:Boot entry
set -euo pipefail

log() { printf '\033[1;32m[install]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[install]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[install]\033[0m %s\n' "$*" >&2; exit 1; }

if [ -z "${PREFIX:-}" ]; then
  die "Not running inside Termux (\$PREFIX is unset). Run this script from the Termux app."
fi
if [ ! -x "$PREFIX/bin/pkg" ] && ! command -v pkg >/dev/null 2>&1; then
  die "pkg not found — this script must run inside Termux (pkg is Termux's package manager)."
fi

# --- 1. Packages -----------------------------------------------------------
log "Installing/verifying packages..."
pkg install -y termux-api || die "pkg install termux-api failed"
for opt in android-tools cloudflared; do
  if command -v "$opt" >/dev/null 2>&1; then
    log "$opt already installed"
  else
    log "Installing $opt (optional)..."
    pkg install -y "$opt" || warn "could not install $opt (optional)"
  fi
done

# --- 2. Storage + API bridge ------------------------------------------------
log "Granting storage access (follow the Android prompt if shown)..."
termux-setup-storage || warn "termux-setup-storage failed — run it manually if /sdcard access is needed"

log "Checking Termux:API bridge..."
out="$(termux-battery-status 2>&1 || true)"
if printf '%s' "$out" | grep -q '"level"'; then
  log "termux-battery-status OK"
else
  warn "termux-battery-status returned: $(printf '%s' "$out" | head -1)"
  warn "The Termux:API app must be installed from F-Droid (same store as Termux)"
  warn "and granted permissions in Settings > Apps > Termux:API."
fi

# --- 3. Source / repo resolution -------------------------------------------
REPO_DIR=""
if [ -f "$PWD/scripts/install.sh" ] && [ -f "$PWD/cmd/termux-mcp/main.go" ]; then
  REPO_DIR="$PWD"   # running from a checkout
  log "Using local checkout at $REPO_DIR"
else
  if [ -n "${TERMUX_MCP_REPO:-}" ]; then
    REPO_URL="$TERMUX_MCP_REPO"
  elif [ -d "$HOME/termux-mcp/.git" ]; then
    log "Using existing clone at $HOME/termux-mcp"
    REPO_DIR="$HOME/termux-mcp"
  else
    # Canonical repo so the remote one-liner works out of the box.
    # Forks: export TERMUX_MCP_REPO=<your fork url>.
    REPO_URL="https://github.com/TecnicalBot/termux-mcp.git"
  fi
  if [ -z "$REPO_DIR" ]; then
    git clone --depth 1 "$REPO_URL" "$HOME/termux-mcp"
    REPO_DIR="$HOME/termux-mcp"
    log "Cloned source to $REPO_DIR"
  fi
fi
cd "$REPO_DIR"

# --- 4. Binary -------------------------------------------------------------
# arch_of maps uname -m to the asset names used by the release workflow.
arch_of() {
  case "$(uname -m)" in
    aarch64|arm64)   echo arm64 ;;
    armv7l|armv8l|arm) echo arm ;;
    x86_64|amd64)    echo amd64 ;;
    *)               echo "" ;;
  esac
}

# repo_slug_of normalizes any GitHub URL form to "owner/repo".
repo_slug_of() {
  echo "$1" | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)$|\1|; s|\.git$||'
}

# install_prebuilt downloads the raw release binary for this device's
# architecture and installs it to $PREFIX/bin. Returns 1 when it cannot
# (no curl, unknown arch, no repo slug, no release yet, or download failure).
install_prebuilt() {
  [ "${TERMUX_MCP_SOURCE:-no}" = "yes" ] && return 1
  command -v curl >/dev/null 2>&1 || return 1

  local arch slug tag url tmp
  arch="$(arch_of)"
  [ -n "$arch" ] || return 1

  slug=""
  if [ -n "${TERMUX_MCP_REPO:-}" ]; then
    slug="$(repo_slug_of "$TERMUX_MCP_REPO")"
  elif git remote get-url origin >/dev/null 2>&1; then
    slug="$(repo_slug_of "$(git remote get-url origin)")"
  fi
  [ -n "$slug" ] || return 1

  if [ -n "${TERMUX_MCP_VERSION:-}" ]; then
    tag="$TERMUX_MCP_VERSION"
  else
    tag="$(curl -fsSL --max-time 15 "https://api.github.com/repos/$slug/releases/latest" \
      2>/dev/null | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
  fi
  [ -n "$tag" ] || return 1

  # Raw binary asset, e.g. termux-mcp-v0.1.0-linux-arm64.
  url="https://github.com/$slug/releases/download/$tag/termux-mcp-$tag-linux-$arch"
  log "Downloading prebuilt $tag ($arch) ..."
  # Download to a temp name, then atomically move into place, so a failed
  # download never leaves a broken binary at the final path.
  if curl -fsSL --max-time 120 -o "$PREFIX/bin/termux-mcp.new" "$url"; then
    chmod +x "$PREFIX/bin/termux-mcp.new"
    mv "$PREFIX/bin/termux-mcp.new" "$PREFIX/bin/termux-mcp"
    log "Installed: $("$PREFIX/bin/termux-mcp" version)"
    return 0
  fi
  rm -f "$PREFIX/bin/termux-mcp.new"
  warn "Prebuilt download failed; falling back to source build"
  return 1
}

build_from_source() {
  if ! command -v go >/dev/null 2>&1; then
    log "Installing golang (needed for source build) ..."
    pkg install -y golang || die "pkg install golang failed"
  fi
  log "Building termux-mcp from source ..."
  go mod tidy
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
  go build -trimpath -ldflags="-s -w -X termux-mcp/internal/version.Version=$VERSION" \
    -o "$PREFIX/bin/termux-mcp" ./cmd/termux-mcp
  chmod +x "$PREFIX/bin/termux-mcp"
  log "Installed: $("$PREFIX/bin/termux-mcp" version)"
}

if ! install_prebuilt; then
  build_from_source
fi

# --- 5. Config --------------------------------------------------------------
DATA_DIR="$PREFIX/var/lib/termux-mcp"
CONFIG="$DATA_DIR/config.yaml"
mkdir -p "$DATA_DIR"
chmod 700 "$DATA_DIR"
if [ ! -f "$CONFIG" ]; then
  log "Creating config from config.example.yaml ..."
  sed "s|\$PREFIX|$PREFIX|g" "$REPO_DIR/config.example.yaml" > "$CONFIG"
  chmod 600 "$CONFIG"
fi

# --- 6. Auth token ----------------------------------------------------------
# Tokens are 64 hex chars, stored in auth.token inside the config (see
# cmd/termux-mcp/main.go: newToken/persistToken).
if grep -Eq '^\s*token:\s+"?[[:xdigit:]]' "$CONFIG"; then
  log "Auth token already set in config"
else
  log "Generating auth token ..."
  if "$PREFIX/bin/termux-mcp" token new --write --config "$CONFIG"; then
    chmod 600 "$CONFIG"
  else
    warn "could not persist a token (run: termux-mcp token new --write --config $CONFIG)"
  fi
fi

# --- 7. Service + boot ------------------------------------------------------
if [ "${TERMUX_MCP_SERVICE:-yes}" = "yes" ] && [ -d "$PREFIX/var/service" ]; then
  SERVICE_DIR="$PREFIX/var/service/termux-mcp"
  mkdir -p "$SERVICE_DIR"
  cat > "$SERVICE_DIR/run" <<EOF
#!/data/data/com.termux/files/usr/bin/bash
exec termux-mcp serve http --config $CONFIG
EOF
  chmod +x "$SERVICE_DIR/run"
  mkdir -p "$SERVICE_DIR/log"
  cat > "$SERVICE_DIR/log/run" <<'EOF'
#!/data/data/com.termux/files/usr/bin/bash
mkdir -p "$PREFIX/var/log/termux-mcp"
exec svlogd -tt "$PREFIX/var/log/termux-mcp"
EOF
  chmod +x "$SERVICE_DIR/log/run"
  if command -v sv >/dev/null 2>&1; then
    sv up termux-mcp 2>/dev/null || true
    log "Service started: sv up termux-mcp"
  fi
  log "Supervised service registered at $SERVICE_DIR"
fi

if [ "${TERMUX_MCP_BOOT:-yes}" = "yes" ]; then
  mkdir -p "$HOME/.termux/boot"
  cp "$REPO_DIR/scripts/boot.sh" "$HOME/.termux/boot/start-mcp.sh"
  chmod +x "$HOME/.termux/boot/start-mcp.sh"
  log "Termux:Boot entry installed at ~/.termux/boot/start-mcp.sh"
fi

log "Install complete. Verifying environment..."
"$PREFIX/bin/termux-mcp" doctor --config "$CONFIG" || true
