// Package singleflight 实现请求合并（Request Coalescing）
// 高并发秒杀场景下，同一商品同一用户的重复请求合并为一次 Redis 操作
// 结果广播给所有等待的 goroutine，大幅降低 Redis 压力
// 设计：函数立即执行，结果缓存到 mergeWindow 超时后清理，后续请求复用缓存结果
package singleflight

import (
	"sync"
	"time"
)

// Result 合并请求的结果
type Result struct {
	Value interface{}
	Err   error
	// 等待该结果的请求数（用于监控）
	WaiterCount int
}

// call 表示一个正在进行的请求（含缓存结果）
type call struct {
	wg    sync.WaitGroup
	val   interface{}
	err   error
	count int       // 等待者数量（含后续请求）
	done  bool      // 是否已完成
	ts    time.Time // 请求开始时间
}

// Group 请求合并组
type Group struct {
	mu    sync.Mutex
	calls map[string]*call
}

// NewGroup 创建请求合并组
func NewGroup() *Group {
	g := &Group{
		calls: make(map[string]*call),
	}
	go g.cleanupLoop()
	return g
}

// Do 执行请求合并
// key: 合并键（同一 key 的并发请求会合并为一次执行）
// fn: 实际执行函数（只会被调用一次）
// mergeWindow: 结果缓存窗口（超时后清理缓存，新请求重新执行 fn）
//
// 优化：函数立即执行，不等待 mergeWindow。后续请求在 mergeWindow 内命中缓存，直接返回。
func (g *Group) Do(key string, fn func() (interface{}, error), mergeWindow time.Duration) (interface{}, error, int) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		// 已有进行中的请求或缓存结果
		if c.done {
			// 结果已产生，直接返回缓存（不阻塞）
			val, err := c.val, c.err
			c.count++
			g.mu.Unlock()
			return val, err, c.count
		}
		// 正在执行中，加入等待
		c.count++
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, c.count
	}

	// 创建新请求，立即执行
	c := &call{ts: time.Now(), count: 1}
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()

	// 立即执行，不等待 mergeWindow
	c.val, c.err = fn()
	c.done = true
	c.wg.Done()

	// 结果缓存 mergeWindow 后清理，期间新请求复用缓存
	if mergeWindow > 0 {
		time.AfterFunc(mergeWindow, func() {
			g.mu.Lock()
			delete(g.calls, key)
			g.mu.Unlock()
		})
	} else {
		g.mu.Lock()
		delete(g.calls, key)
		g.mu.Unlock()
	}

	return c.val, c.err, c.count
}

// [速度优化] ShardedGroup 分片请求合并，将全局锁拆分为 N 个分片锁
// 通过 key hash 定位分片，消除高并发下的 mutex 竞争瓶颈
// 256 分片可将锁竞争概率降低 256 倍
type ShardedGroup struct {
	shards    []*Group
	shardMask uint64
}

// NewShardedGroup 创建分片合并组，numShards 必须是 2 的幂
func NewShardedGroup(numShards int) *ShardedGroup {
	if numShards <= 0 || (numShards&(numShards-1)) != 0 {
		numShards = 256 // 默认 256 分片
	}
	shards := make([]*Group, numShards)
	for i := range shards {
		shards[i] = NewGroup()
	}
	return &ShardedGroup{shards: shards, shardMask: uint64(numShards - 1)}
}

// Do 分片版 Do，通过 key 哈希路由到对应分片
func (sg *ShardedGroup) Do(key string, fn func() (interface{}, error), mergeWindow time.Duration) (interface{}, error, int) {
	idx := sg.hash(key) & sg.shardMask
	return sg.shards[idx].Do(key, fn, mergeWindow)
}

// hash FNV-1a 哈希，快速计算 key 的分片索引
func (sg *ShardedGroup) hash(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// Stats 汇总所有分片的统计
func (sg *ShardedGroup) Stats() (activeCalls int, totalWaited int) {
	for _, shard := range sg.shards {
		a, t := shard.Stats()
		activeCalls += a
		totalWaited += t
	}
	return
}
func (g *Group) DoChan(key string, fn func() (interface{}, error), mergeWindow time.Duration) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		val, err, count := g.Do(key, fn, mergeWindow)
		ch <- Result{Value: val, Err: err, WaiterCount: count}
	}()
	return ch
}

// Stats 获取合并统计
func (g *Group) Stats() (activeCalls int, totalWaited int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	activeCalls = len(g.calls)
	for _, c := range g.calls {
		totalWaited += c.count - 1 // 减去发起者
	}
	return
}

// cleanupLoop 定期清理超时的 call（防御性清理，防止死锁）
func (g *Group) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		g.mu.Lock()
		now := time.Now()
		for key, c := range g.calls {
			if now.Sub(c.ts) > 10*time.Second {
				if !c.done {
					c.err = ErrTimeout
					c.wg.Done()
				}
				delete(g.calls, key)
			}
		}
		g.mu.Unlock()
	}
}

// ErrTimeout 请求合并超时错误
var ErrTimeout = &mergeTimeoutError{}

type mergeTimeoutError struct{}

func (e *mergeTimeoutError) Error() string {
	return "请求合并超时，请重试"
}