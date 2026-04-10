package llmclient

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"

	"video-max/pkg/logger"
)

// 字节豆包（Doubao）火山引擎方舟（ARK）平台的默认配置
const (
	// doubaoDefaultBaseURL ARK 平台 API 默认地址
	// 所有豆包大模型的 Chat Completions 接口均通过此域名访问
	doubaoDefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"

	// doubaoDefaultModel 豆包默认模型接入点
	// 用户需要在火山引擎控制台创建「推理接入点」后获取实际的 endpoint ID
	// 格式通常为 ep-xxxxxxxxxxxx-xxxxx
	doubaoDefaultModel = "ep-xxxxx-your-endpoint-id"
)

// DoubaoClient 字节跳动豆包大模型的 LLMClient 实现
// 基于火山引擎方舟平台（Volcengine ARK），API 格式与 OpenAI 高度兼容
// 但有以下差异点需要注意：
//   - BaseURL 固定为 https://ark.cn-beijing.volces.com/api/v3
//   - Model 字段填的是「推理接入点 ID」(ep-xxx)，不是模型名称
//   - API Key 在火山引擎控制台的「API 密钥管理」中获取
//   - 支持 Vision（图文混输）和 Function Calling，但部分模型可能不支持
type DoubaoClient struct {
	client *openai.Client
	model  string // 推理接入点 ID，如 ep-20241024100000-xxxxx
}

// NewDoubaoClient 创建豆包大模型客户端实例
//
// 参数说明：
//   - apiKey: 火山引擎 ARK 平台的 API Key（在控制台「API 密钥管理」中创建）
//   - baseURL: 留空则使用默认的 ARK 平台地址
//   - model: 推理接入点 ID（ep-xxx 格式），在控制台「模型推理」→「推理接入点」中创建
//
// 使用示例 config.yaml:
//
//	llm:
//	  provider: "doubao"
//	  api_key: "your-ark-api-key"
//	  base_url: ""                          # 留空使用默认
//	  model: "ep-20241024100000-xxxxx"      # 接入点 ID
func NewDoubaoClient(apiKey string, baseURL string, model string) *DoubaoClient {
	if baseURL == "" {
		baseURL = doubaoDefaultBaseURL
	}
	if model == "" {
		model = doubaoDefaultModel
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	logger.Log.Infow("豆包 (Doubao) LLM 客户端初始化",
		"base_url", baseURL,
		"model/endpoint", model,
	)

	return &DoubaoClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

func (c *DoubaoClient) Provider() string {
	return "doubao"
}

// Chat 实现 LLMClient 接口
// 豆包的 Chat Completions API 与 OpenAI 格式完全兼容
// 使用相同的 go-openai SDK 发送请求，仅 BaseURL 和 Model(endpoint) 不同
//
// 支持的能力（取决于接入点绑定的具体模型）：
//   - 纯文本对话 (所有豆包模型)
//   - 多模态 Vision 图文混输 (Doubao-Vision 系列)
//   - Function Calling 工具调用 (Doubao-Pro 系列)
func (c *DoubaoClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 构建消息列表
	messages := make([]openai.ChatCompletionMessage, 0)

	// 1. 添加系统提示词
	if req.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}

	// 2. 添加历史对话记录（用于 ReAct 多轮循环）
	messages = append(messages, req.History...)

	// 3. 构建用户消息（复用公共的多模态消息构建函数）
	userMsg, err := buildUserMessage(req.UserMessage, req.ImagePaths)
	if err != nil {
		return nil, fmt.Errorf("构建用户消息失败: %w", err)
	}
	messages = append(messages, userMsg)

	// 4. 构建请求参数
	chatReq := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	}

	// 5. 注册工具（如果有）
	if len(req.Tools) > 0 {
		chatReq.Tools = buildToolDefinitions(req.Tools)
	}

	// 6. 发送请求
	logger.Log.Debugw("豆包 (Doubao) 请求发送中",
		"endpoint", c.model,
		"message_count", len(messages),
		"tool_count", len(req.Tools),
	)

	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("调用豆包 API 失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("豆包返回了空的响应")
	}

	choice := resp.Choices[0]

	// 记录 Token 消耗（豆包按 Token 计费，方便成本追踪）
	logger.Log.Debugw("豆包 (Doubao) 请求完成",
		"endpoint", c.model,
		"prompt_tokens", resp.Usage.PromptTokens,
		"completion_tokens", resp.Usage.CompletionTokens,
		"total_tokens", resp.Usage.TotalTokens,
	)

	return &ChatResponse{
		Content:   choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
		Usage:     resp.Usage,
	}, nil
}
