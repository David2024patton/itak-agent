$ErrorActionPreference = "Stop"
Write-Host "Building iTaK Agent for Windows (amd64)..." -ForegroundColor Cyan

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = 0

go build -ldflags="-s -w" -trimpath -o itakagent.exe ./cmd/itakagent/

Write-Host "Build complete! Generated: itakagent.exe" -ForegroundColor Green
