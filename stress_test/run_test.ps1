$ErrorActionPreference = "Continue"
$BaseUrl = "http://localhost:8080"
$Secret = "miaosha-sign-secret-2026"

function GenSign($Method, $Path, $Body) {
    $ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds().ToString()
    $signStr = "$ts`n$Method`n$Path`n$Body"
    $hmac = New-Object System.Security.Cryptography.HMACSHA256
    $hmac.Key = [Text.Encoding]::UTF8.GetBytes($Secret)
    $hash = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($signStr))
    return @{ts=$ts; sign=[BitConverter]::ToString($hash).Replace("-","").ToLower()}
}

function Call($m, $p, $b, $hdr) {
    $all = @{"Content-Type"="application/json"}
    if ($hdr) { $hdr.Keys | % { $all[$_] = $hdr[$_] } }
    $bodyStr = if ($b) { $b | ConvertTo-Json -Compress } else { "" }
    $s = GenSign -Method $m -Path $p -Body $bodyStr
    $all["X-Timestamp"] = $s.ts
    $all["X-Sign"] = $s.sign
    try {
        $prm = @{Uri="$BaseUrl$p"; Method=$m; Headers=$all; ErrorAction="Stop"}
        if ($b -and $m -ne "GET") { $prm.Body = $bodyStr }
        $r = Invoke-RestMethod @prm
        return @{ok=$true; data=$r}
    } catch {
        return @{ok=$false; code=$_.Exception.Response.StatusCode.value__; err=$_.Exception.Message}
    }
}

