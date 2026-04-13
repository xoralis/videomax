package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"video-max/internal/tools"
	"video-max/pkg/logger"
)

// 字节豆包（Doubao）火山引擎方舟（ARK）平台配置
// 使用 Responses API：https://www.volcengine.com/docs/82379/1585135
const (
	// doubaoDefaultBaseURL ARK 平台 API 默认地址
	doubaoDefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	// doubaoResponsesPath Responses API 端点路径
	doubaoResponsesPath = "/responses"
)

// ==================== Responses API 请求结构体 ====================

// doubaoRequest Responses API 请求体
type doubaoRequest struct {
	Model        string          `json:"model"`                          // 模型 ID 或推理接入点 ID (ep-xxx)
	Input        json.RawMessage `json:"input"`                          // 输入内容：string 或 message 数组
	Instructions string          `json:"instructions,omitempty"`         // 系统指令（替代 messages 中的 system role）
	Tools        []doubaoToolDef `json:"tools,omitempty"`                // Function Calling 工具定义
	PrevRespID   string          `json:"previous_response_id,omitempty"` // 上一次响应 ID（有状态对话）
	Stream       bool            `json:"stream"`                         // 是否流式输出，默认 false
}

// doubaoToolDef Responses API 的工具定义格式
// 注意：Responses API 使用扁平结构，name/description/parameters 直接放在顶层
// 这与 Chat Completions API 的嵌套 function 包装器不同
type doubaoToolDef struct {
	Type        string      `json:"type"`        // "function"
	Name        string      `json:"name"`        // 函数名称
	Description string      `json:"description"` // 函数描述
	Parameters  interface{} `json:"parameters"`  // JSON Schema 格式的参数定义
}

// doubaoInputMessage Responses API input 数组中的消息元素
type doubaoInputMessage struct {
	Role    string      `json:"role"`    // "user", "assistant", "tool"
	Content interface{} `json:"content"` // string 或 content 数组
}

// doubaoContentPart 多模态 content 数组中的元素
type doubaoContentPart struct {
	Type   string `json:"type"`              // "input_text", "input_image"
	Text   string `json:"text,omitempty"`    // type=input_text 时
	FileID string `json:"file_id,omitempty"` // type=input_image 时，使用 file_id
}

// doubaoFunctionCallInput 传回的历史工具调用指令
type doubaoFunctionCallInput struct {
	Type      string `json:"type"` // "function_call"
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// doubaoToolCallInput 传入工具调用结果的消息
type doubaoToolCallInput struct {
	Type   string `json:"type"`    // "function_call_output"
	CallID string `json:"call_id"` // 对应的 function_call 的 call_id
	Output string `json:"output"`  // 工具执行结果
}

// ==================== Responses API 响应结构体 ====================

// doubaoResponse Responses API 响应体
type doubaoResponse struct {
	ID     string             `json:"id"`     // 响应唯一标识（可用于 previous_response_id）
	Object string             `json:"object"` // 固定 "response"
	Output []doubaoOutputItem `json:"output"` // 输出段列表
	Usage  *doubaoUsage       `json:"usage"`  // Token 消耗
	Error  *doubaoError       `json:"error"`  // 错误信息
}

// doubaoOutputItem 输出段，type 决定具体内容
type doubaoOutputItem struct {
	Type string `json:"type"` // "message", "function_call", "reasoning"

	// type="message" 时的字段
	Role    string                `json:"role,omitempty"`
	Content []doubaoOutputContent `json:"content,omitempty"`

	// type="function_call" 时的字段
	Name      string `json:"name,omitempty"`      // 函数名称
	Arguments string `json:"arguments,omitempty"` // JSON 参数
	CallID    string `json:"call_id,omitempty"`   // 调用 ID

	// type="reasoning" 时的字段
	Summary []doubaoSummaryItem `json:"summary,omitempty"` // 推理过程摘要
}

// doubaoOutputContent 输出段中的 content 元素
type doubaoOutputContent struct {
	Type string `json:"type"` // "output_text"
	Text string `json:"text"` // 文本内容
}

// doubaoSummaryItem 推理摘要
type doubaoSummaryItem struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
}

