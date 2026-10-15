#!/usr/bin/env bash
set -euo pipefail

# ────────────────────────────────────────────────────────────
# isola install script
# Usage: curl -fsSL https://raw.githubusercontent.com/dm0xa/isola/main/scripts/install.sh | bash
# ────────────────────────────────────────────────────────────

readonly DOWNLOAD_BASE="${ISOLA_DOWNLOAD_BASE:-https://github.com/dm0xa/isola/releases/latest/download}"
readonly INSTALL_DIR="${ISOLA_INSTALL_DIR:-/opt/isola}"
readonly BINARY_NAME="isola"
readonly VERSION="${ISOLA_VERSION:-1.0.0}"

# Per-session sandbox image the isola binary launches at runtime.
readonly BROWSER_IMAGE="isola-browser:electron"
# Published build of the same image. The installer uses it when present so a
# host with no source checkout can still start sessions; set this to empty to
# skip the registry entirely and always build locally.
readonly BROWSER_IMAGE_REMOTE="${ISOLA_BROWSER_IMAGE_REMOTE:-ghcr.io/dm0xa/isola-browser:electron}"

# Source checkout root — only needed to build BROWSER_IMAGE. Empty when this
# script is piped straight from curl with no checkout nearby.
PROJECT_ROOT=""
_script_path="${BASH_SOURCE[0]:-}"
if [[ -n "$_script_path" && -f "$_script_path" ]]; then
    PROJECT_ROOT="$(cd "$(dirname "$_script_path")/.." && pwd)"
elif [[ -f "./docker-compose.yml" && -d "./browsers" ]]; then
    PROJECT_ROOT="$(pwd)"
fi
readonly PROJECT_ROOT

# ── colour helpers ──────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Colour

info()    { printf "${CYAN}●${NC} %s\n" "$*"; }
success() { printf "${GREEN}✓${NC} %s\n" "$*"; }
warn()    { printf "${YELLOW}⚠${NC} %s\n" "$*"; }
error()   { printf "${RED}✗${NC} %s\n" "$*"; }

# ── detect OS / arch ────────────────────────────────────────────────
detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux)  os="linux" ;;
        Darwin) os="darwin" ;;
        *)      error "Unsupported OS: $(uname -s)"; exit 1 ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)            error "Unsupported arch: $(uname -m)"; exit 1 ;;
    esac

    echo "${os}/${arch}"
}

# ── detect and stop existing isola process ─────────────────────────────
stop_isola_if_running() {
    local isola_pid
    isola_pid="$(pgrep -x isola 2>/dev/null | head -1 || true)"

    if [[ -z "$isola_pid" ]]; then
        info "No running isola process detected."
        return 0
    fi

    warn "isola is already running (PID $isola_pid). It must be stopped before upgrading."
    printf "${CYAN}Stop isola now? [Y/n] ${NC}"
    local answer
    read -r answer </dev/tty
    if [[ "$answer" =~ ^[Nn] ]]; then
        warn "Skipping shutdown. Stop it manually before restarting."
        return 0
    fi

    kill "$isola_pid" 2>/dev/null || true
    sleep 1
    if kill -0 "$isola_pid" >/dev/null 2>&1; then
        kill -9 "$isola_pid" 2>/dev/null || true
    fi
    success "isola stopped."
}

