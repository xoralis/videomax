package video

import (
	"bytes"
	"context"
	"encoding/base64"
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

	"video-max/pkg/logger"
)

// Seedance API 的接口地址（火山引擎）
const (
	seedanceBaseURL      = "https://ark.cn-beijing.volces.com/api/v3"
	seedanceGeneratePath = "/contents/generations/tasks"    // 提交生成任务
	seedanceQueryPath    = "/contents/generations/tasks/%s" // 查询任务状态，%s 填 task_id
)

// ---- 提交任务：请求与响应结构体 ----

// seedanceContent 请求体中 content 数组的单个元素
type seedanceContent struct {
	Type     string            `json:"type"`                // "text" 或 "image_url"
	Text     string            `json:"text,omitempty"`      // type=text 时的内容
	ImageURL *seedanceImageURL `json:"image_url,omitempty"` // type=image_url 时的内容
	Role     string            `json:"role,omitempty"`      // "reference_image"/"reference_video"/"first_frame"...
}

// seedanceImageURL content 中 image_url 对象
type seedanceImageURL struct {
	URL string `json:"url"` // 支持 URL 或 base64 data URI （data:image/jpeg;base64,...）
}

// seedanceGenerateReq 提交视频生成任务的请求体
// 参考：https://www.volcengine.com/docs/82379/1541595
type seedanceGenerateReq struct {
	Model     string            `json:"model"`               // 模型 ID，如 doubao-seedance-2-0-260128
	Content   []seedanceContent `json:"content"`             // 多模态内容数组（文本 + 图片）
	Seed      int               `json:"seed,omitempty"`      // 随机种子，用于生成可复现的结果
	Duration  float64           `json:"duration,omitempty"`  // 视频时长（秒），仅当 type="video" 时有效
	Ratio     string            `json:"ratio,omitempty"`     // 画面比例，如 "16:9"、"9:16"、"1:1"
	Watermark string            `json:"watermark,omitempty"` // 水印，如 "volcengine"、"none"
	TaskType  string            `json:"task_type,omitempty"` // 任务类型，如 "i2v"、"r2v"
}

