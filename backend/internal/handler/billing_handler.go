// Package handler 用户端计费 handler。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/model"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// BillingHandler 用户端计费 handler。
type BillingHandler struct {
	billing *service.BillingService
	cdk     *service.CDKService
}

// NewBillingHandler 构造。
func NewBillingHandler(b *service.BillingService, cdk *service.CDKService) *BillingHandler {
	return &BillingHandler{billing: b, cdk: cdk}
}

// Logs GET /api/v1/billing/logs?page=&page_size=
func (h *BillingHandler) Logs(c *gin.Context) {
	uid := middleware.MustUID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	logs, total, err := h.billing.ListWalletLogs(c.Request.Context(), uid, page, pageSize)
	if err != nil {
		response.Fail(c, err)
		return
	}
	out := make([]*dto.WalletLogResp, 0, len(logs))
	for _, l := range logs {
		r := &dto.WalletLogResp{
			ID:           l.ID,
			Direction:    l.Direction,
			BizType:      l.BizType,
			BizID:        l.BizID,
			Points:       l.Points,
			PointsBefore: l.PointsBefore,
			PointsAfter:  l.PointsAfter,
			CreatedAt:    l.CreatedAt.Unix(),
		}
		if l.Remark != nil {
			r.Remark = *l.Remark
		}
		out = append(out, r)
	}
	response.Page(c, out, total, page, pageSize)
}

// RedeemCDK POST /api/v1/billing/cdk/redeem
func (h *BillingHandler) RedeemCDK(c *gin.Context) {
	var req dto.CDKRedeemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	uid := middleware.MustUID(c)
	pts, err := h.cdk.Redeem(c.Request.Context(), uid, req.Code)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{
		"points":  pts,
		"biz":     model.BizCDK,
		"message": "兑换成功",
	})
}
