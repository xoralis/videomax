package queue

import (
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"

	"video-max/pkg/logger"
)

// VideoTaskMessage Kafka 消息体结构
// 从 HTTP Handler 投递到 Kafka Topic 的消息格式
type VideoTaskMessage struct {
	TaskID      string   `json:"task_id"`
	UserIdea    string   `json:"user_idea"`
	ImagePaths  []string `json:"image_paths"`
	AspectRatio string   `json:"aspect_ratio"`
}

// Producer Kafka 消息生产者封装
// 负责将需要异步处理的任务投递到指定的 Kafka Topic
type Producer struct {
	producer sarama.SyncProducer
	topic    string
}

// NewProducer 创建 Kafka 消息生产者实例
func NewProducer(producer sarama.SyncProducer, topic string) *Producer {
	return &Producer{
		producer: producer,
		topic:    topic,
	}
}

// PublishVideoTask 向 Kafka 投递一条视频生成任务消息
// Handler 校验通过后调用此方法，将任务推入消息队列等待后台消费
func (p *Producer) PublishVideoTask(msg VideoTaskMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化 Kafka 消息失败: %w", err)
	}

	kafkaMsg := &sarama.ProducerMessage{
		Topic: p.topic,
		Key:   sarama.StringEncoder(msg.TaskID), // 使用 TaskID 作为分区键确保同一任务的消息有序
		Value: sarama.ByteEncoder(data),
	}

	partition, offset, err := p.producer.SendMessage(kafkaMsg)
	if err != nil {
		return fmt.Errorf("发送 Kafka 消息失败: %w", err)
	}

	logger.Log.Infow("Kafka 消息投递成功",
		"task_id", msg.TaskID,
		"topic", p.topic,
		"partition", partition,
		"offset", offset,
	)
	return nil
}
