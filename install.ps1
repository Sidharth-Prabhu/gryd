# Gryd Windows PowerShell One-Line Web Installer
# Usage:
#   irm https://raw.githubusercontent.com/Sidharth-Prabhu/gryd/main/install.ps1 | iex

$ErrorActionPreference = 'Stop'

function Install-Gryd {
    [CmdletBinding()]
    param(
        [string]$Version = $env:GRYD_VERSION
    )

    Write-Host @"
   ______ _____  __  ______ 
  / ____// __ \ \ \/ // __ \
 / / __ / /_/ /  \  // / / /
/ /_/ // _, _/   / // /_/ / 
\____//_/ |_|   /_//_____/  
"@ -ForegroundColor Magenta
    Write-Host "Installing Gryd for Windows..." -ForegroundColor Cyan

    # 1. Detect Architecture
    $arch = if ([System.Environment]::Is64BitOperatingSystem) {
        if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") {
            "arm64"
        } else {
            "amd64"
        }
    } else {
        Write-Error "Gryd requires a 64-bit version of Windows (x64 or arm64)."
        return
    }

    $repo = "Sidharth-Prabhu/gryd"

    # 2. Determine Version / Tag
    if (-not $Version) {
        $releaseUrl = "https://api.github.com/repos/$repo/releases/latest"
        try {
            $ProgressPreference = 'SilentlyContinue'
            $release = Invoke-RestMethod -Uri $releaseUrl -Headers @{ "User-Agent" = "gryd-installer" }
            $tag = $release.tag_name
        } catch {
            Write-Host "Notice: Fetching latest release redirect..." -ForegroundColor DarkGray
            $tag = "latest"
        }
    } else {
        $tag = $Version
        if (-not $tag.StartsWith("v")) {
            $tag = "v$tag"
        }
    }

    # 3. Construct Download URL
    if ($tag -eq "latest") {
        $downloadUrl = "https://github.com/$repo/releases/latest/download/gryd_windows_$arch.zip"
    } else {
        $downloadUrl = "https://github.com/$repo/releases/download/$tag/gryd_windows_$arch.zip"
    }

    # 4. Set destination paths
    $installDir = Join-Path $HOME ".gryd\bin"
    if (-not (Test-Path $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }

    $tempZip = Join-Path ([System.IO.Path]::GetTempPath()) ("gryd_" + [System.Guid]::NewGuid().ToString() + ".zip")
    $tempExtract = Join-Path ([System.IO.Path]::GetTempPath()) ("gryd_extract_" + [System.Guid]::NewGuid().ToString())

    # 5. Download and Extract
    try {
        Write-Host "Downloading Gryd ($tag, $arch)..." -ForegroundColor Cyan
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempZip -UseBasicParsing

        Write-Host "Extracting binary..." -ForegroundColor Cyan
        Expand-Archive -Path $tempZip -DestinationPath $tempExtract -Force

        $exeFile = Join-Path $tempExtract "gryd.exe"
        if (-not (Test-Path $exeFile)) {
            $found = Get-ChildItem -Path $tempExtract -Filter "gryd.exe" -Recurse | Select-Object -First 1
            if ($found) {
                $exeFile = $found.FullName
            } else {
                throw "Could not find gryd.exe in downloaded archive."
            }
        }

        $targetExe = Join-Path $installDir "gryd.exe"
        Copy-Item -Path $exeFile -Destination $targetExe -Force
    } finally {
        if (Test-Path $tempZip) { Remove-Item -Path $tempZip -Force -ErrorAction SilentlyContinue }
        if (Test-Path $tempExtract) { Remove-Item -Path $tempExtract -Recurse -Force -ErrorAction SilentlyContinue }
    }

    # 6. Update User PATH persistently
    try {
        $userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
        $pathList = if ($userPath) { $userPath -split ';' } else { @() }
        if ($pathList -notcontains $installDir) {
            $newUserPath = if ([string]::IsNullOrEmpty($userPath)) { $installDir } else { "$userPath;$installDir" }
            [Environment]::SetEnvironmentVariable("Path", $newUserPath, [EnvironmentVariableTarget]::User)
            Write-Host "✓ Added $installDir to persistent User PATH." -ForegroundColor Green
        }
    } catch {
        Write-Host "Warning: Could not automatically update User PATH: $_" -ForegroundColor Yellow
    }

    # Update current process PATH
    if (($env:Path -split ';') -notcontains $installDir) {
        $env:Path = "$env:Path;$installDir"
    }

    # 7. Verification & Success Message
    Write-Host "`n✓ Gryd installed successfully to $targetExe" -ForegroundColor Green
    if ($IsWindows -or ($PSVersionTable.PSVersion.Major -ge 6 -and [System.Environment]::OSVersion.Platform -eq "Win32NT")) {
        try {
            & $targetExe version
        } catch {}
    }

    Write-Host @"

Get started:
  gryd --help       # Display help & commands
  gryd init         # Configure your multi-service stack
  gryd run          # Start your services in interactive TUI

"@ -ForegroundColor White
}

Install-Gryd
