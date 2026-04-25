package image

import (
	"bytes"
	"fmt"
	stdimage "image"
	"image/jpeg"

	"golang.org/x/image/draw"
)

const (
	DefaultThumbMaxBytes = 10 * 1024
	minThumbSide         = 96
)

var (
	thumbWidths    = []int{512, 448, 384, 320, 288, 256, 224, 192, 160, 128, 96}
	thumbQualities = []int{55, 45, 35, 28, 22, 16, 10, 5}
)

// MakeThumbJPEG 把原图压缩成 JPEG 缩略图，尽量控制在 maxBytes 内。
func MakeThumbJPEG(srcBytes []byte, maxBytes int) ([]byte, string, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultThumbMaxBytes
	}
	src, _, err := stdimage.Decode(bytes.NewReader(srcBytes))
	if err != nil {
		return nil, "", fmt.Errorf("thumb decode: %w", err)
	}

	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, "", fmt.Errorf("thumb decode: invalid size %dx%d", sw, sh)
	}

	var best []byte
	for _, maxW := range thumbWidths {
		tw, th := fitThumb(sw, sh, maxW)
		dst := stdimage.NewRGBA(stdimage.Rect(0, 0, tw, th))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

		for _, q := range thumbQualities {
			buf := bytes.NewBuffer(make([]byte, 0, maxBytes))
			if err := jpeg.Encode(buf, dst, &jpeg.Options{Quality: q}); err != nil {
				return nil, "", fmt.Errorf("thumb jpeg encode: %w", err)
			}
			out := buf.Bytes()
			if len(out) <= maxBytes {
				return out, "image/jpeg", nil
			}
			if len(best) == 0 || len(out) < len(best) {
				best = append(best[:0], out...)
			}
		}
	}

	if len(best) > 0 {
		return best, "image/jpeg", nil
	}
	return nil, "", fmt.Errorf("thumb jpeg encode: no output")
}

func fitThumb(sw, sh, maxW int) (int, int) {
	if maxW < minThumbSide {
		maxW = minThumbSide
	}
	if sw <= maxW {
		if sw < minThumbSide {
			return minThumbSide, max(minThumbSide, sh*minThumbSide/max(sw, 1))
		}
		return sw, sh
	}
	th := sh * maxW / sw
	if th < minThumbSide {
		th = minThumbSide
	}
	return maxW, th
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
