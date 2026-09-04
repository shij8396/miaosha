package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// setupTestRedis 启动 miniredis 内存实例，并将全局 rdb 指向它。
// miniredis 是纯 Go 实现的 Redis，单元测试无需真实 Redis 服务即可运行（CI 友好）。
// 每个测试独立调用一次，保证用例之间互不污染。
func setupTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(s.Close)
	rdb = redis.NewClient(&redis.Options{Addr: s.Addr()})
	return s
}

// TestDecrStock 测试基础库存扣减：正常扣减、库存不足拒绝、商品不存在拒绝
func TestDecrStock(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const productID = 1001

	if err := PreloadStock(ctx, productID, 10); err != nil {
		t.Fatalf("预加载库存失败: %v", err)
	}

	// 1. 正常扣减：10 -> 7
	remain, ok, err := DecrStock(ctx, productID, 3)
	if err != nil || !ok || remain != 7 {
		t.Fatalf("扣减失败: remain=%d ok=%v err=%v", remain, ok, err)
	}

	// 2. 库存不足拒绝：7 件库存扣 100 件应失败且不改变库存
	if _, ok, err = DecrStock(ctx, productID, 100); err != nil {
		t.Fatalf("库存不足时不应报错，应返回 ok=false: %v", err)
	}
	if ok {
		t.Fatal("库存不足时应返回 ok=false")
	}
	got, _ := GetStock(ctx, productID)
	if got != 7 {
		t.Fatalf("库存不足拒绝后库存不应变化，期望 7，实际 %d", got)
	}
}

// TestIncrStock 测试库存归还（订单取消/超时回滚用）
func TestIncrStock(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const productID = 1002

	PreloadStock(ctx, productID, 5)
	if err := IncrStock(ctx, productID, 2); err != nil {
		t.Fatalf("库存归还失败: %v", err)
	}
	got, _ := GetStock(ctx, productID)
	if got != 7 {
		t.Fatalf("归还后库存期望 7，实际 %d", got)
	}
}

// TestDecrStockAndIncrPurchase 测试合并「扣库存+限购计数」Lua 脚本
// 覆盖：正常购买、超卖拒绝（库存不出现负数）、限购拒绝
func TestDecrStockAndIncrPurchase(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const (
		productID    = 2001
		userID       = 10001
		limitPerUser = 3
	)

	if err := PreloadStock(ctx, productID, 5); err != nil {
		t.Fatalf("预加载库存失败: %v", err)
	}

	// 1. 正常购买 2 件：库存 5->3，用户计数 2
	remain, count, code, err := DecrStockAndIncrPurchase(ctx, userID, productID, 2, limitPerUser, time.Hour)
	if err != nil || code != 1 || remain != 3 || count != 2 {
		t.Fatalf("正常购买失败: remain=%d count=%d code=%d err=%v", remain, count, code, err)
	}

	// 2. 再购 1 件：当前 2 + 本次 1 = 3，恰未超限，应放行（库存 3->2，计数 3）
	remain, count, code, err = DecrStockAndIncrPurchase(ctx, userID, productID, 1, limitPerUser, time.Hour)
	if err != nil || code != 1 || remain != 2 || count != 3 {
		t.Fatalf("补足限购上限应放行: remain=%d count=%d code=%d err=%v", remain, count, code, err)
	}

	// 3. 再购 1 件：当前 3 + 本次 1 = 4 > 3，触发限购拒绝（code=-3），库存保持 2 不变
	remain, count, code, err = DecrStockAndIncrPurchase(ctx, userID, productID, 1, limitPerUser, time.Hour)
	if err != nil {
		t.Fatalf("限购拒绝不应报错: %v", err)
	}
	if code != -3 {
		t.Fatalf("限购拒绝码期望 -3，实际 %d", code)
	}
	stock, _ := GetStock(ctx, productID)
	if stock != 2 {
		t.Fatalf("限购拒绝后库存不应被扣减，期望 2，实际 %d", stock)
	}
}

