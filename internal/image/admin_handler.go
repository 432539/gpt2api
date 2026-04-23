package image

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/432539/gpt2api/pkg/resp"
)

// AdminHandler 管理员视角下的生成记录接口。
type AdminHandler struct {
	dao *DAO
}

// NewAdminHandler 构造。
func NewAdminHandler(dao *DAO) *AdminHandler {
	return &AdminHandler{dao: dao}
}

// List GET /api/admin/image-tasks
// 查询参数:page / page_size / user_id / keyword(prompt 或邮箱模糊) / status
func (h *AdminHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)

	f := AdminTaskFilter{
		UserID:  userID,
		Keyword: c.Query("keyword"),
		Status:  c.Query("status"),
	}

	rows, total, err := h.dao.ListAdmin(c.Request.Context(), f, size, (page-1)*size)
	if err != nil {
		resp.Internal(c, err.Error())
		return
	}

	// 把 result_urls JSON bytes 解成可读字符串数组后输出
	type rowOut struct {
		AdminTaskRow
		ResultURLsParsed []string `json:"result_urls_parsed"`
	}
	out := make([]rowOut, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowOut{
			AdminTaskRow:     r,
			ResultURLsParsed: r.DisplayImageURLs(ImageProxyTTL),
		})
	}

	resp.OK(c, gin.H{
		"list":      out,
		"total":     total,
		"page":      page,
		"page_size": size,
	})
}
