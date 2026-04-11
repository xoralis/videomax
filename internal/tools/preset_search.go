package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// PresetSearchTool 最佳实践查询工具
// 该工具在 ReAct 循环中被 Visual Agent 调用
// 当大模型不确定某个视频供应商的参数规范时（如分辨率要求、时长限制等），
// 它会通过 Thought 判断需要查阅规则，然后 Act 调用此工具获取信息
type PresetSearchTool struct{}

// presetSearchParams 工具入参的 Go 结构体映射
type presetSearchParams struct {
	Provider string `json:"provider"` // 视频供应商名称，如 bytedance, kling
	Category string `json:"category"` // 查询类别，如 resolution, duration, style
}

// providerPresets 各供应商的预设最佳实践规则库（硬编码版本，后续可替换为数据库查询）
var providerPresets = map[string]map[string]string{
	"bytedance": {
		"resolution":    "推荐分辨率: 1920x1080 (16:9) 或 1080x1920 (9:16)。最大支持 4K。",
		"duration":      "支持视频时长: 5秒 或 10秒。推荐使用 5 秒以获得最佳质量。",
		"style":         "支持的风格关键词: cinematic, anime, realistic, watercolor, cyberpunk, fantasy。",
		"camera":        "推荐运镜关键词: tracking shot, dolly zoom, pan left/right, tilt up/down, static, aerial view。",
		"best_practice": "提示词建议: 1.主体描述放在最前面 2.运镜指令紧跟其后 3.风格和光影放最后 4.避免否定词。",
	},
	"kling": {
		"resolution":    "推荐分辨率: 1280x720 或 1920x1080。",
		"duration":      "支持视频时长: 5秒、10秒。专业版支持延长至 30 秒。",
		"style":         "支持的风格关键词: realistic, cartoon, oil painting, 3d render。",
		"camera":        "推荐运镜关键词: push in, pull out, orbit, static, handheld。",
		"best_practice": "提示词建议: 1.使用中文效果更佳 2.画面主体+动作+环境三段式结构 3.添加 'high quality, 4K' 等质量词。",
	},
}

func (t *PresetSearchTool) Name() string {
	return "search_best_practices"
}

func (t *PresetSearchTool) Description() string {
	return "查询指定视频生成供应商的最佳实践规则，包括推荐分辨率、支持时长、风格关键词、运镜指令等。当你不确定某个平台的参数规范时，调用此工具获取权威信息。"
}

func (t *PresetSearchTool) ParametersSchema() string {
	return `{
		"type": "object",
		"properties": {
			"provider": {
				"type": "string",
				"description": "视频供应商标识，如 bytedance, kling"
			},
			"category": {
				"type": "string",
				"description": "查询类别: resolution, duration, style, camera, best_practice",
				"enum": ["resolution", "duration", "style", "camera", "best_practice"]
			}
		},
		"required": ["provider", "category"]
	}`
}

// Execute 执行查询操作，返回对应供应商和类别的最佳实践文本
func (t *PresetSearchTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var params presetSearchParams
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return "", fmt.Errorf("解析工具参数失败: %w", err)
	}

	// 查找供应商规则
	categories, ok := providerPresets[params.Provider]
	if !ok {
		return fmt.Sprintf("未找到供应商 '%s' 的预设规则。当前支持的供应商: bytedance, kling", params.Provider), nil
	}

	// 查找具体类别
	result, ok := categories[params.Category]
	if !ok {
		return fmt.Sprintf("供应商 '%s' 下未找到类别 '%s'。可用类别: resolution, duration, style, camera, best_practice", params.Provider, params.Category), nil
	}

	return result, nil
}
