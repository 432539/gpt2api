// Package service 账号池调度。
//
// 职责：
//   1. 周期性从 DB 装载可用账号到内存（带 TTL 缓存）；
//   2. 提供 Pick(provider) 返回当前应调度的账号（RoundRobin / WeightedRR）；
//   3. 调度结果回写：MarkUsed / MarkFailed（含熔断冷却）。
//
// 不在本组件内做：HTTP 调用 / 计费 / 任务编排。
package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/logger"
)

// AccountPool 多 provider 共用一个池实例，内部按 provider 分桶。
type AccountPool struct {
	repo      *repo.AccountRepo
	cacheTTL  time.Duration
	mu        sync.RWMutex
	buckets   map[string]*providerBucket // key: provider
}

type providerBucket struct {
	loadedAt time.Time
	items    []*model.Account
	cursor   uint64 // RR 游标
	weights  []int  // 权重展开缓存
	wIdx     []int  // weights -> items 索引
}

// NewAccountPool 构造。
func NewAccountPool(r *repo.AccountRepo, cacheTTL time.Duration) *AccountPool {
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Second
	}
	return &AccountPool{
		repo:     r,
		cacheTTL: cacheTTL,
		buckets:  make(map[string]*providerBucket),
	}
}

// Pick 取一个可用账号。strategy: round_robin / weighted_rr / random（默认 round_robin）。
func (p *AccountPool) Pick(ctx context.Context, provider, strategy string) (*model.Account, error) {
	bucket, err := p.getBucket(ctx, provider)
	if err != nil {
		return nil, err
	}
	if len(bucket.items) == 0 {
		return nil, errcode.NoAvailableAcc
	}

	switch strategy {
	case "weighted_rr":
		return p.pickWeighted(bucket), nil
	default:
		return p.pickRR(bucket), nil
	}
}

// MarkUsed 调度成功回写。
func (p *AccountPool) MarkUsed(ctx context.Context, accountID uint64) {
	if err := p.repo.MarkUsed(ctx, accountID); err != nil {
		logger.FromCtx(ctx).Warn("account.mark_used", zap.Uint64("id", accountID), zap.Error(err))
	}
}

// MarkFailed 调度失败回写：reason 写入 last_error；cooldown>0 时进入熔断。
func (p *AccountPool) MarkFailed(ctx context.Context, accountID uint64, reason string, cooldown time.Duration) {
	if err := p.repo.MarkFailed(ctx, accountID, truncate(reason, 240), cooldown); err != nil {
		logger.FromCtx(ctx).Warn("account.mark_failed", zap.Uint64("id", accountID), zap.Error(err))
	}
	if cooldown > 0 {
		// 熔断后强制下一次取重新加载
		p.invalidate(accountIDProvider(p, accountID))
	}
}

// Reload 强制重新装载某 provider（管理后台 CRUD 后调用）。
func (p *AccountPool) Reload(provider string) { p.invalidate(provider) }

// Stats 返回各 provider 当前池中可用数量（用于仪表盘）。
func (p *AccountPool) Stats() map[string]int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]int, len(p.buckets))
	for k, b := range p.buckets {
		out[k] = len(b.items)
	}
	return out
}

// === internal ===

func (p *AccountPool) getBucket(ctx context.Context, provider string) (*providerBucket, error) {
	p.mu.RLock()
	b, ok := p.buckets[provider]
	p.mu.RUnlock()
	if ok && time.Since(b.loadedAt) < p.cacheTTL {
		return b, nil
	}
	return p.loadBucket(ctx, provider)
}

func (p *AccountPool) loadBucket(ctx context.Context, provider string) (*providerBucket, error) {
	items, err := p.repo.AvailableByProvider(ctx, provider)
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	b := &providerBucket{
		loadedAt: time.Now(),
		items:    items,
	}
	for i, it := range items {
		w := it.Weight
		if w <= 0 {
			w = 1
		}
		for j := 0; j < w; j++ {
			b.weights = append(b.weights, w)
			b.wIdx = append(b.wIdx, i)
		}
	}
	p.mu.Lock()
	p.buckets[provider] = b
	p.mu.Unlock()
	return b, nil
}

func (p *AccountPool) pickRR(b *providerBucket) *model.Account {
	n := uint64(len(b.items))
	if n == 0 {
		return nil
	}
	idx := atomic.AddUint64(&b.cursor, 1) % n
	return b.items[idx]
}

func (p *AccountPool) pickWeighted(b *providerBucket) *model.Account {
	n := uint64(len(b.wIdx))
	if n == 0 {
		return nil
	}
	idx := atomic.AddUint64(&b.cursor, 1) % n
	return b.items[b.wIdx[idx]]
}

func (p *AccountPool) invalidate(provider string) {
	if provider == "" {
		return
	}
	p.mu.Lock()
	delete(p.buckets, provider)
	p.mu.Unlock()
}

// accountIDProvider 通过 ID 反查 provider（仅供失败回写后的 invalidate 使用）。
// 为避免额外 SQL，从内存桶中查找；找不到则忽略。
func accountIDProvider(p *AccountPool, id uint64) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for prov, b := range p.buckets {
		for _, it := range b.items {
			if it.ID == id {
				return prov
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
