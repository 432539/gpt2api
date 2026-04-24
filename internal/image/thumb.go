package image

import (
	"bytes"
	"fmt"
	gimage "image"
	"image/jpeg"
	"image/png"

	_ "image/gif"
	_ "image/jpeg"

	"golang.org/x/image/draw"

	_ "golang.org/x/image/webp"
)

const (
	ImageVariantOriginal = ""
	ImageVariantThumb    = "thumb"

	ThumbMaxEdge = 320
)

func NormalizeImageVariant(s string) string {
	switch s {
	case ImageVariantThumb:
		return ImageVariantThumb
	default:
		return ImageVariantOriginal
	}
}

func DoThumbnail(src []byte) ([]byte, string, error) {
	if len(src) == 0 {
		return nil, "", fmt.Errorf("image thumb: empty source")
	}
	srcImg, _, err := gimage.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", fmt.Errorf("image thumb: decode: %w", err)
	}
	b := srcImg.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, "", fmt.Errorf("image thumb: invalid bounds")
	}

	dw, dh := thumbSize(sw, sh, ThumbMaxEdge)
	dst := gimage.NewRGBA(gimage.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), srcImg, b, draw.Src, nil)

	var buf bytes.Buffer
	if opaque, ok := any(dst).(interface{ Opaque() bool }); !ok || opaque.Opaque() {
		enc := jpeg.Options{Quality: 82}
		if err := jpeg.Encode(&buf, dst, &enc); err != nil {
			return nil, "", fmt.Errorf("image thumb: jpeg encode: %w", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	}

	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, "", fmt.Errorf("image thumb: png encode: %w", err)
	}
	return buf.Bytes(), "image/png", nil
}

func thumbSize(sw, sh, maxEdge int) (int, int) {
	if maxEdge <= 0 {
		maxEdge = ThumbMaxEdge
	}
	if sw <= 0 {
		sw = 1
	}
	if sh <= 0 {
		sh = 1
	}
	long := sw
	if sh > long {
		long = sh
	}
	if long <= maxEdge {
		return sw, sh
	}
	if sw >= sh {
		dw := maxEdge
		dh := int(float64(sh) * float64(maxEdge) / float64(sw))
		if dh < 1 {
			dh = 1
		}
		return dw, dh
	}
	dh := maxEdge
	dw := int(float64(sw) * float64(maxEdge) / float64(sh))
	if dw < 1 {
		dw = 1
	}
	return dw, dh
}