// doubaoUsage Token 消耗
type doubaoUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// doubaoError 错误信息
type doubaoError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ==================== DoubaoClient 实现 ====================

// DoubaoClient 字节跳动豆包大模型的 LLMClient 实现
// 使用 Responses API（非 Chat Completions API）
// 通过原生 net/http 发送请求，不依赖任何第三方 SDK
//
// Responses API 与 Chat Completions API 的核心区别：
//   - 端点: POST /v3/responses（而非 /v3/chat/completions）
//   - 系统指令: 使用独立的 instructions 字段（而非 messages 中的 system role）
//   - 输出格式: output 数组包含 message / function_call / reasoning 多种段（而非 choices）
//   - 有状态对话: 支持 previous_response_id 链式引用（无需手动传完整历史）
type DoubaoClient struct {
	apiKey     string
	baseURL    string
	model      string // 模型 ID 或推理接入点 ID (ep-xxx)
	httpClient *http.Client
}

// NewDoubaoClient 创建豆包大模型客户端实例
//
// 参数说明：
//   - apiKey: 火山引擎 ARK 平台的 API Key
//   - baseURL: 留空则使用默认 https://ark.cn-beijing.volces.com/api/v3
//   - model: 模型 ID 或推理接入点 ID (ep-xxx)
func NewDoubaoClient(apiKey string, baseURL string, model string) *DoubaoClient {
	if baseURL == "" {
		baseURL = doubaoDefaultBaseURL
	}

	logger.Log.Infow("豆包 (Doubao) LLM 客户端初始化 [Responses API]",
		"base_url", baseURL,
		"model", model,
	)

	return &DoubaoClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		// Timeout=0：不设客户端级固定超时，完全由调用方通过 context 控制截止时间。
		// LLM 推理耗时不可预测（通常 30s-5min），固定超时会误杀正常请求。
		httpClient: &http.Client{Timeout: 0},
	}
}

func (c *DoubaoClient) Provider() string {
	return "doubao"
}

