# 秒杀系统高并发优化方案

> Status: ALIGNED
> Author: 宋家豪
> Last updated: 2026-07-21

## 背景

当前系统在 1000 并发压测中 P99 延迟 1723ms、QPS 575，核心瓶颈在 MQ 单 Channel 串行发布、Redis 多次往返、秒杀接口同步写 MySQL。本次优化目标：单机支撑 5,000-10,000 并发，P99 < 100ms，QPS > 5,000。

## 前置条件

- 当前压测基线：1000 并发 / P99 1,723ms / QPS 575 / 成功率 87-100%
- 单机硬件：Windows 笔记本，Docker 运行全部中间件
- 优化环境：Sentinel 压测期间禁用，生产环境恢复

## 优化范围

### In scope

| 编号 | 优化项 | 改动文件 | 预期收益 |
|------|--------|---------|---------|
| P0-1 | MQ Channel 连接池 | `mq/rabbitmq.go` | 消除全局串行锁，QPS 提升 10-50x |
| P0-2 | Redis Pipeline 合并 | `service/seckill_service.go` | 2 次 RTT → 1 次，延迟减半 |
| P0-3 | 订单异步写入 | `service/seckill_service.go`, `mq/consumer.go`, `dao/` | 秒杀接口不阻塞 MySQL 写入 |

### Out of scope

- 订单查询 Redis 缓存（不在热路径）
- MySQL 分表优化（已有 16 分表）
- 中间件集群化（单机环境不需要）
- CDN/WAF/多机房（需要外部基础设施）
- 前端改动

## 假设

1. Sentinel 在压测期间禁用（`enabled: false`），压测完成后恢复
2. 单机环境中间件（MySQL/Redis/RabbitMQ/Kafka）全部通过 Docker 运行
3. 压测用户已通过 `stress_test/cmd/setup.go` 批量注册（1000 个 stress_user_*）
4. 压测商品已创建（ID=8，库存 10000，限购 3 件/人）

---

## 技术方案

### P0-1: MQ Channel 连接池

**当前问题：**

`mq/rabbitmq.go` 全局单 Channel + `publishMu` 互斥锁，1000 并发全部排队等锁。

```go
// 当前代码 —— 全局瓶颈
var channel *amqp.Channel
var publishMu sync.Mutex

func PublishOrder(ctx context.Context, exchange, routingKey string, body []byte) error {
    publishMu.Lock()         // ← 1000 个 goroutine 排队等这把锁
    defer publishMu.Unlock()
    return channel.PublishWithContext(...)
}
```

**目标设计：**

Channel 池化，每个 goroutine 从池中获取独立 Channel，用完归还。连接池大小 = 20（可配置）。

```go
// 新架构
type ChannelPool struct {
    pool    chan *amqp.Channel
    conn    *amqp.Connection
    mu      sync.Mutex
    size    int
}

func NewChannelPool(conn *amqp.Connection, size int) (*ChannelPool, error) {
    p := &ChannelPool{
        pool: make(chan *amqp.Channel, size),
        conn: conn,
        size: size,
    }
    for i := 0; i < size; i++ {
        ch, err := conn.Channel()
        if err != nil {
            return nil, err
        }
        // 设置 Channel QoS 防止单 Channel 内存爆炸
        ch.Qos(size, 0, false)
        p.pool <- ch
    }
    return p, nil
}

func (p *ChannelPool) Get() (*amqp.Channel, error) {
    select {
    case ch := <-p.pool:
        return ch, nil
    default:
        // 池中无空闲 Channel，动态创建（但不回池，避免无限膨胀）
        return p.conn.Channel()
    }
}

func (p *ChannelPool) Put(ch *amqp.Channel) {
    select {
    case p.pool <- ch:
        // 归还成功
    default:
        // 池已满，关闭多余 Channel
        ch.Close()
    }
}
```

**改动清单：**

| 文件 | 改动 |
|------|------|
| `mq/rabbitmq.go` | 新增 `ChannelPool` 结构体 + `Get`/`Put`/`NewChannelPool` 方法 |
| `mq/rabbitmq.go` | `Init()` 中创建 `ChannelPool` 替代全局 `channel` |
| `mq/rabbitmq.go` | `PublishOrder()` 从池中 `Get` Channel，`defer Put` 归还 |
| `mq/rabbitmq.go` | `PublishDelay()` 同样改为池化 |
| `mq/rabbitmq.go` | 删除 `publishMu` 全局锁 |
| `config/config.go` | 新增 `ChannelPoolSize int` 配置项 |
| `config/config.yaml` | 新增 `channel_pool_size: 20` |

---

### P0-2: Redis Pipeline 合并

**当前问题：**

`service/seckill_service.go` 中 Redis 操作分 3 次独立往返：

```go
// 当前代码 —— 3 次 RTT
stockLeft, err := redisClient.Decr(ctx, stockKey)     // RTT 1
// ... 中间逻辑 ...
userCount, err := redisClient.Incr(ctx, limitKey)      // RTT 2
// ... 中间逻辑 ...
redisClient.SetNX(ctx, idempotentKey, "1", 5*time.Second) // RTT 3
```

**目标设计：**

将库存扣减 + 限购计数合并为 1 次 Redis Pipeline（幂等校验独立，因为它需要条件判断）。

