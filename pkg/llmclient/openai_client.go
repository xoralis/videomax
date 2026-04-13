package llmclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sashabaranov/go-openai"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"video-max/internal/tools"
	"video-max/pkg/logger"
)

// OpenAIClient 封装与 OpenAI 兼容格式大模型 API 交互的客户端
// 内部使用 go-openai SDK，但对外只暴露自定义的 LLMClient 接口类型
// 支持纯文本对话、多模态（图文混输）对话、以及 Function Calling
type OpenAIClient struct {
	client *openai.Client
	model  string
}

// NewOpenAIClient 创建 OpenAI 兼容客户端实例
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
// 内部使用 go-openai SDK 的类型进行请求，响应后转换为自定义类型返回
func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	ctx, span := otel.Tracer("videomax").Start(ctx, "openai.Chat",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.system", "openai"),
			attribute.String("gen_ai.request.model", c.model),
			attribute.String("gen_ai.prompt", req.SystemPrompt+"\n"+req.UserMessage),
		))
	defer span.End()
	// 构建 go-openai 消息列表
	messages := make([]openai.ChatCompletionMessage, 0)

	// 1. 添加系统提示词
	if req.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}

	// 2. 将自定义 History 转换为 go-openai 消息格式
	for _, msg := range req.History {
		oaiMsg := openai.ChatCompletionMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		// 转换 ToolCalls
		if len(msg.ToolCalls) > 0 {
			oaiMsg.ToolCalls = make([]openai.ToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				oaiMsg.ToolCalls[i] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolType(tc.Type),
					Function: openai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
		messages = append(messages, oaiMsg)
	}

	// 3. 构建用户消息（可能包含图片的多模态消息）
	userMsg, err := buildOpenAIUserMessage(req.UserMessage, req.ImagePaths)
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
		chatReq.Tools = buildOpenAIToolDefinitions(req.Tools)
	}

	// 6. 发送请求
	logger.Log.Debugw("OpenAI 请求发送中",
		"model", c.model,
		"message_count", len(messages),
		"tool_count", len(req.Tools),
	)

	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("调用 OpenAI API 失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI 返回了空的响应")
	}

	choice := resp.Choices[0]

	// 7. 将 go-openai 响应转换为自定义类型
	result := &ChatResponse{
		Content: choice.Message.Content,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// 转换 ToolCalls
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	span.SetAttributes(
		attribute.String("gen_ai.completion", result.Content),
		attribute.Int("gen_ai.usage.input_tokens", result.Usage.PromptTokens),
		attribute.Int("gen_ai.usage.output_tokens", result.Usage.CompletionTokens),
	)
	return result, nil
}

// ==================== go-openai SDK 专用的内部工具函数 ====================

// buildOpenAIUserMessage 构建 go-openai 格式的用户消息
func buildOpenAIUserMessage(text string, imagePaths []string) (openai.ChatCompletionMessage, error) {
	if len(imagePaths) == 0 {
		return openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: text,
		}, nil
	}

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

// buildOpenAIToolDefinitions 将 AITool 列表转换为 go-openai 的 Tool 格式
func buildOpenAIToolDefinitions(aiTools []tools.AITool) []openai.Tool {
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
