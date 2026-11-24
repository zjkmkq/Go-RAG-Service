# Ensure the three layers of the RAG service are up: postgres container -> Ollama -> RAG service
# Called by start-server.bat. Keep this file ASCII-only: PowerShell 5.1 reads BOM-less
# files as ANSI, so non-ASCII text here would show up garbled.

$ErrorActionPreference = "Continue"
Set-Location $PSScriptRoot

Write-Host "[1/3] database container rag-postgres" -ForegroundColor Cyan
$running = docker ps --filter "name=rag-postgres" --filter "status=running" -q 2>$null
if ($running) {
    Write-Host "      already running" -ForegroundColor Green
} else {
    Write-Host "      not running, starting..." -ForegroundColor Yellow
    docker start rag-postgres 2>$null | Out-Null
    Start-Sleep -Seconds 3
    $running = docker ps --filter "name=rag-postgres" --filter "status=running" -q 2>$null
    if ($running) {
        Write-Host "      started" -ForegroundColor Green
    } else {
        Write-Host "      FAILED. Open Docker Desktop, wait until it shows Running, then retry." -ForegroundColor Red
        exit 1
    }
}

Write-Host "[2/3] Ollama embedding service" -ForegroundColor Cyan
$ollamaOK = $false
try {
    $v = Invoke-RestMethod -Uri "http://127.0.0.1:11434/api/version" -TimeoutSec 3
    Write-Host "      already running ($($v.version))" -ForegroundColor Green
    $ollamaOK = $true
} catch {
    # Locate ollama.exe without hardcoding a machine-specific path.
    # Order: OLLAMA_BIN env var -> ollama on PATH -> default install locations.
    $exe = $null
    if ($env:OLLAMA_BIN -and (Test-Path $env:OLLAMA_BIN)) {
        $exe = $env:OLLAMA_BIN
    } else {
        $onPath = Get-Command ollama -ErrorAction SilentlyContinue
        if ($onPath) { $exe = $onPath.Source }
    }
    if (-not $exe) {
        $candidates = @(
            "$env:LOCALAPPDATA\Programs\Ollama\ollama.exe",
            "${env:ProgramFiles}\Ollama\ollama.exe"
        )
        foreach ($c in $candidates) { if (Test-Path $c) { $exe = $c; break } }
    }
    if ($exe) {
        Write-Host "      not running, launching $exe" -ForegroundColor Yellow
        Start-Process -FilePath $exe -ArgumentList "serve" -WindowStyle Hidden
        Start-Sleep -Seconds 6
        try {
            $v = Invoke-RestMethod -Uri "http://127.0.0.1:11434/api/version" -TimeoutSec 3
            Write-Host "      started ($($v.version))" -ForegroundColor Green
            $ollamaOK = $true
        } catch {
            Write-Host "      FAILED to start. Launch 'ollama serve' manually." -ForegroundColor Red
        }
    } else {
        Write-Host "      ollama.exe not found. Install Ollama or set OLLAMA_BIN." -ForegroundColor Red
    }
}
if (-not $ollamaOK) { exit 1 }

Write-Host "[3/3] RAG service" -ForegroundColor Cyan
try {
    $h = Invoke-RestMethod -Uri "http://127.0.0.1:8080/health" -TimeoutSec 3
    if ($h -eq "ok") {
        Write-Host "      already running" -ForegroundColor Green
        exit 0
    }
} catch {}

Write-Host "      starting in a new window..." -ForegroundColor Yellow

# 启动新进程有两种失败可能：
#   1. Start-Process 在某些环境下会抛 ArgumentException
#      （环境变量里同时存在 HTTPS_PROXY 和 https_proxy 时，构造环境字典会撞键）
#   2. 执行策略/安全软件拦截
# 所以这里给出两条路径，都失败就打印手动启动命令，不要静默失败。
$started = $false
try {
    $p = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/server" -WorkingDirectory $PSScriptRoot -PassThru -ErrorAction Stop
    Write-Host "      started (pid $($p.Id))" -ForegroundColor Green
    $started = $true
} catch {
    Write-Host "      Start-Process failed: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host "      trying WMI..." -ForegroundColor Yellow
    try {
        $cmdLine = "cmd /c cd /d `"$PSScriptRoot`" && go run ./cmd/server"
        $r = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{ CommandLine = $cmdLine }
        if ($r.ReturnValue -eq 0) {
            Write-Host "      started via WMI (pid $($r.ProcessId)), no console window" -ForegroundColor Green
            $started = $true
        } else {
            Write-Host "      WMI returned $($r.ReturnValue)" -ForegroundColor Red
        }
    } catch {
        Write-Host "      WMI failed: $($_.Exception.Message)" -ForegroundColor Red
    }
}

if (-not $started) {
    Write-Host ""
    Write-Host "      Could not start the service automatically." -ForegroundColor Red
    Write-Host "      Run these two lines in a terminal window:" -ForegroundColor Yellow
    Write-Host "          cd $PSScriptRoot" -ForegroundColor Yellow
    Write-Host "          go run ./cmd/server" -ForegroundColor Yellow
    exit 1
}

for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 1
    try {
        $h = Invoke-RestMethod -Uri "http://127.0.0.1:8080/health" -TimeoutSec 2
        if ($h -eq "ok") {
            Write-Host "      ready in $($i + 1)s" -ForegroundColor Green
            exit 0
        }
    } catch {}
}
Write-Host "      not up within 30s, check the server window for errors" -ForegroundColor Red
exit 1
