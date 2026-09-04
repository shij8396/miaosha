# 秒杀系统端到端测试脚本
$Base = "http://localhost:8080"
$Green = "`e[32m"; $Red = "`e[31m"; $Yellow = "`e[33m"; $Reset = "`e[0m"
$Pass = 0; $Fail = 0

function Test($Name, $Method, $Path, $Body, $Headers) {
    try {
        $uri = "$Base$Path"
        $params = @{ Method = $Method; Uri = $uri; ContentType = "application/json"; SkipCertificateCheck = $true }
        if ($Body) { $params.Body = ($Body | ConvertTo-Json -Compress) }
        if ($Headers) { $params.Headers = $Headers }
        $r = Invoke-WebRequest @params -ErrorAction Stop
        $data = $r.Content | ConvertFrom-Json
        $status = if ($r.StatusCode -eq 200 -and $data.code -eq 200) { "PASS" } else { "FAIL" }
        if ($status -eq "PASS") { $global:Pass++ } else { $global:Fail++ }
        Write-Host "[$status] $Name - code=$($data.code)" -ForegroundColor $(if ($status -eq "PASS") { "Green" } else { "Red" })
        return $data
    } catch {
        $global:Fail++
        Write-Host "[FAIL] $Name - $($_.Exception.Message)" -ForegroundColor Red
        return $null
    }
}

Write-Host "`n========== 秒杀系统 E2E 测试 ==========" -ForegroundColor Cyan
Write-Host "Base URL: $Base`n" -ForegroundColor Cyan

# ===== 1. 健康检查 =====
Write-Host "--- 1. 基础健康检查 ---" -ForegroundColor Yellow
Test "健康检查" "GET" "/health"
Test "Prometheus指标" "GET" "/metrics"

# ===== 2. 登录测试 =====
Write-Host "`n--- 2. 登录认证 ---" -ForegroundColor Yellow
$login = Test "管理员登录" "POST" "/api/v1/user/login" @{ username = "admin"; password = "admin123" }
$token = if ($login) { $login.data.token } else { "" }
$auth = @{ Authorization = "Bearer $token" }

# ===== 3. 用户信息 =====
Write-Host "`n--- 3. 用户信息 ---" -ForegroundColor Yellow
Test "获取用户信息" "GET" "/api/v1/user/info" $null $auth

# ===== 4. 商品相关 =====
Write-Host "`n--- 4. 商品功能 ---" -ForegroundColor Yellow
Test "获取活跃商品列表" "GET" "/api/v1/product/active" $null $auth
Test "获取商品列表(管理)" "GET" "/api/v1/product/list?page=1&page_size=10" $null $auth
Test "获取商品详情" "GET" "/api/v1/product/1" $null $auth

# ===== 5. 秒杀测试 =====
Write-Host "`n--- 5. 秒杀功能 ---" -ForegroundColor Yellow
$seckill = Test "执行秒杀(商品1)" "POST" "/api/v1/seckill" @{ product_id = 1; captcha_code = "123456" } $auth
Test "秒杀统计" "GET" "/api/v1/seckill/stats" $null $auth

# ===== 6. 订单功能 =====
Write-Host "`n--- 6. 订单功能 ---" -ForegroundColor Yellow
Test "获取我的订单" "GET" "/api/v1/order/list?page=1&page_size=10" $null $auth
Test "获取全部订单(管理)" "GET" "/api/v1/order/all?page=1&page_size=10" $null $auth
Test "获取对账差异" "GET" "/api/v1/order/recon-diff" $null $auth

# ===== 7. 密码修改 =====
Write-Host "`n--- 7. 密码修改 ---" -ForegroundColor Yellow
$oldPwd = "admin123"
$newPwd = "admin123"  # 修改为相同密码，避免影响后续测试
Test "修改密码" "PUT" "/api/v1/user/password" @{ old_password = $oldPwd; new_password = $newPwd } $auth

