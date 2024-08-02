package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/ollystack/realtime-processor/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	messagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_rt_kafka_messages_consumed_total",
			Help: "Total number of messages consumed from Kafka",
		},
		[]string{"topic"},
	)
	processingLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_rt_kafka_processing_latency_seconds",
			Help:    "Message processing latency",
			Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1},
		},
		[]string{"topic"},
	)
)

// MessageHandler is a function that handles a consumed message
type MessageHandler func(msg []byte) error

// Consumer wraps Sarama consumer group
type Consumer struct {
	group  sarama.ConsumerGroup
	config config.KafkaConfig
	logger *zap.Logger
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(cfg config.KafkaConfig, logger *zap.Logger) (*Consumer, error) {
	saramaConfig := sarama.NewConfig()

	// Consumer settings - optimized for low latency
	saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest // Start from latest for real-time
	saramaConfig.Consumer.Group.Session.Timeout = 10 * time.Second
	saramaConfig.Consumer.Group.Heartbeat.Interval = 3 * time.Second
	saramaConfig.Consumer.MaxProcessingTime = 1 * time.Second // Low timeout for real-time

	// Low-latency fetch settings
	saramaConfig.Consumer.Fetch.Min = 1
	saramaConfig.Consumer.Fetch.Default = 256 * 1024  // 256KB - smaller for lower latency
	saramaConfig.Consumer.MaxWaitTime = 100 * time.Millisecond // Wait max 100ms

	group, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	logger.Info("Kafka consumer group created (real-time mode)",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("group_id", cfg.GroupID),
	)

	return &Consumer{
		group:  group,
		config: cfg,
		logger: logger,
	}, nil
}

// Start starts consuming from all configured topics
func (c *Consumer) Start(ctx context.Context, handlers map[string]MessageHandler) error {
	topics := make([]string, 0, len(handlers))
	for topic := range handlers {
		topics = append(topics, topic)
	}

	handler := &consumerGroupHandler{
		handlers: handlers,
		logger:   c.logger,
	}

	c.logger.Info("Starting real-time consumer",
		zap.Strings("topics", topics),
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.group.Consume(ctx, topics, handler); err != nil {
				c.logger.Error("Consumer error", zap.Error(err))
				time.Sleep(time.Second)
			}
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	return c.group.Close()
}

type consumerGroupHandler struct {
	handlers map[string]MessageHandler
	logger   *zap.Logger
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.logger.Info("Real-time consumer group setup",
		zap.Int32("generation", session.GenerationID()),
	)
	return nil
}

func (h *consumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	topic := claim.Topic()

	handler, ok := h.handlers[topic]
	if !ok {
		return nil
	}

	for {
		select {
		case <-session.Context().Done():
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			start := time.Now()

			// Process message with low latency
			if err := handler(msg.Value); err != nil {
				h.logger.Warn("Failed to process message",
					zap.String("topic", topic),
					zap.Error(err),
				)
				// Still mark as processed to avoid blocking
			}

			session.MarkMessage(msg, "")

			messagesConsumed.WithLabelValues(topic).Inc()
			processingLatency.WithLabelValues(topic).Observe(time.Since(start).Seconds())
		}
	}
}
