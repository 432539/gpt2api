// Package response 统一 HTTP 响应。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/logger"
)

// Body 统一响应结构。
type Body struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    any    `json:"data,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

// PageData 分页响应通用结构。
type PageData struct {
	List     any   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

const traceHeader = "X-Request-Id"

// OK 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{
		Code:    0,
		Msg:     "ok",
		Data:    data,
		TraceID: c.GetHeader(traceHeader),
	})
}

// Page 分页响应。
func Page(c *gin.Context, list any, total int64, page, pageSize int) {
	OK(c, PageData{List: list, Total: total, Page: page, PageSize: pageSize})
}

// Fail 失败响应：根据 errcode 决定 HTTP / code / msg。
// 注意：HTTP 401 / 403 / 429 / 5xx 用真实 HTTP 状态码；其余业务错误统一 200 + 业务码。
func Fail(c *gin.Context, err error) {
	if err == nil {
		OK(c, nil)
		return
	}

	be, ok := errcode.As(err)
	if !ok {
		be = errcode.Internal.Wrap(err)
	}

	logger.FromCtx(c.Request.Context()).Warn("response.fail",
		zap.Int("code", be.Code),
		zap.String("msg", be.Msg),
		zap.Error(be.Unwrap()),
	)

	httpCode := http.StatusOK
	switch {
	case be.HTTPStatus() >= 500, be.HTTPStatus() == 401, be.HTTPStatus() == 403, be.HTTPStatus() == 429, be.HTTPStatus() == 413:
		httpCode = be.HTTPStatus()
	}

	c.AbortWithStatusJSON(httpCode, Body{
		Code:    be.Code,
		Msg:     be.Msg,
		TraceID: c.GetHeader(traceHeader),
	})
}
