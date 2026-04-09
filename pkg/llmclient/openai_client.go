package llmclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"

	"video-max/internal/tools"
	"video-max/pkg/logger"
)

// Client 封装与 OpenAI 兼容格式大模型 API 交互的客户端
// 支持纯文本对话和多模态（图文混输）对话
// 所有 Agent 共用此客户端发送请求
type Client struct {
	client *openai.Client
	model  string
}

// NewClient 创建 LLM 客户端实例
// apiKey: 大模型 API 密钥
// baseURL: API 基础地址（兼容 OpenAI 格式的第三方接口）
// model: 模型名称（如 gpt-4o）
func NewClient(apiKey string, baseURL string, model string) *Client {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	return &Client{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

// ChatRequest 一次对话请求的参数封装
type ChatRequest struct {
	SystemPrompt string            // 系统提示词，定义 Agent 的角色和行为规范
	UserMessage  string            // 用户消息内容
	ImagePaths   []string          // 可选：需要传入的图片本地路径（Base64 编码后发送）
	Tools        []tools.AITool    // 可选：注册给大模型可用的工具列表（Function Calling）
	History      []openai.ChatCompletionMessage // 可选：历史对话上下文（用于 ReAct 多轮循环）
}

// ChatResponse 一次对话请求的响应封装
type ChatResponse struct {
	Content   string                          // 大模型返回的文本内容
	ToolCalls []openai.ToolCall               // 大模型请求调用的工具列表（如果有）
	Usage     openai.Usage                    // Token 消耗统计
}

// Chat 发起一次大模型对话请求
// 支持纯文本、图文混输、以及带有 Function Calling 的请求
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
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
	userMsg, err := c.buildUserMessage(req.UserMessage, req.ImagePaths)
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
		chatReq.Tools = c.buildToolDefinitions(req.Tools)
	}

	// 6. 发送请求
	logger.Log.Debugw("LLM 请求发送中",
		"model", c.model,
		"message_count", len(messages),
		"tool_count", len(req.Tools),
	)

	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("调用大模型 API 失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("大模型返回了空的响应")
	}

	choice := resp.Choices[0]
	return &ChatResponse{
		Content:   choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
		Usage:     resp.Usage,
	}, nil
}

// buildUserMessage 构建用户消息
// 如果包含图片路径，则读取文件转为 Base64，构建多模态消息
func (c *Client) buildUserMessage(text string, imagePaths []string) (openai.ChatCompletionMessage, error) {
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
		// 读取本地图片文件并转为 Base64
		data, err := os.ReadFile(path)
		if err != nil {
			return openai.ChatCompletionMessage{}, fmt.Errorf("读取第 %d 张图片失败 (%s): %w", i+1, path, err)
		}

		b64 := base64.StdEncoding.EncodeToString(data)
		dataURI := fmt.Sprintf("data:image/png;base64,%s", b64)

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
func (c *Client) buildToolDefinitions(aiTools []tools.AITool) []openai.Tool {
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
