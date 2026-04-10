package llmclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sashabaranov/go-openai"

	"video-max/internal/tools"
	"video-max/pkg/logger"
)

// OpenAIClient 封装与 OpenAI 兼容格式大模型 API 交互的客户端
// 支持纯文本对话、多模态（图文混输）对话、以及 Function Calling
// 兼容所有采用 OpenAI API 格式的第三方服务（如 DeepSeek、Moonshot 等）
type OpenAIClient struct {
	client *openai.Client
	model  string
}

// NewOpenAIClient 创建 OpenAI 兼容客户端实例
// apiKey: 大模型 API 密钥
// baseURL: API 基础地址（留空则使用 OpenAI 官方地址）
// model: 模型名称（如 gpt-4o）
func NewOpenAIClient(apiKey string, baseURL string, model string) *OpenAIClient {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	return &OpenAIClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

func (c *OpenAIClient) Provider() string {
	return "openai"
}

// Chat 实现 LLMClient 接口
// 发起一次大模型对话请求，支持纯文本、图文混输、以及带有 Function Calling 的请求
func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 构建消息列表
	messages := make([]openai.ChatCompletionMessage, 0)

	// 1. 添加系统提示词
	if req.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}

	// 2. 添加历史对话记录（用于 ReAct 多轮循环，保持上下文连续性）
	messages = append(messages, req.History...)

	// 3. 构建用户消息（可能包含图片的多模态消息）
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

	// 5. 如果注册了工具，将其转换为 OpenAI Function Calling 格式
	if len(req.Tools) > 0 {
		chatReq.Tools = buildToolDefinitions(req.Tools)
	}

	// 6. 发送请求
	logger.Log.Debugw("OpenAI 请求发送中",
		"model", c.model,
		"message_count", len(messages),
		"tool_count", len(req.Tools),
	)

	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("调用 OpenAI API 失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI 返回了空的响应")
	}

	choice := resp.Choices[0]
	return &ChatResponse{
		Content:   choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
		Usage:     resp.Usage,
	}, nil
}

// ---- 以下为 OpenAI 和 Doubao 共用的工具函数 ----

// buildUserMessage 构建用户消息
// 如果包含图片路径，则读取文件转为 Base64，构建多模态消息
func buildUserMessage(text string, imagePaths []string) (openai.ChatCompletionMessage, error) {
	// 无图片时返回纯文本消息
	if len(imagePaths) == 0 {
		return openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: text,
		}, nil
	}

	// 有图片时构建多模态消息（图文混输）
	parts := []openai.ChatMessagePart{
		{
			Type: openai.ChatMessagePartTypeText,
			Text: text,
		},
	}

	for i, path := range imagePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			return openai.ChatCompletionMessage{}, fmt.Errorf("读取第 %d 张图片失败 (%s): %w", i+1, path, err)
		}

		// 根据文件扩展名确定 MIME 类型
		mimeType := detectMIME(filepath.Ext(path))
		b64 := base64.StdEncoding.EncodeToString(data)
		dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)

		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL:    dataURI,
				Detail: openai.ImageURLDetailAuto,
			},
		})
	}

	return openai.ChatCompletionMessage{
		Role:         openai.ChatMessageRoleUser,
		MultiContent: parts,
	}, nil
}

// buildToolDefinitions 将系统注册的 AITool 列表转换为 OpenAI Function Calling 格式
func buildToolDefinitions(aiTools []tools.AITool) []openai.Tool {
	oaiTools := make([]openai.Tool, 0, len(aiTools))
	for _, t := range aiTools {
		oaiTools = append(oaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.ParametersSchema(),
			},
		})
	}
	return oaiTools
}

// detectMIME 根据文件扩展名返回对应的 MIME 类型
func detectMIME(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}