// Chat 实现 LLMClient 接口
// 将通用的 ChatRequest 转换为 Responses API 格式进行请求
//
// 映射关系：
//
//	ChatRequest.SystemPrompt → doubaoRequest.Instructions
//	ChatRequest.History      → doubaoRequest.Input (消息数组)
//	ChatRequest.UserMessage  → 追加到 Input 的最后一条 user 消息
//	ChatRequest.ImagePaths   → user 消息中的 input_image content part
//	ChatRequest.Tools        → doubaoRequest.Tools
//
// 响应映射：
//
//	output[type="message"]       → ChatResponse.Content
//	output[type="function_call"] → ChatResponse.ToolCalls
//	usage                        → ChatResponse.Usage
func (c *DoubaoClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	ctx, span := otel.Tracer("videomax").Start(ctx, "doubao.Chat",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.system", "doubao"),
			attribute.String("gen_ai.request.model", c.model),
			attribute.String("gen_ai.prompt", req.SystemPrompt+"\n"+req.UserMessage),
		))
	defer span.End()

	// 1. 构建 input 消息数组
	inputMsgs := make([]interface{}, 0)

	// 将 History 中的消息转换为 Responses API 格式
	for _, msg := range req.History {
		switch msg.Role {
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// assistant 请求工具调用：在 Responses API 中，历史 function_call 必须用专门的 type 且没有 role
				// 如果大模型同时返回了思考或文本
				if msg.Content != "" {
					inputMsgs = append(inputMsgs, doubaoInputMessage{
						Role:    "assistant",
						Content: msg.Content,
					})
				}
				// 遍历添加函数调用历史
				for _, tc := range msg.ToolCalls {
					inputMsgs = append(inputMsgs, doubaoFunctionCallInput{
						Type:      "function_call",
						CallID:    tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
			} else {
				inputMsgs = append(inputMsgs, doubaoInputMessage{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case "tool":
			// 工具结果：使用 function_call_output 类型
			inputMsgs = append(inputMsgs, doubaoToolCallInput{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: msg.Content,
			})
		default:
			// user 或其他角色
			inputMsgs = append(inputMsgs, doubaoInputMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// 2. 构建当前用户消息（支持多模态图片）
	if req.UserMessage != "" || len(req.ImagePaths) > 0 {
		userContent, err := c.buildMultimodalContent(ctx, req.UserMessage, req.ImagePaths)
		if err != nil {
			return nil, fmt.Errorf("构建多模态消息失败: %w", err)
		}
		inputMsgs = append(inputMsgs, doubaoInputMessage{
			Role:    "user",
			Content: userContent,
		})
	}

	// 3. 序列化 input
	inputJSON, err := json.Marshal(inputMsgs)
	if err != nil {
		return nil, fmt.Errorf("序列化 input 失败: %w", err)
	}

	// 4. 构建请求体
	reqBody := doubaoRequest{
		Model:        c.model,
		Input:        inputJSON,
		Instructions: req.SystemPrompt,
		Stream:       false,
	}

	// 5. 注册工具（如果有）
	if len(req.Tools) > 0 {
		reqBody.Tools = c.buildTools(req.Tools)
	}

	// 6. 发送 HTTP 请求
	logger.Log.Debugw("豆包 Responses API 请求发送中",
		"model", c.model,
		"input_count", len(inputMsgs),
		"tool_count", len(req.Tools),
	)

	var respBody doubaoResponse
	if err := c.doPost(ctx, c.baseURL+doubaoResponsesPath, reqBody, &respBody); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("调用豆包 Responses API 失败: %w", err)
	}

	// 7. 检查错误
	if respBody.Error != nil {
		return nil, fmt.Errorf("豆包 API 错误 [%s]: %s", respBody.Error.Code, respBody.Error.Message)
	}

	// 8. 解析 output 数组
	result := &ChatResponse{}
	for _, item := range respBody.Output {
		switch item.Type {
		case "message":
			// 提取文本内容
			for _, c := range item.Content {
				if c.Type == "output_text" {
					result.Content += c.Text
				}
			}
		case "function_call":
			// 提取工具调用
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		case "reasoning":
			// 深度思考的推理过程，目前仅记录日志
			for _, s := range item.Summary {
				logger.Log.Debugw("豆包 Reasoning", "summary", s.Text)
			}
		}
	}

	// 9. 填充 Usage
	if respBody.Usage != nil {
		result.Usage = Usage{
			PromptTokens:     respBody.Usage.InputTokens,
			CompletionTokens: respBody.Usage.OutputTokens,
			TotalTokens:      respBody.Usage.TotalTokens,
		}
	}

	logger.Log.Debugw("豆包 Responses API 请求完成",
		"model", c.model,
		"response_id", respBody.ID,
		"content_length", len(result.Content),
		"tool_calls", len(result.ToolCalls),
		"total_tokens", result.Usage.TotalTokens,
	)

	span.SetAttributes(
		attribute.String("gen_ai.completion", result.Content),
		attribute.Int("gen_ai.usage.input_tokens", result.Usage.PromptTokens),
		attribute.Int("gen_ai.usage.output_tokens", result.Usage.CompletionTokens),
	)
	return result, nil
}

// ==================== 内部工具函数 ====================

// buildMultimodalContent 构建多模态 content
// 如果有图片，返回 content parts 数组；否则返回纯文本字符串
func (c *DoubaoClient) buildMultimodalContent(ctx context.Context, text string, imagePaths []string) (interface{}, error) {
	if len(imagePaths) == 0 {
		return text, nil
	}

	// 有图片时构建 content parts 数组
	parts := make([]doubaoContentPart, 0)

	// 文本部分
	if text != "" {
		parts = append(parts, doubaoContentPart{
			Type: "input_text",
			Text: text,
		})
	}

	// 图片部分（Responses API 使用 input_image 和 file_id）
	for i, path := range imagePaths {
		fileID, err := c.uploadFile(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("上传第 %d 张图片失败 (%s): %w", i+1, path, err)
		}

		parts = append(parts, doubaoContentPart{
			Type:   "input_image",
			FileID: fileID,
		})
	}

	return parts, nil
}

// doubaoFileResponse 文件上传响应
type doubaoFileResponse struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Filename string `json:"filename"`
	Status   string `json:"status"`
}

// uploadFile 上传本地图片到火山引擎获取 file_id
func (c *DoubaoClient) uploadFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("无法打开文件: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 设置 purpose = user_data
	_ = writer.WriteField("purpose", "user_data")

	// 添加文件部分
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", fmt.Errorf("创建 multipart 表单失败: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("复制文件数据失败: %w", err)
	}
	writer.Close()

	// 构造请求
	reqURL := c.baseURL + "/files"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return "", fmt.Errorf("创建上传请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 执行请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传请求发送失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取上传响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("API 错误响应 %d: %s", resp.StatusCode, string(respBytes))
	}

	var fileResp doubaoFileResponse
	if err := json.Unmarshal(respBytes, &fileResp); err != nil {
		return "", fmt.Errorf("解析上传响应 JSON 失败: %w", err)
	}

	logger.Log.Debugw("豆包图片上传请求成功", "filename", filepath.Base(path), "file_id", fileResp.ID, "status", fileResp.Status)

	// 如果状态是 processing，需要轮询等待它就绪
	if fileResp.Status == "processing" {
		err = c.waitForFileReady(ctx, fileResp.ID)
		if err != nil {
			return "", fmt.Errorf("等待图片处理完成失败: %w", err)
		}
	}

	return fileResp.ID, nil
}

// waitForFileReady 轮询查询文件状态直到不是 processing
func (c *DoubaoClient) waitForFileReady(ctx context.Context, fileID string) error {
	reqURL := fmt.Sprintf("%s/files/%s", c.baseURL, fileID)

	// 最多轮询 15 次，每次间隔 1 秒
	for i := 0; i < 15; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if resp.StatusCode >= 400 {
			return fmt.Errorf("查询文件状态失败 %d: %s", resp.StatusCode, string(respBytes))
		}

		var fileResp doubaoFileResponse
		if err := json.Unmarshal(respBytes, &fileResp); err != nil {
			return err
		}

		if fileResp.Status != "processing" {
			logger.Log.Debugw("豆包图片处理完成", "file_id", fileID, "final_status", fileResp.Status)
			return nil
		}

		logger.Log.Debugw("豆包图片处理中，等待...", "file_id", fileID, "attempt", i+1)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// 继续下一次循环
		}
	}

	return fmt.Errorf("等待文件处理超时")
}

// buildTools 将 AITool 列表转换为 Responses API 的工具定义格式
// Responses API 工具格式（扁平结构）：
//
//	{"type": "function", "name": "xxx", "description": "xxx", "parameters": {...}}
func (c *DoubaoClient) buildTools(aiTools []tools.AITool) []doubaoToolDef {
	defs := make([]doubaoToolDef, 0, len(aiTools))
	for _, t := range aiTools {
		defs = append(defs, doubaoToolDef{
			Type:        "function",
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.ParametersSchema(),
		})
	}
	return defs
}

// doPost 发送 POST 请求并解析 JSON 响应
func (c *DoubaoClient) doPost(ctx context.Context, url string, body interface{}, dest interface{}) error {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("序列化请求体失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	// ARK 认证：Bearer Token
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP 请求发送失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API 返回状态码 %d，响应: %s", resp.StatusCode, string(respBytes))
	}

	if err := json.Unmarshal(respBytes, dest); err != nil {
		return fmt.Errorf("解析响应 JSON 失败: %w (原始响应: %s)", err, string(respBytes))
	}

	return nil
}
