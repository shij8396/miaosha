package mq

import (
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/miaosha/config"
	"github.com/miaosha/log"
)

// OrderHandler 订单处理接口，由 service 层实现，避免循环依赖
type OrderHandler interface {
	CreateOrder(msg map[string]interface{}) error
	ProcessTimeoutOrder(msg map[string]interface{}) error
}

// [修复] 最大重试次数，超过此次数后消息转入死信队列
const maxRetryCount = 3

// [修复] 获取当前重试次数，从消息 Headers 中读取 x-retry-count
func getRetryCount(msg amqp.Delivery) int32 {
	if msg.Headers == nil {
		return 0
	}
	if count, ok := msg.Headers["x-retry-count"]; ok {
		switch v := count.(type) {
		case int32:
			return v
		case int64:
			return int32(v)
		case int:
			return int32(v)
		case float64:
			return int32(v)
		}
	}
	return 0
}

func StartOrderConsumer(handler OrderHandler) {
	cfg := config.GetConfig()
	concurrency := cfg.RabbitMQ.Consumer.Concurrency
	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			msgs, err := ConsumeOrder(cfg.RabbitMQ.Queue.Order)
			if err != nil {
				log.L().Errorw("订单消费者启动失败", "worker_id", workerID, "error", err)
				return
			}
			log.L().Infow("订单消费者已启动", "worker_id", workerID)
			for msg := range msgs {
				var orderMsg map[string]interface{}
				if err := json.Unmarshal(msg.Body, &orderMsg); err != nil {
					log.L().Errorw("订单消息解析失败", "worker_id", workerID, "error", err)
					msg.Nack(false, false)
					continue
				}
				if err := handler.CreateOrder(orderMsg); err != nil {
					// [修复] 检查重试次数，避免无限重试导致死循环
					retryCount := getRetryCount(msg)
					log.L().Errorw("订单创建失败", "worker_id", workerID, "error", err, "retry_count", retryCount)
					if retryCount >= maxRetryCount {
						// [修复] 超过最大重试次数，不再重新入队，消息将被丢弃或转入死信
						log.L().Errorw("订单创建超过最大重试次数，放弃重试", "worker_id", workerID, "order_no", orderMsg["order_no"], "retry_count", retryCount)
						msg.Nack(false, false)
					} else {
						// [修复] 重新入队并增加重试计数，避免无限循环
						if msg.Headers == nil {
							msg.Headers = amqp.Table{}
						}
						msg.Headers["x-retry-count"] = retryCount + 1
						msg.Nack(false, true)
					}
					continue
				}
				msg.Ack(false)
				log.L().Infow("订单创建成功", "worker_id", workerID, "order_no", orderMsg["order_no"])
			}
		}(i)
	}
}

func StartDeadLetterConsumer(handler OrderHandler) {
	cfg := config.GetConfig()
	concurrency := cfg.RabbitMQ.Consumer.Concurrency
	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			msgs, err := ConsumeDeadLetter(cfg.RabbitMQ.Queue.DeadLetter)
			if err != nil {
				log.L().Errorw("死信队列消费者启动失败", "worker_id", workerID, "error", err)
				return
			}
			log.L().Infow("死信队列消费者已启动", "worker_id", workerID)
			for msg := range msgs {
				var timeoutMsg map[string]interface{}
				if err := json.Unmarshal(msg.Body, &timeoutMsg); err != nil {
					log.L().Errorw("超时消息解析失败", "worker_id", workerID, "error", err)
					msg.Ack(false)
					continue
				}
				if timeoutMsg["type"] != "order_timeout_check" {
					msg.Ack(false)
					continue
				}
				if err := handler.ProcessTimeoutOrder(timeoutMsg); err != nil {
					// [修复] 死信队列消费者也需要重试限制，避免无限重试
					retryCount := getRetryCount(msg)
					log.L().Errorw("超时订单处理失败", "worker_id", workerID, "error", err, "retry_count", retryCount)
					if retryCount >= maxRetryCount {
						log.L().Errorw("超时订单处理超过最大重试次数，放弃重试", "worker_id", workerID, "order_no", timeoutMsg["order_no"], "retry_count", retryCount)
						msg.Nack(false, false)
					} else {
						if msg.Headers == nil {
							msg.Headers = amqp.Table{}
						}
						msg.Headers["x-retry-count"] = retryCount + 1
						msg.Nack(false, true)
					}
					continue
				}
				msg.Ack(false)
				log.L().Infow("超时订单已自动取消", "worker_id", workerID, "order_no", timeoutMsg["order_no"])
			}
		}(i)
	}
}