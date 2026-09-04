# ============================================================
# 企业级秒杀系统 — 重度模拟测试脚本
# 测试覆盖：健康检查、用户认证、商品管理、并发秒杀、
#           订单管理、中间件状态、Sentinel限流、日志完整性
# ============================================================
param(
    [string]$BaseUrl = "http://localhost:8080",
    [int]$ConcurrentUsers = 10,
    [int]$SeckillRounds = 3
)

$ErrorActionPreference = "Continue"
$Results = @()
$Passed = 0
$Failed = 0

# ---------- 工具函数 ----------
function GenSign {
    param([string]$Method, [string]$Path, [string]$Body, [string]$Secret)
    $timestamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds().ToString()
    $signStr = "$timestamp`n$Method`n$Path`n$Body"
    $hmac = New-Object System.Security.Cryptography.HMACSHA256
    $hmac.Key = [Text.Encoding]::UTF8.GetBytes($Secret)
    $hash = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($signStr))
    $sign = [BitConverter]::ToString($hash).Replace("-","").ToLower()
    return @{timestamp=$timestamp; sign=$sign}
}

function ApiCall {
    param([string]$Method, [string]$Path, $Body, $Headers)
    $uri = "$BaseUrl$Path"
    $allHeaders = @{}
    if ($Headers) { $Headers.GetEnumerator() | ForEach-Object { $allHeaders[$_.Key] = $_.Value } }

    # 自动签名
    $sign = GenSign -Method $Method -Path $Path -Body ($Body | ConvertTo-Json -Compress) -Secret "miaosha-sign-secret-2026"
    $allHeaders["X-Timestamp"] = $sign.timestamp
    $allHeaders["X-Sign"] = $sign.sign
    $allHeaders["Content-Type"] = "application/json"

    try {
        $params = @{ Uri=$uri; Method=$Method; Headers=$allHeaders; SkipCertificateCheck=$true; ErrorAction="Stop" }
        if ($Body -and $Method -ne "GET") { $params.Body = ($Body | ConvertTo-Json -Compress) }
        $resp = Invoke-RestMethod @params
        return @{Success=$true; Data=$resp}
    } catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        try {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $body = $reader.ReadToEnd()
            return @{Success=$false; StatusCode=$statusCode; Error=$body}
        } catch {
            return @{Success=$false; StatusCode=$statusCode; Error=$_.Exception.Message}
        }
    }
}

function TestResult {
    param([string]$Name, [bool]$Pass, $Detail)
    $result = @{Name=$Name; Pass=$Pass; Detail=$Detail; Time=(Get-Date -Format "HH:mm:ss")}
    $Results += $result
    if ($Pass) { $global:Passed++; Write-Host "  [PASS] $Name" -ForegroundColor Green }
    else { $global:Failed++; Write-Host "  [FAIL] $Name - $Detail" -ForegroundColor Red }
}

Write-Host @"
`n============================================================
  企业级秒杀系统 — 重度模拟测试
  时间: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')
  目标: $BaseUrl
  并发用户: $ConcurrentUsers | 秒杀轮次: $SeckillRounds
============================================================
"@ -ForegroundColor Cyan

# ============================================================
# 阶段 1: 健康检查
# ============================================================
Write-Host "`n>>> 阶段 1: 健康检查" -ForegroundColor Yellow

$r = ApiCall -Method "GET" -Path "/health"
TestResult "Liveness探针" ($r.Success -and $r.Data.status -eq "ok") "status=$($r.Data.status)"

$r = ApiCall -Method "GET" -Path "/health?type=readiness"
$depsOk = $r.Success -and $r.Data.deps.mysql -eq "up" -and $r.Data.deps.redis -eq "up" -and $r.Data.deps.rabbitmq -eq "up"
TestResult "Readiness探针(MySQL+Redis+RabbitMQ)" $depsOk "deps=$($r.Data.deps | ConvertTo-Json -Compress)"

$r = ApiCall -Method "GET" -Path "/swagger/index.html"
TestResult "Swagger文档" $r.Success "HTTP $($r.StatusCode)"

# ============================================================
# 阶段 2: 用户认证
# ============================================================
Write-Host "`n>>> 阶段 2: 用户认证" -ForegroundColor Yellow

# 登录
$r = ApiCall -Method "POST" -Path "/api/v1/user/login" -Body @{username="admin";password="admin123"}
$token = $r.Data.data.token
$hasToken = $r.Success -and $token -ne $null
TestResult "管理员登录" $hasToken "token=$($token.Substring(0,20))..."

