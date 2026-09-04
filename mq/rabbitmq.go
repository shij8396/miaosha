package mq

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miaosha/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// [P0-1] ChannelPoolItem 池化 Channel 包装体，每个 Channel 绑定独立互斥锁，
// 替代全局 publishMu 串行瓶颈，实现 N 路并发发布（N = ChannelPoolSize，默认 20）。
type channelPoolItem struct {
	ch *amqp.Channel
	mu sync.Mutex
}

// [P0-1] ChannelPool MQ Channel 连接池，通过 round-robin 轮询分发发布请求，
// 将全局串行锁拆分为 per-channel 锁，理论上吞吐量提升 ChannelPoolSize 倍。
type ChannelPool struct {
	items []*channelPoolItem
	next  atomic.Uint64
}

// [P0-1] NewChannelPool 创建 Channel 连接池，在连接上批量创建 Channel。
// size 建议值为 20-50，单机环境下过大无益（受限于 CPU 核心数和网络带宽）。
func NewChannelPool(conn *amqp.Connection, size int) (*ChannelPool, error) {
	items := make([]*channelPoolItem, size)
	for i := 0; i < size; i++ {
		ch, err := conn.Channel()
		if err != nil {
			// 清理已创建的 Channel，避免资源泄漏
			for j := 0; j < i; j++ {
				items[j].ch.Close()
			}
			return nil, fmt.Errorf("创建ChannelPool第%d个Channel失败: %w", i, err)
		}
		items[i] = &channelPoolItem{ch: ch}
	}
	return &ChannelPool{items: items}, nil
}

// [P0-1] Get 通过 atomic 轮询获取一个 Channel，无锁竞争。
// 各 Channel 的 publish 操作由各自的 mu 保护，N 个 Channel 支持 N 路并发发布。
func (p *ChannelPool) Get() *channelPoolItem {
	idx := p.next.Add(1) % uint64(len(p.items))
	return p.items[idx]
}

func (p *ChannelPool) Close() {
	for _, item := range p.items {
		item.ch.Close()
	}
}

var (
	conn *amqp.Connection
	// [P0-1] 移除全局 channel + publishMu，改用 ChannelPool 实现 N 路并发发布
	channelPool *ChannelPool
	consumeCh   *amqp.Channel
	mqOnce      sync.Once
	// [修复] 缓存全局配置，供 PublishDelay 等函数使用
	globalCfg *config.RabbitMQConfig

	// [增强] MQ 发布/消费计数（数据大屏真实堆积统计）
	mqPublished int64 // atomic，累计发布成功条数
	mqConsumed  int64 // atomic，累计消费完成（Ack）条数
)

// [增强] MQStats MQ 实时统计（供数据大屏展示）
type MQStats struct {
	Published int64 `json:"published"` // 累计发布
	Consumed  int64 `json:"consumed"`  // 累计消费
	Backlog   int64 `json:"backlog"`   // 当前堆积（发布-消费，近似值）
	Connected bool  `json:"connected"` // MQ 连接是否可用
}

// GetMQStats 获取 MQ 实时统计；MQ 不可用时 Connected=false，堆积按发布-消费近似
func GetMQStats() MQStats {
	published := atomic.LoadInt64(&mqPublished)
	consumed := atomic.LoadInt64(&mqConsumed)
	backlog := published - consumed
	if backlog < 0 {
		backlog = 0
	}
	return MQStats{
		Published: published,
		Consumed:  consumed,
		Backlog:   backlog,
		Connected: conn != nil && !conn.IsClosed(),
	}
}

// IncConsumed 消费者 Ack 成功后调用（consumer.go 内埋点）
func IncConsumed() {
	atomic.AddInt64(&mqConsumed, 1)
}