# ── set up as system service (systemd or launchd) ─────────────────
setup_service() {
    local os="$1"

    if [[ "$os" == "linux" ]]; then
        local svc_file="/etc/systemd/system/isola.service"
        if [[ -f "$svc_file" ]]; then
            warn "Service file already exists at ${svc_file}, skipping."
            return 0
        fi

        info "Creating systemd service..."
        local env_file="${INSTALL_DIR}/isola.env"
        if [[ ! -f "$env_file" ]]; then
            cat > "$env_file" <<ENVEOF
# isola environment — edit as needed
ISOLA_PORT=8080
# Session snapshot + per-session policy files. Must be writable by the isola
# user; the built-in default (/var/lib/isola) is root-owned, so leaving this
# unset silently disables persistence.
ISOLA_DATA_DIR=${INSTALL_DIR}/data
# Per-session sandbox container limits. The CPU limit must not exceed this
# host's core count: the Docker daemon rejects a --cpus value above it. Left
# unset, isola uses min(3, host cores). Uncomment to pin different caps:
# ISOLA_SESSION_MEMORY_MB=2048
# ISOLA_SESSION_CPUS=2
# ISOLA_SESSION_PIDS=512
ENVEOF
            chmod 600 "$env_file"
            info "Created env file: ${env_file}"
        fi
        if [[ $EUID -ne 0 ]]; then
            warn "Need root to create systemd service. Run:"
            printf "  sudo tee ${svc_file} <<'UNIT'\n"
            cat <<UNIT
[Unit]
Description=isola
After=network-online.target docker.service
Wants=network-online.target docker.service

[Service]
Type=simple
User=isola
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
EnvironmentFile=${env_file}
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
            printf "\n  sudo useradd --system --user-group --home-dir %s --shell /usr/sbin/nologin isola\n" "${INSTALL_DIR}"
            printf "  sudo chown -R isola:isola %s\n" "${INSTALL_DIR}"
            printf "  sudo usermod -aG docker isola   # lets the service reach the Docker socket\n"
            printf "\n  sudo systemctl daemon-reload\n"
            printf "  sudo systemctl enable --now isola\n"
        else
            cat > "$svc_file" <<UNIT
[Unit]
Description=isola
After=network-online.target docker.service
Wants=network-online.target docker.service

[Service]
Type=simple
User=isola
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
EnvironmentFile=${env_file}
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
            # The unit runs as User=isola; create it if this is a fresh host,
            # otherwise systemd fails the start with "user isola not found".
            if ! getent passwd isola >/dev/null 2>&1; then
                useradd --system --user-group --home-dir "${INSTALL_DIR}" --shell /usr/sbin/nologin isola 2>/dev/null \
                    || useradd -r -U -d "${INSTALL_DIR}" -s /sbin/nologin isola
            fi
            chown -R isola:isola "${INSTALL_DIR}" 2>/dev/null || true
            # The unit runs as User=isola, which needs the Docker group to reach
            # /var/run/docker.sock — without it every session launch fails with a
            # permission error even though the daemon is running.
            usermod -aG docker isola 2>/dev/null || true
            systemctl daemon-reload
            systemctl enable --now isola
            success "systemd service installed and started."
        fi

    elif [[ "$os" == "darwin" ]]; then
        local plist="$HOME/Library/LaunchAgents/com.isola.plist"
        if [[ -f "$plist" ]]; then
            warn "LaunchAgent already exists at ${plist}, skipping."
            return 0
        fi

        info "Creating LaunchAgent..."
        mkdir -p "$HOME/Library/LaunchAgents"
        cat > "$plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.isola</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${BINARY_NAME}</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>ISOLA_PORT</key>
        <string>8080</string>
        <key>ISOLA_DATA_DIR</key>
        <string>${INSTALL_DIR}/data</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${INSTALL_DIR}/isola.log</string>
    <key>StandardErrorPath</key>
    <string>${INSTALL_DIR}/isola.log</string>
</dict>
</plist>
PLIST
        launchctl load "$plist"
        success "LaunchAgent installed and started."
        info "Edit env vars in: ${plist}"
        info "Logs: ${INSTALL_DIR}/isola.log"
    fi
}

# ── install Docker when the CLI is missing ──────────────────────────
install_docker() {
    local os="$1"

    if [[ "$os" == "linux" ]]; then
        info "Installing Docker via the official convenience script (requires sudo)..."
        curl -fsSL https://get.docker.com -o /tmp/get-docker.sh || return 1
        if [[ $EUID -eq 0 ]]; then
            sh /tmp/get-docker.sh || { rm -f /tmp/get-docker.sh; return 1; }
        else
            sudo sh /tmp/get-docker.sh || { rm -f /tmp/get-docker.sh; return 1; }
        fi
        rm -f /tmp/get-docker.sh
        if command -v systemctl >/dev/null 2>&1; then
            sudo systemctl enable --now docker || true
        fi
    elif [[ "$os" == "darwin" ]]; then
        if command -v brew >/dev/null 2>&1; then
            info "Installing Docker Desktop via Homebrew..."
            brew install --cask docker || return 1
            info "Launch Docker Desktop once to finish setup: open -a Docker"
        else
            error "Homebrew not found. Install Docker Desktop: https://www.docker.com/products/docker-desktop/"
            return 1
        fi
    else
        error "Automatic Docker install is not supported on ${os}."
        return 1
    fi

    command -v docker >/dev/null 2>&1
}

# ── fetch the published sandbox image ──────────────────────────────
# Returns 0 only when the image is now present under the local name. The
# published image may be absent, private, or unreachable, so every failure is
# non-fatal: the caller falls back to building from a source checkout.
pull_browser_image() {
    [[ -n "${BROWSER_IMAGE_REMOTE}" ]] || return 1

    info "Fetching the session image from ${BROWSER_IMAGE_REMOTE}…"
    if docker pull "${BROWSER_IMAGE_REMOTE}" >/dev/null 2>&1; then
        # The binary launches the image by its local name, so re-tag the pull.
        docker tag "${BROWSER_IMAGE_REMOTE}" "${BROWSER_IMAGE}"
        return 0
    fi

    warn "Could not fetch ${BROWSER_IMAGE_REMOTE}."
    info "  Private or not published yet? Authenticate, then re-run:"
    info "  docker login ghcr.io"
    return 1
}

