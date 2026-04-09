package video

import (
	"context"
	"fmt"
	"time"

	"video-max/pkg/logger"
)

// ByteDanceClient 字节跳动视频生成服务的 VideoProvider 具体实现
// 封装了与字节 PixelDance/Seedance API 交互的全部细节
type ByteDanceClient struct {
	apiKey  string
	baseURL string
}

// NewByteDanceClient 创建字节跳动客户端实例
func NewByteDanceClient(apiKey string, baseURL string) *ByteDanceClient {
	return &ByteDanceClient{
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

func (c *ByteDanceClient) Name() string {
	return ProviderByteDance
}

// GenerateVideo 向字节跳动 API 提交视频生成任务
// 内部流程：
// 1. 如果有参考图片，先通过 Assets 接口上传图片获取远端 ID
// 2. 使用上传后的图片 ID + Prompt 组装生成请求
// 3. 提交生成任务，返回外部任务 ID 供后续轮询
func (c *ByteDanceClient) GenerateVideo(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	logger.Log.Infow("字节跳动: 开始提交视频生成任务",
		"prompt_length", len(req.Prompt),
		"image_count", len(req.ImagePaths),
		"aspect_ratio", req.AspectRatio,
	)

	// TODO: 实现实际的 HTTP 请求逻辑
	// 步骤一: 上传图片素材（如果有）
	// 步骤二: 构造生成请求 JSON
	// 步骤三: 发送 POST 请求到字节 API
	// 步骤四: 解析响应获取 task_id

	// 当前为桩代码 (Stub)，返回模拟结果
	mockTaskID := fmt.Sprintf("bd_%d", time.Now().UnixMilli())
	logger.Log.Infow("字节跳动: 任务提交成功 (桩代码)",
		"provider_task_id", mockTaskID,
	)

	return &GenerateResult{
		ProviderTaskID:   mockTaskID,
		EstimatedWaitSec: 60,
	}, nil
}

// CheckStatus 查询字节跳动视频生成任务的当前状态
func (c *ByteDanceClient) CheckStatus(ctx context.Context, providerTaskID string) (*TaskStatus, error) {
	logger.Log.Infow("字节跳动: 查询任务状态",
		"provider_task_id", providerTaskID,
	)

	// TODO: 实现实际的状态查询 HTTP 请求
	// 发送 GET 请求到字节 API 查询 task_id 对应的任务状态
	// 解析响应判断是否完成、是否失败、获取视频 URL

	// 当前为桩代码 (Stub)，模拟返回已完成状态
	return &TaskStatus{
		IsFinished: true,
		IsFailed:   false,
		VideoURL:   "https://example.com/mock_video.mp4",
		ErrorMsg:   "",
	}, nil
}
