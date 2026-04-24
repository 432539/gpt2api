package gateway

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/432539/gpt2api/internal/apikey"
	"github.com/432539/gpt2api/internal/billing"
	"github.com/432539/gpt2api/internal/image"
	modelpkg "github.com/432539/gpt2api/internal/model"
	"github.com/432539/gpt2api/internal/usage"
	"github.com/432539/gpt2api/pkg/logger"
)

const imageTaskWriteTimeout = 5 * time.Second

type imageAsyncRequest struct {
	TaskID        string
	RefID         string
	APIKey        *apikey.APIKey
	Model         *modelpkg.Model
	Prompt        string
	N             int
	Ratio         float64
	MaxAttempts   int
	References    []image.ReferenceImage
	EstimatedCost int64
	BillingAction string
	StartAt       time.Time
	ClientIP      string
	UserAgent     string
}

type imageAsyncResult struct {
	Response    ImageGenResponse
	HTTPStatus  int
	ErrorCode   string
	ErrorDetail string
}

func imageShouldWait(wait *bool) bool {
	return wait == nil || *wait
}

func imageTaskTimeout(refs []image.ReferenceImage) time.Duration {
	if len(refs) > 0 {
		return 8 * time.Minute
	}
	return 7 * time.Minute
}

func imageWriteContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), imageTaskWriteTimeout)
}

func imageHTTPStatusByCode(code string) int {
	switch code {
	case image.ErrNoAccount, image.ErrRateLimited:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

func buildImageResponse(taskID string, res *image.RunResult) ImageGenResponse {
	out := ImageGenResponse{
		Created: time.Now().Unix(),
		TaskID:  taskID,
		Data:    make([]ImageGenData, 0, len(res.SignedURLs)),
	}
	for i := range res.SignedURLs {
		d := ImageGenData{URL: image.BuildImageProxyURL(taskID, i, image.ImageProxyTTL)}
		if i < len(res.FileIDs) {
			d.FileID = strings.TrimPrefix(res.FileIDs[i], "sed:")
		}
		out.Data = append(out.Data, d)
	}
	return out
}

func (h *ImagesHandler) runImageTaskAsync(req imageAsyncRequest) <-chan imageAsyncResult {
	ch := make(chan imageAsyncResult, 1)
	go func() {
		defer close(ch)
		ch <- h.executeImageTask(req)
	}()
	return ch
}

func (h *ImagesHandler) executeImageTask(req imageAsyncRequest) imageAsyncResult {
	rec := &usage.Log{
		UserID:    req.APIKey.UserID,
		KeyID:     req.APIKey.ID,
		RequestID: req.RefID,
		Type:      usage.TypeImage,
		ModelID:   req.Model.ID,
		IP:        req.ClientIP,
		UA:        req.UserAgent,
	}
	defer func() {
		rec.DurationMs = int(time.Since(req.StartAt).Milliseconds())
		if rec.Status == "" {
			rec.Status = usage.StatusFailed
		}
		if h.Usage != nil {
			h.Usage.Write(rec)
		}
	}()

	runCtx, cancel := context.WithTimeout(context.Background(), imageTaskTimeout(req.References))
	defer cancel()

	res := h.Runner.Run(runCtx, image.RunOptions{
		TaskID:        req.TaskID,
		UserID:        req.APIKey.UserID,
		KeyID:         req.APIKey.ID,
		ModelID:       req.Model.ID,
		UpstreamModel: req.Model.UpstreamModelSlug,
		Prompt:        maybeAppendClaritySuffix(req.Prompt),
		N:             req.N,
		MaxAttempts:   req.MaxAttempts,
		References:    req.References,
	})
	rec.AccountID = res.AccountID

	if res.Status != image.StatusSuccess {
		rec.Status = usage.StatusFailed
		rec.ErrorCode = ifEmpty(res.ErrorCode, "upstream_error")
		if req.EstimatedCost > 0 {
			ctx, cancel := imageWriteContext()
			if err := h.Billing.Refund(ctx, req.APIKey.UserID, req.APIKey.ID, req.EstimatedCost, req.RefID, req.BillingAction+" refund"); err != nil {
				logger.L().Error("image refund failed",
					zap.String("task_id", req.TaskID),
					zap.String("ref", req.RefID),
					zap.Error(err))
			}
			cancel()
		}
		return imageAsyncResult{
			HTTPStatus:  imageHTTPStatusByCode(res.ErrorCode),
			ErrorCode:   ifEmpty(res.ErrorCode, "upstream_error"),
			ErrorDetail: localizeImageErr(res.ErrorCode, res.ErrorMessage),
		}
	}

	actualCost := billing.ComputeImageCost(req.Model, len(res.SignedURLs), req.Ratio)
	if req.EstimatedCost > 0 {
		ctx, cancel := imageWriteContext()
		if err := h.Billing.Settle(ctx, req.APIKey.UserID, req.APIKey.ID, req.EstimatedCost, actualCost, req.RefID, req.BillingAction+" settle"); err != nil {
			logger.L().Error("image settle failed",
				zap.String("task_id", req.TaskID),
				zap.String("ref", req.RefID),
				zap.Error(err))
		}
		cancel()
	}

	if h.Keys != nil {
		ctx, cancel := imageWriteContext()
		if err := h.Keys.DAO().TouchUsage(ctx, req.APIKey.ID, req.ClientIP, actualCost); err != nil {
			logger.L().Warn("image touch usage failed",
				zap.String("task_id", req.TaskID),
				zap.Uint64("key_id", req.APIKey.ID),
				zap.Error(err))
		}
		cancel()
	}

	if h.DAO != nil {
		ctx, cancel := imageWriteContext()
		if err := h.DAO.UpdateCost(ctx, req.TaskID, actualCost); err != nil {
			logger.L().Warn("image update cost failed",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
		}
		cancel()
	}

	rec.Status = usage.StatusSuccess
	rec.CreditCost = actualCost
	rec.ImageCount = len(res.SignedURLs)
	return imageAsyncResult{Response: buildImageResponse(req.TaskID, res)}
}
