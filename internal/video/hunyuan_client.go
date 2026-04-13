package video

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"video-max/pkg/logger"
)

// 腾讯混元视频生成 API 常量
// 文档参考：https://cloud.tencent.com/document/product/1616/124465
const (
	hunyuanHost      = "vclm.tencentcloudapi.com"
	hunyuanEndpoint  = "https://vclm.tencentcloudapi.com"
	hunyuanRegion    = "ap-guangzhou"
	hunyuanVersion   = "2024-05-23"
	hunyuanService   = "vclm"
	hunyuanAlgorithm = "TC3-HMAC-SHA256"

	// HY 图生视频接口 Action
	hunyuanActionSubmit = "SubmitImageToVideoGeneralJob"
	hunyuanActionQuery  = "DescribeImageToVideoGeneralJob"

	// 接口 Prompt 字符上限（UTF-8 字符数）
	hunyuanMaxPromptRunes = 2000
)

// ==================== 请求/响应结构体 ====================

// hunyuanImageField 图片字段（支持 URL 或 Base64）
type hunyuanImageField struct {
	Url    string `json:"Url,omitempty"`    // 图片 URL（与 Base64 二选一）
	Base64 string `json:"Base64,omitempty"` // 图片 Base64 编码（不含 data: 前缀）
}

// hunyuanSubmitReq 提交图生视频任务请求体
type hunyuanSubmitReq struct {
	Image      hunyuanImageField `json:"Image"`
	Prompt     string            `json:"Prompt,omitempty"`     // 视频生成提示词，≤2000 UTF-8 字符
	Resolution string            `json:"Resolution,omitempty"` // 分辨率：480p/720p/1080p，默认 720p
	Fps        int               `json:"Fps,omitempty"`        // 帧率：16/24/30，默认 24
}

