# GOTorch Setup Script (Run As Administrator)
# Purpose: One-time setup for GOTorch on Windows. Must be run elevated.
# Creates a Windows Firewall allow rule so GOTorch never triggers a popup.

param(
    [string]$GOTorchPath = "E:\.agent\GOAgent\gotorch.exe"
)

$ErrorActionPreference = "Stop"

# Check if running as admin.
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "[ERROR] This script must be run as Administrator." -ForegroundColor Red
    Write-Host "Right-click PowerShell and select 'Run as administrator', then run this script again."
    exit 1
}

# Remove any existing GOTorch firewall rules.
$existing = netsh advfirewall firewall show rule name="GOTorch Inference Engine" 2>$null
if ($existing -match "GOTorch") {
    Write-Host "[INFO] Removing existing firewall rule..." -ForegroundColor Yellow
    netsh advfirewall firewall delete rule name="GOTorch Inference Engine" | Out-Null
}

# Add inbound allow rule for the GOTorch binary.
Write-Host "[INFO] Adding firewall rule for: $GOTorchPath" -ForegroundColor Cyan
netsh advfirewall firewall add rule `
    name="GOTorch Inference Engine" `
    dir=in `
    action=allow `
    program="$GOTorchPath" `
    enable=yes `
    profile=any | Out-Null

Write-Host "[PASS] Firewall rule added. GOTorch will no longer trigger firewall prompts." -ForegroundColor Green

# Also add outbound rule (for model downloads).
netsh advfirewall firewall add rule `
    name="GOTorch Outbound" `
    dir=out `
    action=allow `
    program="$GOTorchPath" `
    enable=yes `
    profile=any | Out-Null

Write-Host "[PASS] Outbound rule added (for model downloads)." -ForegroundColor Green

# Also add goagent.exe if it exists.
$goagentPath = Join-Path (Split-Path $GOTorchPath -Parent) "goagent.exe"
if (Test-Path $goagentPath) {
    $existing2 = netsh advfirewall firewall show rule name="GOAgent Framework" 2>$null
    if (-not ($existing2 -match "GOAgent")) {
        netsh advfirewall firewall add rule `
            name="GOAgent Framework" `
            dir=in `
            action=allow `
            program="$goagentPath" `
            enable=yes `
            profile=any | Out-Null
        netsh advfirewall firewall add rule `
            name="GOAgent Outbound" `
            dir=out `
            action=allow `
            program="$goagentPath" `
            enable=yes `
            profile=any | Out-Null
        Write-Host "[PASS] GOAgent firewall rules added." -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "Setup complete. GOTorch and GOAgent will not trigger Windows Firewall." -ForegroundColor Green
