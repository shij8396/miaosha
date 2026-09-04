# Temp regression test: captcha enforcement + product detail SKU API
$secret = 'miaosha-sign-secret-2026'
$base = 'http://127.0.0.1:8080'

function Get-Sign($timestamp, $method, $path, $body) {
    $payload = $timestamp + $method + $path + $body
    $hmac = New-Object System.Security.Cryptography.HMACSHA256
    $hmac.Key = [Text.Encoding]::UTF8.GetBytes($secret)
    $hash = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($payload))
    return ([BitConverter]::ToString($hash) -replace '-', '').ToLower()
}

function Invoke-Api($method, $path, $bodyObj, $token) {
    $body = ''
    if ($null -ne $bodyObj) { $body = $bodyObj | ConvertTo-Json -Compress -Depth 6 }
    $ts = [Math]::Floor([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()).ToString()
    $signPath = $path.Split('?')[0]
    $sign = Get-Sign $ts $method $signPath $body
    $headers = @{ 'X-Timestamp' = $ts; 'X-Sign' = $sign }
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    $uri = $base + $path
    if ($method -eq 'GET') {
        return Invoke-RestMethod -Uri $uri -Headers $headers -TimeoutSec 10
    }
    return Invoke-RestMethod -Uri $uri -Method $method -Headers $headers -ContentType 'application/json' -Body $body -TimeoutSec 10
}

# 1. register a fresh test user then login
$testUser = 'sku_test_' + (Get-Random -Maximum 99999)
try {
    $reg = Invoke-Api 'POST' '/api/v1/user/register' @{ username = $testUser; password = 'test123' } $null
    Write-Host "=== 0. register: code=$($reg.code) msg=$($reg.message)"
} catch { Write-Host "=== 0. register: HTTP error $($_.Exception.Message)" }
$login = Invoke-Api 'POST' '/api/v1/user/login' @{ username = $testUser; password = 'test123' } $null
Write-Host "=== 1. login: code=$($login.code) msg=$($login.message)"
$token = $login.data.token
Write-Host "uid=$($login.data.user_id) role=$($login.data.role)"

# 2. active products
$active = Invoke-Api 'GET' '/api/v1/product/active' $null $token
Write-Host "`n=== 2. active products: code=$($active.code)"
$products = @($active.data)
Write-Host "count=$($products.Count)"
if ($products.Count -eq 0) { Write-Host 'no products, exit'; exit }
$prodId = $products[0].id
Write-Host "test product id=$prodId name=$($products[0].name)"

# 3. product detail (with SKUs)
$detail = Invoke-Api 'GET' "/api/v1/product/$prodId" $null $token
Write-Host "`n=== 3. product detail: code=$($detail.code)"
Write-Host ($detail.data | ConvertTo-Json -Depth 6)

# 4. captcha + path token
$captcha = Invoke-Api 'GET' "/api/v1/seckill/captcha?product_id=$prodId" $null $token
Write-Host "`n=== 4. captcha: code=$($captcha.code) expr=$($captcha.data.expression) id=$($captcha.data.captcha_id)"
$path = Invoke-Api 'GET' "/api/v1/seckill/path?product_id=$prodId" $null $token
Write-Host "path token: code=$($path.code) token=$($path.data.path_token)"

# 5. seckill with WRONG captcha answer (must be rejected)
# refresh path token + captcha (they are one-time use)
$path = Invoke-Api 'GET' "/api/v1/seckill/path?product_id=$prodId" $null $token
$captcha = Invoke-Api 'GET' "/api/v1/seckill/captcha?product_id=$prodId" $null $token
$wrongReq = @{
    product_id = $prodId
    quantity = 1
    idempotent_key = "test-wrong-$([Guid]::NewGuid())"
    path_token = $path.data.path_token
    captcha_id = $captcha.data.captcha_id
    captcha_code = 99999
}
try {
    $wrong = Invoke-Api 'POST' '/api/v1/seckill' $wrongReq $token
    Write-Host "`n=== 5. wrong answer seckill: code=$($wrong.code) msg=$($wrong.message)"
    if ($wrong.code -eq 400) { Write-Host 'PASS: wrong answer rejected' } else { Write-Host 'FAIL: wrong answer NOT rejected!' }
} catch {
    Write-Host "=== 5. wrong answer seckill: HTTP error $($_.Exception.Message)"
}

# 6. seckill WITHOUT captcha fields (must be rejected)
$path = Invoke-Api 'GET' "/api/v1/seckill/path?product_id=$prodId" $null $token
$noCaptchaReq = @{
    product_id = $prodId
    quantity = 1
    idempotent_key = "test-nocap-$([Guid]::NewGuid())"
    path_token = $path.data.path_token
}
try {
    $nocap = Invoke-Api 'POST' '/api/v1/seckill' $noCaptchaReq $token
    Write-Host "`n=== 6. missing captcha seckill: code=$($nocap.code) msg=$($nocap.message)"
    if ($nocap.code -eq 400) { Write-Host 'PASS: missing captcha rejected' } else { Write-Host 'FAIL: missing captcha NOT rejected!' }
} catch {
    Write-Host "=== 6. missing captcha seckill: HTTP error $($_.Exception.Message)"
}

# 7. seckill with CORRECT captcha answer
$path = Invoke-Api 'GET' "/api/v1/seckill/path?product_id=$prodId" $null $token
$captcha = Invoke-Api 'GET' "/api/v1/seckill/captcha?product_id=$prodId" $null $token
if ($captcha.data.expression -match '(\d+)\s*([+\-])\s*(\d+)') {
    $a = [int]$Matches[1]; $op = $Matches[2]; $b = [int]$Matches[3]
    $answer = if ($op -eq '+') { $a + $b } else { $a - $b }
    $goodReq = @{
        product_id = $prodId
        quantity = 1
        idempotent_key = "test-good-$([Guid]::NewGuid())"
        path_token = $path.data.path_token
        captcha_id = $captcha.data.captcha_id
        captcha_code = $answer
    }
    try {
        $good = Invoke-Api 'POST' '/api/v1/seckill' $goodReq $token
        Write-Host "`n=== 7. correct answer seckill ($($captcha.data.expression)=$answer): code=$($good.code) msg=$($good.message)"
        if ($good.code -eq 200) { Write-Host 'PASS: correct answer accepted' } else { Write-Host 'NOTE: business-layer rejection (limit/stock etc.)' }
    } catch {
        Write-Host "=== 7. correct answer seckill: HTTP error $($_.Exception.Message)"
    }
}

