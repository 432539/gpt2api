package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kleinai/backend/pkg/logger"
)

const HeaderRequestID = "X-Request-Id"

// RequestID 注入 / 透传 trace_id，并把带 trace_id 的 logger 放进 ctx。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Header(HeaderRequestID, id)
		c.Request.Header.Set(HeaderRequestID, id)

		ctx := logger.Inject(c.Request.Context(), zap.String("trace_id", id))
		c.Request = c.Request.WithContext(ctx)
		c.Set(HeaderRequestID, id)
		c.Next()
	}
}
