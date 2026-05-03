package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/dto"
	"github.com/kleinai/backend/internal/repo"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

type AdminBillingHandler struct {
	wallet *repo.WalletRepo
}

func NewAdminBillingHandler(wallet *repo.WalletRepo) *AdminBillingHandler {
	return &AdminBillingHandler{wallet: wallet}
}

func (h *AdminBillingHandler) WalletLogs(c *gin.Context) {
	var req dto.AdminWalletLogListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	rows, total, err := h.wallet.ListAdminLogs(c.Request.Context(), repo.AdminWalletLogFilter{
		Keyword:   req.Keyword,
		UserID:    req.UserID,
		BizType:   req.BizType,
		Direction: req.Direction,
		Page:      req.Page,
		PageSize:  req.PageSize,
	})
	if err != nil {
		response.Fail(c, errcode.DBError.Wrap(err))
		return
	}
	out := make([]*dto.AdminWalletLogResp, 0, len(rows))
	for _, row := range rows {
		item := &dto.AdminWalletLogResp{
			ID:           row.ID,
			CreatedAt:    row.CreatedAt.Unix(),
			UserID:       row.UserID,
			UserLabel:    row.UserLabel,
			Direction:    row.Direction,
			BizType:      row.BizType,
			BizID:        row.BizID,
			Points:       row.Points,
			PointsBefore: row.PointsBefore,
			PointsAfter:  row.PointsAfter,
		}
		if row.Remark != nil {
			item.Remark = *row.Remark
		}
		out = append(out, item)
	}
	page, pageSize := req.Page, req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	response.Page(c, out, total, page, pageSize)
}