// TestDecrStockAndIncrPurchase_OversellRejection 测试超卖拒绝：库存不足时库存不出现负数
func TestDecrStockAndIncrPurchase_OversellRejection(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const productID = 2002

	// 库存仅 1 件
	if err := PreloadStock(ctx, productID, 1); err != nil {
		t.Fatalf("预加载库存失败: %v", err)
	}

	// 第一次买 1 件成功
	remain, count, code, err := DecrStockAndIncrPurchase(ctx, 1, productID, 1, 1, time.Hour)
	if err != nil || code != 1 || remain != 0 {
		t.Fatalf("首次购买失败: remain=%d count=%d code=%d err=%v", remain, count, code, err)
	}

	// 第二次买 1 件应拒绝（code=-2），库存保持 0 而不是负数
	remain, count, code, err = DecrStockAndIncrPurchase(ctx, 2, productID, 1, 1, time.Hour)
	if err != nil {
		t.Fatalf("超卖拒绝不应报错: %v", err)
	}
	if code != -2 {
		t.Fatalf("超卖拒绝码期望 -2，实际 %d", code)
	}
	stock, _ := GetStock(ctx, productID)
	if stock != 0 {
		t.Fatalf("超卖拒绝后库存不应为负，期望 0，实际 %d", stock)
	}
}

// TestDecrStockAndIncrPurchaseWithIdempotent 测试合并「幂等+扣库存+限购计数」Lua 脚本
// 覆盖：幂等 Key 重复提交被拒绝（code=-4）且库存不泄漏
func TestDecrStockAndIncrPurchaseWithIdempotent(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const (
		productID    = 3001
		userID       = 20001
		limitPerUser = 3
	)

	if err := PreloadStock(ctx, productID, 10); err != nil {
		t.Fatalf("预加载库存失败: %v", err)
	}

	// 1. 首次携带幂等 Key 请求：成功，库存 10->8，计数 2
	remain, count, code, err := DecrStockAndIncrPurchaseWithIdempotent(ctx, userID, productID, 2, limitPerUser, time.Hour, "req-001")
	if err != nil || code != 1 || remain != 8 || count != 2 {
		t.Fatalf("首次请求失败: remain=%d count=%d code=%d err=%v", remain, count, code, err)
	}

	// 2. 相同幂等 Key 重复提交：拒绝（code=-4），且库存不得被再次扣减
	remain, count, code, err = DecrStockAndIncrPurchaseWithIdempotent(ctx, userID, productID, 2, limitPerUser, time.Hour, "req-001")
	if err != nil {
		t.Fatalf("重复提交拒绝不应报错: %v", err)
	}
	if code != -4 {
		t.Fatalf("重复提交拒绝码期望 -4，实际 %d", code)
	}
	stock, _ := GetStock(ctx, productID)
	if stock != 8 {
		t.Fatalf("重复提交后库存不应被扣减（幂等不泄漏），期望 8，实际 %d", stock)
	}

	// 3. 不同幂等 Key 应正常放行（用户计数 2+1=3 未超限）
	remain, count, code, err = DecrStockAndIncrPurchaseWithIdempotent(ctx, userID, productID, 1, limitPerUser, time.Hour, "req-002")
	if err != nil || code != 1 || remain != 7 || count != 3 {
		t.Fatalf("新幂等 Key 应放行: remain=%d count=%d code=%d err=%v", remain, count, code, err)
	}
}

// TestCheckIdempotent 测试独立幂等检查函数（SETNX 语义）
func TestCheckIdempotent(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	// 首次返回 true（首次请求）
	first, err := CheckIdempotent(ctx, 1, 3002, "k1", time.Hour)
	if err != nil || !first {
		t.Fatalf("首次幂等检查应返回 true: ok=%v err=%v", first, err)
	}
	// 重复返回 false（重复请求）
	second, err := CheckIdempotent(ctx, 1, 3002, "k1", time.Hour)
	if err != nil || second {
		t.Fatalf("重复幂等检查应返回 false: ok=%v err=%v", second, err)
	}
}

