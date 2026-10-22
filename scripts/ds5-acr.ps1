#Requires -Version 5.1
<#
.SYNOPSIS
    Start/stop the Assetto Corsa (Rally) haptics stack on Windows:
    the native ds5d.exe daemon plus ds5bridge.exe reading acpmf_physics.

.EXAMPLE
    .\scripts\ds5-acr.ps1 start
    .\scripts\ds5-acr.ps1 status
    .\scripts\ds5-acr.ps1 stop
#>
param(
    [Parameter(Position = 0)]
    [ValidateSet("start", "stop", "restart", "bridge-restart", "status", "logs")]
    [string]$Action
)

$ErrorActionPreference = "Stop"

$RepoDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BinDir  = if ($env:DS5_BIN)     { $env:DS5_BIN }     else { Join-Path $RepoDir "bin" }
$Profile = if ($env:DS5_PROFILE) { $env:DS5_PROFILE } else { "assetto-corsa" }
$Rate    = if ($env:DS5_RATE)    { $env:DS5_RATE }    else { "125" }
$Layout  = if ($env:DS5_LAYOUT)  { $env:DS5_LAYOUT }  else { "acr" }

$RunDir    = Join-Path $env:LOCALAPPDATA "ds5-acr"
$Ds5dLog   = Join-Path $RunDir "ds5d.log"
$Ds5dErr   = Join-Path $RunDir "ds5d.err.log"
$BridgeLog = Join-Path $RunDir "bridge.log"
$BridgeErr = Join-Path $RunDir "bridge.err.log"

$Ds5dBin   = Join-Path $BinDir "ds5d.exe"
$BridgeBin = Join-Path $BinDir "ds5bridge.exe"

function Require-Binaries {
    if (-not (Test-Path $Ds5dBin)) {
        Write-Error "ds5-acr: missing $Ds5dBin (run: make windows)"
        exit 1
    }
    if (-not (Test-Path $BridgeBin)) {
        Write-Error "ds5-acr: missing $BridgeBin (run: make windows)"
        exit 1
    }
}

function Start-Daemon {
    $existing = Get-Process -Name "ds5d" -ErrorAction SilentlyContinue
    if ($existing) {
        $pids = ($existing | Select-Object -ExpandProperty Id) -join " "
        Write-Host "ds5d already running (pid $pids) -- leaving it alone"
        return
    }
    Remove-Item -Path $Ds5dLog, $Ds5dErr -Force -ErrorAction SilentlyContinue
    $proc = Start-Process -FilePath $Ds5dBin `
        -ArgumentList @("-profile", $Profile, "-rate", $Rate, "-trace") `
        -RedirectStandardOutput $Ds5dLog `
        -RedirectStandardError $Ds5dErr `
        -WindowStyle Hidden -PassThru
    Start-Sleep -Seconds 2
    if ($proc.HasExited) {
        Write-Host "ds5d failed to start:" -ForegroundColor Red
        if (Test-Path $Ds5dErr) { Get-Content $Ds5dErr -Tail 10 }
        exit 1
    }
    Write-Host "ds5d     : started (pid $($proc.Id), profile $Profile, rate $Rate Hz)"
}

function Start-Bridge {
    $existing = Get-Process -Name "ds5bridge" -ErrorAction SilentlyContinue
    if ($existing) {
        $pids = ($existing | Select-Object -ExpandProperty Id) -join " "
        Write-Host "bridge already running (pid $pids) -- leaving it alone"
        return
    }
    Remove-Item -Path $BridgeLog, $BridgeErr -Force -ErrorAction SilentlyContinue
    $proc = Start-Process -FilePath $BridgeBin `
        -ArgumentList @("-layout", $Layout, "-debug") `
        -RedirectStandardOutput $BridgeLog `
        -RedirectStandardError $BridgeErr `
        -WindowStyle Hidden -PassThru
    Write-Host "bridge   : started (pid $($proc.Id), layout $Layout)"
}

function Stop-All {
    $bridgeProcs = Get-Process -Name "ds5bridge" -ErrorAction SilentlyContinue
    if ($bridgeProcs) {
        $bridgeProcs | Stop-Process -Force
        Write-Host "bridge   : stopped"
    }
    $ds5dProcs = Get-Process -Name "ds5d" -ErrorAction SilentlyContinue
    if ($ds5dProcs) {
        $ds5dProcs | Stop-Process -Force
        Write-Host "ds5d     : stopped"
    }
    Start-Sleep -Milliseconds 500
    Write-Host "controller is back to default control"
}

function Show-Status {
    $dProcs = Get-Process -Name "ds5d" -ErrorAction SilentlyContinue
    $bProcs = Get-Process -Name "ds5bridge" -ErrorAction SilentlyContinue
    $dPids = if ($dProcs) { ($dProcs | Select-Object -ExpandProperty Id) -join " " } else { "not running" }
    $bPids = if ($bProcs) { ($bProcs | Select-Object -ExpandProperty Id) -join " " } else { "not running" }

    Write-Host "ds5d     : $dPids"
    Write-Host "bridge   : $bPids"

    foreach ($f in @($Ds5dErr, $Ds5dLog)) {
        if ($dProcs -and (Test-Path $f)) {
            $sig = Select-String -Path $f -Pattern "signals:" -ErrorAction SilentlyContinue | Select-Object -Last 1
            if ($sig) {
                Write-Host "signals  : $($sig.Line)"
                break
            }
        }
    }

    foreach ($f in @($BridgeErr, $BridgeLog)) {
        if ($bProcs -and (Test-Path $f)) {
            $line = Select-String -Path $f -Pattern "streaming|not found|packetId|EVENT" -ErrorAction SilentlyContinue | Select-Object -Last 1
            if ($line) {
                Write-Host "bridge   : $($line.Line)"
                break
            }
        }
    }
}

switch ($Action) {
    "start" {
        Require-Binaries
        New-Item -ItemType Directory -Path $RunDir -Force | Out-Null
        Start-Daemon
        Start-Bridge
        Write-Host ""
        Write-Host "log: $Ds5dErr"
        Write-Host "log: $BridgeErr"
        Write-Host "start a session in the game -- the bridge connects once acpmf_physics appears"
    }
    "stop" {
        Stop-All
    }
    "restart" {
        Stop-All
        New-Item -ItemType Directory -Path $RunDir -Force | Out-Null
        Require-Binaries
        Start-Daemon
        Start-Bridge
    }
    "bridge-restart" {
        $bProcs = Get-Process -Name "ds5bridge" -ErrorAction SilentlyContinue
        if ($bProcs) {
            $bProcs | Stop-Process -Force
        }
        New-Item -ItemType Directory -Path $RunDir -Force | Out-Null
        Require-Binaries
        Start-Daemon
        Start-Bridge
    }
    "status" {
        Show-Status
    }
    "logs" {
        $files = @($Ds5dErr, $Ds5dLog, $BridgeErr, $BridgeLog) | Where-Object { Test-Path $_ }
        if (-not $files) {
            Write-Host "No log files found in $RunDir"
            exit 0
        }
        Get-Content -Path $files -Tail 40 -Wait
    }
    default {
        Write-Host "usage: .\scripts\ds5-acr.ps1 {start|stop|restart|bridge-restart|status|logs}"
        exit 2
    }
}
