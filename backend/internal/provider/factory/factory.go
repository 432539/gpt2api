// Package factory 根据环境变量选择 真实 / mock provider。
//
// env：
//   KLEIN_PROVIDER_GPT  = "real" | "mock"   (默认 mock)
//   KLEIN_PROVIDER_GROK = "real" | "mock"   (默认 mock)
//   KLEIN_GPT_BASE_URL  = 默认 base url（账号未配置 base_url 时使用）
//   KLEIN_GROK_BASE_URL = 默认 base url
//
// 这样可以做：开发期 mock，生产期 real，无需改代码。
package factory

import (
	"os"
	"strings"

	"github.com/kleinai/backend/internal/provider"
	"github.com/kleinai/backend/internal/provider/gpt"
	"github.com/kleinai/backend/internal/provider/grok"
	"github.com/kleinai/backend/internal/provider/mock"
)

// Build 根据环境变量构造 provider 集。
func Build() map[string]provider.Provider {
	return map[string]provider.Provider{
		"gpt":  buildGPT(),
		"grok": buildGrok(),
	}
}

func buildGPT() provider.Provider {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("KLEIN_PROVIDER_GPT")))
	switch mode {
	case "real", "live", "prod":
		return gpt.New(strings.TrimSpace(os.Getenv("KLEIN_GPT_BASE_URL")))
	default:
		return mock.New("gpt")
	}
}

func buildGrok() provider.Provider {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("KLEIN_PROVIDER_GROK")))
	switch mode {
	case "real", "live", "prod":
		return grok.New(strings.TrimSpace(os.Getenv("KLEIN_GROK_BASE_URL")))
	default:
		return mock.New("grok")
	}
}