# 获取用户信息
$authHeaders = @{"Authorization"="Bearer $token"}
$r = ApiCall -Method "GET" -Path "/api/v1/user/info" -Headers $authHeaders
TestResult "获取用户信息" ($r.Success -and $r.Data.data.username -eq "admin") "role=$($r.Data.data.role)"

# 注册新用户
$testUser = "testuser_$(Get-Random -Minimum 1000 -Maximum 9999)"
$r = ApiCall -Method "POST" -Path "/api/v1/user/register" -Body @{username=$testUser;password="test123";nickname="测试用户";phone="13800138000"}
TestResult "注册新用户($testUser)" $r.Success "user_id=$($r.Data.data.user_id)"

# 新用户登录
$r = ApiCall -Method "POST" -Path "/api/v1/user/login" -Body @{username=$testUser;password="test123"}
$userToken = $r.Data.data.token
TestResult "新用户登录" ($r.Success -and $userToken -ne $null) ""

# 无Token访问
$r = ApiCall -Method "GET" -Path "/api/v1/user/info"
TestResult "无Token拦截(401)" ($r.StatusCode -eq 401) "status=$($r.StatusCode)"

# 错误密码
$r = ApiCall -Method "POST" -Path "/api/v1/user/login" -Body @{username="admin";password="wrongpassword"}
TestResult "错误密码拒绝" ($r.StatusCode -eq 400) ""

# ============================================================
# 阶段 3: 商品管理
# ============================================================
Write-Host "`n>>> 阶段 3: 商品管理" -ForegroundColor Yellow

# 创建商品
$suffix = Get-Random -Minimum 1000 -Maximum 9999
$r = ApiCall -Method "POST" -Path "/api/v1/product" -Body @{
    name="测试商品-$suffix"
    description="重度模拟测试秒杀商品"
    price=99.99
    seckill_price=9.99
    stock=100
    limit_num=3
    status=1
    start_time=(Get-Date).AddMinutes(-1).ToString("yyyy-MM-dd HH:mm:ss")
    end_time=(Get-Date).AddHours(2).ToString("yyyy-MM-dd HH:mm:ss")
} -Headers $authHeaders
$productId = $r.Data.data.id
TestResult "创建秒杀商品" ($r.Success -and $productId -gt 0) "product_id=$productId"

# 创建第二个商品
$r = ApiCall -Method "POST" -Path "/api/v1/product" -Body @{
    name="测试商品2-$suffix"
    description="第二个测试商品"
    price=199.99
    seckill_price=19.99
    stock=50
    limit_num=1
    status=1
    start_time=(Get-Date).AddMinutes(-1).ToString("yyyy-MM-dd HH:mm:ss")
    end_time=(Get-Date).AddHours(2).ToString("yyyy-MM-dd HH:mm:ss")
} -Headers $authHeaders
$productId2 = $r.Data.data.id
TestResult "创建第二个商品" ($r.Success -and $productId2 -gt 0) "product_id=$productId2"

# 获取活跃商品
$r = ApiCall -Method "GET" -Path "/api/v1/product/active" -Headers $authHeaders
TestResult "获取活跃商品" ($r.Success -and $r.Data.data.Count -ge 2) "count=$($r.Data.data.Count)"

# 获取商品详情
$r = ApiCall -Method "GET" -Path "/api/v1/product/$productId" -Headers $authHeaders
TestResult "获取商品详情" ($r.Success -and $r.Data.data.id -eq $productId) "name=$($r.Data.data.name)"

# 活动配置缓存预热
$r = ApiCall -Method "POST" -Path "/api/v1/activity/cache-warmup" -Body @{product_id=$productId} -Headers $authHeaders
TestResult "缓存预热" $r.Success ""

# ============================================================
# 阶段 4: 并发秒杀（核心压力测试）
# ============================================================
Write-Host "`n>>> 阶段 4: 并发秒杀（$ConcurrentUsers 并发 × $SeckillRounds 轮）" -ForegroundColor Yellow

$seckillSuccess = 0
$seckillFail = 0
$seckillErrors = @()

