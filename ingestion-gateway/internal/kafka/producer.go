package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/ollystack/ingestion-gateway/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	messagesProduced = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_kafka_messages_produced_total",
			Help: "Total number of messages produced to Kafka",
		},
		[]string{"topic", "status"},
	)
	bytesProduced = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollystack_kafka_bytes_produced_total",
			Help: "Total bytes produced to Kafka",
		},
		[]string{"topic"},
	)
	produceLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollystack_kafka_produce_latency_seconds",
			Help:    "Latency of Kafka produce operations",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"topic"},
	)
)

// Producer wraps Sarama async producer with metrics
type Producer struct {
	producer sarama.AsyncProducer
	config   config.KafkaConfig
	logger   *zap.Logger
	ready    atomic.Bool
}

// NewProducer creates a new Kafka producer
func NewProducer(cfg config.KafkaConfig, logger *zap.Logger) (*Producer, error) {
	saramaConfig := sarama.NewConfig()

	// Producer settings
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true
	saramaConfig.Producer.RequiredAcks = parseAcks(cfg.Acks)
	saramaConfig.Producer.Retry.Max = cfg.Retries
	saramaConfig.Producer.Retry.Backoff = time.Duration(cfg.RetryBackoffMs) * time.Millisecond
	saramaConfig.Producer.Flush.Bytes = cfg.BatchSize
	saramaConfig.Producer.Flush.Frequency = time.Duration(cfg.LingerMs) * time.Millisecond
	saramaConfig.Producer.MaxMessageBytes = cfg.MaxMessageBytes

	// Compression
	saramaConfig.Producer.Compression = parseCompression(cfg.Compression)

	// Idempotent producer for exactly-once semantics
	saramaConfig.Producer.Idempotent = true
	saramaConfig.Net.MaxOpenRequests = 1 // Required for idempotent

	// TLS configuration
	if cfg.TLS.Enabled {
		tlsConfig, err := createTLSConfig(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}

	// SASL configuration
	if cfg.SASL.Enabled {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = cfg.SASL.Username
		saramaConfig.Net.SASL.Password = cfg.SASL.Password
		saramaConfig.Net.SASL.Mechanism = parseSASLMechanism(cfg.SASL.Mechanism)
	}

	// Create producer
	producer, err := sarama.NewAsyncProducer(cfg.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	p := &Producer{
		producer: producer,
		config:   cfg,
		logger:   logger,
	}
	p.ready.Store(true)

	// Start success/error handlers
	go p.handleSuccesses()
	go p.handleErrors()

	logger.Info("Kafka producer initialized",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("compression", cfg.Compression),
	)

	return p, nil
}

// SendMetrics sends metrics to the metrics topic
func (p *Producer) SendMetrics(tenantID string, key []byte, value []byte) {
	p.send(p.config.MetricsTopic, tenantID, key, value)
}

// SendLogs sends logs to the logs topic
func (p *Producer) SendLogs(tenantID string, key []byte, value []byte) {
	p.send(p.config.LogsTopic, tenantID, key, value)
}

// SendTraces sends traces to the traces topic
func (p *Producer) SendTraces(tenantID string, key []byte, value []byte) {
	p.send(p.config.TracesTopic, tenantID, key, value)
}

// SendAlert sends an alert to the alerts topic
func (p *Producer) SendAlert(tenantID string, key []byte, value []byte) {
	p.send(p.config.AlertsTopic, tenantID, key, value)
}

func (p *Producer) send(topic, tenantID string, key, value []byte) {
	if !p.ready.Load() {
		p.logger.Warn("Producer not ready, dropping message",
			zap.String("topic", topic),
		)
		messagesProduced.WithLabelValues(topic, "dropped").Inc()
		return
	}

	// Add tenant ID to headers for routing
	headers := []sarama.RecordHeader{
		{
			Key:   []byte("tenant_id"),
			Value: []byte(tenantID),
		},
		{
			Key:   []byte("timestamp"),
			Value: []byte(fmt.Sprintf("%d", time.Now().UnixNano())),
		},
	}

	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.ByteEncoder(key),
		Value:     sarama.ByteEncoder(value),
		Headers:   headers,
		Timestamp: time.Now(),
		Metadata:  topic, // Used for metrics
	}

	p.producer.Input() <- msg
	bytesProduced.WithLabelValues(topic).Add(float64(len(value)))
}

func (p *Producer) handleSuccesses() {
	for msg := range p.producer.Successes() {
		topic := msg.Topic
		messagesProduced.WithLabelValues(topic, "success").Inc()

		latency := time.Since(msg.Timestamp).Seconds()
		produceLatency.WithLabelValues(topic).Observe(latency)
	}
}

func (p *Producer) handleErrors() {
	for err := range p.producer.Errors() {
		topic := err.Msg.Topic
		messagesProduced.WithLabelValues(topic, "error").Inc()

		p.logger.Error("Failed to produce message",
			zap.String("topic", topic),
			zap.Error(err.Err),
		)
	}
}

// IsReady returns whether the producer is ready
func (p *Producer) IsReady() bool {
	return p.ready.Load()
}

// Close closes the producer
func (p *Producer) Close() error {
	p.ready.Store(false)
	return p.producer.Close()
}

func parseAcks(acks string) sarama.RequiredAcks {
	switch acks {
	case "0":
		return sarama.NoResponse
	case "1":
		return sarama.WaitForLocal
	default:
		return sarama.WaitForAll
	}
}

func parseCompression(compression string) sarama.CompressionCodec {
	switch compression {
	case "gzip":
		return sarama.CompressionGZIP
	case "snappy":
		return sarama.CompressionSnappy
	case "lz4":
		return sarama.CompressionLZ4
	case "zstd":
		return sarama.CompressionZSTD
	default:
		return sarama.CompressionNone
	}
}

func parseSASLMechanism(mechanism string) sarama.SASLMechanism {
	switch mechanism {
	case "SCRAM-SHA-256":
		return sarama.SASLTypeSCRAMSHA256
	case "SCRAM-SHA-512":
		return sarama.SASLTypeSCRAMSHA512
	default:
		return sarama.SASLTypePlaintext
	}
}

func createTLSConfig(cfg config.TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load cert/key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}
