#Requires -Version 5.1
<#
.SYNOPSIS
    Start/stop the Richard Burns Rally (RBR) haptics and virtual pad stack on Windows:
    the native ds5vpad.exe (DirectInput 8 via ViGEm DS4) plus ds5d.exe reading NGP UDP 6776.

.EXAMPLE
    .\scripts\ds5-rbr.ps1 start
    .\scripts\ds5-rbr.ps1 status
    .\scripts\ds5-rbr.ps1 stop
#>
param(
    [Parameter(Position = 0)]
    [ValidateSet("start", "stop", "restart", "vpad-restart", "status", "logs")]
    [string]$Action
)

$ErrorActionPreference = "Stop"

$RepoDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BinDir  = if ($env:DS5_BIN)     { $env:DS5_BIN }     else { Join-Path $RepoDir "bin" }
$Profile = if ($env:DS5_PROFILE) { $env:DS5_PROFILE } else { "rbr" }
$Rate    = if ($env:DS5_RATE)    { $env:DS5_RATE }    else { "125" }
$Proto   = if ($env:DS5_PROTO)   { $env:DS5_PROTO }   else { "dinput8" }

$RunDir   = if ($env:DS5_RUN_DIR) { $env:DS5_RUN_DIR } else { Join-Path $env:LOCALAPPDATA "ds5-rbr" }
$Ds5dLog  = Join-Path $RunDir "ds5d.log"
$Ds5dErr  = Join-Path $RunDir "ds5d.err.log"
$VpadLog  = Join-Path $RunDir "vpad.log"
$VpadErr  = Join-Path $RunDir "vpad.err.log"

$Ds5dBin  = Join-Path $BinDir "ds5d.exe"
$VpadBin  = Join-Path $BinDir "ds5vpad.exe"

function Require-Binaries {
    if (-not (Test-Path $Ds5dBin)) {
        Write-Error "ds5-rbr: missing $Ds5dBin (run: make windows)"
        exit 1
    }
    if (-not (Test-Path $VpadBin)) {
        Write-Error "ds5-rbr: missing $VpadBin (run: make windows)"
        exit 1
    }
}

function Start-VPad {
    $existing = Get-Process -Name "ds5vpad" -ErrorAction SilentlyContinue
    if ($existing) {
        $pids = ($existing | Select-Object -ExpandProperty Id) -join " "
        Write-Host "ds5vpad already running (pid $pids) -- leaving it alone"
        return
    }
    Remove-Item -Path $VpadLog, $VpadErr -Force -ErrorAction SilentlyContinue
    $proc = Start-Process -FilePath $VpadBin `
        -ArgumentList @("-proto", $Proto) `
        -RedirectStandardOutput $VpadLog `
        -RedirectStandardError $VpadErr `
        -WindowStyle Hidden -PassThru
    Start-Sleep -Seconds 1
    if ($proc.HasExited) {
        Write-Host "ds5vpad failed to start:" -ForegroundColor Red
        if (Test-Path $VpadErr) { Get-Content $VpadErr -Tail 10 }
        exit 1
    }
    Write-Host "ds5vpad  : started (pid $($proc.Id), proto $Proto)"
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

function Stop-All {
    $vpadProcs = Get-Process -Name "ds5vpad" -ErrorAction SilentlyContinue
    if ($vpadProcs) {
        $vpadProcs | Stop-Process -Force
        Write-Host "ds5vpad  : stopped"
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
    $vProcs = Get-Process -Name "ds5vpad" -ErrorAction SilentlyContinue
    $dPids = if ($dProcs) { ($dProcs | Select-Object -ExpandProperty Id) -join " " } else { "not running" }
    $vPids = if ($vProcs) { ($vProcs | Select-Object -ExpandProperty Id) -join " " } else { "not running" }

    Write-Host "ds5d     : $dPids"
    Write-Host "ds5vpad  : $vPids"

    foreach ($f in @($Ds5dErr, $Ds5dLog)) {
        if ($dProcs -and (Test-Path $f)) {
            $sig = Select-String -Path $f -Pattern "signals:" -ErrorAction SilentlyContinue | Select-Object -Last 1
            if ($sig) {
                Write-Host "signals  : $($sig.Line)"
                break
            }
        }
    }

    foreach ($f in @($VpadErr, $VpadLog)) {
        if ($vProcs -and (Test-Path $f)) {
            $line = Select-String -Path $f -Pattern "reading|forwarding|vpad|ipc" -ErrorAction SilentlyContinue | Select-Object -Last 1
            if ($line) {
                Write-Host "vpad     : $($line.Line)"
                break
            }
        }
    }
}

switch ($Action) {
    "start" {
        Require-Binaries
        New-Item -ItemType Directory -Path $RunDir -Force | Out-Null
        Start-VPad
        Start-Daemon
        Write-Host ""
        Write-Host "log: $VpadErr"
        Write-Host "log: $Ds5dErr"
        Write-Host "launch Richard Burns Rally (RSF) -- NGP UDP telemetry (port 6776) and virtual DirectInput 8 gamepad are active"
    }
    "stop" {
        Stop-All
    }
    "restart" {
        Stop-All
        New-Item -ItemType Directory -Path $RunDir -Force | Out-Null
        Require-Binaries
        Start-VPad
        Start-Daemon
    }
    "vpad-restart" {
        $vProcs = Get-Process -Name "ds5vpad" -ErrorAction SilentlyContinue
        if ($vProcs) {
            $vProcs | Stop-Process -Force
        }
        New-Item -ItemType Directory -Path $RunDir -Force | Out-Null
        Require-Binaries
        Start-VPad
    }
    "status" {
        Show-Status
    }
    "logs" {
        $files = @($Ds5dErr, $Ds5dLog, $VpadErr, $VpadLog) | Where-Object { Test-Path $_ }
        if (-not $files) {
            Write-Host "No log files found in $RunDir"
            exit 0
        }
        Get-Content -Path $files -Tail 40 -Wait
    }
    default {
        Write-Host "usage: .\scripts\ds5-rbr.ps1 {start|stop|restart|vpad-restart|status|logs}"
        exit 2
    }
}