// seedanceGenerateResp 提交任务的响应体
type seedanceGenerateResp struct {
	ID     string `json:"id"`     // 任务 ID，用于后续轮询
	Status string `json:"status"` // 任务状态
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ---- 查询任务：响应结构体 ----

// seedanceQueryResp 查询任务状态的响应体
type seedanceQueryResp struct {
	ID     string `json:"id,omitempty"`     // 任务 ID
	Model  string `json:"model,omitempty"`  // 模型 ID
	Status string `json:"status,omitempty"` // queued / running / succeeded / failed / cancelled
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Param   string `json:"param,omitempty"`
		Type    string `json:"type,omitempty"`
	} `json:"error,omitempty"`

	Content *struct {
		VideoURL string `json:"video_url"` // 生成成功时的视频 URL
	} `json:"content"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	CreatedAt             int64   `json:"created_at,omitempty"`
	UpdatedAt             int64   `json:"updated_at,omitempty"`
	Seed                  int     `json:"seed,omitempty"`
	Resolution            string  `json:"resolution,omitempty"`
	Ratio                 string  `json:"ratio,omitempty"`
	Duration              float64 `json:"duration,omitempty"`
	FramesPerSecond       float64 `json:"frames_per_second,omitempty"`
	ServiceTier           string  `json:"service_tier,omitempty"`
	ExecutionExpiresAfter int     `json:"execution_expires_after,omitempty"`
	GenerateAudio         bool    `json:"generate_audio,omitempty"`
	Draft                 bool    `json:"draft,omitempty"`
}

// ---- 客户端实现 ----

// ByteDanceClient 字节跳动 Seedance 视频生成服务的 VideoProvider 实现
type ByteDanceClient struct {
	apiKey     string
	baseURL    string // 允许配置覆盖默认 BaseURL，方便代理或测试
	model      string // 如 doubao-seedance-2-0-260128
	httpClient *http.Client
}

// NewByteDanceClient 创建字节跳动客户端实例
func NewByteDanceClient(apiKey string, baseURL string, model string) *ByteDanceClient {
	if baseURL == "" {
		baseURL = seedanceBaseURL
	}
	if model == "" {
		// 默认使用 Seedance 2.0 高质量版本
		model = "doubao-seedance-2-0-260128"
	}
	return &ByteDanceClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *ByteDanceClient) Name() string {
	return ProviderByteDance
}

// GenerateVideo 向 Seedance API 提交视频生成任务
// 内部流程：
// 1. 读取本地图片，转为 Base64 data URI（无需另行上传）
// 2. 组装多模态 content 数组（文本提示词 + 多张图片）
// 3. POST 到 /contents/generations/tasks
// 4. 返回 task_id 供后续轮询
func (c *ByteDanceClient) GenerateVideo(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	ctx, span := otel.Tracer("videomax").Start(ctx, "bytedance.GenerateVideo",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "tool"),
			attribute.String("video.provider", ProviderByteDance),
			attribute.String("video.model", c.model),
			attribute.String("gen_ai.prompt", req.Prompt),
			attribute.Int("video.image_count", len(req.ImagePaths)),
			attribute.String("video.aspect_ratio", req.AspectRatio),
		))
	defer span.End()

	logger.Log.Infow("Seedance: 开始提交视频生成任务",
		"model", c.model,
		"prompt_length", len(req.Prompt),
		"image_count", len(req.ImagePaths),
		"aspect_ratio", req.AspectRatio,
	)

	// 组装 content 数组
	var contents []seedanceContent

	// 提示词
	contents = append(contents, seedanceContent{
		Type: "text",
		Text: req.Prompt,
	})

	// 放入参考图片（如有）
	for i, imgPath := range req.ImagePaths {
		b64URI, err := imageToBase64URI(imgPath)
		if err != nil {
			return nil, fmt.Errorf("读取第 %d 张参考图失败 (%s): %w", i+1, imgPath, err)
		}
		contents = append(contents, seedanceContent{
			Type: "image_url",
			ImageURL: &seedanceImageURL{
				URL: b64URI,
			},
			// TODO 兼容不同模型格式
			// Role: "reference_image", seedance 1.0不支持参考图参数
		})
	}

	// 构造请求体
	body := seedanceGenerateReq{
		Model:   c.model,
		Content: contents,
	}
	if req.AspectRatio != "" {
		body.Ratio = req.AspectRatio
	}
	if req.Duration > 0 {
		body.Duration = float64(req.Duration)
	}
	// body.TaskType = "i2v"

	// 发送 POST 请求
	var respBody seedanceGenerateResp
	if err := c.doRequest(ctx, http.MethodPost, c.baseURL+seedanceGeneratePath, body, &respBody); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("Seedance 提交任务请求失败: %w", err)
	}

	// 检查响应是否有错误
	if respBody.Error != nil {
		return nil, fmt.Errorf("Seedance API 错误 [%s]: %s", respBody.Error.Code, respBody.Error.Message)
	}
	if respBody.ID == "" {
		return nil, fmt.Errorf("Seedance API 返回了空的 task_id")
	}

	logger.Log.Infow("Seedance: 视频生成任务提交成功",
		"task_id", respBody.ID,
		"status", respBody.Status,
	)

	span.SetAttributes(attribute.String("video.provider_task_id", respBody.ID))
	return &GenerateResult{
		ProviderTaskID:   respBody.ID,
		EstimatedWaitSec: 120, // Seedance 通常 1-3 分钟
	}, nil
}

// CheckStatus 查询 Seedance 视频生成任务的当前状态
// Seedance 任务状态枚举：
//   - queued   : 排队中
//   - running  : 生成中
//   - succeeded: 生成成功，可从 choices[0].message.content 中提取视频 URL
//   - failed   : 生成失败，从 error 字段读取原因
//   - cancelled: 已取消
func (c *ByteDanceClient) CheckStatus(ctx context.Context, providerTaskID string) (*TaskStatus, error) {
	ctx, span := otel.Tracer("videomax").Start(ctx, "bytedance.CheckStatus",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "tool"),
			attribute.String("video.provider", ProviderByteDance),
			attribute.String("video.provider_task_id", providerTaskID),
		))
	defer span.End()

	logger.Log.Infow("Seedance: 查询任务状态", "provider_task_id", providerTaskID)

	url := fmt.Sprintf(c.baseURL+seedanceQueryPath, providerTaskID)
	var respBody seedanceQueryResp
	if err := c.doRequest(ctx, http.MethodGet, url, nil, &respBody); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("Seedance 查询任务状态请求失败: %w", err)
	}

	switch respBody.Status {
	case "succeeded":
		// 从 choices 中提取视频 URL
		videoURL := extractVideoURL(respBody)
		logger.Log.Infow("Seedance: 视频生成成功",
			"provider_task_id", providerTaskID,
			"video_url", videoURL,
		)
		span.SetAttributes(
			attribute.String("video.status", "succeeded"),
			attribute.String("video.url", videoURL),
		)
		return &TaskStatus{
			IsFinished: true,
			IsFailed:   false,
			VideoURL:   videoURL,
		}, nil

	case "failed", "cancelled":
		errMsg := "未知错误"
		if respBody.Error != nil {
			errMsg = fmt.Sprintf("[%s] %s", respBody.Error.Code, respBody.Error.Message)
		}
		logger.Log.Warnw("Seedance: 视频生成失败",
			"provider_task_id", providerTaskID,
			"status", respBody.Status,
			"error", errMsg,
		)
		span.SetAttributes(attribute.String("video.status", respBody.Status))
		span.SetStatus(codes.Error, errMsg)
		return &TaskStatus{
			IsFinished: true,
			IsFailed:   true,
			ErrorMsg:   errMsg,
		}, nil

	default:
		// "queued" 或 "running"，任务仍在进行中
		logger.Log.Infow("Seedance: 任务进行中",
			"provider_task_id", providerTaskID,
			"status", respBody.Status,
		)
		span.SetAttributes(attribute.String("video.status", respBody.Status))
		return &TaskStatus{
			IsFinished: false,
			IsFailed:   false,
		}, nil
	}
}

// ---- 工具函数 ----

// doRequest 发送 HTTP 请求并将响应解析到 dest 结构体中
// method: "GET" 或 "POST"，body 为 nil 时不发送请求体
func (c *ByteDanceClient) doRequest(ctx context.Context, method string, url string, body any, dest any) error {
	var bodyReader io.Reader

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("序列化请求体失败: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	// 设置认证头：Bearer API Key
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP 请求发送失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体失败: %w", err)
	}

	// HTTP 4xx/5xx 状态码视为错误
	if resp.StatusCode >= 400 {
		return fmt.Errorf("API 返回非预期状态码 %d，响应: %s", resp.StatusCode, string(respBytes))
	}

	if err := json.Unmarshal(respBytes, dest); err != nil {
		return fmt.Errorf("解析响应 JSON 失败: %w (原始响应: %s)", err, string(respBytes))
	}

	return nil
}

// imageToBase64URI 将本地图片文件读取并转换为 Base64 data URI 格式
// 格式例如: data:image/jpeg;base64,/9j/4AAQSkZJRg...
func imageToBase64URI(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// 根据文件扩展名判断 MIME 类型
	ext := filepath.Ext(filePath)
	mimeType := "image/jpeg" // 默认
	switch ext {
	case ".png":
		mimeType = "image/png"
	case ".webp":
		mimeType = "image/webp"
	case ".gif":
		mimeType = "image/gif"
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, b64), nil
}

// extractVideoURL 从 Seedance 查询响应中提取视频 URL
func extractVideoURL(resp seedanceQueryResp) string {
	if resp.Content == nil || resp.Content.VideoURL == "" {
		return ""
	}
	return resp.Content.VideoURL
}

// 以下为兼容占位，实际未使用但预留给带 assets 接口的扩展
var _ = multipart.NewWriter
