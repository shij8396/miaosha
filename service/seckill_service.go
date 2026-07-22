package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/miaosha/config"
	"github.com/miaosha/dao"
	"github.com/miaosha/kafka"
	"github.com/miaosha/log"
	"github.com/miaosha/model"
	"github.com/miaosha/monitor"
	"github.com/miaosha/mq"
	redisClient "github.com/miaosha/redis"
	"github.com/miaosha/utils"
)

// [优化] sync.Pool 热路径对象复用，减少 GC 压力
// 每个秒杀请求创建 orderMsg map，用 Pool 避免重复分配（每秒减少数千次 map 分配）
var orderMsgPool = sync.Pool{
	New: func() interface{} {
		return make(map[string]interface{}, 10)
	},
}

type SeckillService struct {
	idGen utils.IDGenerator
}

func NewSeckillService(idGen utils.IDGenerator) *SeckillService {
	return &SeckillService{idGen: idGen}
}

func (s *SeckillService) ExecuteSeckill(ctx context.Context, userID, productID int64, quantity int, idempotentKey string, clientIP string, traceID string) (*model.SeckillResponse, error) {
	cfg := config.GetConfig()
	logger := log.WithTraceID(traceID)
	startTime := time.Now()
	now := time.Now()

	// [速度优化] 优先从 Redis 缓存读取商品信息，命中则跳过 DB 查询（节省 5-10ms）
	productName, seckillPrice, limitPerUser, status, startTimeT, endTimeT, cacheErr := redisClient.GetProductCache(ctx, productID)
	if cacheErr != nil {
		// 缓存未命中，回退 DB 查询并写入缓存
		product, err := dao.GetProductByID(productID)
		if err != nil { return nil, fmt.Errorf("商品不存在") }
		productName = product.Name
		seckillPrice = product.SeckillPrice
		limitPerUser = product.LimitPerUser
		status = int(product.Status)
		startTimeT = product.StartTime
		endTimeT = product.EndTime
		// 同步写入缓存，确保后续请求立即命中（HMSET 本地 Redis 耗时 <1ms）
		redisClient.SetProductCache(context.Background(), productID, productName, seckillPrice, limitPerUser, status, startTimeT, endTimeT, endTimeT.Sub(now))
	}

	if status != 1 { return nil, utils.NewSeckillError(utils.ErrProductOffline, "商品已下架", 400) }
	if now.Before(startTimeT) || now.After(endTimeT) { return nil, utils.NewSeckillError(utils.ErrNotInSeckillTime, "不在秒杀活动时间内", 400) }

	if limitPerUser <= 0 { limitPerUser = 1 }
	if quantity > limitPerUser {
		return nil, utils.NewSeckillError(utils.ErrExceedLimit, fmt.Sprintf("超出单用户限购数量（每人限购%d件）", limitPerUser), 400)
	}

	// [速度优化] 合并幂等性检查 + 库存扣减 + 限购计数为单次 Redis Lua 原子操作
	// 原来 2 次 RTT → 1 次 RTT，节省 ~1ms
	expireTime := endTimeT.Sub(now)
	remainStock, newCount, code, err := redisClient.DecrStockAndIncrPurchaseWithIdempotent(ctx, userID, productID, quantity, limitPerUser, expireTime, idempotentKey)
	if err != nil {
		logger.Errorw("Redis合并操作失败", "error", err, "product_id", productID)
		monitor.IncSeckillFail(fmt.Sprintf("%d", productID), "redis_error")
		return nil, utils.NewSeckillError(utils.ErrRedisDown, "Redis服务异常，请稍后重试", 503)
	}
	switch code {
	case -4:
		logger.Warnw("检测到重复提交", "user_id", userID, "product_id", productID)
		return nil, utils.NewSeckillError(utils.ErrDuplicateSubmit, "请勿重复提交，您的请求正在处理中", 400)
	case -1:
		monitor.IncSeckillFail(fmt.Sprintf("%d", productID), "redis_error")
		return nil, utils.NewSeckillError(utils.ErrRedisDown, "Redis服务异常，请稍后重试", 503)
	case -2:
		monitor.IncSeckillFail(fmt.Sprintf("%d", productID), "out_of_stock")
		go trackBehavior(userID, productID, "fail", clientIP, "库存不足", startTime, traceID)
		return nil, utils.NewSeckillError(utils.ErrStockInsufficient, "库存不足，秒杀失败", 400)
	case -3:
		monitor.IncSeckillFail(fmt.Sprintf("%d", productID), "duplicate")
		return nil, utils.NewSeckillError(utils.ErrAlreadyPurchased, fmt.Sprintf("您已参与过该商品的秒杀（每人限购%d件，已购买%d件）", limitPerUser, newCount), 400)
	}
	logger.Infow("Redis库存扣减+幂等+用户计数成功", "product_id", productID, "remain_stock", remainStock, "user_count", newCount, "limit", limitPerUser)

	orderNo, err := utils.GenerateOrderNo(s.idGen)
	if err != nil {
		// [P0-3] 订单号生成失败，回滚 Redis 已扣减资源
		_ = redisClient.IncrStock(ctx, productID, quantity)
		_ = redisClient.RemoveUserPurchased(ctx, userID, productID)
		monitor.IncSeckillFail(fmt.Sprintf("%d", productID), "order_no_error")
		return nil, fmt.Errorf("生成订单编号失败: %w", err)
	}

	// [优化] 使用 sync.Pool 复用 map 和 JSON buffer，避免每秒数万次分配
	orderMsg := orderMsgPool.Get().(map[string]interface{})
	orderMsg["order_no"] = orderNo
	orderMsg["user_id"] = userID
	orderMsg["product_id"] = productID
	orderMsg["product_name"] = productName
	orderMsg["seckill_price"] = seckillPrice
	orderMsg["quantity"] = quantity
	orderMsg["total_amount"] = seckillPrice * float64(quantity)
	orderMsg["timestamp"] = now.UnixMilli()

	msgBody, err := json.Marshal(orderMsg)

	// 清理并归还 map 到 Pool
	for k := range orderMsg {
		delete(orderMsg, k)
	}
	orderMsgPool.Put(orderMsg)

	if err != nil {
		// Marshal 失败：回滚 Redis 已扣减资源
		_ = redisClient.IncrStock(ctx, productID, quantity)
		_ = redisClient.RemoveUserPurchased(ctx, userID, productID)
		monitor.IncSeckillFail(fmt.Sprintf("%d", productID), "marshal_error")
		return nil, fmt.Errorf("订单消息序列化失败: %w", err)
	}

	// [P0-1] ChannelPool 模式下 PublishOrder 已无全局锁竞争，发布延迟 <1ms（本地 RabbitMQ）
	// [P0-3] 同步发布订单消息确保可靠性，失败时回滚 Redis 资源
	err = mq.PublishOrder(ctx, cfg.RabbitMQ.Exchange.Order, cfg.RabbitMQ.Queue.Order, msgBody)
	if err != nil {
		_ = redisClient.IncrStock(ctx, productID, quantity)
		_ = redisClient.RemoveUserPurchased(ctx, userID, productID)
		logger.Errorw("RabbitMQ消息发送失败，已回滚Redis库存", "error", err, "order_no", orderNo)
		monitor.IncSeckillFail(fmt.Sprintf("%d", productID), "mq_error")
		return nil, utils.NewSeckillError(utils.ErrMQDown, "消息队列异常，请稍后重试", 503)
	}

	// [P0-3] 延迟消息和用户行为追踪均为 fire-and-forget，不阻塞秒杀响应
	delayMsg := map[string]interface{}{"order_no": orderNo, "user_id": userID, "product_id": productID, "quantity": quantity, "type": "order_timeout_check"}
	delayBody := []byte(utils.ToJSON(delayMsg))
	_ = mq.PublishDelay(ctx, cfg.RabbitMQ.Exchange.Order, cfg.RabbitMQ.Queue.Delay, delayBody)

	go trackBehavior(userID, productID, "success", clientIP, "", startTime, traceID)
	monitor.IncSeckillSuccess(fmt.Sprintf("%d", productID))

	costMs := time.Since(startTime).Milliseconds()
	logger.Infow("秒杀排队成功", "order_no", orderNo, "user_id", userID, "product_id", productID, "cost_ms", costMs)

	// [P0-3] 秒杀响应立即返回 "queued" 状态，不等待 MySQL 写入和延迟消息确认
	return &model.SeckillResponse{OrderNo: orderNo, Status: "queued", Message: "秒杀排队中，请稍后查看订单状态"}, nil
}

func trackBehavior(userID, productID int64, action, clientIP, failReason string, startTime time.Time, traceID string) {
	cfg := config.GetConfig()
	track := &model.BehaviorTrack{
		TraceID: traceID, UserID: userID, ProductID: productID,
		Action: action, RequestIP: clientIP, Result: 0, CostMs: time.Since(startTime).Milliseconds(),
		FailReason: failReason, InstanceID: cfg.Server.InstanceID, Timestamp: time.Now().UnixMilli(),
	}
	if action == "success" { track.Result = 1 }
	kafka.TrackBehavior(track, cfg.Kafka.Topic)
}