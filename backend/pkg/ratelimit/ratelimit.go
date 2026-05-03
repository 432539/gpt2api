// Package ratelimit 基于 Redis 的滑动窗口限流。
package ratelimit

import (
	"context"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

// Limiter 包装 redis_rate.Limiter。
type Limiter struct {
	inner *redis_rate.Limiter
}

// New 创建。
func New(rdb *redis.Client) *Limiter {
	return &Limiter{inner: redis_rate.NewLimiter(rdb)}
}

// AllowN 在 ratePerMin/min 速率下尝试获取 n 个令牌。
func (l *Limiter) AllowN(ctx context.Context, key string, ratePerMin, n int) (*redis_rate.Result, error) {
	return l.inner.AllowN(ctx, key, redis_rate.PerMinute(ratePerMin), n)
}

// Allow 简便：取 1 个令牌。
func (l *Limiter) Allow(ctx context.Context, key string, ratePerMin int) (*redis_rate.Result, error) {
	return l.AllowN(ctx, key, ratePerMin, 1)
}

// Reset 删除 key（用于撤销限流）。
func (l *Limiter) Reset(ctx context.Context, key string) error {
	return l.inner.Reset(ctx, key)
}

// RetryAfter 工具函数：把 redis_rate.Result.RetryAfter 转 seconds。
func RetryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	return int(d.Seconds()) + 1
}
