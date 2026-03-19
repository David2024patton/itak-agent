@echo off
:: ──────────────────────────────────────────────────────────────────
::  iTaK Agent - One-Click Setup (Windows)
::
::  What this does:
::    1. Checks that Docker Desktop is running
::    2. Builds the iTaK Agent image
::    3. Starts the container on port 42800
::    4. Opens the dashboard in your browser
::
::  Usage:  Double-click this file, or run:  setup.bat
:: ──────────────────────────────────────────────────────────────────
title iTaK Agent Setup
color 0A

echo.
echo  ╔══════════════════════════════════════════╗
echo  ║        iTaK Agent - Quick Setup          ║
echo  ╚══════════════════════════════════════════╝
echo.

:: ── Step 1: Check Docker ─────────────────────────────────────────
echo  [1/4] Checking Docker...
docker info >nul 2>&1
if %errorlevel% neq 0 (
    color 0C
    echo.
    echo  ERROR: Docker is not running!
    echo.
    echo  Please start Docker Desktop and try again.
    echo  Download: https://www.docker.com/products/docker-desktop
    echo.
    pause
    exit /b 1
)
echo        Docker is running.

:: ── Step 2: Find project root ────────────────────────────────────
echo  [2/4] Locating project...

:: This script lives in Agent/ - the build context is the parent dir.
set "SCRIPT_DIR=%~dp0"
:: Remove trailing backslash
set "SCRIPT_DIR=%SCRIPT_DIR:~0,-1%"
:: Go up one level to iTaK Eco
for %%I in ("%SCRIPT_DIR%") do set "PROJECT_ROOT=%%~dpI"
set "PROJECT_ROOT=%PROJECT_ROOT:~0,-1%"
set "COMPOSE_FILE=%SCRIPT_DIR%\docker-compose.yml"

if not exist "%COMPOSE_FILE%" (
    color 0C
    echo  ERROR: docker-compose.yml not found at %COMPOSE_FILE%
    pause
    exit /b 1
)
echo        Project root: %PROJECT_ROOT%

:: ── Step 3: Build ────────────────────────────────────────────────
echo  [3/4] Building iTaK Agent (this takes ~30s on first run)...
echo.
cd /d "%PROJECT_ROOT%"
docker compose -f "%COMPOSE_FILE%" build
echo.

:: ── Step 4: Start ────────────────────────────────────────────────
echo  [4/4] Starting iTaK Agent...
docker compose -f "%COMPOSE_FILE%" up -d

:: Wait for the server to be ready.
echo.
echo  Waiting for agent to start...
timeout /t 3 /nobreak >nul

:: ── Done ─────────────────────────────────────────────────────────
echo.
echo  ╔══════════════════════════════════════════╗
echo  ║            Setup Complete!               ║
echo  ╠══════════════════════════════════════════╣
echo  ║                                          ║
echo  ║  Dashboard:  http://localhost:42800       ║
echo  ║  API:        http://localhost:42800/v1    ║
echo  ║  Health:     http://localhost:42800/health║
echo  ║                                          ║
echo  ║  Stop:       docker stop itak-agent      ║
echo  ║  Logs:       docker logs itak-agent      ║
echo  ║  Restart:    docker restart itak-agent    ║
echo  ║                                          ║
echo  ╚══════════════════════════════════════════╝
echo.

:: Open browser.
start http://localhost:42800

echo  Press any key to exit...
pause >nul
