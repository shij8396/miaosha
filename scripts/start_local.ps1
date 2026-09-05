# ============================================================
# 本地一键启动脚本（Windows / PowerShell 5.1+）
# 职责：确保 Docker Redis 就绪 → 构建后端 → 启动 → 健康检查
# 用法：
#   powershell -ExecutionPolicy Bypass -File scripts/start_local.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/start_local.ps1 -Restart   # 强制重启后端
# 依赖：Docker Desktop、本地 MySQL(3306)、Go 1.26.5
# 注意：本地 Redis 密码通过环境变量 MIAOSHA_REDIS_PASSWORD 注入，不写进 config.yaml
# ============================================================
param(
    [switch]$Restart,                    # 后端已在运行时强制重启
    [string]$RedisPassword = "miaosha_redis_2026",  # 本地 miaosha-redis-node1 容器密码
    [int]$RedisPort = 6379,
    [string]$RedisContainer = "miaosha-redis-node1"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot

# [已知问题] GOMODCACHE 默认指向 D:\Program Files\Go\pkg\mod 会权限拒绝，临时改用用户目录
$env:GOMODCACHE = "$env:USERPROFILE\go\pkg\mod"

# ---- Docker CLI（不在 PATH 时用完整路径）----
$docker = "C:\Users\宋家豪\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe"
if (-not (Test-Path $docker)) { $docker = "docker" }

function Ensure-Docker {
    & $docker ps *> $null
    if ($LASTEXITCODE -eq 0) { Write-Host "[1/5] Docker engine 已就绪" -ForegroundColor Green; return }
    Write-Host "[1/5] Docker Desktop 未就绪，尝试启动..."
    Start-Process "C:\Users\宋家豪\AppData\Local\Programs\DockerDesktop\Docker Desktop.exe" -ErrorAction SilentlyContinue
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Seconds 2
        & $docker ps *> $null
        if ($LASTEXITCODE -eq 0) { Write-Host "[1/5] Docker engine 就绪" -ForegroundColor Green; return }
    }
    Write-Error "Docker engine 启动超时（30s），请手动打开 Docker Desktop 后重试"
}

function Ensure-Redis {
    $running = & $docker ps --filter "name=$RedisContainer" --format "{{.Names}}"
    if ($running) {
        Write-Host "[2/5] Redis 容器 $RedisContainer 运行中" -ForegroundColor Green
    } else {
        $existed = & $docker ps -a --filter "name=$RedisContainer" --format "{{.Names}}"
        if ($existed) {
            Write-Host "[2/5] 启动已有 Redis 容器 $RedisContainer ..."
            & $docker start $RedisContainer | Out-Null
        } else {
            Write-Host "[2/5] 创建 Redis 容器 $RedisContainer ..."
            & $docker run -d --name $RedisContainer -p "${RedisPort}:6379" --restart unless-stopped redis:7-alpine redis-server --port 6379 --appendonly yes --requirepass $RedisPassword | Out-Null
        }
        Start-Sleep -Seconds 2
    }
    # 等待端口可连
    for ($i = 0; $i -lt 15; $i++) {
        $c = New-Object Net.Sockets.TcpClient
        try {
            $r = $c.BeginConnect("127.0.0.1", $RedisPort, $null, $null)
            if ($r.AsyncWaitHandle.WaitOne(1000)) { $c.EndConnect($r); $c.Close(); return }
        } catch { } finally { $c.Close() }
        Start-Sleep -Seconds 1
    }
    Write-Error "Redis 端口 ${RedisPort} 未就绪，请检查容器日志"
}

function Ensure-MySQL {
    $c = New-Object Net.Sockets.TcpClient
    try {
        $r = $c.BeginConnect("127.0.0.1", 3306, $null, $null)
        if ($r.AsyncWaitHandle.WaitOne(1000)) { $c.EndConnect($r); Write-Host "[3/5] MySQL 3306 就绪" -ForegroundColor Green; $c.Close(); return }
    } catch { } finally { $c.Close() }
    Write-Error "本地 MySQL(3306) 未启动，请先启动 MySQL 服务后重试"
}

function Build-Backend {
    Write-Host "[4/5] 构建 miaosha.exe ..."
    Push-Location $Root
    try {
        go build -o miaosha.exe ./cmd
        if ($LASTEXITCODE -ne 0) { Write-Error "go build 失败" }
        Write-Host "[4/5] 构建完成 $(Get-Item miaosha.exe | Select-Object -ExpandProperty LastWriteTime)" -ForegroundColor Green
    } finally { Pop-Location }
}

function Start-Backend {
    $proc = Get-Process -Name miaosha -ErrorAction SilentlyContinue
    $listener = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
    if ($proc -and $listener -and -not $Restart) {
        Write-Host "[5/5] 后端已在运行 PID=$($proc.Id)，跳过启动（-Restart 强制重启）" -ForegroundColor Yellow
        return $proc
    }
    if ($proc) {
        Write-Host "[5/5] 停止旧后端 PID=$($proc.Id) ..."
        Stop-Process -Id $proc.Id -Force
        Start-Sleep -Seconds 2
    }
    Write-Host "[5/5] 启动后端（MIAOSHA_REDIS_PASSWORD 注入）..."
    $env:MIAOSHA_REDIS_PASSWORD = $RedisPassword
    $logs = Join-Path $Root "logs"
    if (-not (Test-Path $logs)) { New-Item -ItemType Directory -Path $logs | Out-Null }
    $p = Start-Process -FilePath (Join-Path $Root "miaosha.exe") -WorkingDirectory $Root `
        -RedirectStandardOutput (Join-Path $logs "stdout.log") `
        -RedirectStandardError (Join-Path $logs "stderr.log") -PassThru
    return $p
}

function Verify-Health {
    for ($i = 0; $i -lt 20; $i++) {
        Start-Sleep -Seconds 1
        try {
            $r = Invoke-RestMethod -Uri "http://127.0.0.1:8080/health" -TimeoutSec 3
            if ($r.code -eq 200) {
                Write-Host "[✓] 健康检查 PASS: $($r.data.status) @ $($r.data.time)" -ForegroundColor Green
                return $true
            }
        } catch { }
    }
    Write-Host "[✗] 健康检查 FAIL，请查看 logs\stdout.log / logs\stderr.log" -ForegroundColor Red
    return $false
}

Write-Host "========== 本地秒杀系统一键启动 ==========" -ForegroundColor Cyan
Ensure-Docker
Ensure-Redis
Ensure-MySQL
Build-Backend
$p = Start-Backend
$ok = Verify-Health
if (-not $ok) { exit 1 }
Write-Host "========== 启动完成（PID=$($p.Id)）==========" -ForegroundColor Cyan