$seckillScript = {
    param($BaseUrl, $Token, $ProductId, $Round, $Secret)
    $results = @()
    $idempotentKey = [Guid]::NewGuid().ToString()
    $body = @{product_id=$ProductId; quantity=1; idempotent_key=$idempotentKey} | ConvertTo-Json -Compress

    $timestamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds().ToString()
    $signStr = "$timestamp`nPOST`n/api/v1/seckill`n$body"
    $hmac = New-Object System.Security.Cryptography.HMACSHA256
    $hmac.Key = [Text.Encoding]::UTF8.GetBytes($Secret)
    $hash = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($signStr))
    $sign = [BitConverter]::ToString($hash).Replace("-","").ToLower()

    $headers = @{
        "Authorization"="Bearer $Token"
        "Content-Type"="application/json"
        "X-Timestamp"=$timestamp
        "X-Sign"=$sign
    }

    try {
        $resp = Invoke-RestMethod -Uri "$BaseUrl/api/v1/seckill" -Method POST -Body $body -Headers $headers -SkipCertificateCheck -ErrorAction Stop
        $results += "SUCCESS: order_no=$($resp.data.order_no)"
    } catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        try {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $msg = $reader.ReadToEnd()
            $results += "FAIL($statusCode): $msg"
        } catch {
            $results += "FAIL($statusCode): $($_.Exception.Message)"
        }
    }
    return $results
}

# 预热一次（确保库存已在 Redis）
$r = ApiCall -Method "POST" -Path "/api/v1/seckill" -Body @{product_id=$productId; quantity=1; idempotent_key=[Guid]::NewGuid().ToString()} -Headers $authHeaders
Write-Host "  预热请求: $($r.Data.data.order_no)"

# 并发秒杀
for ($round = 1; $round -le $SeckillRounds; $round++) {
    Write-Host "  --- 第 $round 轮 ---" -ForegroundColor DarkGray
    $jobs = @()
    for ($i = 0; $i -lt $ConcurrentUsers; $i++) {
        $jobs += Start-Job -ScriptBlock $seckillScript -ArgumentList $BaseUrl,$userToken,$productId,$round,"miaosha-sign-secret-2026"
    }
    $jobs | Wait-Job | Out-Null
    foreach ($job in $jobs) {
        $result = $job | Receive-Job
        $job | Remove-Job
        foreach ($line in $result) {
            if ($line -like "SUCCESS:*") { $seckillSuccess++ }
            else { $seckillFail++; $seckillErrors += $line }
        }
    }
    Write-Host "    本轮: 成功=$seckillSuccess, 失败=$seckillFail"
    Start-Sleep -Milliseconds 500
}

TestResult "并发秒杀(总计$($ConcurrentUsers * $SeckillRounds)次)" ($seckillSuccess -gt 0) "成功=$seckillSuccess, 失败=$seckillFail"

# 测试幂等性（重复提交）
$key = [Guid]::NewGuid().ToString()
$r1 = ApiCall -Method "POST" -Path "/api/v1/seckill" -Body @{product_id=$productId; quantity=1; idempotent_key=$key} -Headers $authHeaders
Start-Sleep -Milliseconds 200
$r2 = ApiCall -Method "POST" -Path "/api/v1/seckill" -Body @{product_id=$productId; quantity=1; idempotent_key=$key} -Headers $authHeaders
$dupBlocked = $r2.StatusCode -eq 400 -or ($r2.Data -and $r2.Data.code -eq 400)
TestResult "幂等性防重复提交" $dupBlocked "第1次=$($r1.Success), 第2次拦截=$($r2.StatusCode)"

# 测试库存不足
Write-Host "  测试库存耗尽场景..."
for ($i = 0; $i -lt 20; $i++) {
    $r = ApiCall -Method "POST" -Path "/api/v1/seckill" -Body @{product_id=$productId2; quantity=1; idempotent_key=[Guid]::NewGuid().ToString()} -Headers $authHeaders
    if ($r.StatusCode -eq 400) { break }
}
$r = ApiCall -Method "POST" -Path "/api/v1/seckill" -Body @{product_id=$productId2; quantity=1; idempotent_key=[Guid]::NewGuid().ToString()} -Headers $authHeaders
TestResult "库存不足提示" ($r.StatusCode -eq 400) ""

# ============================================================
# 阶段 5: 订单管理
# ============================================================
Write-Host "`n>>> 阶段 5: 订单管理" -ForegroundColor Yellow

$r = ApiCall -Method "GET" -Path "/api/v1/order/list?page=1&page_size=10" -Headers $authHeaders
$orderCount = $r.Data.data.list.Count
TestResult "订单列表(管理员)" ($r.Success -and $orderCount -gt 0) "count=$orderCount"

$r = ApiCall -Method "GET" -Path "/api/v1/order/all?page=1&page_size=10" -Headers $authHeaders
TestResult "全部订单(管理员)" $r.Success "total=$($r.Data.data.total)"

# 用户订单
$userAuthHeaders = @{"Authorization"="Bearer $userToken"}
$r = ApiCall -Method "GET" -Path "/api/v1/order/list?page=1&page_size=10" -Headers $userAuthHeaders
TestResult "用户订单列表" $r.Success "count=$($r.Data.data.list.Count)"

