package image

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ErrLocalImageNotFound 表示本地图片仓库里还没有对应文件。
var ErrLocalImageNotFound = errors.New("image local store: file not found")

// LocalStore 负责把生成图片以 task_id + idx 的形式落到本地磁盘。
// 文件名固定为 `<idx>.bin`,内容保留远端返回的原始字节,Content-Type 在读取时嗅探。
type LocalStore struct {
	dir string
}

// NewLocalStore 构造本地图片仓库。dir 为空时回退到 data/images。
func NewLocalStore(dir string) *LocalStore {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = filepath.Join("data", "images")
	}
	return &LocalStore{dir: dir}
}

// Dir 返回根目录。
func (s *LocalStore) Dir() string { return s.dir }

// Save 把单张图片写入本地。
func (s *LocalStore) Save(taskID string, idx int, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty image data")
	}
	path, err := s.filePath(taskID, idx, ImageVariantOriginal)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return "", err
	}
	_ = s.saveThumb(taskID, idx, data)
	return path, nil
}

// Load 读取单张图片。返回 (bytes, contentType)。
func (s *LocalStore) Load(taskID string, idx int) ([]byte, string, error) {
	return s.LoadVariant(taskID, idx, ImageVariantOriginal)
}

// LoadVariant 读取指定变体。thumb 不存在时会尝试从原图即时生成并回写本地缓存。
func (s *LocalStore) LoadVariant(taskID string, idx int, variant string) ([]byte, string, error) {
	variant = NormalizeImageVariant(variant)
	path, err := s.filePath(taskID, idx, variant)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if variant == ImageVariantThumb {
				return s.buildThumbFromOriginal(taskID, idx)
			}
			return nil, "", ErrLocalImageNotFound
		}
		return nil, "", err
	}
	return data, detectContentType(data), nil
}

func (s *LocalStore) saveThumb(taskID string, idx int, original []byte) error {
	thumb, _, err := DoThumbnail(original)
	if err != nil {
		return err
	}
	path, err := s.filePath(taskID, idx, ImageVariantThumb)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, thumb)
}

func (s *LocalStore) buildThumbFromOriginal(taskID string, idx int) ([]byte, string, error) {
	origPath, err := s.filePath(taskID, idx, ImageVariantOriginal)
	if err != nil {
		return nil, "", err
	}
	original, err := os.ReadFile(origPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrLocalImageNotFound
		}
		return nil, "", err
	}
	thumb, ct, err := DoThumbnail(original)
	if err != nil {
		return nil, "", err
	}
	path, err := s.filePath(taskID, idx, ImageVariantThumb)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", err
	}
	if err := writeFileAtomic(path, thumb); err != nil {
		return nil, "", err
	}
	return thumb, ct, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func detectContentType(data []byte) string {
	ct := "application/octet-stream"
	if len(data) == 0 {
		return ct
	}
	n := len(data)
	if n > 512 {
		n = 512
	}
	return http.DetectContentType(data[:n])
}

func (s *LocalStore) filePath(taskID string, idx int, variant string) (string, error) {
	if idx < 0 {
		return "", fmt.Errorf("invalid image index: %d", idx)
	}
	if err := validateTaskID(taskID); err != nil {
		return "", err
	}
	variant = NormalizeImageVariant(variant)
	if variant == ImageVariantThumb {
		return filepath.Join(s.dir, taskID, fmt.Sprintf("%d.thumb.bin", idx)), nil
	}
	return filepath.Join(s.dir, taskID, fmt.Sprintf("%d.bin", idx)), nil
}

func validateTaskID(taskID string) error {
	if taskID == "" {
		return errors.New("empty task id")
	}
	for _, r := range taskID {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("invalid task id: %q", taskID)
		}
	}
	return nil
}
