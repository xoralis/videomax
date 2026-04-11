package video

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video-max/pkg/logger"
)

// 可灵（Kling AI）API 配置
// 文档参考：https://klingai.com/document-api/apiReference/model/imageToVideo
const (
	klingDefaultBaseURL            = "https://api-beijing.klingai.com/v1"
	klingImage2VideoPath           = "/videos/image2video"
	klingQueryImage2VideoPath      = "/videos/image2video/%s" // %s = task_id
	klingMultiImage2VideoPath      = "/videos/multi-image2video"
	klingQueryMultiImage2VideoPath = "/videos/multi-image2video/%s"
)

// ==================== JWT 认证 ====================

// klingJWTHeader JWT 头部
type klingJWTHeader struct {
	Alg string `json:"alg"` // "HS256"
	Typ string `json:"typ"` // "JWT"
}

// klingJWTPayload JWT 载荷
type klingJWTPayload struct {
	Iss string `json:"iss"` // Access Key
	Exp int64  `json:"exp"` // 过期时间 (Unix 秒)
	Nbf int64  `json:"nbf"` // 生效时间 (Unix 秒)
}

// ==================== 请求结构体 ====================

// klingImageListItem 多图请求列表元素
type klingImageListItem struct {
	Image string `json:"image"`
}

// klingImage2VideoReq 图生视频请求体（复用于单图和多图）
type klingImage2VideoReq struct {
	ModelName   string               `json:"model_name"`                // 模型名称，如 "kling-v1", "kling-v2-6", "kling-v3"
	Image       string               `json:"image,omitempty"`           // 单图: 图片 Base64
	ImageList   []klingImageListItem `json:"image_list,omitempty"`      // 多图: 图片列表
	ImageTail   string               `json:"image_tail,omitempty"`      // 尾帧图片（可选）
	Prompt      string               `json:"prompt,omitempty"`          // 文本提示词
	NegPrompt   string               `json:"negative_prompt,omitempty"` // 负向提示词
	Duration    string               `json:"duration,omitempty"`        // 时长："5" 或 "10"
	Mode        string               `json:"mode,omitempty"`            // 模式："std" 或 "pro"
	AspectRatio string               `json:"aspect_ratio,omitempty"`    // 画面比例："16:9", "9:16", "1:1"
	CallbackURL string               `json:"callback_url,omitempty"`    // 回调地址（可选）
}

// ==================== 响应结构体 ====================

// klingAPIResponse 可灵统一响应外壳
type klingAPIResponse struct {
	Code      int             `json:"code"` // 0 表示成功
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"` // 具体数据由业务 unmarshal
}

// klingTaskData 任务创建/查询的 data 字段
type klingTaskData struct {
	TaskID        string           `json:"task_id"`
	TaskStatus    string           `json:"task_status"`     // submitted / processing / succeed / failed
	TaskStatusMsg string           `json:"task_status_msg"` // 失败原因
	TaskResult    *klingTaskResult `json:"task_result"`
}

// klingTaskResult 任务结果
type klingTaskResult struct {
	Videos []klingVideo `json:"videos"`
}

// klingVideo 视频结果
type klingVideo struct {
	ID           string `json:"id"`
	URL          string `json:"url"`           // 视频直链
	WatermarkURL string `json:"watermark_url"` // 带水印版本
	Duration     string `json:"duration"`
}

// ==================== KlingClient 实现 ====================

// KlingClient 可灵视频生成服务的 VideoProvider 实现
// 使用 JWT (HS256) 认证，通过 Access Key 和 Secret Key 签发短期令牌
type KlingClient struct {
	accessKey  string // AK
	secretKey  string // SK
	baseURL    string
	model      string // 如 "kling-v1", "kling-v2-6", "kling-v3"
	httpClient *http.Client
}

// NewKlingClient 创建可灵客户端实例
//
// 参数说明：
//   - apiKey: 格式为 "access_key:secret_key"（用冒号分隔 AK 和 SK）
//   - baseURL: 留空则使用默认 https://api-beijing.klingai.com/v1
//   - model: 模型名称，如 "kling-v1", "kling-v2-6", "kling-v3"
func NewKlingClient(apiKey string, baseURL string, model string) *KlingClient {
	if baseURL == "" {
		baseURL = klingDefaultBaseURL
	}
	if model == "" {
		model = "kling-v1"
	}

	// apiKey 格式: "access_key:secret_key"
	parts := strings.SplitN(apiKey, ":", 2)
	ak := parts[0]
	sk := ""
	if len(parts) > 1 {
		sk = parts[1]
	}

	logger.Log.Infow("可灵 (Kling) 视频客户端初始化",
		"base_url", baseURL,
		"model", model,
	)

	return &KlingClient{
		accessKey:  ak,
		secretKey:  sk,
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 600 * time.Second},
	}
}

