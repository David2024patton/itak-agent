# iTaK Agent Installer for Windows
# Usage: irm https://raw.githubusercontent.com/David2024patton/itak-agent/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = "David2024patton/itak-agent"
$BinaryName = "itak-agent.exe"
$InstallDir = "$env:LOCALAPPDATA\iTaK Agent"
$DataDir = "$env:USERPROFILE\.itak-agent"

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Host "32-bit Windows is not supported." -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "  ┌──────────────────────────────────┐" -ForegroundColor Cyan
Write-Host "  │     iTaK Agent Installer          │" -ForegroundColor Cyan
Write-Host "  │     Windows / $Arch               │" -ForegroundColor Cyan
Write-Host "  └──────────────────────────────────┘" -ForegroundColor Cyan
Write-Host ""

# Create install directory
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# Get latest release
Write-Host "→ Finding latest release..." -ForegroundColor Yellow
try {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Tag = $Release.tag_name
    $DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/itak-agent-windows-$Arch.exe"

    Write-Host "→ Downloading iTaK Agent $Tag..." -ForegroundColor Yellow
    Invoke-WebRequest -Uri $DownloadUrl -OutFile "$InstallDir\$BinaryName" -UseBasicParsing
} catch {
    Write-Host "  No releases found. Please build from source:" -ForegroundColor Red
    Write-Host "    git clone https://github.com/$Repo.git" -ForegroundColor White
    Write-Host "    cd itak-agent" -ForegroundColor White
    Write-Host "    go build -o itak-agent.exe ./cmd/itakagent" -ForegroundColor White
    exit 1
}

# Create data directory
if (-not (Test-Path $DataDir)) {
    New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
}

# Add to PATH if not already there
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($CurrentPath -notlike "*$InstallDir*") {
    Write-Host "→ Adding to PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$CurrentPath;$InstallDir", "User")
    $env:Path = "$env:Path;$InstallDir"
}

# Verify
Write-Host ""
Write-Host "✓ Installed successfully!" -ForegroundColor Green
Write-Host "  Binary: $InstallDir\$BinaryName" -ForegroundColor Gray
Write-Host "  Data:   $DataDir" -ForegroundColor Gray
Write-Host ""
Write-Host "  Start the agent:" -ForegroundColor White
Write-Host "    itak-agent --port 42800" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Then open http://localhost:42800 in your browser." -ForegroundColor White
Write-Host ""
Write-Host "  NOTE: Restart your terminal for PATH changes to take effect." -ForegroundColor Yellow
Write-Host ""
