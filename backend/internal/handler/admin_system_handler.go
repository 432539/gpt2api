package handler

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kleinai/backend/internal/middleware"
	"github.com/kleinai/backend/internal/service"
	"github.com/kleinai/backend/pkg/errcode"
	"github.com/kleinai/backend/pkg/response"
)

// AdminSystemHandler /admin/api/v1/system 资源 handler。
type AdminSystemHandler struct {
	svc *service.SystemConfigService
}

// NewAdminSystemHandler 构造。
func NewAdminSystemHandler(svc *service.SystemConfigService) *AdminSystemHandler {
	return &AdminSystemHandler{svc: svc}
}

// GetSettings GET /admin/api/v1/system/settings
func (h *AdminSystemHandler) GetSettings(c *gin.Context) {
	all, err := h.svc.GetAll(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, all)
}

// UpdateSettings PUT /admin/api/v1/system/settings
//
// Body: { "<key>": <any-json>, ... }
func (h *AdminSystemHandler) UpdateSettings(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	if len(body) == 0 {
		response.OK(c, gin.H{"updated": 0})
		return
	}
	uid := middleware.UID(c)
	if err := h.svc.UpsertMany(c.Request.Context(), body, uid); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"updated": len(body)})
}

// CacheStats GET /admin/api/v1/system/cache
func (h *AdminSystemHandler) CacheStats(c *gin.Context) {
	root, err := generatedCacheRoot()
	if err != nil {
		response.Fail(c, errcode.Internal.Wrap(err))
		return
	}
	files, bytes, err := walkCache(root, nil)
	if err != nil {
		response.Fail(c, errcode.Internal.Wrap(err))
		return
	}
	response.OK(c, gin.H{
		"root":  root,
		"files": files,
		"bytes": bytes,
	})
}

// CleanCache DELETE /admin/api/v1/system/cache
func (h *AdminSystemHandler) CleanCache(c *gin.Context) {
	var body struct {
		Days int  `json:"days"`
		All  bool `json:"all"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, errcode.InvalidParam.Wrap(err))
		return
	}
	if !body.All && body.Days <= 0 {
		response.Fail(c, errcode.InvalidParam.WithMsg("days must be greater than 0"))
		return
	}
	root, err := generatedCacheRoot()
	if err != nil {
		response.Fail(c, errcode.Internal.Wrap(err))
		return
	}
	cutoff := time.Now().Add(-time.Duration(body.Days) * 24 * time.Hour)
	deletedFiles, deletedBytes, err := cleanCache(root, body.All, cutoff)
	if err != nil {
		response.Fail(c, errcode.Internal.Wrap(err))
		return
	}
	_, remainBytes, _ := walkCache(root, nil)
	response.OK(c, gin.H{
		"deleted_files": deletedFiles,
		"deleted_bytes": deletedBytes,
		"remain_bytes":  remainBytes,
	})
}

func generatedCacheRoot() (string, error) {
	root := strings.TrimSpace(os.Getenv("KLEIN_STORAGE_ROOT"))
	if root == "" {
		root = "/app/storage/public"
	}
	root = filepath.Clean(root)
	cacheRoot := filepath.Clean(filepath.Join(root, "generated"))
	if root == "." || cacheRoot == "." || !strings.HasPrefix(cacheRoot, root) {
		return "", os.ErrInvalid
	}
	return cacheRoot, nil
}

func walkCache(root string, visit func(path string, info fs.FileInfo) error) (int64, int64, error) {
	var files int64
	var bytes int64
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 0, 0, nil
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		if visit != nil {
			return visit(path, info)
		}
		return nil
	})
	return files, bytes, err
}

func cleanCache(root string, all bool, cutoff time.Time) (int64, int64, error) {
	var deletedFiles int64
	var deletedBytes int64
	_, _, err := walkCache(root, func(path string, info fs.FileInfo) error {
		if !all && info.ModTime().After(cutoff) {
			return nil
		}
		size := info.Size()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		deletedFiles++
		deletedBytes += size
		return nil
	})
	if err != nil {
		return deletedFiles, deletedBytes, err
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root || !d.IsDir() {
			return nil
		}
		_ = os.Remove(path)
		return nil
	})
	return deletedFiles, deletedBytes, nil
}