func Init(cfg *config.RabbitMQConfig) error {
	var initErr error
	mqOnce.Do(func() {
		globalCfg = cfg
		amqpCfg := amqp.Config{
			Heartbeat: time.Duration(cfg.Heartbeat) * time.Second,
			Locale:    "en_US",
		}
		for _, url := range cfg.URLs {
			var err error
			// [修复] 在连接 URL 中显式添加 vhost，确保连接到正确的虚拟主机
			finalURL := url
			if cfg.Vhost != "" && !strings.Contains(url, "/"+cfg.Vhost) {
				// 移除可能已存在的尾部斜杠后追加 vhost
				finalURL = strings.TrimRight(url, "/") + "/" + cfg.Vhost
			}
			conn, err = amqp.DialConfig(finalURL, amqpCfg)
			if err == nil {
				break
			}
		}
		if conn == nil {
			initErr = fmt.Errorf("RabbitMQ集群所有节点连接失败")
			return
		}

		// [P0-1] 创建 ChannelPool 替代全局单 Channel，实现 N 路并发发布
		pool, err := NewChannelPool(conn, cfg.ChannelPoolSize)
		if err != nil {
			initErr = fmt.Errorf("创建ChannelPool失败: %w", err)
			return
		}
		channelPool = pool

		// [P0-1] 使用池中 Channel 声明交换机/队列，声明完成后归还
		initItem := channelPool.Get()
		initItem.mu.Lock()
		initCh := initItem.ch

		// 声明交换机
		err = initCh.ExchangeDeclare(cfg.Exchange.Order, "direct", true, false, false, false, nil)
		if err != nil {
			initItem.mu.Unlock()
			initErr = fmt.Errorf("声明订单交换机失败: %w", err)
			return
		}
		err = initCh.ExchangeDeclare(cfg.Exchange.DeadLetter, "direct", true, false, false, false, nil)
		if err != nil {
			initItem.mu.Unlock()
			initErr = fmt.Errorf("声明死信交换机失败: %w", err)
			return
		}

		// [修复] 使用 TTL + 死信交换实现延迟消息（不依赖 x-delayed-message 插件）
		// 消息发到延迟队列后，TTL 到期自动转入死信队列

		// 延迟队列（使用 TTL + 死信机制实现延迟，必须先声明再绑定）
		delayArgs := amqp.Table{
			"x-dead-letter-exchange":    cfg.Exchange.DeadLetter,
			"x-dead-letter-routing-key": cfg.Queue.DeadLetter,
			"x-message-ttl":             int32(cfg.DelayTTLMs),
		}
		_, err = initCh.QueueDeclare(cfg.Queue.Delay, true, false, false, false, delayArgs)
		if err != nil {
			initItem.mu.Unlock()
			initErr = fmt.Errorf("声明延迟队列失败: %w", err)
			return
		}
		// [修复] 延迟队列绑定到订单交换机，PublishDelay 通过此路由发送延迟消息
		initCh.QueueBind(cfg.Queue.Delay, cfg.Queue.Delay, cfg.Exchange.Order, false, nil)

		// 订单队列
		_, err = initCh.QueueDeclare(cfg.Queue.Order, true, false, false, false, nil)
		if err != nil {
			initItem.mu.Unlock()
			initErr = fmt.Errorf("声明订单队列失败: %w", err)
			return
		}
		initCh.QueueBind(cfg.Queue.Order, cfg.Queue.Order, cfg.Exchange.Order, false, nil)

		// 死信队列
		_, err = initCh.QueueDeclare(cfg.Queue.DeadLetter, true, false, false, false, nil)
		if err != nil {
			initItem.mu.Unlock()
			initErr = fmt.Errorf("声明死信队列失败: %w", err)
			return
		}
		initCh.QueueBind(cfg.Queue.DeadLetter, cfg.Queue.DeadLetter, cfg.Exchange.DeadLetter, false, nil)

		initCh.Qos(cfg.Consumer.Prefetch, 0, false)
		initItem.mu.Unlock()

		// [修复] 创建独立的消费者 Channel，避免与发布 Channel 冲突
		consumeCh, err = conn.Channel()
		if err != nil {
			initErr = fmt.Errorf("创建消费者Channel失败: %w", err)
			return
		}
		consumeCh.Qos(cfg.Consumer.Prefetch, 0, false)
	})
	return initErr
}

// [P0-1] GetChannel 已废弃，ChannelPool 模式下不再暴露单 Channel。
// 保留此函数以兼容旧调用方，返回 nil 表示需改用 ChannelPool 模式。
func GetChannel() *amqp.Channel { return nil }

// [P0-1] PublishOrder 从 ChannelPool 获取 Channel，发布后归还。
// 每个 Channel 由独立 mu 保护，N 路并发发布互不阻塞。
func PublishOrder(ctx context.Context, exchange, routingKey string, body []byte) error {
	if channelPool == nil {
		return fmt.Errorf("ChannelPool未初始化")
	}
	item := channelPool.Get()
	item.mu.Lock()
	defer item.mu.Unlock()
	err := item.ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body, Timestamp: time.Now(),
	})
	// [增强] 发布成功计数（大屏 MQ 堆积统计）
	if err == nil {
		atomic.AddInt64(&mqPublished, 1)
	}
	return err
}

// [P0-1] PublishDelay 从 ChannelPool 获取 Channel 发送延迟消息。
// 延迟消息发送到订单交换机，路由到延迟队列（TTL 自动触发死信转发）。
func PublishDelay(ctx context.Context, exchange, routingKey string, body []byte) error {
	if channelPool == nil {
		return fmt.Errorf("ChannelPool未初始化")
	}
	// [修复] 延迟消息发送到订单交换机，路由到延迟队列（TTL 自动触发死信转发）
	actualExchange := exchange
	actualRoutingKey := routingKey
	if actualExchange == "" && globalCfg != nil {
		actualExchange = globalCfg.Exchange.Order
	}
	if actualRoutingKey == "" && globalCfg != nil {
		actualRoutingKey = globalCfg.Queue.Delay
	}
	item := channelPool.Get()
	item.mu.Lock()
	defer item.mu.Unlock()
	err := item.ch.PublishWithContext(ctx, actualExchange, actualRoutingKey, false, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body, Timestamp: time.Now(),
	})
	// [增强] 延迟消息发布成功同样计入发布计数
	if err == nil {
		atomic.AddInt64(&mqPublished, 1)
	}
	return err
}

func ConsumeOrder(queueName string) (<-chan amqp.Delivery, error) {
	if consumeCh == nil {
		return nil, fmt.Errorf("消费者Channel未初始化")
	}
	return consumeCh.Consume(queueName, "", false, false, false, false, nil)
}

func ConsumeDeadLetter(queueName string) (<-chan amqp.Delivery, error) {
	if consumeCh == nil {
		return nil, fmt.Errorf("消费者Channel未初始化")
	}
	return consumeCh.Consume(queueName, "", false, false, false, false, nil)
}

func Close() {
	if consumeCh != nil {
		consumeCh.Close()
	}
	// [P0-1] 关闭 ChannelPool 而非单 Channel
	if channelPool != nil {
		channelPool.Close()
	}
	if conn != nil {
		conn.Close()
	}
}

// [修复] Ping 检查 RabbitMQ 连接是否正常，用于健康检查探针
func Ping() error {
	if conn == nil {
		return fmt.Errorf("RabbitMQ连接未初始化")
	}
	if conn.IsClosed() {
		return fmt.Errorf("RabbitMQ连接已关闭")
	}
	return nil
}
