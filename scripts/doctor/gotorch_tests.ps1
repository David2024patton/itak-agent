# GOTorch Unit Test Runner (Doctor Agent)
# Purpose: Run all GOTorch unit tests. Requires GOTORCH_LIB set to the llama.cpp lib directory.
# Uses 'go test' which only creates in-process HTTP servers (no firewall popup).
# Exit code 0 = all tests pass, non-zero = failure.

param(
    [string]$LibDir = (Join-Path $env:USERPROFILE ".gotorch\lib")
)

Write-Host "GOTorch Unit Test Runner" -ForegroundColor Cyan
Write-Host "Lib dir: $LibDir"
Write-Host "---"

# Set env for llama.cpp DLL loading.
$env:GOTORCH_LIB = $LibDir
$env:PATH = "$LibDir;$env:PATH"

# Find GOAgent root (assume we're running from scripts/doctor/).
$goagentRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
if (-not (Test-Path (Join-Path $goagentRoot "go.mod"))) {
    $goagentRoot = "e:\.agent\GOAgent"
}

Write-Host "GOAgent root: $goagentRoot"
Write-Host "---"

Push-Location $goagentRoot
try {
    $output = go test ./pkg/torch/... -count=1 -short 2>&1
    $exitCode = $LASTEXITCODE
    
    foreach ($line in $output) {
        if ($line -match "^ok") {
            Write-Host "[PASS] $line" -ForegroundColor Green
        }
        elseif ($line -match "FAIL") {
            Write-Host "[FAIL] $line" -ForegroundColor Red
        }
        else {
            Write-Host "       $line"
        }
    }
    
    Write-Host "---"
    if ($exitCode -eq 0) {
        Write-Host "All tests passed." -ForegroundColor Green
    }
    else {
        Write-Host "Some tests failed." -ForegroundColor Red
    }
    
    exit $exitCode
}
finally {
    Pop-Location
}
