package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/miaosha/config"
	"github.com/miaosha/model"
	"github.com/miaosha/utils"
	"go.uber.org/zap"
)

var (
	producer     sarama.AsyncProducer
	producerOnce sync.Once
	logger       *zap.SugaredLogger
)

func InitProducer(cfg *config.KafkaConfig, log *zap.SugaredLogger) error {
	var initErr error
	producerOnce.Do(func() {
		logger = log
		saramaConfig := sarama.NewConfig()
		saramaConfig.Producer.Return.Successes = false
		saramaConfig.Producer.Return.Errors = true
		saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
		saramaConfig.Producer.Compression = sarama.CompressionSnappy
		saramaConfig.Producer.MaxMessageBytes = cfg.Producer.MaxMessageBytes
		saramaConfig.Producer.Flush.Bytes = cfg.Producer.BatchSize
		saramaConfig.Producer.Flush.Frequency = time.Duration(cfg.Producer.LingerMs) * time.Millisecond
		var err error
		producer, err = sarama.NewAsyncProducer(cfg.Brokers, saramaConfig)
		if err != nil { initErr = fmt.Errorf("创建Kafka生产者失败: %w", err); return }
		go func() {
			for { select { case err := <-producer.Errors(): if err != nil { logger.Errorw("Kafka消息发送失败", "topic", err.Msg.Topic, "error", err.Err.Error()) } } }
		}()
	})
	return initErr
}

// [修复] TrackBehavior 改为异步发送，不阻塞主业务流程
// 使用 goroutine + context 超时控制（5秒），发送失败后重试 + 死信队列
func TrackBehavior(track *model.BehaviorTrack, topic string) {
	if producer == nil {
		return
	}
	data := utils.ToJSON(track)
	msg := &sarama.ProducerMessage{Topic: topic, Key: sarama.StringEncoder(fmt.Sprintf("%d", track.UserID)), Value: sarama.StringEncoder(data)}

	// [修复] 使用 goroutine 异步发送，避免同步阻塞主业务
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// [修复] 最多重试 3 次，间隔 500ms 指数退避
		maxRetries := 3
		for i := 0; i < maxRetries; i++ {
			select {
			case producer.Input() <- msg:
				return // 发送成功
			case <-ctx.Done():
				if i == maxRetries-1 {
					// [修复] 重试耗尽，发送到死信队列
					if logger != nil {
						logger.Warnw("Kafka重试耗尽，消息进入死信队列", "user_id", track.UserID, "topic", topic)
					}
					dlqMsg := &sarama.ProducerMessage{Topic: topic + "_dlq", Key: sarama.StringEncoder(fmt.Sprintf("%d", track.UserID)), Value: sarama.StringEncoder(data)}
					dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer dlqCancel()
					select {
					case producer.Input() <- dlqMsg:
					case <-dlqCtx.Done():
						if logger != nil { logger.Errorw("Kafka死信队列发送失败", "user_id", track.UserID) }
					}
				} else {
					if logger != nil { logger.Warnw("Kafka异步发送失败，重试中", "attempt", i+1, "user_id", track.UserID) }
					time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
				}
				return
			}
		}
	}()
}

func GetProducer() sarama.AsyncProducer { return producer }
func Close() error { if producer != nil { return producer.Close() }; return nil }