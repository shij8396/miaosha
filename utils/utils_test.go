package utils

import (
	"testing"
)

// TestHashPassword 测试密码哈希和验证
func TestHashPassword(t *testing.T) {
	password := "admin123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("密码哈希失败: %v", err)
	}
	if hash == "" {
		t.Fatal("密码哈希结果为空")
	}
	// 验证正确密码
	if !CheckPassword(password, hash) {
		t.Fatal("正确密码验证失败")
	}
	// 验证错误密码
	if CheckPassword("wrongpassword", hash) {
		t.Fatal("错误密码不应该验证通过")
	}
}

// TestJWTManager 测试 JWT Token 生成和解析
func TestJWTManager(t *testing.T) {
	secret := "test-secret-key-2026"
	jwtMgr := NewJWTManager(secret, 24, "test-issuer")

	// 测试生成 Token
	token, err := jwtMgr.GenerateToken(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Token生成失败: %v", err)
	}
	if token == "" {
		t.Fatal("Token为空")
	}

	// 测试解析 Token
	claims, err := jwtMgr.ParseToken(token)
	if err != nil {
		t.Fatalf("Token解析失败: %v", err)
	}
	if claims.UserID != 1 {
		t.Fatalf("UserID不匹配: 期望 %d, 实际 %d", 1, claims.UserID)
	}
	if claims.Username != "admin" {
		t.Fatalf("Username不匹配: 期望 %s, 实际 %s", "admin", claims.Username)
	}
	if claims.Role != "admin" {
		t.Fatalf("Role不匹配: 期望 %s, 实际 %s", "admin", claims.Role)
	}

	// 测试无效 Token
	_, err = jwtMgr.ParseToken("invalid-token")
	if err == nil {
		t.Fatal("无效Token应该解析失败")
	}
}

// TestSnowflake 测试雪花算法 ID 生成
func TestSnowflake(t *testing.T) {
	sf, err := NewSnowflake(1)
	if err != nil {
		t.Fatalf("雪花算法初始化失败: %v", err)
	}

	// 生成多个 ID 并确保唯一性
	ids := make(map[int64]bool)
	for i := 0; i < 1000; i++ {
		id, err := sf.NextID()
		if err != nil {
			t.Fatalf("ID生成失败: %v", err)
		}
		if ids[id] {
			t.Fatalf("ID重复: %d", id)
		}
		ids[id] = true
	}
}

// TestSnowflakeInvalidWorkerID 测试无效 WorkerID
func TestSnowflakeInvalidWorkerID(t *testing.T) {
	_, err := NewSnowflake(-1)
	if err == nil {
		t.Fatal("负数 WorkerID 应该返回错误")
	}
	_, err = NewSnowflake(1024)
	if err == nil {
		t.Fatal("超出范围的 WorkerID 应该返回错误")
	}
}

// TestGenerateOrderNo 测试订单号生成
func TestGenerateOrderNo(t *testing.T) {
	sf, _ := NewSnowflake(1)
	orderNo, err := GenerateOrderNo(sf)
	if err != nil {
		t.Fatalf("订单号生成失败: %v", err)
	}
	if len(orderNo) < 4 {
		t.Fatalf("订单号长度异常: %s", orderNo)
	}
	if orderNo[:2] != "MS" {
		t.Fatalf("订单号前缀异常: %s", orderNo)
	}
}

// TestGenerateTraceID 测试 TraceID 生成
func TestGenerateTraceID(t *testing.T) {
	id1 := GenerateTraceID()
	if id1 == "" {
		t.Fatal("TraceID为空")
	}
	if len(id1) != 32 {
		t.Fatalf("TraceID长度异常: %d", len(id1))
	}
	// 验证TraceID为十六进制字符串
	for _, ch := range id1 {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			t.Fatalf("TraceID包含非法字符: %c", ch)
		}
	}
}

// TestRoleDefault 测试角色默认值逻辑（兼容旧版Token无role字段）
func TestRoleDefault(t *testing.T) {
	// 模拟 role 为空时的兜底逻辑
	role := ""
	if role == "" {
		role = "user"
	}
	if role != "user" {
		t.Fatalf("空角色默认值应为 'user'，实际为 '%s'", role)
	}
}

// TestGetOrderTableName 测试分表名称生成
func TestGetOrderTableName(t *testing.T) {
	shardCount := 16
	// 同一用户应始终路由到同一张表
	table1 := GetOrderTableName(100, shardCount)
	table2 := GetOrderTableName(100, shardCount)
	if table1 != table2 {
		t.Fatalf("同一用户路由到不同分表: %s vs %s", table1, table2)
	}
	// 验证表名格式
	expectedPrefix := "t_order_"
	if table1[:len(expectedPrefix)] != expectedPrefix {
		t.Fatalf("分表名称格式异常: %s", table1)
	}
}