```go
// 新代码 —— 库存扣减 + 限购计数 = 1 次 RTT
pipe := redisClient.Pipeline()
decrCmd := pipe.Decr(ctx, stockKey)
incrCmd := pipe.Incr(ctx, limitKey)
_, err := pipe.Exec(ctx)

stockLeft, _ := decrCmd.Result()
userCount, _ := incrCmd.Result()
```

**改动清单：**

| 文件 | 改动 |
|------|------|
| `service/seckill_service.go` | 将 `Decr` + `Incr` 改为 Pipeline 批量执行 |
| `service/seckill_service.go` | 幂等校验 `SetNX` 保持独立（需要条件判断） |

---

### P0-3: 订单异步写入

**当前问题：**

秒杀接口内部同步调用 DAO 写 MySQL 订单表，数据库写入延迟（~5-20ms）直接拖慢秒杀响应。

```
当前流程：Redis 扣库存 → MQ 发消息 → 消费者写 MySQL（已是异步）
问题：MQ 消费者写 MySQL 前，用户查不到订单
```

实际上当前系统已经是异步的（MQ 消费者写 MySQL），但 P0-1 的 MQ 池化会消除 MQ 发布阻塞，让这个异步链路真正生效。核心改动：

**目标设计：**

1. 秒杀接口不再等待 MQ 消费者创建订单，直接返回 "排队中"
2. 消费者批量写入 MySQL 后，通过 WebSocket 推送订单状态变更
3. 用户查询订单时，如果订单还在排队中，返回 "处理中" 状态

**改动清单：**

| 文件 | 改动 |
|------|------|
| `service/seckill_service.go` | 秒杀成功后立即返回 `status: "queued"`，不再等 MySQL |
| `mq/consumer.go` | 消费者创建订单后调用 `wsHub.PushToUser(userID, orderStatus)` |
| `dao/order_dao.go` | 无改动（已有批量写入逻辑） |

注：当前代码已基本实现异步流程（Redis 扣库存 → MQ 消息 → 消费者写 MySQL），P0-3 主要是确认和加固这个链路，确保 Redis 库存回滚机制正确。

---

## 测试方案

### 测试工具

使用现有 `stress_test/cmd/main.go`，已支持：
- 批量注册 1000 测试用户
- 限速登录获取 Token 池
- 多并发级别（100/500/1000/5000）
- QPS/P50/P99/成功率/错误分布采集

### 测试步骤

```
1. 启动后端（Sentinel 禁用）
2. 运行 stress_test/cmd/setup.go 初始化压测商品
3. 运行 stress_test/cmd/main.go 执行压测
4. 记录优化前后对比数据
5. 恢复 Sentinel 配置
```

### 成功标准

| 并发级别 | 成功率 | P99 | QPS |
|---------|--------|-----|-----|
| 100 | > 99% | < 200ms | > 500 |
| 500 | > 95% | < 500ms | > 1,000 |
| 1000 | > 90% | < 800ms | > 2,000 |
| 5000 | > 80% | < 2000ms | > 5,000 |

对比基线（优化前）：

| 并发级别 | 成功率 | P99 | QPS |
|---------|--------|-----|-----|
| 100 | 100% | 243ms | 410 |
| 500 | 87% | 745ms | 662 |
| 1000 | 100% | 1,724ms | 575 |

### 测试用例

| 用例 | 输入 | 预期输出 |
|------|------|---------|
| TC-1 单次秒杀 | 1 用户 1 商品 | code=200, order_no 非空 |
| TC-2 重复幂等 | 相同 idempotent_key 5s 内重发 | 返回相同结果，不重复扣库存 |
| TC-3 库存不足 | 库存=0 时秒杀 | code≠200, msg="库存不足" |
| TC-4 限购拦截 | 超过 limit_per_user 后重试 | code≠200, msg 含 "限购" |
| TC-5 100 并发 | 100 用户同时秒杀 | 成功率 > 99%, P99 < 200ms |
| TC-6 500 并发 | 500 用户同时秒杀 | 成功率 > 95%, P99 < 500ms |
| TC-7 1000 并发 | 1000 用户同时秒杀 | 成功率 > 90%, P99 < 800ms |
| TC-8 5000 并发 | 5000 用户同时秒杀 | 成功率 > 80%, P99 < 2000ms |
| TC-9 超卖验证 | 压测后检查库存 | 实际订单数 ≤ 初始库存 |
| TC-10 连接池泄漏 | 压测后检查 goroutine 数 | goroutine 数不持续增长 |

---

## 核心实体

| 实体 | 类型 | 关键字段 | 关系 |
|------|------|---------|------|
| ChannelPool | 新增结构体 | pool(chan *amqp.Channel), size(int) | 替代全局 channel |
| SeckillService | 已有 | stockKey, limitKey, idempotentKey | 库存扣减改为 Pipeline |
| Order | 已有 | order_no, status(queued/pending/paid/cancelled) | 消费者异步写入 |

---

## 面试元数据

- Mode: --quick
- Waves: 1
- Final ambiguity: 10%
- Status: PASSED

### Clarity breakdown

| 维度 | 分数 | 权重 | 加权 |
|------|------|------|------|
| Goal | 0.9 | 0.40 | 0.36 |
| Scope | 0.9 | 0.25 | 0.225 |
| AC | 0.9 | 0.25 | 0.225 |
| Context | 0.9 | 0.10 | 0.09 |
| **Ambiguity** | | | **10.0%** |