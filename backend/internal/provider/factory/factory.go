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

const (
	astraflowBaseURL   = "https://api-us-ca.umodelverse.ai/v1"
	astraflowCNBaseURL = "https://api.modelverse.cn/v1"
)

// Build 根据环境变量构造 provider 集。
func Build() map[string]provider.Provider {
	return map[string]provider.Provider{
		"gpt":           buildGPT(),
		"grok":          buildGrok(),
		"astraflow":     buildAstraflow(),
		"astraflow_cn":  buildAstraflowCN(),
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
// buildAstraflow 构造 Astraflow 全球端点 provider（OpenAI 兼容，复用 gpt 包）。
// 环境变量：KLEIN_PROVIDER_ASTRAFLOW=real|mock  ASTRAFLOW_API_KEY=sk-...
func buildAstraflow() provider.Provider {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("KLEIN_PROVIDER_ASTRAFLOW")))
	switch mode {
	case "real", "live", "prod":
		base := strings.TrimSpace(os.Getenv("KLEIN_ASTRAFLOW_BASE_URL"))
		if base == "" {
			base = astraflowBaseURL
		}
		return gpt.New(base)
	default:
		return mock.New("astraflow")
	}
}

// buildAstraflowCN 构造 Astraflow 中国端点 provider（OpenAI 兼容，复用 gpt 包）。
// 环境变量：KLEIN_PROVIDER_ASTRAFLOW_CN=real|mock  ASTRAFLOW_CN_API_KEY=sk-...
func buildAstraflowCN() provider.Provider {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("KLEIN_PROVIDER_ASTRAFLOW_CN")))
	switch mode {
	case "real", "live", "prod":
		base := strings.TrimSpace(os.Getenv("KLEIN_ASTRAFLOW_CN_BASE_URL"))
		if base == "" {
			base = astraflowCNBaseURL
		}
		return gpt.New(base)
	default:
		return mock.New("astraflow_cn")
	}
}
