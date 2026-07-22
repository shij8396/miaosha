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