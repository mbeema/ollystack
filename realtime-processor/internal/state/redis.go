package state

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/ollystack/realtime-processor/internal/config"
	"go.uber.org/zap"
)

// RedisStore provides state management using Redis
type RedisStore struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisStore creates a new Redis state store
func NewRedisStore(cfg config.RedisConfig, logger *zap.Logger) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:       cfg.Address,
		Password:   cfg.Password,
		DB:         cfg.DB,
		MaxRetries: cfg.MaxRetries,
		PoolSize:   cfg.PoolSize,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Connected to Redis", zap.String("address", cfg.Address))

	return &RedisStore{
		client: client,
		logger: logger,
	}, nil
}

// Get retrieves a value from Redis
func (s *RedisStore) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

// Set stores a value in Redis
func (s *RedisStore) Set(ctx context.Context, key, value string) error {
	return s.client.Set(ctx, key, value, 0).Err()
}

// SetWithExpiry stores a value in Redis with expiration
func (s *RedisStore) SetWithExpiry(ctx context.Context, key, value string, expiry time.Duration) error {
	return s.client.Set(ctx, key, value, expiry).Err()
}

// Exists checks if a key exists
func (s *RedisStore) Exists(ctx context.Context, key string) (bool, error) {
	result, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// Delete removes a key
func (s *RedisStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

// Increment increments a value
func (s *RedisStore) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	return s.client.IncrBy(ctx, key, delta).Result()
}

// IncrementWithExpiry increments a value and sets expiry
func (s *RedisStore) IncrementWithExpiry(ctx context.Context, key string, delta int64, expiry time.Duration) (int64, error) {
	pipe := s.client.Pipeline()
	incr := pipe.IncrBy(ctx, key, delta)
	pipe.Expire(ctx, key, expiry)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	return incr.Val(), nil
}

// GetInt retrieves an integer value
func (s *RedisStore) GetInt(ctx context.Context, key string) (int64, error) {
	result, err := s.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return result, err
}

// SetHash stores a hash
func (s *RedisStore) SetHash(ctx context.Context, key string, values map[string]interface{}) error {
	return s.client.HSet(ctx, key, values).Err()
}

// GetHash retrieves a hash
func (s *RedisStore) GetHash(ctx context.Context, key string) (map[string]string, error) {
	return s.client.HGetAll(ctx, key).Result()
}

// IncrementHashField increments a hash field
func (s *RedisStore) IncrementHashField(ctx context.Context, key, field string, delta int64) (int64, error) {
	return s.client.HIncrBy(ctx, key, field, delta).Result()
}

// AddToSortedSet adds a member to a sorted set
func (s *RedisStore) AddToSortedSet(ctx context.Context, key string, score float64, member string) error {
	return s.client.ZAdd(ctx, key, &redis.Z{
		Score:  score,
		Member: member,
	}).Err()
}

// GetSortedSetRange retrieves a range from a sorted set
func (s *RedisStore) GetSortedSetRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return s.client.ZRange(ctx, key, start, stop).Result()
}

// Close closes the Redis connection
func (s *RedisStore) Close() error {
	return s.client.Close()
}
