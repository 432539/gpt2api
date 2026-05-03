// Package service 生成任务编排：创建 → 预扣 → 调度账号 → 调用 provider → 结算 / 退款。
//
// 当前实现为同步 inline 执行（开发期）。生产建议替换为 asynq 投递到 worker。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/logger"
)

// GenerationService 生成调度服务。
type GenerationService struct {
	db        *gorm.DB
	repo      *repo.GenerationRepo
	pool      *AccountPool
	billing   *BillingService
	providers map[string]provider.Provider // key: "gpt" / "grok"
	priceFn   PriceFunc
	aes       *crypto.AESGCM // 用于解密 account.credential_enc
}

// PriceFunc 模型计费：返回单次成本（点 *100）。
type PriceFunc func(modelCode string, kind provider.Kind, params map[string]any) int64

// NewGenerationService 构造。aes 必须非空（账号凭证加密强制）。
func NewGenerationService(db *gorm.DB, r *repo.GenerationRepo, pool *AccountPool, billing *BillingService, providers map[string]provider.Provider, priceFn PriceFunc, aes *crypto.AESGCM) *GenerationService {
	return &GenerationService{
		db:        db,
		repo:      r,
		pool:      pool,
		billing:   billing,
		providers: providers,
		priceFn:   priceFn,
		aes:       aes,
	}
}

// CreateRequest 创建生成请求 DTO（被 handler 填充）。
type CreateRequest struct {
	UserID       uint64
	APIKeyID     *uint64
	Kind         provider.Kind
	Mode         provider.Mode
	ModelCode    string
	Provider     string
	Prompt       string
	NegPrompt    string
	Params       map[string]any
	RefAssets    []string
	Count        int
	IdemKey      string
	ClientIP     string
}

// Create 同步创建 + 触发任务。返回最终 task。
func (s *GenerationService) Create(ctx context.Context, req CreateRequest) (*model.GenerationTask, error) {
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.IdemKey == "" {
		req.IdemKey = uuid.NewString()
	}

	if existing, err := s.repo.GetByIdem(ctx, req.UserID, req.IdemKey); err == nil && existing != nil {
		return existing, nil
	}

	cost := int64(0)
	if s.priceFn != nil {
		cost = s.priceFn(req.ModelCode, req.Kind, req.Params) * int64(req.Count)
	}
	if cost <= 0 {
		return nil, errcode.InvalidParam.WithMsg("model price not configured")
	}

	taskID := newULID()
	paramsJSON, _ := json.Marshal(req.Params)
	var refJSON *string
	if len(req.RefAssets) > 0 {
		b, _ := json.Marshal(req.RefAssets)
		s := string(b)
		refJSON = &s
	}
	t := &model.GenerationTask{
		TaskID:     taskID,
		UserID:     req.UserID,
		Kind:       string(req.Kind),
		Mode:       string(req.Mode),
		ModelCode:  req.ModelCode,
		Prompt:     req.Prompt,
		Params:     string(paramsJSON),
		RefAssets:  refJSON,
		Count:      req.Count,
		CostPoints: cost,
		IdemKey:    req.IdemKey,
		Provider:   req.Provider,
		Status:     model.GenStatusPending,
		FromAPIKeyID: req.APIKeyID,
	}
	if req.NegPrompt != "" {
		ng := req.NegPrompt
		t.NegPrompt = &ng
	}
	if req.ClientIP != "" {
		ip := req.ClientIP
		t.ClientIP = &ip
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}

	if err := s.billing.PreDeduct(ctx, PreDeductReq{
		UserID:     req.UserID,
		TaskID:     taskID,
		Kind:       string(req.Kind),
		ModelCode:  req.ModelCode,
		Count:      req.Count,
		UnitPoints: cost / int64(req.Count),
	}); err != nil {
		_ = s.repo.SetFailed(ctx, taskID, err.Error())
		return nil, err
	}

	go s.runTask(context.Background(), t)
	return t, nil
}