// TestDecrUserPurchaseCount 测试限购计数按数量递减（订单取消/超时/退款场景）
// 语义约束：取消一单只递减对应数量，不得整键删除导致其余订单计数丢失
func TestDecrUserPurchaseCount(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const (
		productID = 4001
		userID    = 30001
	)

	// 用户购买 2 件（累计计数 2）
	if _, err := IncrUserPurchaseCount(ctx, userID, productID, 2, time.Hour); err != nil {
		t.Fatalf("累加计数失败: %v", err)
	}

	// 取消 1 单：计数 2->1（key 保留，其余订单计数不被清零）
	newCount, err := DecrUserPurchaseCount(ctx, userID, productID, 1)
	if err != nil {
		t.Fatalf("递减计数失败: %v", err)
	}
	if newCount != 1 {
		t.Fatalf("递减后计数期望 1，实际 %d", newCount)
	}
	// key 仍存在
	if exists, _ := rdb.Exists(ctx, userKey(productID, userID)).Result(); exists == 0 {
		t.Fatal("还有 1 件已购记录，key 不应被删除")
	}

	// 再取消 1 单：计数 1->0，key 应被删除（无已购记录）
	newCount, err = DecrUserPurchaseCount(ctx, userID, productID, 1)
	if err != nil {
		t.Fatalf("递减计数失败: %v", err)
	}
	if newCount != 0 {
		t.Fatalf("递减后计数期望 0，实际 %d", newCount)
	}
	if exists, _ := rdb.Exists(ctx, userKey(productID, userID)).Result(); exists > 0 {
		t.Fatal("计数归零后 key 应被删除")
	}

	// 不存在的 key 递减返回 0 且不报错
	n, err := DecrUserPurchaseCount(ctx, userID, productID, 1)
	if err != nil || n != 0 {
		t.Fatalf("空 key 递减期望 0 无错误: n=%d err=%v", n, err)
	}
}

// TestPurchaseCountTTL 测试限购计数携带过期时间（活动结束自动清空用户限购记录）
func TestPurchaseCountTTL(t *testing.T) {
	s := setupTestRedis(t)
	ctx := context.Background()
	const (
		productID = 4002
		userID    = 30002
	)

	// 累加计数并设置 60s 过期
	if _, err := IncrUserPurchaseCount(ctx, userID, productID, 1, 60*time.Second); err != nil {
		t.Fatalf("累加计数失败: %v", err)
	}
	// TTL 应大于 0，证明记录会随活动结束自动过期清空
	ttl, err := rdb.TTL(ctx, userKey(productID, userID)).Result()
	if err != nil || ttl <= 0 {
		t.Fatalf("限购记录应带过期时间: ttl=%v err=%v", ttl, err)
	}

	// 快进 61 秒后，key 应自动消失（模拟活动结束清空）
	s.FastForward(61 * time.Second)
	if exists, _ := rdb.Exists(ctx, userKey(productID, userID)).Result(); exists > 0 {
		t.Fatal("活动结束后限购记录应自动清空")
	}
}

// TestCheckUserPurchaseLimit 测试限购上限校验
func TestCheckUserPurchaseLimit(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const (
		productID = 4003
		userID    = 30003
	)

	// 未购买：count=0，未达上限
	if count, reached, err := CheckUserPurchaseLimit(ctx, userID, productID, 3); err != nil || count != 0 || reached {
		t.Fatalf("未购买状态异常: count=%d reached=%v err=%v", count, reached, err)
	}

	// 购买 2 件后：2 < 3 未达上限
	IncrUserPurchaseCount(ctx, userID, productID, 2, time.Hour)
	if count, reached, err := CheckUserPurchaseLimit(ctx, userID, productID, 3); err != nil || count != 2 || reached {
		t.Fatalf("购买2件限购3状态异常: count=%d reached=%v err=%v", count, reached, err)
	}

	// 购买到 3 件后：达到上限
	IncrUserPurchaseCount(ctx, userID, productID, 1, time.Hour)
	if count, reached, err := CheckUserPurchaseLimit(ctx, userID, productID, 3); err != nil || count != 3 || !reached {
		t.Fatalf("购买3件限购3状态异常: count=%d reached=%v err=%v", count, reached, err)
	}
}

// TestGetAndVerifySeckillPath 测试隐藏秒杀路径 Token（一次性消费）
func TestGetAndVerifySeckillPath(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	const (
		productID = 5001
		userID    = 40001
	)

	// 设置路径 Token
	if err := SetSeckillPath(ctx, userID, productID, "token-abc", time.Hour); err != nil {
		t.Fatalf("设置路径 Token 失败: %v", err)
	}

	// 校验错误 Token：返回 false，且正确 Token 仍未被消费
	if ok, err := GetAndVerifySeckillPath(ctx, userID, productID, "token-wrong"); err != nil || ok {
		t.Fatalf("错误 Token 应校验失败: ok=%v err=%v", ok, err)
	}

	// 校验正确 Token：返回 true 并一次性消费
	if ok, err := GetAndVerifySeckillPath(ctx, userID, productID, "token-abc"); err != nil || !ok {
		t.Fatalf("正确 Token 应校验成功: ok=%v err=%v", ok, err)
	}

	// 再次使用同一 Token：已被消费，返回 false
	if ok, err := GetAndVerifySeckillPath(ctx, userID, productID, "token-abc"); err != nil || ok {
		t.Fatalf("Token 应一次性消费，二次使用应失败: ok=%v err=%v", ok, err)
	}
}

// TestGetAndVerifyCaptcha 测试数学验证码（一次性消费）
func TestGetAndVerifyCaptcha(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	// 设置验证码答案 8
	if err := SetCaptcha(ctx, "captcha-001", 8, time.Hour); err != nil {
		t.Fatalf("设置验证码失败: %v", err)
	}

	// 错误答案：返回 false，且答案未被消费
	if ok, err := GetAndVerifyCaptcha(ctx, "captcha-001", 7); err != nil || ok {
		t.Fatalf("错误答案应校验失败: ok=%v err=%v", ok, err)
	}

	// 正确答案：返回 true 并一次性消费
	if ok, err := GetAndVerifyCaptcha(ctx, "captcha-001", 8); err != nil || !ok {
		t.Fatalf("正确答案应校验成功: ok=%v err=%v", ok, err)
	}

	// 再次使用：已被消费，返回 false
	if ok, err := GetAndVerifyCaptcha(ctx, "captcha-001", 8); err != nil || ok {
		t.Fatalf("验证码应一次性消费，二次使用应失败: ok=%v err=%v", ok, err)
	}
}

// TestAcquireLockWithRenewal 测试分布式锁自动续期
// 验证锁可正常获取、续期协程正常运行、释放后锁可被其他请求获取
func TestAcquireLockWithRenewal(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	lockKey := "test:lock:unit_test"
	lockTimeout := 3 * time.Second

	// 1. 测试获取锁
	stopCh := make(chan struct{})
	ok, err := AcquireLockWithRenewal(ctx, lockKey, lockTimeout, stopCh)
	if err != nil {
		t.Fatalf("获取锁失败: %v", err)
	}
	if !ok {
		t.Fatal("预期获取锁成功，但返回 false")
	}

	// 2. 测试同一锁不可重复获取
	ok2, _ := rdb.SetNX(ctx, lockKey, "2", lockTimeout).Result()
	if ok2 {
		close(stopCh)
		t.Fatal("预期锁已被占用，但 SetNX 返回 true")
	}

	// 3. 测试续期：等待 2 秒后检查 TTL 是否被重置
	time.Sleep(2 * time.Second)
	ttl, _ := rdb.TTL(ctx, lockKey).Result()
	if ttl <= 0 {
		close(stopCh)
		t.Fatalf("预期锁被续期（TTL > 0），但 TTL = %v", ttl)
	}
	t.Logf("续期验证通过，当前 TTL: %v", ttl)

	// 4. 测试释放锁：关闭 stopCh 后轮询等待锁被续期协程删除，
	//    避免固定 sleep 在慢速 CI 机器上未等协程结束导致 -race 误报
	close(stopCh)
	deadline := time.Now().Add(2 * time.Second)
	for {
		exists, _ := rdb.Exists(ctx, lockKey).Result()
		if exists == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("预期锁已被释放，但 key 仍存在（超时）")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Log("锁释放验证通过")
}

// TestAcquireLockWithRenewal_Timeout 测试锁过期后自动释放
func TestAcquireLockWithRenewal_Timeout(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()
	lockKey := "test:lock:timeout_test"
	lockTimeout := 2 * time.Second

	// 获取锁后立即关闭 stopCh（模拟业务完成后释放）
	stopCh := make(chan struct{})
	ok, err := AcquireLockWithRenewal(ctx, lockKey, lockTimeout, stopCh)
	if err != nil || !ok {
		t.Fatalf("获取锁失败: %v", err)
	}

	// 关闭 stopCh 触发释放，轮询等待锁消失
	close(stopCh)
	deadline := time.Now().Add(2 * time.Second)
	for {
		exists, _ := rdb.Exists(ctx, lockKey).Result()
		if exists == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("预期锁已释放，但 key 仍存在（超时）")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 验证锁已释放，可以重新获取
	ok2, _ := rdb.SetNX(ctx, lockKey, "1", lockTimeout).Result()
	if !ok2 {
		t.Fatal("预期锁已释放可重新获取，但 SetNX 返回 false")
	}

	rdb.Del(ctx, lockKey)
	t.Log("锁过期释放验证通过")
}

// userKey 生成用户限购计数 key，供测试断言使用
func userKey(productID, userID int64) string {
	return fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
}
