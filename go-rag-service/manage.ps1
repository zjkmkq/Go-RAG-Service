param(
    [ValidateSet("status", "stop", "stopall")]
    [string]$Action = "status"
)

$ErrorActionPreference = "Continue"
Set-Location $PSScriptRoot

$Root = $PSScriptRoot

# 统一用 127.0.0.1 而不是 localhost：Ollama 只监听 IPv4，
# localhost 会先解析到 ::1 并耗掉整个超时才回退，导致把运行中的服务判成已停止。
$OllamaUrl = "http://127.0.0.1:11434"
$ServerUrl = "http://127.0.0.1:8080"

# ---------- 状态 ----------

function Get-ServiceStatus {
    $s = @{}

    $s.Container = [bool](docker ps --filter "name=rag-postgres" --filter "status=running" -q 2>$null)

    try {
        $v = Invoke-RestMethod -Uri "$OllamaUrl/api/version" -TimeoutSec 5
        $s.Ollama = $true
        $s.OllamaVer = $v.version
    } catch { $s.Ollama = $false; $s.OllamaVer = "" }

    try {
        $h = Invoke-RestMethod -Uri "$ServerUrl/health" -TimeoutSec 5
        $s.Server = ($h -eq "ok")
    } catch { $s.Server = $false }

    if ($s.Server) {
        try {
            $d = Invoke-RestMethod -Uri "$ServerUrl/api/v1/documents" -TimeoutSec 5
            $s.Docs = $d.total
            $sum = ($d.documents | Measure-Object -Property chunk_count -Sum).Sum
            $s.Chunks = if ($null -eq $sum) { 0 } else { [int]$sum }
        } catch { $s.Docs = $null; $s.Chunks = $null }
    }
    return $s
}

# 两个注意点：
# 1. 不要用 {0,-26} 定宽对齐，中文占两列宽却只算一个字符，补位后会折行错乱
# 2. 整行一次 Write-Host 输出，不要用 -NoNewline 拼接，否则重定向时会被拆成多行
function Write-Dot($on, $label, $extra) {
    if ($on) {
        $line = "    [*] $label  运行中"
        if ($extra) { $line += "  ($extra)" }
        Write-Host $line -ForegroundColor Green
    } else {
        Write-Host "    [ ] $label  已停止" -ForegroundColor DarkGray
    }
}

function Show-Status {
    $s = Get-ServiceStatus

    Write-Host ""
    Write-Host "  组件状态" -ForegroundColor Cyan
    Write-Dot $s.Container "PostgreSQL 容器 rag-postgres"
    $ver = if ($s.OllamaVer) { "v" + $s.OllamaVer } else { "" }
    Write-Dot $s.Ollama "Ollama 嵌入服务 :11434" $ver
    Write-Dot $s.Server "RAG 服务 :8080"

    if ($null -ne $s.Docs) {
        Write-Host "    知识库：$($s.Docs) 个文档 / $($s.Chunks) 个片段" -ForegroundColor DarkGray
    } elseif ($s.Server) {
        Write-Host "    知识库：读取失败" -ForegroundColor Yellow
    } else {
        Write-Host "    知识库：服务未运行，无法查询" -ForegroundColor DarkGray
    }
    Write-Host ""
    return $s
}

# ---------- 停止 ----------

function Stop-RagServer {
    $conns = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
    if (-not $conns) {
        Write-Host "  RAG 服务本来就没在跑" -ForegroundColor DarkGray
        return $true
    }

    foreach ($c in $conns) {
        $procId = $c.OwningProcess
        $proc = Get-CimInstance Win32_Process -Filter "ProcessId = $procId" -ErrorAction SilentlyContinue

        # go run 会留下一个 go.exe 父进程，先杀它，避免它把子进程重新拉起
        if ($proc -and $proc.ParentProcessId) {
            $parent = Get-CimInstance Win32_Process -Filter "ProcessId = $($proc.ParentProcessId)" -ErrorAction SilentlyContinue
            if ($parent -and $parent.Name -eq "go.exe") {
                Stop-Process -Id $parent.ProcessId -Force -ErrorAction SilentlyContinue
            }
        }
        Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
    }

    for ($i = 0; $i -lt 10; $i++) {
        Start-Sleep -Milliseconds 300
        if (-not (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue)) {
            Write-Host "  已停止 RAG 服务，8080 已释放" -ForegroundColor Green
            return $true
        }
    }
    Write-Host "  停止失败：8080 仍被占用" -ForegroundColor Red
    return $false
}

function Stop-Ollama {
    $p = Get-Process -Name ollama -ErrorAction SilentlyContinue
    if (-not $p) {
        Write-Host "  Ollama 本来就没在跑" -ForegroundColor DarkGray
        return
    }
    Stop-Process -Name ollama -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 500
    if (Get-Process -Name ollama -ErrorAction SilentlyContinue) {
        Write-Host "  Ollama 停止失败" -ForegroundColor Red
    } else {
        Write-Host "  已停止 Ollama（省约 40MB）" -ForegroundColor Green
    }
}

function Stop-Container {
    $running = docker ps --filter "name=rag-postgres" --filter "status=running" -q 2>$null
    if (-not $running) {
        Write-Host "  postgres 容器本来就没在跑" -ForegroundColor DarkGray
        return
    }
    docker stop rag-postgres 2>$null | Out-Null
    Write-Host "  已停止 postgres 容器（数据保留在 volume，下次启动还在）" -ForegroundColor Green
}

function Stop-All {
    Write-Host "  停止全部..." -ForegroundColor Cyan
    [void](Stop-RagServer)
    Stop-Ollama
    Stop-Container
    Write-Host ""
    Write-Host "  提示：Docker Desktop 本体（约 600MB）仍在后台运行。" -ForegroundColor DarkGray
    Write-Host "       需要的话从托盘图标退出它，但下次启动要等 1-2 分钟。" -ForegroundColor DarkGray
}

# ---------- 入口 ----------

switch ($Action) {
    "status"  { [void](Show-Status) }
    "stop"    { [void](Stop-RagServer) }
    "stopall" { Stop-All }
}
