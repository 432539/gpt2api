package model

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	mysqlDrv "github.com/go-sql-driver/mysql"

	"github.com/432539/gpt2api/internal/audit"
	"github.com/432539/gpt2api/pkg/resp"
)

// slug 正则:字母开头,可含字母/数字/点/短横/下划线,2~64 位。
var slugRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._\-]{1,63}$`)

// AdminHandler 管理员视角的模型 CRUD。
// 写路径:成功后刷 Registry + 落审计。
type AdminHandler struct {
	dao      *DAO
	registry *Registry
	auditDAO *audit.DAO
}

// NewAdminHandler registry 与 auditDAO 可为 nil(不做缓存刷新/审计)。
func NewAdminHandler(dao *DAO, registry *Registry, auditDAO *audit.DAO) *AdminHandler {
	return &AdminHandler{dao: dao, registry: registry, auditDAO: auditDAO}
}

// ---- 请求体 ----

// upsertReq 创建 / 更新共用的入参。Slug 在更新时会被忽略。
type upsertReq struct {
	Slug                string `json:"slug"`
	Type                string `json:"type"`
	Provider            string `json:"provider"`
	UpstreamModelSlug   string `json:"upstream_model_slug"`
	InputPricePer1M     int64  `json:"input_price_per_1m"`
	OutputPricePer1M    int64  `json:"output_price_per_1m"`
	CacheReadPricePer1M int64  `json:"cache_read_price_per_1m"`
	ImagePricePerCall   int64  `json:"image_price_per_call"`
	Description         string `json:"description"`
	Enabled             *bool  `json:"enabled,omitempty"`
}

func (r *upsertReq) validate(forCreate bool) error {
	r.Slug = strings.TrimSpace(r.Slug)
	r.UpstreamModelSlug = strings.TrimSpace(r.UpstreamModelSlug)
	r.Type = strings.TrimSpace(strings.ToLower(r.Type))
	r.Provider = strings.TrimSpace(strings.ToLower(r.Provider))

	if forCreate {
		if !slugRe.MatchString(r.Slug) {
			return errors.New("slug 非法:需字母开头,2-64 位字母/数字/点/下划线/短横")
		}
	}
	if r.Type != TypeChat && r.Type != TypeImage {
		return errors.New("type 只能为 chat 或 image")
	}
	if r.Provider != ProviderChatGPT && r.Provider != ProviderMiniMax {
		return errors.New("provider 只能为空(chatgpt)或 minimax")
	}
	if r.UpstreamModelSlug == "" {
		return errors.New("upstream_model_slug 不能为空")
	}
	for _, v := range []int64{
		r.InputPricePer1M, r.OutputPricePer1M,
		r.CacheReadPricePer1M, r.ImagePricePerCall,
	} {
		if v < 0 {
			return errors.New("价格不能为负数")
		}
	}
	if r.Type == TypeChat && r.InputPricePer1M == 0 && r.OutputPricePer1M == 0 {
		return errors.New("chat 模型至少配置 input 或 output 价格")
	}
	if r.Type == TypeImage && r.ImagePricePerCall == 0 {
		return errors.New("image 模型需要配置 image_price_per_call")
	}
	if len(r.Description) > 255 {
		return errors.New("description 超过 255 字")
	}
	return nil
}

// GET /api/admin/models
// 返回所有(含禁用)模型定义。
func (h *AdminHandler) List(c *gin.Context) {
	rows, err := h.dao.List(c.Request.Context())
	if err != nil {
		resp.Internal(c, err.Error())
		return
	}
	resp.OK(c, gin.H{"items": rows, "total": len(rows)})
}

// POST /api/admin/models
func (h *AdminHandler) Create(c *gin.Context) {
	var req upsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, err.Error())
		return
	}
	if err := req.validate(true); err != nil {
		resp.BadRequest(c, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	m := &Model{
		Slug: req.Slug, Type: req.Type,
		Provider:            req.Provider,
		UpstreamModelSlug:   req.UpstreamModelSlug,
		InputPricePer1M:     req.InputPricePer1M,
		OutputPricePer1M:    req.OutputPricePer1M,
		CacheReadPricePer1M: req.CacheReadPricePer1M,
		ImagePricePerCall:   req.ImagePricePerCall,
		Description:         req.Description,
		Enabled:             enabled,
	}
	if err := h.dao.Create(c.Request.Context(), m); err != nil {
		if isDupSlug(err) {
			resp.BadRequest(c, "slug 已存在")
			return
		}
		resp.Internal(c, err.Error())
		return
	}
	h.reloadRegistry(c)
	audit.Record(c, h.auditDAO, "models.create", strconv.FormatUint(m.ID, 10), gin.H{
		"slug": m.Slug, "type": m.Type,
	})
	resp.OK(c, m)
}

// PUT /api/admin/models/:id
func (h *AdminHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.BadRequest(c, "invalid id")
		return
	}
	var req upsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, err.Error())
		return
	}
	if err := req.validate(false); err != nil {
		resp.BadRequest(c, err.Error())
		return
	}
	cur, err := h.dao.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			resp.NotFound(c, "model not found")
			return
		}
		resp.Internal(c, err.Error())
		return
	}
	cur.Type = req.Type
	cur.Provider = req.Provider
	cur.UpstreamModelSlug = req.UpstreamModelSlug
	cur.InputPricePer1M = req.InputPricePer1M
	cur.OutputPricePer1M = req.OutputPricePer1M
	cur.CacheReadPricePer1M = req.CacheReadPricePer1M
	cur.ImagePricePerCall = req.ImagePricePerCall
	cur.Description = req.Description
	if req.Enabled != nil {
		cur.Enabled = *req.Enabled
	}
	if err := h.dao.Update(c.Request.Context(), cur); err != nil {
		resp.Internal(c, err.Error())
		return
	}
	h.reloadRegistry(c)
	audit.Record(c, h.auditDAO, "models.update", strconv.FormatUint(id, 10), gin.H{
		"slug": cur.Slug,
	})
	resp.OK(c, cur)
}

// PATCH /api/admin/models/:id/enabled  body: {"enabled":true|false}
func (h *AdminHandler) SetEnabled(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.BadRequest(c, "invalid id")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		resp.BadRequest(c, err.Error())
		return
	}
	if err := h.dao.SetEnabled(c.Request.Context(), id, body.Enabled); err != nil {
		if errors.Is(err, ErrNotFound) {
			resp.NotFound(c, "model not found")
			return
		}
		resp.Internal(c, err.Error())
		return
	}
	h.reloadRegistry(c)
	audit.Record(c, h.auditDAO, "models.set_enabled", strconv.FormatUint(id, 10), gin.H{
		"enabled": body.Enabled,
	})
	resp.OK(c, gin.H{"id": id, "enabled": body.Enabled})
}

// DELETE /api/admin/models/:id
func (h *AdminHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.BadRequest(c, "invalid id")
		return
	}
	if err := h.dao.SoftDelete(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			resp.NotFound(c, "model not found")
			return
		}
		resp.Internal(c, err.Error())
		return
	}
	h.reloadRegistry(c)
	audit.Record(c, h.auditDAO, "models.delete", strconv.FormatUint(id, 10), nil)
	resp.OK(c, gin.H{"deleted": id})
}

// GET /api/me/models
// 普通用户视角,只返回 enabled 模型,用于「生成面板」下拉选择。
func (h *AdminHandler) ListEnabledForMe(c *gin.Context) {
	rows, err := h.dao.ListEnabled(c.Request.Context())
	if err != nil {
		resp.Internal(c, err.Error())
		return
	}
	type simple struct {
		ID          uint64 `json:"id"`
		Slug        string `json:"slug"`
		Type        string `json:"type"`
		Description string `json:"description"`
	}
	out := make([]simple, 0, len(rows))
	for _, m := range rows {
		out = append(out, simple{
			ID: m.ID, Slug: m.Slug, Type: m.Type, Description: m.Description,
		})
	}
	resp.OK(c, gin.H{"items": out, "total": len(out)})
}

// ---- 内部 ----

func (h *AdminHandler) reloadRegistry(c *gin.Context) {
	if h.registry == nil {
		return
	}
	_ = h.registry.Reload(c.Request.Context())
}

// isDupSlug 判定 MySQL 1062 Duplicate entry。
func isDupSlug(err error) bool {
	var me *mysqlDrv.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