# ── ensure Docker + the per-session browser image ───────────────────
ensure_docker() {
    local os="$1"

    # 1. Is the Docker CLI available?
    if ! command -v docker >/dev/null 2>&1; then
        warn "Docker is required but was not found on this host."

        printf "${CYAN}Install Docker now? [Y/n] ${NC}"
        local answer
        read -r answer </dev/tty
        if [[ "$answer" =~ ^[Nn] ]]; then
            warn "Skipping Docker install — isola cannot start sessions without it."
            info "Install Docker later: https://docs.docker.com/engine/install/"
            return 0
        fi

        if ! install_docker "$os"; then
            error "Docker installation did not complete: https://docs.docker.com/engine/install/"
            return 0
        fi
    fi

    # 2. Is the daemon reachable?
    if ! docker info >/dev/null 2>&1; then
        warn "Docker CLI found but the daemon is not responding."
        info "Start Docker, then re-run this installer:"
        info "  Linux:  sudo systemctl start docker"
        info "  macOS:  open -a Docker"
        info "If this is a permissions error, join the docker group and log in again:"
        info "  sudo usermod -aG docker ${USER}"
        return 0
    fi
    success "Docker detected."

    # 3. Is the per-session browser image present?
    if docker image inspect "${BROWSER_IMAGE}" >/dev/null 2>&1; then
        success "Session image ${BROWSER_IMAGE} is ready."
        return 0
    fi

    warn "The per-session image ${BROWSER_IMAGE} is not present."

    # The published image comes first: it is the only path that works without a
    # source checkout. It is non-interactive, so a piped install never blocks.
    if pull_browser_image; then
        success "Session image ${BROWSER_IMAGE} fetched and tagged."
        return 0
    fi

    if [[ -z "$PROJECT_ROOT" || ! -f "${PROJECT_ROOT}/docker-compose.yml" ]]; then
        info "Build it once from a source checkout:"
        info "  git clone https://github.com/dm0xa/isola.git && cd isola"
        info "  docker compose up --build"
        return 0
    fi

    # No TTY (piped install): proceed without prompting rather than blocking on
    # a read, matching the prompt's [Y/n] default.
    local build_answer=""
    if [[ -r /dev/tty ]]; then
        printf "${CYAN}Build ${BROWSER_IMAGE} now? [Y/n] ${NC}"
        read -r build_answer </dev/tty || build_answer=""
    fi
    if [[ "$build_answer" =~ ^[Nn] ]]; then
        info "Skipping build. Build it later from the source checkout:"
        info "  cd ${PROJECT_ROOT} && docker compose up --build"
        return 0
    fi

    info "Building ${BROWSER_IMAGE} (the first build takes a while)…"
    if ( cd "$PROJECT_ROOT" && docker compose build browser-node-electron ); then
        success "Session image ${BROWSER_IMAGE} built."
    else
        error "Build failed. Try: cd ${PROJECT_ROOT} && docker compose up --build"
    fi
}

# ── ensure the Docker network session containers join ───────────────
# The orchestrator starts every session container with `--network browser-net`
# (that name is hard-coded) and never creates the network itself. The Compose
# stack defines it; a binary-only install must create it here, or every session
# fails to provision with "network browser-net not found".
#
# ICC is disabled so one session container cannot reach another: when the
# backend runs on the host rather than as a container on this bridge, it cannot
# source-restrict the stream/CDP ports to itself, so the bridge is the isolation.
ensure_browser_network() {
    if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
        return 0
    fi

    if docker network inspect browser-net >/dev/null 2>&1; then
        success "Docker network browser-net is ready."
        return 0
    fi

    info "Creating Docker network browser-net (session containers join it)…"
    # The bridge interface name is capped at 15 characters (IFNAMSIZ - 1), so it
    # is abbreviated here; br-isola-browser would be 16 and create would fail.
    if docker network create \
        --driver bridge \
        --opt com.docker.network.bridge.enable_icc=false \
        --opt com.docker.network.bridge.enable_ip_masquerade=true \
        --opt com.docker.network.bridge.name=br-isola-brows \
        browser-net >/dev/null; then
        success "Docker network browser-net created."
    else
        error "Could not create browser-net — sessions will not start until it exists."
        info "  docker network create browser-net"
    fi
}

