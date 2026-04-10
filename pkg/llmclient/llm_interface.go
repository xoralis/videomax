package llmclient

import (
	"context"

	"video-max/internal/tools"
)

// ==================== 自定义消息类型（不依赖任何第三方 SDK） ====================

// Message 对话消息，Agent 的所有历史记录均使用此结构
type Message struct {
	Role       string     `json:"role"`                  // "system", "user", "assistant", "tool"
	Content    string     `json:"content,omitempty"`      // 文本内容
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // 大模型请求调用的工具（assistant 角色时）
	ToolCallID string     `json:"tool_call_id,omitempty"` // 对应工具调用的 ID（tool 角色时）
}

// ToolCall 大模型请求调用的工具描述
type ToolCall struct {
	ID       string       `json:"id"`       // 工具调用的唯一 ID
	Type     string       `json:"type"`     // 固定为 "function"
	Function FunctionCall `json:"function"` // 函数名称和参数
}

// FunctionCall 工具函数的名称和参数
type FunctionCall struct {
	Name      string `json:"name"`      // 函数名称
	Arguments string `json:"arguments"` // JSON 格式的参数字符串
}

// Usage Token 消耗统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ==================== 请求与响应封装 ====================

// ChatRequest 一次对话请求的参数封装
// 所有 LLM 客户端实现（OpenAI、Doubao 等）共用此结构
type ChatRequest struct {
	SystemPrompt string         // 系统提示词，定义 Agent 的角色和行为规范
	UserMessage  string         // 用户消息内容
	ImagePaths   []string       // 可选：需要传入的图片本地路径（Base64 编码后发送）
	Tools        []tools.AITool // 可选：注册给大模型可用的工具列表（Function Calling）
	History      []Message      // 可选：历史对话上下文（用于 ReAct 多轮循环）
}

// ChatResponse 一次对话请求的响应封装
type ChatResponse struct {
	Content   string     // 大模型返回的文本内容
	ToolCalls []ToolCall // 大模型请求调用的工具列表（如果有）
	Usage     Usage      // Token 消耗统计
}

// ==================== LLM 客户端接口 ====================

// LLMClient 大模型客户端的统一接口约束
// 所有 Agent 只依赖此接口，不直接耦合任何具体的大模型供应商
// 新增供应商（如 Google Gemini、Anthropic Claude）时，只需实现此接口即可
type LLMClient interface {
	// Chat 发起一次大模型对话请求
	// 支持纯文本、图文混输（Vision）、以及带有 Function Calling 的请求
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Provider 返回当前客户端的供应商标识（如 "openai"、"doubao"）
	Provider() string
}

// ==================== 工厂函数 ====================

// NewLLMClient LLM 客户端工厂函数
// 根据供应商标识创建对应的 LLMClient 实现
// 支持的供应商：
//   - "openai": OpenAI 官方 API 或兼容格式的第三方接口
//   - "doubao": 字节跳动豆包大模型（火山引擎方舟 ARK 平台，使用 Responses API）
//
// 如果 provider 为空，默认使用 OpenAI
func NewLLMClient(provider string, apiKey string, baseURL string, model string) LLMClient {
	switch provider {
	case "doubao":
		return NewDoubaoClient(apiKey, baseURL, model)
	default:
		// "openai" 或空值均走 OpenAI 兼容客户端
		return NewOpenAIClient(apiKey, baseURL, model)
	}
}