func (c *KlingClient) Name() string {
	return ProviderKling
}

// GenerateVideo 向可灵提交图生视频任务
// 流程：
//  1. 读取本地图片，转为纯 Base64（不含 data:image 前缀）
//  2. 组装 image2video 请求体
//  3. POST 到 /videos/image2video
//  4. 返回 task_id 供后续轮询
func (c *KlingClient) GenerateVideo(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	logger.Log.Infow("可灵: 开始提交图生视频任务",
		"model", c.model,
		"prompt_length", len(req.Prompt),
		"image_count", len(req.ImagePaths),
		"aspect_ratio", req.AspectRatio,
	)

	// 构建请求体
	klingReq := klingImage2VideoReq{
		ModelName:   c.model,
		Prompt:      req.Prompt,
		Mode:        "std",
		AspectRatio: req.AspectRatio,
	}

	// 设置时长
	if req.Length > 0 && req.Length <= 5 {
		klingReq.Duration = "5"
	} else if req.Length > 5 {
		klingReq.Duration = "10"
	} else {
		klingReq.Duration = "5" // 默认 5 秒
	}

	// 区分单图还是多图接口
	var reqPath string
	var taskPrefix string

	if len(req.ImagePaths) > 1 {
		// 使用多图生视频接口
		reqPath = klingMultiImage2VideoPath
		taskPrefix = "multi:"
		for i, imgPath := range req.ImagePaths {
			// 可灵的多图接口限制 2-4 张，通常不超这范围，超出则截断。
			if i >= 4 {
				break
			}
			b64, err := imageToRawBase64(imgPath)
			if err != nil {
				return nil, fmt.Errorf("读取多张参考图失败: %w", err)
			}
			klingReq.ImageList = append(klingReq.ImageList, klingImageListItem{Image: b64})
		}
	} else if len(req.ImagePaths) == 1 {
		// 使用单图生视频接口
		reqPath = klingImage2VideoPath
		taskPrefix = "single:"
		b64, err := imageToRawBase64(req.ImagePaths[0])
		if err != nil {
			return nil, fmt.Errorf("读取单张参考图失败: %w", err)
		}
		klingReq.Image = b64
	}

	// 发送请求
	var apiResp klingAPIResponse
	if err := c.doRequest(ctx, http.MethodPost, c.baseURL+reqPath, klingReq, &apiResp); err != nil {
		return nil, fmt.Errorf("可灵提交任务请求失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("可灵 API 错误 [%d]: %s", apiResp.Code, apiResp.Message)
	}

	// 解析 task data
	var taskData klingTaskData
	if err := json.Unmarshal(apiResp.Data, &taskData); err != nil {
		return nil, fmt.Errorf("解析可灵任务响应失败: %w", err)
	}

	if taskData.TaskID == "" {
		return nil, fmt.Errorf("可灵 API 返回了空的 task_id")
	}

	logger.Log.Infow("可灵: 图生视频任务提交成功",
		"task_id", taskData.TaskID,
		"status", taskData.TaskStatus,
	)

	// 修改了 task_id 存储方式，前缀用来供 CheckStatus 判断轮询哪个接口
	return &GenerateResult{
		ProviderTaskID:   taskPrefix + taskData.TaskID,
		EstimatedWaitSec: 120,
	}, nil
}

// CheckStatus 查询可灵视频生成任务的当前状态
// 状态枚举：
//   - submitted  : 已提交
//   - processing : 处理中
//   - succeed    : 生成成功，从 task_result.videos[0].url 获取视频链接
//   - failed     : 生成失败，从 task_status_msg 读取原因
func (c *KlingClient) CheckStatus(ctx context.Context, providerTaskID string) (*TaskStatus, error) {
	logger.Log.Infow("可灵: 查询任务状态", "provider_task_id", providerTaskID)

	realTaskID := providerTaskID
	reqPath := klingQueryImage2VideoPath

	if strings.HasPrefix(providerTaskID, "multi:") {
		reqPath = klingQueryMultiImage2VideoPath
		realTaskID = strings.TrimPrefix(providerTaskID, "multi:")
	} else if strings.HasPrefix(providerTaskID, "single:") {
		reqPath = klingQueryImage2VideoPath
		realTaskID = strings.TrimPrefix(providerTaskID, "single:")
	}

	url := fmt.Sprintf(c.baseURL+reqPath, realTaskID)
	var apiResp klingAPIResponse
	if err := c.doRequest(ctx, http.MethodGet, url, nil, &apiResp); err != nil {
		return nil, fmt.Errorf("可灵查询任务状态请求失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("可灵 API 错误 [%d]: %s", apiResp.Code, apiResp.Message)
	}

	var taskData klingTaskData
	if err := json.Unmarshal(apiResp.Data, &taskData); err != nil {
		return nil, fmt.Errorf("解析可灵任务状态响应失败: %w", err)
	}

	switch taskData.TaskStatus {
	case "succeed":
		videoURL := ""
		if taskData.TaskResult != nil && len(taskData.TaskResult.Videos) > 0 {
			videoURL = taskData.TaskResult.Videos[0].URL
		}
		logger.Log.Infow("可灵: 视频生成成功",
			"provider_task_id", providerTaskID,
			"video_url", videoURL,
		)
		return &TaskStatus{
			IsFinished: true,
			IsFailed:   false,
			VideoURL:   videoURL,
		}, nil

	case "failed":
		errMsg := taskData.TaskStatusMsg
		if errMsg == "" {
			errMsg = "未知错误"
		}
		logger.Log.Warnw("可灵: 视频生成失败",
			"provider_task_id", providerTaskID,
			"error", errMsg,
		)
		return &TaskStatus{
			IsFinished: true,
			IsFailed:   true,
			ErrorMsg:   errMsg,
		}, nil

	default:
		// "submitted" 或 "processing"
		logger.Log.Infow("可灵: 任务进行中",
			"provider_task_id", providerTaskID,
			"status", taskData.TaskStatus,
		)
		return &TaskStatus{
			IsFinished: false,
			IsFailed:   false,
		}, nil
	}
}

// ==================== 内部工具函数 ====================

// generateJWT 生成可灵认证用的 JWT Token
// 使用 HS256 算法，Access Key 作为 iss，Secret Key 作为签名密钥
func (c *KlingClient) generateJWT() (string, error) {
	now := time.Now().Unix()

	// Header
	header := klingJWTHeader{Alg: "HS256", Typ: "JWT"}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64URLEncode(headerJSON)

	// Payload
	payload := klingJWTPayload{
		Iss: c.accessKey,
		Exp: now + 1800, // 30 分钟有效期
		Nbf: now - 5,    // 允许 5 秒时钟偏移
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64URLEncode(payloadJSON)

	// Signature
	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(signingInput))
	signature := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

// doRequest 发送带 JWT 认证的 HTTP 请求
func (c *KlingClient) doRequest(ctx context.Context, method string, url string, body any, dest *klingAPIResponse) error {
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

	// 生成 JWT Token
	token, err := c.generateJWT()
	if err != nil {
		return fmt.Errorf("生成 JWT 失败: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
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

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API 返回状态码 %d，响应: %s", resp.StatusCode, string(respBytes))
	}

	if err := json.Unmarshal(respBytes, dest); err != nil {
		return fmt.Errorf("解析响应 JSON 失败: %w (原始响应: %s)", err, string(respBytes))
	}

	return nil
}

// imageToRawBase64 将本地图片读取并转换为纯 Base64 字符串（不含 data:image 前缀）
// 可灵 API 要求不能包含 "data:image/png;base64," 前缀
func imageToRawBase64(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取图片文件失败 (%s): %w", filePath, err)
	}

	// 检查文件大小限制（10MB）
	if len(data) > 10*1024*1024 {
		return "", fmt.Errorf("图片文件超过 10MB 限制 (%s): %d bytes", filepath.Base(filePath), len(data))
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// base64URLEncode Base64 URL 编码（JWT 标准要求）
func base64URLEncode(data []byte) string {
	s := base64.StdEncoding.EncodeToString(data)
	s = strings.TrimRight(s, "=")
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}