# ── main ────────────────────────────────────────────────────────────
main() {
    printf "\n${CYAN}╭──────────────────────────────────────╮${NC}\n"
    printf "${CYAN}│    isola Installer                   │${NC}\n"
    printf "${CYAN}╰──────────────────────────────────────╯${NC}\n\n"

    # 1. stop isola if already running
    stop_isola_if_running

    # 2. detect platform
    local platform
    platform="$(detect_platform)"
    local os="${platform%/*}"
    local arch="${platform#*/}"
    info "Platform: ${os}/${arch}"

    # 3. build download URL
    local archive="${BINARY_NAME}-${os}-${arch}-v${VERSION}.tar.gz"
    local download_url="${DOWNLOAD_BASE}/${archive}"
    local checksum_url="${DOWNLOAD_BASE}/checksums.txt"

    info "Downloading isola ${VERSION} for ${os}/${arch}..."
    info "URL: ${download_url}"

    # 4. create install dir
    mkdir -p "${INSTALL_DIR}"

    # 5. download
    local tmp_file
    tmp_file="$(mktemp)"
    curl -fsSL --retry 3 --retry-delay 2 -# -o "$tmp_file" "$download_url" || {
        error "Download failed from ${download_url}"
        rm -f "$tmp_file"
        exit 1
    }

    # 6. verify checksum (mandatory — never install an unverified binary)
    local expected
    expected="$(curl -fsSL "$checksum_url" 2>/dev/null | awk -v f="$archive" '$2==f || $2=="*"f {print $1; exit}' || true)"
    if [[ -z "$expected" ]]; then
        error "Could not fetch a checksum for ${archive} from ${checksum_url}; refusing to install."
        rm -f "$tmp_file"
        exit 1
    fi
    local actual
    actual="$(sha256sum "$tmp_file" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$tmp_file" 2>/dev/null | cut -d' ' -f1)"
    if [[ -z "$actual" || "$expected" != "$actual" ]]; then
        error "Checksum verification failed (expected ${expected}, got ${actual:-none})."
        rm -f "$tmp_file"
        exit 1
    fi
    success "Checksum verified"

    # 7. extract (tar.gz or zip or raw binary)
    if file "$tmp_file" | grep -q 'gzip compressed'; then
        info "Extracting..."
        tar xzf "$tmp_file" -C "${INSTALL_DIR}" "$BINARY_NAME" 2>/dev/null || {
            # binary might be at root of archive without prefix
            tar xzf "$tmp_file" -C "${INSTALL_DIR}" --transform 's|.*/||' --wildcards "*${BINARY_NAME}*" 2>/dev/null
        }
    elif file "$tmp_file" | grep -q 'Zip archive'; then
        info "Extracting..."
        unzip -o "$tmp_file" -d "${INSTALL_DIR}" >/dev/null 2>&1
    elif file "$tmp_file" | grep -q 'ELF\|Mach-O'; then
        # raw binary
        cp "$tmp_file" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        error "Unknown archive format."
        rm -f "$tmp_file"
        exit 1
    fi

    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    rm -f "$tmp_file"

    success "isola installed to ${INSTALL_DIR}/${BINARY_NAME}"

    # 8. optionally set up as a system service
    printf "\n${CYAN}Set up isola as a system service (auto-start on boot)? [Y/n] ${NC}\n"
    local setup_svc
    read -r setup_svc </dev/tty
    if [[ ! "$setup_svc" =~ ^[Nn] ]]; then
        setup_service "$os"
    else
        info "No service created. To run manually:"
        info "  ${INSTALL_DIR}/${BINARY_NAME}"
        if [[ "$os" == "linux" && "$EUID" -ne 0 ]]; then
            info "The invoking user needs Docker socket access:"
            info "  sudo usermod -aG docker ${USER}   # then log out and back in"
        fi
    fi

    # 9. ensure Docker and the per-session browser image
    ensure_docker "$os"

    # 10. ensure the network session containers join
    ensure_browser_network

    # 11. next steps
    printf "\n${GREEN}┌─────────────────────────────────────────────┐${NC}\n"
    printf "${GREEN}│  installation complete!                     │${NC}\n"
    printf "${GREEN}│                                             │${NC}\n"
    printf "${GREEN}│  Start isola:                               │${NC}\n"
    printf "${GREEN}│    ${INSTALL_DIR}/${BINARY_NAME}             ${NC}\n"
    printf "${GREEN}└─────────────────────────────────────────────┘${NC}\n\n"
    info "Transports: isola (Electron) only"
}

main "$@"
