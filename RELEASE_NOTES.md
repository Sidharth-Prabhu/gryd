# Gryd v0.3.0 (Phase 3 Release) 🚀

Gryd v0.3.0 introduces **zero-configuration local domain routing**, cross-platform system elevation, automated multi-distribution release pipelines (Homebrew, WinGet, Docker), and an interactive configuration management suite.

---

### What's New in v0.3.0

#### ⚡ Local Domain Routing & Reverse Proxy
- Route services to human-friendly local domains (e.g. `http://api.gryd.test`, `http://web.localhost`).
- Interactive setup with `gryd config --domain-routing` and removal with `gryd config --remove-domain-routing`.
- Manage system hosts mappings with `gryd hosts list`, `gryd hosts sync`, and `gryd hosts clean`.
- Support for port 80 loopback redirection so you can browse clean URLs without ports.

#### 🛠️ Config Management CLI
- **`gryd config`**: Display current `gryd.yaml` configuration with syntax-highlighted comment headers.
- **`gryd config --json`**: Output parsed configuration formatted as clean, indented JSON for automation and CI/CD pipelines.
- **`gryd config --edit` (`-e`)**: Check for installed terminal text editors (`nano` and `vim`), prompt the user to choose, save preferred editor to `~/.gryd/preferences.json`, and open `gryd.yaml` directly.

#### 🛡️ Cross-Platform Privilege Elevation
- Automatic privilege escalation via native OS dialogs (macOS Touch ID / system dialog, Linux Polkit / sudo, Windows UAC) for hosts file updates.
- Only prompts for elevation when domain routing is explicitly configured.

#### 🐳 Multi-Arch Docker Container & Automated Distribution
- Docker container images for `linux/amd64` and `linux/arm64` published to GitHub Container Registry (`ghcr.io/sidharth-prabhu/gryd`).
- Automated Homebrew formula synchronization to `Sidharth-Prabhu/homebrew-tap`.
- Automated WinGet package submission to `microsoft/winget-pkgs`.

---

### Installation & Upgrade

#### Homebrew (macOS / Linux)
```bash
brew tap Sidharth-Prabhu/tap
brew install gryd
# or upgrade
brew upgrade gryd
```

#### Windows (PowerShell One-Line Installer)
```powershell
irm https://raw.githubusercontent.com/Sidharth-Prabhu/gryd/main/install.ps1 | iex
```

#### Windows (WinGet)
```powershell
winget install SidharthPrabhu.Gryd
# or upgrade
winget upgrade SidharthPrabhu.Gryd
```

#### In-Place Upgrade
```bash
gryd update
```

#### Docker
```bash
docker pull ghcr.io/sidharth-prabhu/gryd:latest
```

---

### Checksums
See `checksums.txt` attached below for SHA256 hashes of all platform binary archives.
