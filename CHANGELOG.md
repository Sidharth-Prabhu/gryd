# Changelog

All notable changes to **Gryd** are documented in this file.
This project adheres to [Semantic Versioning](https://semver.org/).

---

## [0.3.0] - 2026-11-15 (Phase 3 Release)

### ⚡ Highlights
- **Zero-Config Local Domain Routing**: Access your development microservices via human-friendly custom local domains (e.g. `http://api.gryd.test` or `http://web.localhost`) without remembering port numbers.
- **Cross-Platform Elevation**: Automatic integration with native OS authorization (macOS Touch ID / system dialog, Linux Polkit / sudo, Windows UAC) for seamless `/etc/hosts` and loopback port forwarding synchronization.
- **Config Management CLI**: Inspect, format as JSON, or open your `gryd.yaml` in verified terminal editors (`nano` or `vim`) with persistent editor preferences.
- **Automated Multi-Platform Packaging & Distribution**: Homebrew Formula automated updates, WinGet package submission pipeline, and multi-architecture Docker container images published to GitHub Container Registry (`ghcr.io`).

### 🚀 Added
- **`gryd config`**: Display current `gryd.yaml` configuration with syntax-highlighted comment headers.
- **`gryd config --json`**: Output the parsed configuration as clean, indented JSON for CI/CD scripting.
- **`gryd config --edit` (`-e`)**: Interactively detect available system editors (`nano` and `vim`), prompt the user to choose, save preferred editor to `~/.gryd/preferences.json`, and open `gryd.yaml`.
- **`gryd config --domain-routing`**: Interactive configuration wizard to set up custom subdomains and base domains across services.
- **`gryd config --remove-domain-routing`**: Revert domain routing and cleanly remove managed host entries.
- **`gryd hosts [list|sync|clean]`**: Local domain manager to inspect DNS status, synchronize `/etc/hosts`, and clean up development domains.
- **Loopback Port Forwarding**: Native loopback redirection for port 80 to eliminate port numbers in browser navigation.
- **Interactive TUI Domain Badges**: Active local domain URLs (`http://<domain>:8080`) are now prominently displayed in the status card and service rows.
- **Docker Support**: Multi-architecture container image (`linux/amd64`, `linux/arm64`) with GHCR workflow.

### 🛡️ Fixed & Improved
- Fixed `ERR_CONNECTION_REFUSED` on macOS loopback routing by updating packet filter rules across `lo0` and physical network interfaces.
- Fixed non-domain projects displaying elevation prompts on `gryd run`.
- Fixed JSON serialization of nested YAML mapping nodes in `gryd config --json`.
- Added MIT license file to resolve WinGet package URL validation checks.

---

## [0.2.1] - 2026-11-14

### 🚀 Added
- Local development domain reverse proxy with RFC 6761 `*.localhost` support.
- Cross-platform hosts file synchronization for `.test` domains.
- Deterministic service sorting in CLI output and dashboards.

### 🛡️ Fixed
- Proxy server lifecycle synchronized with interactive TUI runner sessions.

---

## [0.2.0] - 2026-11-13

### 🚀 Added
- Intelligent framework detection wizard (`gryd init`) for Node.js, Python, Go, Rust, Ruby, PHP, and Docker.
- Built-in background update checker and `gryd update` self-update command.
- Git Version Control (`gryd vc`) integration with status cards, fast commit-and-push, and safe branch merging.
- Interactive port conflict detection and automatic port reassignment.
- Sidecar dependency health checks and topological startup tiers.