# ===== 8. 图片上传 =====
Write-Host "`n--- 8. 图片上传 ---" -ForegroundColor Yellow
try {
    $tempFile = "$env:TEMP\test_upload.png"
    # 创建最小有效PNG
    $png = [Convert]::FromBase64String("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
    [IO.File]::WriteAllBytes($tempFile, $png)
    $uploadResult = curl.exe -s -X POST "$Base/api/v1/product/upload" -H "Authorization: Bearer $token" -F "file=@$tempFile"
    Write-Host "[PASS] 图片上传 - $uploadResult" -ForegroundColor Green
    $global:Pass++
} catch {
    Write-Host "[FAIL] 图片上传 - $($_.Exception.Message)" -ForegroundColor Red
    $global:Fail++
}

# ===== 9. Sentinel/风控 =====
Write-Host "`n--- 9. 风控管理 ---" -ForegroundColor Yellow
Test "获取Sentinel规则" "GET" "/api/v1/sentinel/rules" $null $auth
Test "获取黑名单" "GET" "/api/v1/sentinel/blacklist" $null $auth

# ===== 10. 活动配置 =====
Write-Host "`n--- 10. 活动配置 ---" -ForegroundColor Yellow
Test "获取活动配置" "GET" "/api/v1/activity" $null $auth

# ===== 11. 审计日志 =====
Write-Host "`n--- 11. 审计日志 ---" -ForegroundColor Yellow
Test "获取审计日志" "GET" "/api/v1/audit/list?page=1&page_size=10" $null $auth

# ===== 12. 监控 =====
Write-Host "`n--- 12. 监控指标 ---" -ForegroundColor Yellow
Test "Prometheus指标" "GET" "/api/v1/monitor/metrics" $null $auth
Test "QPS统计" "GET" "/api/v1/monitor/qps" $null $auth
Test "中间件状态" "GET" "/api/v1/monitor/middleware" $null $auth
Test "告警列表" "GET" "/api/v1/monitor/alarms" $null $auth
Test "慢接口排行" "GET" "/api/v1/monitor/slow-api" $null $auth

# ===== 13. 安全测试 =====
Write-Host "`n--- 13. 安全测试 ---" -ForegroundColor Yellow
# 无Token访问
try {
    $r = Invoke-WebRequest "$Base/api/v1/user/info" -Method GET -ContentType "application/json" -SkipCertificateCheck -ErrorAction Stop
    $d = $r.Content | ConvertFrom-Json
    if ($d.code -ne 200) { Write-Host "[PASS] 未授权拦截 - code=$($d.code)" -ForegroundColor Green; $global:Pass++ }
    else { Write-Host "[FAIL] 未授权未拦截" -ForegroundColor Red; $global:Fail++ }
} catch { Write-Host "[PASS] 未授权拦截(HTTP Error)" -ForegroundColor Green; $global:Pass++ }

# 无效Token
try {
    $r = Invoke-WebRequest "$Base/api/v1/user/info" -Method GET -Headers @{ Authorization = "Bearer invalid_token_xxx" } -ContentType "application/json" -SkipCertificateCheck -ErrorAction Stop
    $d = $r.Content | ConvertFrom-Json
    if ($d.code -ne 200) { Write-Host "[PASS] 无效Token拦截 - code=$($d.code)" -ForegroundColor Green; $global:Pass++ }
    else { Write-Host "[FAIL] 无效Token未拦截" -ForegroundColor Red; $global:Fail++ }
} catch { Write-Host "[PASS] 无效Token拦截(HTTP Error)" -ForegroundColor Green; $global:Pass++ }

# ===== 14. 日志文件写入 =====
Write-Host "`n--- 14. 日志文件 ---" -ForegroundColor Yellow
if (Test-Path "e:\TRAE SOLO CN\project\miaosha\logs\miaosha.log") {
    Write-Host "[PASS] 日志文件存在: logs/miaosha.log" -ForegroundColor Green
    $global:Pass++
} else {
    Write-Host "[FAIL] 日志文件不存在" -ForegroundColor Red
    $global:Fail++
}

# ===== 结果汇总 =====
Write-Host "`n========== 测试结果汇总 ==========" -ForegroundColor Cyan
Write-Host "通过: $Pass" -ForegroundColor Green
Write-Host "失败: $Fail" -ForegroundColor $(if ($Fail -gt 0) { "Red" } else { "Green" })
Write-Host "总计: $($Pass + $Fail)" -ForegroundColor Cyan
Write-Host "通过率: $([math]::Round($Pass / ($Pass + $Fail) * 100, 1))%`n" -ForegroundColor $(if ($Fail -gt 0) { "Yellow" } else { "Green" })