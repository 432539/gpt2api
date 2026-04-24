package image

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// ImageProxyTTL 单条签名 URL 的默认有效期。
const ImageProxyTTL = 24 * time.Hour

var imageProxySecret []byte

func init() {
	imageProxySecret = make([]byte, 32)
	if _, err := rand.Read(imageProxySecret); err != nil {
		for i := range imageProxySecret {
			imageProxySecret[i] = byte(i*31 + 7)
		}
	}
}

// BuildImageProxyURL 生成原图代理 URL。返回绝对 path(不含 host)。
func BuildImageProxyURL(taskID string, idx int, ttl time.Duration) string {
	return buildImageProxyURL(taskID, idx, ttl, ImageVariantOriginal)
}

// BuildImageThumbURL 生成缩略图代理 URL。返回绝对 path(不含 host)。
func BuildImageThumbURL(taskID string, idx int, ttl time.Duration) string {
	return buildImageProxyURL(taskID, idx, ttl, ImageVariantThumb)
}

func buildImageProxyURL(taskID string, idx int, ttl time.Duration, variant string) string {
	if ttl <= 0 {
		ttl = ImageProxyTTL
	}
	variant = NormalizeImageVariant(variant)
	expMs := time.Now().Add(ttl).UnixMilli()
	sig := computeImageProxySig(taskID, idx, expMs, variant)
	if variant == ImageVariantThumb {
		return fmt.Sprintf("/p/img/%s/%d?variant=%s&exp=%d&sig=%s", taskID, idx, variant, expMs, sig)
	}
	return fmt.Sprintf("/p/img/%s/%d?exp=%d&sig=%s", taskID, idx, expMs, sig)
}

// VerifyImageProxySig 校验图片代理 URL 的签名与过期时间。
func VerifyImageProxySig(taskID string, idx int, expMs int64, sig string, variant string) bool {
	if expMs < time.Now().UnixMilli() {
		return false
	}
	want := computeImageProxySig(taskID, idx, expMs, NormalizeImageVariant(variant))
	return hmac.Equal([]byte(sig), []byte(want))
}

func computeImageProxySig(taskID string, idx int, expMs int64, variant string) string {
	mac := hmac.New(sha256.New, imageProxySecret)
	fmt.Fprintf(mac, "%s|%d|%d|%s", taskID, idx, expMs, NormalizeImageVariant(variant))
	return hex.EncodeToString(mac.Sum(nil))[:24]
}
