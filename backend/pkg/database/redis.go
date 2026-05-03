package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/kleinai/backend/pkg/config"
	"github.com/kleinai/backend/pkg/logger"
)

// NewRedis 创建 go-redis 客户端并 ping 一次。
func NewRedis(c *config.Redis) (*redis.Client, error) {
	if c.Addr == "" {
		return nil, fmt.Errorf("redis addr empty")
	}
	pool := c.PoolSize
	if pool <= 0 {
		pool = 50
	}

	cli := redis.NewClient(&redis.Options{
		Addr:         c.Addr,
		Password:     c.Password,
		DB:           c.DB,
		PoolSize:     pool,
		MinIdleConns: 10,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	logger.L().Info("redis connected", zap.String("addr", c.Addr), zap.Int("db", c.DB))
	return cli, nil
}
