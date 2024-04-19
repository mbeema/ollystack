package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/ollystack/storage-consumer/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	messagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_kafka_messages_consumed_total",
			Help: "Total number of messages consumed from Kafka",
		},
		[]string{"topic"},
	)
	bytesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_kafka_bytes_consumed_total",
			Help: "Total bytes consumed from Kafka",
		},
		[]string{"topic"},
	)
	consumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ollystack_kafka_consumer_lag",
			Help: "Consumer lag (messages behind)",
		},
		[]string{"topic", "partition"},
	)
	processingLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_kafka_processing_latency_seconds",
			Help:    "Message processing latency",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"topic"},
	)
)

// MessageHandler is a function that handles a consumed message
type MessageHandler func(msg []byte) error

// Consumer wraps Sarama consumer group
type Consumer struct {
	group    sarama.ConsumerGroup
	config   config.KafkaConfig
	handlers map[string]MessageHandler
	logger   *zap.Logger
	wg       sync.WaitGroup
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(cfg config.KafkaConfig, logger *zap.Logger) (*Consumer, error) {
	saramaConfig := sarama.NewConfig()

	// Consumer settings
	saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	saramaConfig.Consumer.Offsets.Initial = parseOffsetReset(cfg.AutoOffsetReset)
	saramaConfig.Consumer.Group.Session.Timeout = time.Duration(cfg.SessionTimeoutMs) * time.Millisecond
	saramaConfig.Consumer.Group.Heartbeat.Interval = time.Duration(cfg.HeartbeatIntervalMs) * time.Millisecond
	saramaConfig.Consumer.MaxProcessingTime = 60 * time.Second

	// Performance tuning
	saramaConfig.Consumer.Fetch.Min = 1
	saramaConfig.Consumer.Fetch.Default = 1024 * 1024 // 1MB
	saramaConfig.Consumer.Fetch.Max = 10 * 1024 * 1024 // 10MB

	// Create consumer group
	group, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	logger.Info("Kafka consumer group created",
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
	c.handlers = handlers

	topics := make([]string, 0, len(handlers))
	for topic := range handlers {
		topics = append(topics, topic)
	}

	handler := &consumerGroupHandler{
		handlers: handlers,
		logger:   c.logger,
	}

	c.logger.Info("Starting consumer",
		zap.Strings("topics", topics),
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Consume handles reconnection automatically
			if err := c.group.Consume(ctx, topics, handler); err != nil {
				c.logger.Error("Consumer error", zap.Error(err))
				// Brief pause before retry
				time.Sleep(time.Second)
			}
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	return c.group.Close()
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	handlers map[string]MessageHandler
	logger   *zap.Logger
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.logger.Info("Consumer group setup",
		zap.Int32("generation", session.GenerationID()),
	)
	return nil
}

func (h *consumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	h.logger.Info("Consumer group cleanup",
		zap.Int32("generation", session.GenerationID()),
	)
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	topic := claim.Topic()
	partition := claim.Partition()

	h.logger.Debug("Starting to consume",
		zap.String("topic", topic),
		zap.Int32("partition", partition),
	)

	handler, ok := h.handlers[topic]
	if !ok {
		h.logger.Warn("No handler for topic", zap.String("topic", topic))
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

			// Process message
			if err := handler(msg.Value); err != nil {
				h.logger.Error("Failed to process message",
					zap.String("topic", topic),
					zap.Int32("partition", partition),
					zap.Int64("offset", msg.Offset),
					zap.Error(err),
				)
				// Don't commit offset on error
				continue
			}

			// Mark message as processed
			session.MarkMessage(msg, "")

			// Update metrics
			messagesConsumed.WithLabelValues(topic).Inc()
			bytesConsumed.WithLabelValues(topic).Add(float64(len(msg.Value)))
			processingLatency.WithLabelValues(topic).Observe(time.Since(start).Seconds())

			// Update lag metric
			lag := claim.HighWaterMarkOffset() - msg.Offset - 1
			if lag < 0 {
				lag = 0
			}
			consumerLag.WithLabelValues(topic, fmt.Sprintf("%d", partition)).Set(float64(lag))
		}
	}
}

func parseOffsetReset(reset string) int64 {
	switch reset {
	case "earliest":
		return sarama.OffsetOldest
	default:
		return sarama.OffsetNewest
	}
}