# ============================================================
# 阶段 6: 中间件状态 & 监控
# ============================================================
Write-Host "`n>>> 阶段 6: 中间件状态 & 监控" -ForegroundColor Yellow

$r = ApiCall -Method "GET" -Path "/api/v1/monitor/middleware" -Headers $authHeaders
TestResult "中间件状态" ($r.Success -and $r.Data.data -ne $null) ""

$r = ApiCall -Method "GET" -Path "/api/v1/monitor/qps" -Headers $authHeaders
TestResult "QPS监控" ($r.Success) ""

$r = ApiCall -Method "GET" -Path "/api/v1/monitor/slow-api" -Headers $authHeaders
TestResult "慢接口TOP排行" $r.Success ""

$r = ApiCall -Method "GET" -Path "/api/v1/seckill/stats" -Headers $authHeaders
TestResult "秒杀统计" $r.Success ""

$r = ApiCall -Method "GET" -Path "/api/v1/monitor/metrics" -Headers $authHeaders
TestResult "Prometheus指标" ($r.Success) ""

# ============================================================
# 阶段 7: Sentinel & 黑名单
# ============================================================
Write-Host "`n>>> 阶段 7: Sentinel & 黑名单" -ForegroundColor Yellow

$r = ApiCall -Method "GET" -Path "/api/v1/sentinel/rules" -Headers $authHeaders
TestResult "Sentinel规则列表" $r.Success ""

$r = ApiCall -Method "GET" -Path "/api/v1/sentinel/blacklist" -Headers $authHeaders
TestResult "黑名单列表" $r.Success ""

# ============================================================
# 阶段 8: 审计日志 & 对账
# ============================================================
Write-Host "`n>>> 阶段 8: 审计日志 & 对账" -ForegroundColor Yellow

$r = ApiCall -Method "GET" -Path "/api/v1/audit/list?page=1&page_size=10" -Headers $authHeaders
TestResult "审计日志" $r.Success ""

$r = ApiCall -Method "GET" -Path "/api/v1/order/recon-diff?page=1&page_size=10" -Headers $authHeaders
TestResult "库存对账差异" $r.Success "total=$($r.Data.data.total)"

# ============================================================
# 阶段 9: 请求体限制 & 签名验证
# ============================================================
Write-Host "`n>>> 阶段 9: 安全防护" -ForegroundColor Yellow

$r = ApiCall -Method "GET" -Path "/api/v1/user/info" -Headers @{"Authorization"="Bearer invalidtoken123"}
TestResult "无效Token拦截" ($r.StatusCode -eq 401) ""

# 签名验证
$r = ApiCall -Method "GET" -Path "/api/v1/user/info" -Headers @{"Authorization"="Bearer $token"; "X-Timestamp"="bad"; "X-Sign"="bad"}
TestResult "签名验证拦截" ($r.StatusCode -eq 401) ""

# ============================================================
# 阶段 10: 日志完整性
# ============================================================
Write-Host "`n>>> 阶段 10: 日志完整性" -ForegroundColor Yellow

$logExists = Test-Path "e:\TRAE SOLO CN\project\miaosha\logs\miaosha.log"
TestResult "日志文件存在" $logExists ""

if ($logExists) {
    $logSize = (Get-Item "e:\TRAE SOLO CN\project\miaosha\logs\miaosha.log").Length
    $logLines = (Get-Content "e:\TRAE SOLO CN\project\miaosha\logs\miaosha.log" | Measure-Object).Count
    TestResult "日志内容非空" ($logSize -gt 0) "size=$($logSize)bytes, lines=$logLines"
}

# ============================================================
# 汇总报告
# ============================================================
Write-Host @"

============================================================
  测试汇总报告
============================================================
  总测试项: $($Passed + $Failed)
  通过: $Passed (绿色)
  失败: $Failed (红色)
  成功率: $([math]::Round($Passed / ($Passed + $Failed) * 100, 1))%

  并发秒杀统计:
    总请求: $($ConcurrentUsers * $SeckillRounds)
    成功: $seckillSuccess
    失败: $seckillFail
    成功率: $([math]::Round($seckillSuccess / ($ConcurrentUsers * $SeckillRounds) * 100, 1))%
============================================================
"@ -ForegroundColor Cyan

if ($seckillErrors.Count -gt 0) {
    Write-Host "  秒杀失败详情（前 5 条）:" -ForegroundColor Yellow
    $seckillErrors | Select-Object -First 5 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
}

# 返回退出码
if ($Failed -gt 0) { exit 1 } else { exit 0 }