// hunyuanSubmitResp 提交任务响应
type hunyuanSubmitResp struct {
	Response struct {
		JobId     string `json:"JobId"`
		RequestId string `json:"RequestId"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
	} `json:"Response"`
}

// hunyuanQueryReq 查询任务状态请求体
type hunyuanQueryReq struct {
	JobId string `json:"JobId"`
}

// hunyuanQueryResp 查询任务状态响应
type hunyuanQueryResp struct {
	Response struct {
		Status       string `json:"Status"` // WAIT / RUN / FAIL / DONE
		ErrorCode    string `json:"ErrorCode"`
		ErrorMessage string `json:"ErrorMessage"`
		ResultUrl    string `json:"ResultUrl"` // 视频 URL，成功后有效 24 小时
		RequestId    string `json:"RequestId"`
	} `json:"Response"`
}

// ==================== HunyuanClient ====================

// HunyuanClient 腾讯混元视频生成服务的 VideoProvider 实现
// 使用 TC3-HMAC-SHA256 鉴权（不依赖官方 SDK，手动实现签名）
// APIKey 格式为 "SecretId:SecretKey"（与 Kling 的 AK:SK 格式相同）
type HunyuanClient struct {
	secretID   string
	secretKey  string
	model      string // 如 "hunyuan-video"
	httpClient *http.Client
}

// NewHunyuanClient 创建腾讯混元客户端实例
//
// 参数说明：
//   - apiKey: 格式为 "SecretId:SecretKey"（用冒号分隔）
//   - _: baseURL 占位（混元固定使用官方域名，忽略此参数）
//   - model: 模型标识，如 "hunyuan-video"
func NewHunyuanClient(apiKey, _ string, model string) *HunyuanClient {
	parts := strings.SplitN(apiKey, ":", 2)
	secretID, secretKey := "", ""
	if len(parts) == 2 {
		secretID = parts[0]
		secretKey = parts[1]
	}
	return &HunyuanClient{
		secretID:   secretID,
		secretKey:  secretKey,
		model:      model,
		httpClient: &http.Client{Timeout: 0}, // 由 context 控制超时
	}
}

// Name 返回 Provider 名称标识
func (c *HunyuanClient) Name() string {
	return ProviderHunyuan
}

// ==================== TC3-HMAC-SHA256 签名实现 ====================

// sha256hex 对字符串做 SHA256 哈希并返回十六进制小写字符串
func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// hmacSHA256Bytes 返回原始字节的 HMAC-SHA256 结果（用于派生密钥的链式调用）
func hmacSHA256Bytes(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}

// buildAuthHeaders 为指定 Action 和请求体构建腾讯云 TC3-HMAC-SHA256 签名所需的全部 HTTP 头部
//
// 签名流程：
//  1. 拼接规范请求串（CanonicalRequest）
//  2. 拼接待签名字符串（StringToSign）
//  3. 派生签名密钥并计算签名（HMAC 链：TC3+SK → date → service → tc3_request → signature）
//  4. 拼接 Authorization Header
func (c *HunyuanClient) buildAuthHeaders(action, payload string) (map[string]string, error) {
	timestamp := time.Now().Unix()
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	contentType := "application/json; charset=utf-8"

	// 步骤 1：规范请求串
	canonicalHeaders := fmt.Sprintf(
		"content-type:%s\nhost:%s\nx-tc-action:%s\n",
		contentType, hunyuanHost, strings.ToLower(action),
	)
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256hex(payload)
	canonicalRequest := strings.Join([]string{
		"POST", "/", "",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	// 步骤 2：待签名字符串
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, hunyuanService)
	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		hunyuanAlgorithm, timestamp, credentialScope, sha256hex(canonicalRequest))

	// 步骤 3：计算签名（密钥派生链）
	secretDate := hmacSHA256Bytes([]byte("TC3"+c.secretKey), date)
	secretService := hmacSHA256Bytes(secretDate, hunyuanService)
	secretSigning := hmacSHA256Bytes(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256Bytes(secretSigning, stringToSign))

	// 步骤 4：Authorization
	authorization := fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		hunyuanAlgorithm, c.secretID, credentialScope, signedHeaders, signature,
	)

	return map[string]string{
		"Authorization":  authorization,
		"Content-Type":   contentType,
		"Host":           hunyuanHost,
		"X-TC-Action":    action,
		"X-TC-Version":   hunyuanVersion,
		"X-TC-Timestamp": fmt.Sprintf("%d", timestamp),
		"X-TC-Region":    hunyuanRegion,
	}, nil
}

// doPost 向腾讯混元 API 发送 POST 请求并返回原始响应体
func (c *HunyuanClient) doPost(ctx context.Context, action string, body interface{}) ([]byte, error) {
	payloadBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}
	payload := string(payloadBytes)

	headers, err := c.buildAuthHeaders(action, payload)
	if err != nil {
		return nil, fmt.Errorf("构建签名头部失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hunyuanEndpoint, bytes.NewBufferString(payload))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码异常 [%d]: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// ==================== VideoProvider 接口实现 ====================

// GenerateVideo 提交图生视频任务（SubmitImageToVideoGeneralJob）
//
// 约束：
//   - req.ImagePaths 不能为空，混元为图生视频模型（必须提供参考图片）
//   - Prompt 超过 2000 个 UTF-8 字符时自动截断
func (c *HunyuanClient) GenerateVideo(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	tracer := otel.Tracer("hunyuan-client")
	ctx, span := tracer.Start(ctx, "HunyuanClient.GenerateVideo",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	span.SetAttributes(attribute.String("provider", ProviderHunyuan))

	if len(req.ImagePaths) == 0 {
		err := fmt.Errorf("混元模型为图生视频模型，必须提供至少一张参考图片")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// 读取第一张图片并转 Base64
	imgData, err := os.ReadFile(req.ImagePaths[0])
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("读取图片文件失败 [%s]: %w", req.ImagePaths[0], err)
	}
	imgBase64 := base64.StdEncoding.EncodeToString(imgData)

	// 截断超长 Prompt（混元 API 限制 ≤200 UTF-8 字符）
	prompt := truncateRuneString(req.Prompt, hunyuanMaxPromptRunes)
	if utf8.RuneCountInString(req.Prompt) > hunyuanMaxPromptRunes {
		logger.Log.Warnw("混元 Prompt 已截断至200字符",
			"original_len", utf8.RuneCountInString(req.Prompt),
		)
	}

	submitReq := hunyuanSubmitReq{
		Image: hunyuanImageField{
			Base64: imgBase64,
		},
		Prompt:     prompt,
		Resolution: "720p",
		Fps:        24,
	}

	span.SetAttributes(attribute.String("resolution", submitReq.Resolution))

	respBody, err := c.doPost(ctx, hunyuanActionSubmit, submitReq)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var submitResp hunyuanSubmitResp
	if err := json.Unmarshal(respBody, &submitResp); err != nil {
		return nil, fmt.Errorf("解析提交任务响应失败: %w, body=%s", err, string(respBody))
	}
	if submitResp.Response.Error != nil {
		err := fmt.Errorf("混元 API 错误 [%s]: %s",
			submitResp.Response.Error.Code, submitResp.Response.Error.Message)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if submitResp.Response.JobId == "" {
		err := fmt.Errorf("混元 API 未返回 JobId，响应体: %s", string(respBody))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	logger.Log.Infow("混元视频任务已提交",
		"job_id", submitResp.Response.JobId,
	)
	span.SetAttributes(attribute.String("job_id", submitResp.Response.JobId))
	span.SetStatus(codes.Ok, "任务提交成功")

	return &GenerateResult{
		ProviderTaskID:   submitResp.Response.JobId,
		EstimatedWaitSec: 120,
	}, nil
}

// CheckStatus 查询图生视频任务状态（DescribeImageToVideoGeneralJob）
func (c *HunyuanClient) CheckStatus(ctx context.Context, providerTaskID string) (*TaskStatus, error) {
	tracer := otel.Tracer("hunyuan-client")
	ctx, span := tracer.Start(ctx, "HunyuanClient.CheckStatus",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	span.SetAttributes(
		attribute.String("provider", ProviderHunyuan),
		attribute.String("job_id", providerTaskID),
	)

	queryReq := hunyuanQueryReq{JobId: providerTaskID}
	respBody, err := c.doPost(ctx, hunyuanActionQuery, queryReq)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var queryResp hunyuanQueryResp
	if err := json.Unmarshal(respBody, &queryResp); err != nil {
		return nil, fmt.Errorf("解析查询响应失败: %w, body=%s", err, string(respBody))
	}
	if queryResp.Response.ErrorCode != "" {
		err := fmt.Errorf("混元查询 API 错误 [%s]: %s",
			queryResp.Response.ErrorCode, queryResp.Response.ErrorMessage)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	status := &TaskStatus{}
	switch queryResp.Response.Status {
	case "DONE":
		status.IsFinished = true
		status.VideoURL = queryResp.Response.ResultUrl
	case "FAIL":
		status.IsFinished = true
		status.IsFailed = true
		status.ErrorMsg = fmt.Sprintf("[%s] %s",
			queryResp.Response.ErrorCode, queryResp.Response.ErrorMessage)
	case "WAIT", "RUN":
		// 仍在处理中
	default:
		logger.Log.Warnw("混元返回未知任务状态",
			"status", queryResp.Response.Status,
		)
	}

	span.SetAttributes(attribute.String("task_status", queryResp.Response.Status))
	span.SetStatus(codes.Ok, "查询成功")
	return status, nil
}

// truncateRuneString 将字符串截断至最多 maxRunes 个 Unicode 字符
func truncateRuneString(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}