// runTask 后台执行：取池中账号 → 调 provider → 结算 / 退款。
func (s *GenerationService) runTask(ctx context.Context, t *model.GenerationTask) {
	log := logger.L().With(zap.String("task", t.TaskID))

	prov, ok := s.providers[t.Provider]
	if !ok {
		s.failTask(ctx, t, "provider not registered: "+t.Provider)
		return
	}

	acc, err := s.pool.Pick(ctx, t.Provider, "round_robin")
	if err != nil {
		s.failTask(ctx, t, fmt.Sprintf("pick account: %v", err))
		return
	}
	if err := s.repo.SetRunning(ctx, t.TaskID, acc.ID); err != nil {
		log.Warn("set running failed", zap.Error(err))
	}

	var params map[string]any
	_ = json.Unmarshal([]byte(t.Params), &params)
	var refs []string
	if t.RefAssets != nil {
		_ = json.Unmarshal([]byte(*t.RefAssets), &refs)
	}

	provReq := &provider.Request{
		TaskID:    t.TaskID,
		Kind:      provider.Kind(t.Kind),
		Mode:      provider.Mode(t.Mode),
		ModelCode: t.ModelCode,
		Prompt:    t.Prompt,
		Params:    params,
		RefAssets: refs,
		Count:     t.Count,
		Account:   acc,
	}
	if t.NegPrompt != nil {
		provReq.NegPrompt = *t.NegPrompt
	}
	if acc.BaseURL != nil {
		provReq.BaseURL = *acc.BaseURL
	}
	if s.aes != nil && len(acc.CredentialEnc) > 0 {
		plain, derr := s.aes.Decrypt(acc.CredentialEnc)
		if derr != nil {
			s.pool.MarkFailed(ctx, acc.ID, "decrypt credential: "+derr.Error(), 30*time.Minute)
			s.failTask(ctx, t, "decrypt credential failed")
			return
		}
		provReq.Credential = string(plain)
	}

	timeout := 5 * time.Minute
	if t.Kind == "video" {
		timeout = 15 * time.Minute
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := prov.Generate(rctx, provReq)
	if err != nil {
		s.pool.MarkFailed(ctx, acc.ID, err.Error(), 5*time.Minute)
		s.failTask(ctx, t, fmt.Sprintf("provider call: %v", err))
		return
	}
	s.pool.MarkUsed(ctx, acc.ID)

	results := make([]*model.GenerationResult, 0, len(res.Assets))
	for i, a := range res.Assets {
		gr := &model.GenerationResult{
			TaskID: t.TaskID,
			UserID: t.UserID,
			Kind:   t.Kind,
			Seq:    int8(i),
			URL:    a.URL,
			Width:  intPtr(a.Width),
			Height: intPtr(a.Height),
		}
		if a.ThumbURL != "" {
			s := a.ThumbURL
			gr.ThumbURL = &s
		}
		if a.DurationMs > 0 {
			d := a.DurationMs
			gr.DurationMs = &d
		}
		if a.SizeBytes > 0 {
			b := a.SizeBytes
			gr.SizeBytes = &b
		}
		if len(a.Meta) > 0 {
			b, _ := json.Marshal(a.Meta)
			s := string(b)
			gr.Meta = &s
		}
		results = append(results, gr)
	}

	if err := s.repo.SetSucceeded(ctx, t.TaskID, results); err != nil {
		log.Error("set succeeded failed", zap.Error(err))
	}
	if err := s.billing.Settle(ctx, t.TaskID, &acc.ID); err != nil {
		log.Error("settle failed", zap.Error(err))
	}
}

func (s *GenerationService) failTask(ctx context.Context, t *model.GenerationTask, reason string) {
	if err := s.repo.SetFailed(ctx, t.TaskID, reason); err != nil {
		logger.FromCtx(ctx).Warn("gen.fail.update_status", zap.Error(err))
	}
	if err := s.billing.FailRefund(ctx, t.TaskID, reason); err != nil {
		logger.FromCtx(ctx).Warn("gen.fail.refund", zap.Error(err))
	}
}

// === helpers ===

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// newULID 生成一个 26 字符 ULID（Crockford base32 简化版）。
//
// 用 UUID 转 hex 后截 26 位（在严格 ULID 库引入前的过渡方案）。
func newULID() string {
	id := uuid.NewString()
	clean := ""
	for i := 0; i < len(id); i++ {
		ch := id[i]
		if ch == '-' {
			continue
		}
		clean += string(ch)
		if len(clean) == 26 {
			break
		}
	}
	return clean
}

var _ = errors.New
