package redis

import (
	"context"
	"fmt"
	"time"
)

type DistLock struct {
	key    string
	value  string
	expire time.Duration
	cancel context.CancelFunc
}

func NewDistLock(key string, expire time.Duration) *DistLock {
	return &DistLock{
		key:    fmt.Sprintf("lock:%s", key),
		value:  fmt.Sprintf("%d", time.Now().UnixNano()),
		expire: expire,
	}
}

func (l *DistLock) Lock(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := rdb.SetNX(ctx, l.key, l.value, l.expire).Result()
		if err != nil { return fmt.Errorf("分布式锁获取失败: %w", err) }
		if ok {
			renewCtx, cancel := context.WithCancel(context.Background())
			l.cancel = cancel
			go l.renewLoop(renewCtx)
			return nil
		}
		if time.Now().After(deadline) { return fmt.Errorf("获取分布式锁超时: %s", l.key) }
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *DistLock) Unlock(ctx context.Context) error {
	if l.cancel != nil { l.cancel() }
	luaScript := `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('DEL', KEYS[1])
		else return 0 end
	`
	result, err := rdb.Eval(ctx, luaScript, []string{l.key}, l.value).Int()
	if err != nil { return fmt.Errorf("分布式锁释放失败: %w", err) }
	if result == 0 { return fmt.Errorf("锁已被释放或过期") }
	return nil
}

func (l *DistLock) renewLoop(ctx context.Context) {
	ticker := time.NewTicker(l.expire / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C:
			luaScript := `
				if redis.call('GET', KEYS[1]) == ARGV[1] then
					return redis.call('EXPIRE', KEYS[1], ARGV[2])
				else return 0 end
			`
			rdb.Eval(ctx, luaScript, []string{l.key}, l.value, int(l.expire.Seconds()))
		}
	}
}