Write-Host "=== Phase 1: Health Check ===" -ForegroundColor Yellow
$r = Call -m "GET" -p "/health"
if ($r.ok) { Write-Host "  Liveness: PASS status=$($r.data.status)" -ForegroundColor Green }
else { Write-Host "  Liveness: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/health?type=readiness"
if ($r.ok) { Write-Host "  Readiness: PASS mysql=$($r.data.deps.mysql) redis=$($r.data.deps.redis) rabbitmq=$($r.data.deps.rabbitmq)" -ForegroundColor Green }
else { Write-Host "  Readiness: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 2: Auth ===" -ForegroundColor Yellow
$r = Call -m "POST" -p "/api/v1/user/login" -b @{username="admin";password="admin123"}
if ($r.ok) { $token = $r.data.data.token; $hdr = @{"Authorization"="Bearer $token"}; Write-Host "  Admin login: PASS" -ForegroundColor Green }
else { Write-Host "  Admin login: FAIL code=$($r.code)" -ForegroundColor Red }

if ($token) {
    $r = Call -m "GET" -p "/api/v1/user/info" -hdr $hdr
    if ($r.ok) { Write-Host "  User info: PASS role=$($r.data.data.role)" -ForegroundColor Green }
    else { Write-Host "  User info: FAIL" -ForegroundColor Red }
}

$r = Call -m "POST" -p "/api/v1/user/register" -b @{username="tester001";password="test123";nickname="Tester";phone="13800138000"}
if ($r.ok) { Write-Host "  Register: PASS" -ForegroundColor Green }
else { Write-Host "  Register: FAIL code=$($r.code)" -ForegroundColor Red }

$r = Call -m "POST" -p "/api/v1/user/login" -b @{username="tester001";password="test123"}
if ($r.ok) { $userToken = $r.data.data.token; $userHdr = @{"Authorization"="Bearer $userToken"}; Write-Host "  User login: PASS" -ForegroundColor Green }
else { Write-Host "  User login: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/user/info"
if ($r.code -eq 401) { Write-Host "  No token (401): PASS" -ForegroundColor Green }
else { Write-Host "  No token (401): FAIL code=$($r.code)" -ForegroundColor Red }

$r = Call -m "POST" -p "/api/v1/user/login" -b @{username="admin";password="wrong"}
if ($r.code -eq 400) { Write-Host "  Wrong password: PASS" -ForegroundColor Green }
else { Write-Host "  Wrong password: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 3: Product ===" -ForegroundColor Yellow
$now = Get-Date
$r = Call -m "POST" -p "/api/v1/product" -b @{name="TestProduct-A"; description="Test item A"; price=99.99; seckill_price=9.99; stock=100; limit_num=3; status=1; start_time=$now.AddMinutes(-1).ToString("yyyy-MM-dd HH:mm:ss"); end_time=$now.AddHours(2).ToString("yyyy-MM-dd HH:mm:ss")} -hdr $hdr
if ($r.ok) { $pidA = $r.data.data.id; Write-Host "  Create product A: PASS id=$pidA" -ForegroundColor Green }
else { Write-Host "  Create product A: FAIL" -ForegroundColor Red }

$r = Call -m "POST" -p "/api/v1/product" -b @{name="TestProduct-B"; description="Test item B"; price=199.99; seckill_price=19.99; stock=50; limit_num=1; status=1; start_time=$now.AddMinutes(-1).ToString("yyyy-MM-dd HH:mm:ss"); end_time=$now.AddHours(2).ToString("yyyy-MM-dd HH:mm:ss")} -hdr $hdr
if ($r.ok) { $pidB = $r.data.data.id; Write-Host "  Create product B: PASS id=$pidB" -ForegroundColor Green }
else { Write-Host "  Create product B: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/product/active" -hdr $hdr
if ($r.ok) { Write-Host "  Active products: PASS count=$($r.data.data.Count)" -ForegroundColor Green }
else { Write-Host "  Active products: FAIL" -ForegroundColor Red }

$r = Call -m "POST" -p "/api/v1/activity/cache-warmup" -b @{product_id=$pidA} -hdr $hdr
if ($r.ok) { Write-Host "  Cache warmup: PASS" -ForegroundColor Green }
else { Write-Host "  Cache warmup: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 4: Concurrent Seckill ===" -ForegroundColor Yellow
$succ = 0; $fail = 0
$r = Call -m "POST" -p "/api/v1/seckill" -b @{product_id=$pidA; quantity=1; idempotent_key=[Guid]::NewGuid().ToString()} -hdr $hdr
if ($r.ok) { Write-Host "  Warmup: PASS" -ForegroundColor Green }
else { Write-Host "  Warmup: FAIL" -ForegroundColor Red }

for ($i = 0; $i -lt 30; $i++) {
    $r = Call -m "POST" -p "/api/v1/seckill" -b @{product_id=$pidA; quantity=1; idempotent_key=[Guid]::NewGuid().ToString()} -hdr $userHdr
    if ($r.ok) { $succ++ } else { $fail++ }
}
if ($succ -gt 0) { Write-Host "  30 seckills: PASS success=$succ fail=$fail" -ForegroundColor Green }
else { Write-Host "  30 seckills: FAIL success=$succ fail=$fail" -ForegroundColor Red }

$key = [Guid]::NewGuid().ToString()
$r1 = Call -m "POST" -p "/api/v1/seckill" -b @{product_id=$pidA; quantity=1; idempotent_key=$key} -hdr $hdr
Start-Sleep -Milliseconds 200
$r2 = Call -m "POST" -p "/api/v1/seckill" -b @{product_id=$pidA; quantity=1; idempotent_key=$key} -hdr $hdr
if ($r1.ok -and $r2.code -eq 400) { Write-Host "  Idempotency: PASS" -ForegroundColor Green }
else { Write-Host "  Idempotency: FAIL 1st=$($r1.ok) 2nd=$($r2.code)" -ForegroundColor Red }

for ($i = 0; $i -lt 30; $i++) {
    $r = Call -m "POST" -p "/api/v1/seckill" -b @{product_id=$pidB; quantity=1; idempotent_key=[Guid]::NewGuid().ToString()} -hdr $hdr
    if ($r.code -eq 400) { break }
}
$r = Call -m "POST" -p "/api/v1/seckill" -b @{product_id=$pidB; quantity=1; idempotent_key=[Guid]::NewGuid().ToString()} -hdr $hdr
if ($r.code -eq 400) { Write-Host "  Stock depleted: PASS" -ForegroundColor Green }
else { Write-Host "  Stock depleted: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 5: Orders ===" -ForegroundColor Yellow
$r = Call -m "GET" -p "/api/v1/order/all?page=1&page_size=10" -hdr $hdr
if ($r.ok) { Write-Host "  All orders: PASS total=$($r.data.data.total)" -ForegroundColor Green }
else { Write-Host "  All orders: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/order/list?page=1&page_size=10" -hdr $userHdr
if ($r.ok) { Write-Host "  User orders: PASS count=$($r.data.data.list.Count)" -ForegroundColor Green }
else { Write-Host "  User orders: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 6: Monitor ===" -ForegroundColor Yellow
$r = Call -m "GET" -p "/api/v1/monitor/middleware" -hdr $hdr
if ($r.ok) { Write-Host "  Middleware status: PASS" -ForegroundColor Green }
else { Write-Host "  Middleware status: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/monitor/qps" -hdr $hdr
if ($r.ok) { Write-Host "  QPS monitor: PASS" -ForegroundColor Green }
else { Write-Host "  QPS monitor: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/monitor/slow-api" -hdr $hdr
if ($r.ok) { Write-Host "  Slow API TOP: PASS" -ForegroundColor Green }
else { Write-Host "  Slow API TOP: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/seckill/stats" -hdr $hdr
if ($r.ok) { Write-Host "  Seckill stats: PASS" -ForegroundColor Green }
else { Write-Host "  Seckill stats: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/monitor/alarms" -hdr $hdr
if ($r.ok) { Write-Host "  Alarms: PASS" -ForegroundColor Green }
else { Write-Host "  Alarms: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 7: Sentinel ===" -ForegroundColor Yellow
$r = Call -m "GET" -p "/api/v1/sentinel/rules" -hdr $hdr
if ($r.ok) { Write-Host "  Sentinel rules: PASS" -ForegroundColor Green }
else { Write-Host "  Sentinel rules: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/sentinel/blacklist" -hdr $hdr
if ($r.ok) { Write-Host "  Blacklist: PASS" -ForegroundColor Green }
else { Write-Host "  Blacklist: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 8: Audit ===" -ForegroundColor Yellow
$r = Call -m "GET" -p "/api/v1/audit/list?page=1&page_size=10" -hdr $hdr
if ($r.ok) { Write-Host "  Audit logs: PASS" -ForegroundColor Green }
else { Write-Host "  Audit logs: FAIL" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/order/recon-diff?page=1&page_size=10" -hdr $hdr
if ($r.ok) { Write-Host "  Recon diff: PASS total=$($r.data.data.total)" -ForegroundColor Green }
else { Write-Host "  Recon diff: FAIL" -ForegroundColor Red }

Write-Host "=== Phase 9: Security ===" -ForegroundColor Yellow
$r = Call -m "GET" -p "/api/v1/user/info" -hdr @{"Authorization"="Bearer invalidtoken"}
if ($r.code -eq 401) { Write-Host "  Invalid token (401): PASS" -ForegroundColor Green }
else { Write-Host "  Invalid token (401): FAIL code=$($r.code)" -ForegroundColor Red }

$r = Call -m "GET" -p "/api/v1/user/info" -hdr @{"Authorization"="Bearer $token"; "X-Timestamp"="bad"; "X-Sign"="bad"}
if ($r.code -eq 401) { Write-Host "  Bad sign (401): PASS" -ForegroundColor Green }
else { Write-Host "  Bad sign (401): FAIL code=$($r.code)" -ForegroundColor Red }

Write-Host "=== Phase 10: Logs ===" -ForegroundColor Yellow
$logFile = "e:\TRAE SOLO CN\project\miaosha\logs\miaosha.log"
$exists = Test-Path $logFile
$size = if ($exists) { (Get-Item $logFile).Length } else { 0 }
$lines = if ($exists) { (Get-Content $logFile | Measure-Object).Count } else { 0 }
if ($exists -and $size -gt 0) { Write-Host "  Log file: PASS size=$($size)bytes lines=$lines" -ForegroundColor Green }
else { Write-Host "  Log file: FAIL" -ForegroundColor Red }

Write-Host "====================================" -ForegroundColor Cyan
Write-Host "  ALL TESTS COMPLETED" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan