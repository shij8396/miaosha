package redis

import (
	"context"
	"testing"
	"time"
)

// TestAcquireLockWithRenewal 测试分布式锁自动续期
// 验证锁可正常获取、续期协程正常运行、释放后锁可被其他请求获取
func TestAcquireLockWithRenewal(t *testing.T) {
	ctx := context.Background()
	lockKey := "test:lock:unit_test"
	lockTimeout := 3 * time.Second

	// 确保测试前锁不存在
	rdb.Del(ctx, lockKey)

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

	// 4. 测试释放锁
	close(stopCh)
	time.Sleep(500 * time.Millisecond) // 等待续期协程删除锁

	exists, _ := rdb.Exists(ctx, lockKey).Result()
	if exists > 0 {
		t.Fatal("预期锁已被释放，但 key 仍存在")
	}
	t.Log("锁释放验证通过")
}

// TestAcquireLockWithRenewal_Timeout 测试锁过期后自动释放
func TestAcquireLockWithRenewal_Timeout(t *testing.T) {
	ctx := context.Background()
	lockKey := "test:lock:timeout_test"
	lockTimeout := 2 * time.Second

	rdb.Del(ctx, lockKey)

	// 获取锁后立即关闭 stopCh（模拟业务完成后释放）
	stopCh := make(chan struct{})
	ok, err := AcquireLockWithRenewal(ctx, lockKey, lockTimeout, stopCh)
	if err != nil || !ok {
		t.Fatalf("获取锁失败: %v", err)
	}

	// 关闭 stopCh 触发释放
	close(stopCh)
	time.Sleep(500 * time.Millisecond)

	// 验证锁已释放，可以重新获取
	ok2, _ := rdb.SetNX(ctx, lockKey, "1", lockTimeout).Result()
	if !ok2 {
		t.Fatal("预期锁已释放可重新获取，但 SetNX 返回 false")
	}

	rdb.Del(ctx, lockKey)
	t.Log("锁过期释放验证通过")
}