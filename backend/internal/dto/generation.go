// Package dto 生成任务 DTO。
package dto

// CreateImageReq 创建生图任务。
type CreateImageReq struct {
	ModelCode string         `json:"model"        binding:"required,max=64"`
	Prompt    string         `json:"prompt"       binding:"required,min=1,max=4000"`
	NegPrompt string         `json:"neg_prompt"   binding:"omitempty,max=4000"`
	Mode      string         `json:"mode"         binding:"omitempty,oneof=t2i i2i"`
	Count     int            `json:"count"        binding:"omitempty,min=1,max=4"`
	Ratio     string         `json:"ratio"        binding:"omitempty"`
	Quality   string         `json:"quality"      binding:"omitempty,oneof=draft standard hd"`
	RefAssets []string       `json:"ref_assets"   binding:"omitempty,dive,url"`
	Params    map[string]any `json:"params"       binding:"omitempty"`
}

// CreateVideoReq 创建生视频任务。
type CreateVideoReq struct {
	ModelCode string         `json:"model"        binding:"required,max=64"`
	Prompt    string         `json:"prompt"       binding:"required,min=1,max=4000"`
	Mode      string         `json:"mode"         binding:"omitempty,oneof=t2v i2v"`
	Duration  int            `json:"duration"     binding:"omitempty,min=2,max=60"`
	Ratio     string         `json:"ratio"        binding:"omitempty"`
	Quality   string         `json:"quality"      binding:"omitempty,oneof=draft standard hd"`
	RefAssets []string       `json:"ref_assets"   binding:"omitempty,dive,url"`
	Params    map[string]any `json:"params"       binding:"omitempty"`
}

// GenerationTaskResp 任务响应（精简）。
type GenerationTaskResp struct {
	TaskID     string                 `json:"task_id"`
	Kind       string                 `json:"kind"`
	Status     int8                   `json:"status"`
	Progress   int8                   `json:"progress"`
	ModelCode  string                 `json:"model"`
	CostPoints int64                  `json:"cost_points"`
	Error      string                 `json:"error,omitempty"`
	Results    []GenerationResultResp `json:"results,omitempty"`
	CreatedAt  int64                  `json:"created_at"`
}

// GenerationResultResp 单条结果。
type GenerationResultResp struct {
	URL        string `json:"url"`
	ThumbURL   string `json:"thumb_url,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs int    `json:"duration_ms,omitempty"`
}
