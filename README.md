# Gryd (⚡)

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-00ADD8.svg?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey.svg)](https://github.com/Sidharth-Prabhu/gryd)
[![Docker](https://img.shields.io/badge/docker-ghcr.io%2Fsidharth--prabhu%2Fgryd-2496ED.svg?logo=docker)](https://github.com/Sidharth-Prabhu/gryd/pkgs/container/gryd)
[![Homebrew](https://img.shields.io/badge/homebrew-Sidharth--Prabhu%2Ftap-orange.svg)](https://github.com/Sidharth-Prabhu/homebrew-tap)
[![WinGet](https://img.shields.io/badge/winget-SidharthPrabhu.Gryd-blue.svg)](https://github.com/microsoft/winget-pkgs)

**Gryd** is a cross-platform, developer-first command-line orchestrator and interactive terminal user interface (TUI) for local multi-service development environments.

Instead of juggling 5+ terminal tabs for your API backend, frontend, worker queues, MCP servers, and databases—or wrestling with heavy container overhead—Gryd allows developers to configure, launch, monitor, test, tunnel, and manage all services with a single command.

---

## ⚡ Highlights

- **🖥️ Responsive Full-Screen TUI**: Live multi-pane terminal interface powered by Bubble Tea & Lip Gloss, featuring Catppuccin-themed status badges, service navigation, and live scrollable log viewing.
- **🌍 Native Cross-Platform**: Full first-class support for **macOS** (Apple Silicon & Intel), **Linux** (AMD64 & ARM64), and **Windows** (AMD64 & ARM64).
- **🛡️ Process Group Isolation & Auto-Port Collision Resolution**: Uses native Unix process groups (`pgid`) and Windows taskkill process tree termination. When a port is occupied by a stray process or container, Gryd prompts you interactively to `[K]ill` the process, `[A]ssign` a free port, or `[C]ancel`.
- **🩺 Dependency Health Checks & Staged Startup**: Define readiness probes (`http:` or `tcp:`) and `depends_on: { service: { condition: service_healthy } }` so frontend services wait for backends/databases to be genuinely ready before booting.
- **🔍 In-TUI Log Search & Regex Filtering (`/`)**: Real-time log searching with automatic fallback from regex to substring search and keyword highlighting.
- **🏷️ Log Level Cycling (`l`) & Color Differentiation**: Instantly filter logs by level (`ERROR`, `WARN`, `INFO`) with color-coded syntax highlighting across the TUI and CLI.
- **📑 Structured JSON Log Parser (`J`)**: Auto-detects structured JSON logs (Zap, Zerolog, Pino) with toggleable viewing modes (`compact` single-line, `pretty` multiline, or `raw`).
- **📍 Visual Timestamp Markers (`m`)**: Injects high-visibility visual divider markers into the log stream to cleanly track what happened after an action or API request.
- **📊 Real-Time Resource Telemetry & Process Tree (`i` / `p`)**: Displays live CPU% and RSS memory usage badges next to each service in the sidebar; inspect child process trees, threads, and file descriptors.
- **🌐 Instant Public Tunneling (`t` / `gryd tunnel`)**: Expose local ports to public HTTPS URLs via Cloudflare, ngrok, or localtunnel for quick webhook testing.
- **👁️ Built-in File Watcher (`watch` / `ignore`)**: Declarative hot-reload rules per service with 300ms debouncing—no external tools like `nodemon` or `air` required.
- **🔄 Hot Reload (`r`) vs. Hard Restart (`R`)**: Graceful code reload (`r`) keeps container sidecars running, while hard restart (`R`) completely cleans processes, frees ports, and restarts fresh.
- **🔀 Multi-Environment Profiles (`--profile` / `-p`)**: Run customized subsets of services and dependencies with cascading `.env` file support (`.env`, `.env.local`, `.env.<profile>`).
- **🔌 Remote Unix Socket / Named Pipe IPC Daemon**: Control running services from another terminal tab (`gryd service restart <svc>`, `gryd service stop <svc>`, `gryd service exec <svc> -- <cmd>`).
- **🧪 Pre-Flight Boot Verification (`gryd test`)**: Automated integration test boot that verifies port readiness, safely terminates all child processes, and outputs standard CI exit codes or machine-readable JSON (`--json`).
- **🌿 Integrated Version Control (`gryd vc`)**: Full-screen Git branch selector with live search, push workflows, and branch merging.
- **🌐 Built-in Local Domain Reverse Proxy & Routing**: Route local traffic using clean domains (e.g., `http://api.localhost` or `http://api.gryd.test`) completely free without paid DNS or external accounts. Manage hosts with `gryd hosts list` and `gryd hosts sync`.
- **🚀 Built-in Auto-Update System**: Automatically checks GitHub releases in the background, notifies you when an update is available, and upgrades with `gryd update`.

---

## 📦 Installation

### macOS & Linux (via Homebrew)

The easiest way to install and manage Gryd on macOS and Linux is through Homebrew:

```bash
# Add the tap
brew tap Sidharth-Prabhu/tap

# Install gryd
brew install gryd
```

### Windows (via WinGet)

Install Gryd on Windows using the official Windows Package Manager:

```powershell
winget install gryd
```

*Or by Package Identifier:*
```powershell
winget install SidharthPrabhu.Gryd
```

### Docker (macOS, Windows & Linux)

Gryd is published as a multi-architecture container (`linux/amd64` and `linux/arm64`) on GitHub Container Registry (GHCR):

```bash
# Pull the latest image
docker pull ghcr.io/sidharth-prabhu/gryd:latest

# Run interactive TUI mounting your current directory
docker run -it --rm \
  -v "$(pwd)":/workspace \
  -p 8000:8000 \
  -p 3000:3000 \
  ghcr.io/sidharth-prabhu/gryd:latest run
```

*(On Windows PowerShell, use `-v "${PWD}:/workspace"`)*


### Pre-Built Binaries

Download pre-compiled binaries directly from the [GitHub Releases](https://github.com/Sidharth-Prabhu/gryd/releases):

| OS | Architecture | Format |
|---|---|---|
| **macOS** | Apple Silicon (`arm64`) | `.tar.gz` |
| **macOS** | Intel (`amd64`) | `.tar.gz` |
| **Linux** | 64-bit (`amd64`) | `.tar.gz` |
| **Linux** | ARM64 (`arm64`) | `.tar.gz` |
| **Windows** | 64-bit (`amd64`) | `.zip` (with `gryd.exe`) |
| **Windows** | ARM64 (`arm64`) | `.zip` (with `gryd.exe`) |

### From Source (Go 1.21+)

```bash
git clone https://github.com/Sidharth-Prabhu/gryd.git
cd gryd
go build -o /usr/local/bin/gryd .
```

Verify your installation:
```bash
gryd --version
```

---

## 🔄 Updating Gryd

Gryd includes an integrated update checker that runs silently in the background. Whenever a new version is released on GitHub, Gryd will display a subtle notification:

```
╭───────────────────────────────────────────────────╮
│  Update available: v0.2.0 -> v0.3.0               │
│  Run 'gryd update' to install the latest version. │
╰───────────────────────────────────────────────────╯
```

To update directly:
```bash
gryd update
```

Or if installed via a package manager:
```bash
# Homebrew
brew upgrade gryd

# WinGet
winget upgrade gryd
```

---

## 🚀 Quick Start

### 1. Scaffold your configuration
Inside your project root:
```bash
# Launch the interactive configuration wizard
gryd init --wizard

# Or generate a starter template directly
gryd init my-app
```

### 2. Verify everything boots cleanly
```bash
gryd test
```

### 3. Launch the interactive development dashboard
```bash
gryd run
```

---

## ⚙️ Configuration Reference (`gryd.yaml`)

Gryd uses a human-friendly YAML file in your project root:

```yaml
name: my-app
working_directory: .

services:
  backend:
    directory: api
    command: uvicorn main:app --reload --port 8000
    port: 8000
    domain: api.gryd.test
    healthcheck:
      http: http://localhost:8000/health
      interval: 2s
      retries: 5
    env:
      ENVIRONMENT: development
      PORT: "8000"
    watch:
      - "src/**"
      - "config/**"
    ignore:
      - "*.log"
      - ".git/**"

  frontend:
    directory: web
    command: npm run dev
    port: 3000
    domain: web.gryd.test
    depends_on:
      backend:
        condition: service_healthy
    watch:
      - "src/**"
      - "public/**"

  worker:
    directory: workers
    command: python queue_consumer.py
    env_file: .env.worker

profiles:
  api-only:
    services: [backend]
  full:
    services: [backend, frontend, worker]
```

---

## ⌨️ TUI Keyboard Shortcuts

When running in the full-screen dashboard (`gryd run` or `gryd look`):

| Key | Action | Description |
|---|---|---|
| `/` | **Search** | Enter live log search mode (supports plain text & regex). `Enter` to confirm, `Esc` to clear. |
| `l` | **Level Filter** | Cycle log level filter: `ALL` $\rightarrow$ `ERROR` $\rightarrow$ `WARN` $\rightarrow$ `INFO` $\rightarrow$ `ALL`. |
| `J` | **JSON Mode** | Cycle structured JSON log viewing mode: `compact` $\rightarrow$ `pretty` $\rightarrow$ `raw`. |
| `m` | **Marker** | Drop a visual timestamp divider line across the log stream. |
| `t` | **Tunnel** | Toggle a public HTTPS tunnel for the selected service. |
| `i` / `p` | **Inspect Tree** | Toggle detailed process tree view, child PIDs, threads, and file descriptors. |
| `r` | **Hot Reload** | Gracefully reload services in-place without destroying container sidecars. |
| `R` | **Hard Restart** | Hard-terminate all processes and ports and boot the project fresh. |
| `↑` / `↓` / `Tab` | **Navigate** | Switch between individual service logs or combined log view. |
| `u` / `d` | **Scroll** | Scroll up/down by 8 lines (`g` jumps to top, `G` jumps to bottom). |
| `c` | **Detach** | Detach to background mode. Services continue running; re-attach with `gryd look`. |
| `q` / `Ctrl+C` | **Quit** | Gracefully terminate all processes, release ports, and exit. |

---

## 🛠️ CLI Command Reference

### `gryd run [project]`
Starts all services defined in `gryd.yaml` and launches the full-screen interactive TUI dashboard.

```bash
# Run with default profile
gryd run

# Run with a specific profile
gryd run --profile api-only

# Run in detached (daemon) mode without opening TUI
gryd run -d
```

---

### `gryd look [project]`
Attaches the interactive TUI dashboard to an already running background Gryd daemon session.

```bash
gryd look
```

---

### `gryd stop [project]`
Stops all running services, terminates process trees, and frees all occupied network ports.

```bash
gryd stop
```

---

### `gryd restart [project]`
Performs a complete hard restart: terminates all processes and process groups (`SIGTERM` + `SIGKILL`), frees all occupied ports, cleans up state, and boots the project fresh.

```bash
gryd restart
```

---

### `gryd reload [project]`
Performs a graceful hot reload on running services without destroying state or stopping container sidecars.

```bash
gryd reload
```

---

### `gryd test [project]`
Runs pre-flight validation checks to confirm that all configured services boot, bind to their ports, and terminate cleanly without leaving orphan processes.

```bash
# Standard test run
gryd test

# Output machine-readable JSON for CI pipelines
gryd test --json
```

---

### `gryd tunnel [service]`
Establishes a secure, public HTTPS tunnel for a running local service port (powered by Cloudflare Quick Tunnels, ngrok, or localtunnel).

```bash
gryd tunnel backend
```

---

### `gryd service [command]`
Inspect and control individual services running inside an active Gryd session via IPC:

```bash
# Restart a single service
gryd service restart backend

# Stop a single service
gryd service stop worker

# Execute an ad-hoc command within a service context
gryd service exec backend -- npm run seed
```

---

### `gryd vc` (Version Control)
Full-screen interactive Git branch selector, conflict resolver, and push manager.

```bash
# Open interactive Git management dashboard
gryd vc

# Commit and push all changes with a single command
gryd vc push -m "feat: new feature"

# Merge another branch safely
gryd vc merge feature/login
```

---

### `gryd config [flags]`
Interactively configure project settings and developer features in `gryd.yaml`:

```bash
# Interactively assign local subdomains and configure domain routing
gryd config --domain-routing

# Revert and clear domain routing for all services
gryd config --remove-domain-routing
```

> **Cross-Platform Privilege Elevation**: When updating domains with extensions like `.test`, Gryd automatically triggers native privilege escalation (macOS Touch ID / system dialog, Linux Polkit / sudo, or Windows UAC prompt) to synchronize your system hosts file without requiring manual editor changes.

---

### `gryd hosts [command]`
Inspect and synchronize local development domain names (such as `api.localhost` or `*.gryd.test`) into your system hosts file for offline, zero-configuration local routing:

```bash
# List all active local domains for your services and their resolution status
gryd hosts list

# Synchronize .test domains into /etc/hosts (or Windows hosts)
gryd hosts sync

# Remove Gryd managed entries from hosts file
gryd hosts clean
```

---

### `gryd update`
Checks for the latest Gryd release on GitHub, downloads the matching binary for your OS and architecture, replaces the existing binary, and verifies the new version.

```bash
gryd update
```

---

### `gryd version`
Prints version, release tag, platform architecture, and Go runtime information.

```bash
gryd --version
```

---

## 🤖 Continuous Integration & Packaging

Gryd includes automated GitHub Actions pipelines ([`.github/workflows/release.yml`](.github/workflows/release.yml)) configured to:
- Cross-compile native Go binaries for macOS, Linux, and Windows (AMD64 & ARM64).
- Package tarballs and zip archives with SHA256 checksums.
- Automatically update and commit the Homebrew Formula in [`Sidharth-Prabhu/homebrew-tap`](https://github.com/Sidharth-Prabhu/homebrew-tap).
- Automatically generate 1.6.0 portable manifests and submit pull requests to the official Microsoft [`microsoft/winget-pkgs`](https://github.com/microsoft/winget-pkgs) catalog.

---

## 📄 License

Gryd is open source software licensed under the [MIT License](LICENSE).
