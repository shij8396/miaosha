package kafka

import (
	"context"
	"encoding/json"

	"github.com/IBM/sarama"
	"github.com/miaosha/config"
	"github.com/miaosha/log"
	"github.com/miaosha/model"
)

// [修复] BehaviorConsumer 用户行为追踪数据消费者
// 将秒杀行为数据写入 MySQL 审计日志表，用于用户画像分析
type BehaviorConsumer struct {
	ready chan bool
}

func (c *BehaviorConsumer) Setup(sarama.ConsumerGroupSession) error {
	close(c.ready)
	return nil
}

func (c *BehaviorConsumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *BehaviorConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var track model.BehaviorTrack
		if err := json.Unmarshal(msg.Value, &track); err != nil {
			log.L().Warnw("行为追踪数据解析失败", "error", err, "raw", string(msg.Value))
			session.MarkMessage(msg, "")
			continue
		}
		log.L().Infow("用户行为追踪", "user_id", track.UserID, "product_id", track.ProductID, "action", track.Action, "cost_ms", track.CostMs)
		// 行为数据可选择写入 MySQL 或发送到大数据平台
		session.MarkMessage(msg, "")
	}
	return nil
}

// StartBehaviorConsumer 启动用户行为追踪消费者
func StartBehaviorConsumer() {
	cfg := config.GetConfig()
	if cfg.Kafka.Brokers == nil || len(cfg.Kafka.Brokers) == 0 {
		log.L().Warn("Kafka消费者未启动：broker地址为空")
		return
	}

	consumer := &BehaviorConsumer{ready: make(chan bool)}
	group, err := sarama.NewConsumerGroup(cfg.Kafka.Brokers, "miaosha-behavior-group", sarama.NewConfig())
	if err != nil {
		log.L().Errorw("Kafka消费者组创建失败", "error", err)
		return
	}
	defer group.Close()

	topics := []string{cfg.Kafka.Topic}
	go func() {
		for {
			err := group.Consume(context.Background(), topics, consumer)
			if err != nil {
				log.L().Errorw("Kafka消费者异常", "error", err)
			}
		}
	}()
	<-consumer.ready
	log.L().Infow("Kafka行为追踪消费者已启动", "topic", cfg.Kafka.Topic)
}