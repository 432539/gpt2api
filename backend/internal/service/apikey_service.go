// Package service API Key 业务。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/errcode"
)

// APIKeyService 用户 API Key。
type APIKeyService struct {
	repo *repo.APIKeyRepo
}

// NewAPIKeyService 构造。
func NewAPIKeyService(r *repo.APIKeyRepo) *APIKeyService { return &APIKeyService{repo: r} }

// Prefix 用户 Key 前缀；OpenAI 兼容场景下习惯使用 `sk-` 风格。
const KeyPrefix = "sk-klein-"

// Create 创建一个用户 Key。明文仅返回一次。
func (s *APIKeyService) Create(ctx context.Context, userID uint64, req *dto.APIKeyCreateReq) (*dto.APIKeyCreateResp, error) {
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "chat,image,video"
	}

	body, err := crypto.RandomString(40)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	plain := KeyPrefix + body

	salt, err := crypto.RandomString(32)
	if err != nil {
		return nil, errcode.Internal.Wrap(err)
	}
	hash := hashKey(plain, salt)
	last4 := plain[len(plain)-4:]

	k := &model.APIKey{
		UserID:     userID,
		Name:       req.Name,
		Prefix:     KeyPrefix,
		Hash:       hash,
		Salt:       salt,
		Last4:      last4,
		Scope:      scope,
		RPMLimit:   defaultIfZero(req.RPMLimit, 60),
		DailyQuota: req.DailyQuota,
		Status:     1,
	}
	if req.ExpireDays > 0 {
		exp := time.Now().UTC().Add(time.Duration(req.ExpireDays) * 24 * time.Hour)
		k.ExpireAt = &exp
	}
	if err := s.repo.Create(ctx, k); err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	return &dto.APIKeyCreateResp{
		ID:        k.ID,
		Name:      k.Name,
		Plain:     plain,
		Prefix:    k.Prefix,
		Last4:     k.Last4,
		Scope:     k.Scope,
		CreatedAt: k.CreatedAt.Unix(),
	}, nil
}

// List 列出用户 keys。
func (s *APIKeyService) List(ctx context.Context, userID uint64) ([]*dto.APIKeyResp, error) {
	items, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	out := make([]*dto.APIKeyResp, 0, len(items))
	for _, k := range items {
		r := &dto.APIKeyResp{
			ID:         k.ID,
			Name:       k.Name,
			Prefix:     k.Prefix,
			Last4:      k.Last4,
			Mask:       k.Prefix + "****" + k.Last4,
			Scope:      k.Scope,
			RPMLimit:   k.RPMLimit,
			DailyQuota: k.DailyQuota,
			Status:     k.Status,
			CreatedAt:  k.CreatedAt.Unix(),
		}
		if k.ExpireAt != nil {
			r.ExpireAt = k.ExpireAt.Unix()
		}
		if k.LastUsedAt != nil {
			r.LastUsedAt = k.LastUsedAt.Unix()
		}
		out = append(out, r)
	}
	return out, nil
}

// Toggle 启用 / 停用。
func (s *APIKeyService) Toggle(ctx context.Context, userID, id uint64, enable bool) error {
	if _, err := s.repo.GetByID(ctx, userID, id); err != nil {
		return errcode.ResourceMissing
	}
	status := int8(0)
	if enable {
		status = 1
	}
	if err := s.repo.UpdateStatus(ctx, userID, id, status); err != nil {
		return errcode.DBError.Wrap(err)
	}
	return nil
}

// Delete 删除（软删）。
func (s *APIKeyService) Delete(ctx context.Context, userID, id uint64) error {
	if _, err := s.repo.GetByID(ctx, userID, id); err != nil {
		return errcode.ResourceMissing
	}
	if err := s.repo.SoftDelete(ctx, userID, id); err != nil {
		return errcode.DBError.Wrap(err)
	}
	return nil
}

// Verify 通过明文 key 验证 → 返回 model.APIKey。
//
// 算法：
//   1. 校验前缀（拒绝明显非法的请求 Key，避免每次都查 DB）；
//   2. 暴力查 DB（hash 列有唯一索引）；为了拿 salt，需先做候选查询：
//      实际策略：用 last4 + prefix 做候选查询，再逐个 SHA256(plain+salt) 校验。
//
// 这里出于性能与简单考虑：直接基于 hash 列做查找，
// 但 hash = SHA256(plain + salt)，salt 不固定，因此真实流程必须先取候选列表。
func (s *APIKeyService) Verify(ctx context.Context, plain string) (*model.APIKey, error) {
	if !strings.HasPrefix(plain, KeyPrefix) || len(plain) < 16 {
		return nil, errcode.APIKeyInvalid
	}
	last4 := plain[len(plain)-4:]
	cands, err := s.repo.ListByLast4(ctx, last4)
	if err != nil {
		return nil, errcode.DBError.Wrap(err)
	}
	now := time.Now().UTC()
	for _, c := range cands {
		if !c.IsActive(now) {
			continue
		}
		if hashKey(plain, c.Salt) == c.Hash {
			return c, nil
		}
	}
	return nil, errcode.APIKeyInvalid
}

func hashKey(plain, salt string) string {
	h := sha256.New()
	h.Write([]byte(plain))
	h.Write([]byte(salt))
	return hex.EncodeToString(h.Sum(nil))
}

func defaultIfZero(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}
