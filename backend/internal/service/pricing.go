// Package service 模型计费表（开发期内置；后续从 model 表读取并缓存）。
package service

import "github.com/kleinai/backend/internal/provider"

// DefaultPriceTable 默认计费（与 migrations/seed 对齐）。
//
// 单位：点 *100。例：400 = 4 点 / 张图。
var DefaultPriceTable = map[string]int64{
	"img-v3":     400,
	"img-real":   400,
	"img-anime":  300,
	"img-3d":     500,
	"vid-v1":     1500, // 4 秒视频
	"vid-i2v":    2000,
}

// DefaultPriceFn 实现 PriceFunc。
func DefaultPriceFn(modelCode string, kind provider.Kind, params map[string]any) int64 {
	if v, ok := DefaultPriceTable[modelCode]; ok {
		// 视频：按秒倍率
		if kind == provider.KindVideo {
			if dur, ok2 := params["duration"].(float64); ok2 && dur > 4 {
				factor := dur / 4
				return int64(float64(v) * factor)
			}
		}
		return v
	}
	switch kind {
	case provider.KindImage:
		return 400
	case provider.KindVideo:
		return 1500
	}
	return 0
}
