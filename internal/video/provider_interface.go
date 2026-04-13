package video

import "context"

// GenerateRequest 视频生成请求的统一入参结构
// 无论底层对接的是字节、可灵还是 Runway，上层只需要按照此格式组装参数
type GenerateRequest struct {
	// Prompt MAS 多智能体协作最终产出的专业提示词
	Prompt string

	// ImagePaths 用户上传的参考图片在本地的文件路径列表
	// 视频提供商的实现层负责将这些文件上传至各自的 Assets 接口
	ImagePaths []string

	// AspectRatio 视频画面比例，如 "16:9" 或 "9:16"
	AspectRatio string

	// Duration 期望的视频时长（秒），支持 5 或 10
	Duration int
}

// GenerateResult 视频生成请求提交后的响应结构
type GenerateResult struct {
	// ProviderTaskID 视频服务商返回的外部任务 ID，用于后续轮询生成状态
	ProviderTaskID string

	// EstimatedWaitSec 服务商建议的预估等待时间（秒）
	EstimatedWaitSec int
}

// TaskStatus 视频生成任务的当前状态查询结果
type TaskStatus struct {
	// IsFinished 任务是否已经完成（无论成功或失败）
	IsFinished bool

	// IsFailed 任务是否失败
	IsFailed bool

	// VideoURL 当任务成功完成时，此字段包含视频的可下载链接
	VideoURL string

	// ErrorMsg 当任务失败时，包含失败原因描述
	ErrorMsg string
}

// VideoProvider 视频生成服务提供商的统一抽象接口
// 采用工厂模式（Factory Pattern），通过此接口解耦上层业务与具体厂商的 API 实现
// 任何新增的视频供应商只需要实现以下三个方法，即可无缝接入系统
type VideoProvider interface {
	// Name 返回当前提供商的名称标识（如 "bytedance", "kling"）
	Name() string

	// GenerateVideo 向提供商提交一个视频生成任务
	// 内部负责处理图片上传（如有）、参数组装、HTTP 请求发送等细节
	GenerateVideo(ctx context.Context, req GenerateRequest) (*GenerateResult, error)

	// CheckStatus 根据外部任务 ID 查询视频生成进度
	// 由 Worker 定期轮询调用，直到任务完成或超时
	CheckStatus(ctx context.Context, providerTaskID string) (*TaskStatus, error)
}
