package kafka

import (
	"github.com/IBM/sarama"

	"video-max/pkg/logger"
)

// NewSyncProducer 创建 Kafka 同步生产者实例
// 使用同步模式确保消息投递的可靠性（发送后等待 Broker 确认）
func NewSyncProducer(brokers []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true // 同步模式必须开启
	config.Producer.RequiredAcks = sarama.WaitForAll // 等待所有副本确认

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	logger.Log.Infow("Kafka 同步生产者创建成功", "brokers", brokers)
	return producer, nil
}

// NewConsumerGroup 创建 Kafka 消费者组实例
// 消费者组模式支持多实例水平扩展，同一 Group 内的消费者共同消费分区
func NewConsumerGroup(brokers []string, groupID string) (sarama.ConsumerGroup, error) {
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	group, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, err
	}

	logger.Log.Infow("Kafka 消费者组创建成功", "brokers", brokers, "group_id", groupID)
	return group, nil
}