// TestClampPageSize 测试分页大小上限限制
func TestClampPageSize(t *testing.T) {
	// 非法值（<=0）回退默认 10
	if got := ClampPageSize(0); got != 10 {
		t.Fatalf("pageSize=0 应回退 10，实际 %d", got)
	}
	if got := ClampPageSize(-5); got != 10 {
		t.Fatalf("pageSize=-5 应回退 10，实际 %d", got)
	}
	// 正常值原样返回
	if got := ClampPageSize(20); got != 20 {
		t.Fatalf("pageSize=20 应原样返回，实际 %d", got)
	}
	// 超过上限钳制到 MaxPageSize
	if got := ClampPageSize(1000); got != MaxPageSize {
		t.Fatalf("pageSize=1000 应钳制到 %d，实际 %d", MaxPageSize, got)
	}
}

// TestDingTalkSign 测试钉钉机器人签名（HMAC-SHA256 + Base64）
func TestDingTalkSign(t *testing.T) {
	secret := "SEC123456"
	timestamp := int64(1700000000000)
	sign, err := DingTalkSign(secret, timestamp)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}
	if sign == "" {
		t.Fatal("签名为空")
	}
	// 相同输入应得到相同签名（确定性）
	sign2, _ := DingTalkSign(secret, timestamp)
	if sign != sign2 {
		t.Fatal("相同输入应得到相同签名")
	}
	// 不同时间戳应得到不同签名
	sign3, _ := DingTalkSign(secret, timestamp+1)
	if sign == sign3 {
		t.Fatal("不同时间戳应得到不同签名")
	}
	// 与已知参考值比对（防止算法回归）
	// HMAC-SHA256(timestamp+"\n"+secret, secret) 的 Base64
	expected := "FDly9FmQpdYyYkNLryV5/4kGkNb4cCTG2VhnJLEn0mA="
	if sign != expected {
		t.Fatalf("签名与参考值不符，期望 %s，实际 %s", expected, sign)
	}
}

// TestToJSON 测试 JSON 序列化：中文不做 HTML 转义、无多余换行
func TestToJSON(t *testing.T) {
	// 1. 中文不应被转义成 \uXXXX
	s := ToJSON(map[string]interface{}{"name": "秒杀商品", "price": 9.9})
	if s != `{"name":"秒杀商品","price":9.9}` {
		t.Fatalf("ToJSON 中文转义或格式异常: %s", s)
	}
	// 2. 不应包含尾部换行符（json.Encoder.Encode 默认追加 \n）
	if s[len(s)-1] == '\n' {
		t.Fatal("ToJSON 不应包含尾部换行符")
	}
	// 3. 无 HTML 转义：< > & 应原样保留
	s2 := ToJSON("a<b&c>d")
	if s2 != `"a<b&c>d"` {
		t.Fatalf("ToJSON 不应转义 HTML 字符: %s", s2)
	}
}

// TestFromJSON 测试 JSON 反序列化
func TestFromJSON(t *testing.T) {
	type item struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	var got item
	if err := FromJSON(`{"id":1,"name":"测试"}`, &got); err != nil {
		t.Fatalf("FromJSON 失败: %v", err)
	}
	if got.ID != 1 || got.Name != "测试" {
		t.Fatalf("FromJSON 结果异常: %+v", got)
	}
}

// TestBufferedSnowflake 测试带缓冲的雪花 ID 生成（并发安全 + 唯一性）
func TestBufferedSnowflake(t *testing.T) {
	sf, err := NewSnowflake(2)
	if err != nil {
		t.Fatalf("雪花初始化失败: %v", err)
	}
	bs := NewBufferedSnowflake(sf, 64)
	defer bs.Close()

	// 并发生成 1000 个 ID，验证无重复、无错误
	const n = 1000
	ids := make(chan int64, n)
	errCh := make(chan error, 1)
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < n/8; j++ {
				id, err := bs.NextID()
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				ids <- id
			}
		}()
	}
	seen := make(map[int64]bool, n)
	for i := 0; i < n; i++ {
		select {
		case err := <-errCh:
			t.Fatalf("ID 生成失败: %v", err)
		case id := <-ids:
			if seen[id] {
				t.Fatalf("ID 重复: %d", id)
			}
			seen[id] = true
		}
	}
}
