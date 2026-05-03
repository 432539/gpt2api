// Package service 模型计费表（开发期内置；后续从 model 表读取并缓存）。
package service

import (
	"context"
	"encoding/json"

	"github.com/kleinai/backend/internal/provider"
)

// DefaultPriceTable 默认计费（与 migrations/seed 对齐）。
//
// 单位：点 *100。例：400 = 4 点 / 张图。
var DefaultPriceTable = map[string]int64{
	"gpt-4o-mini":        100,
	"gpt-image-2":        0,
	"vid-v1":             1500, // 4 秒视频
	"vid-i2v":            2000,
	"grok-imagine-video": 2000,
}

// ChatPrice is points*100 per 1K tokens.
type ChatPrice struct {
	InputPerK  int64
	OutputPerK int64
}

// DefaultChatPriceFn returns default token prices in points*100 per 1K tokens.
func DefaultChatPriceFn(modelCode string) ChatPrice {
	switch modelCode {
	case "gpt-4o-mini":
		return ChatPrice{InputPerK: 100, OutputPerK: 300}
	case "grok-4.20-fast":
		return ChatPrice{InputPerK: 100, OutputPerK: 300}
	case "grok-4.20-auto":
		return ChatPrice{InputPerK: 150, OutputPerK: 450}
	case "grok-4.20-expert":
		return ChatPrice{InputPerK: 200, OutputPerK: 600}
	case "grok-4.20-heavy":
		return ChatPrice{InputPerK: 400, OutputPerK: 1200}
	case "grok-4.3-beta":
		return ChatPrice{InputPerK: 300, OutputPerK: 900}
	default:
		return ChatPrice{InputPerK: 100, OutputPerK: 300}
	}
}

// DefaultPriceFn 实现 PriceFunc。
func DefaultPriceFn(modelCode string, kind provider.Kind, params map[string]any) int64 {
	if v, ok := DefaultPriceTable[modelCode]; ok {
		// 视频：按秒倍率
		if kind == provider.KindVideo {
			if dur, ok2 := params["duration"].(float64); ok2 {
				dur = float64(normalizeBillingVideoDuration(int(dur)))
				if dur <= 6 {
					return v
				}
				factor := dur / 6
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

func ConfigPriceFn(cfg *SystemConfigService) PriceFunc {
	return func(modelCode string, kind provider.Kind, params map[string]any) int64 {
		if cfg != nil {
			raw := cfg.GetString(context.Background(), "billing.model_prices", "")
			if raw != "" {
				var rows []struct {
					ModelCode  string `json:"model_code"`
					UnitPoints int64  `json:"unit_points"`
					Enabled    *bool  `json:"enabled"`
				}
				if err := json.Unmarshal([]byte(raw), &rows); err == nil {
					for _, row := range rows {
						if row.ModelCode != modelCode {
							continue
						}
						if row.Enabled != nil && !*row.Enabled {
							continue
						}
						if kind == provider.KindVideo {
							if dur, ok2 := params["duration"].(float64); ok2 {
								dur = float64(normalizeBillingVideoDuration(int(dur)))
								if dur <= 6 {
									return row.UnitPoints
								}
								return int64(float64(row.UnitPoints) * (dur / 6))
							}
						}
						return row.UnitPoints
					}
				}
				var prices map[string]int64
				if err := json.Unmarshal([]byte(raw), &prices); err == nil {
					if v, ok := prices[modelCode]; ok {
						if kind == provider.KindVideo {
							if dur, ok2 := params["duration"].(float64); ok2 {
								dur = float64(normalizeBillingVideoDuration(int(dur)))
								if dur <= 6 {
									return v
								}
								return int64(float64(v) * (dur / 6))
							}
						}
						return v
					}
				}
			}
		}
		return DefaultPriceFn(modelCode, kind, params)
	}
}

func ConfigChatPriceFn(cfg *SystemConfigService) func(modelCode string) ChatPrice {
	return func(modelCode string) ChatPrice {
		def := DefaultChatPriceFn(modelCode)
		if cfg == nil {
			return def
		}
		raw := cfg.GetString(context.Background(), "billing.model_prices", "")
		if raw == "" {
			return def
		}
		var rows []struct {
			ModelCode        string `json:"model_code"`
			Kind             string `json:"kind"`
			UnitPoints       *int64 `json:"unit_points"`
			InputUnitPoints  *int64 `json:"input_unit_points"`
			OutputUnitPoints *int64 `json:"output_unit_points"`
			Enabled          *bool  `json:"enabled"`
		}
		if err := json.Unmarshal([]byte(raw), &rows); err == nil {
			for _, row := range rows {
				if row.ModelCode != modelCode {
					continue
				}
				if row.Enabled != nil && !*row.Enabled {
					continue
				}
				if row.InputUnitPoints != nil || row.OutputUnitPoints != nil {
					if row.InputUnitPoints != nil {
						def.InputPerK = *row.InputUnitPoints
					}
					if row.OutputUnitPoints != nil {
						def.OutputPerK = *row.OutputUnitPoints
					}
					return def
				}
				if row.Kind == "text" && row.UnitPoints != nil {
					return ChatPrice{InputPerK: *row.UnitPoints, OutputPerK: *row.UnitPoints}
				}
			}
		}
		return def
	}
}

func ChatCost(price ChatPrice, promptTokens, completionTokens int) int64 {
	if price.InputPerK <= 0 && price.OutputPerK <= 0 {
		return 0
	}
	in := (int64(promptTokens)*price.InputPerK + 999) / 1000
	out := (int64(completionTokens)*price.OutputPerK + 999) / 1000
	total := in + out
	if total <= 0 {
		return 1
	}
	return total
}

func normalizeBillingVideoDuration(sec int) int {
	for _, v := range []int{6, 10} {
		if sec <= v {
			return v
		}
	}
	return 10
}
