package dto

// VideoCreateRequest 视频生成接口的请求体约束
// 前端通过 multipart/form-data 格式提交图片和文本
type VideoCreateRequest struct {
	Idea        string   `form:"idea" binding:"required"`                                              // 用户输入的创意文本（必填）
	AspectRatio string   `form:"aspect_ratio" binding:"oneof=16:9 9:16 1:1 4:3 3:4"`                  // 画面比例（强制枚举校验）
	Duration    int      `form:"duration" binding:"required,oneof=5 10"`                               // 期望视频时长（秒），支持 5 或 10
	Model       string   `form:"model" binding:"required,oneof=doubao-seedance-1-0-pro-250528 kling-v1-6 hunyuan-video"` // 视频生成模型标识
	ImagePaths  []string `form:"-"`                                                                    // 由 Handler 解析 multipart 文件后填充的本地路径列表（不从表单直接绑定）
}

// VideoCreateResponse 视频生成接口的统一响应体
type VideoCreateResponse struct {
	Code   int    `json:"code"`    // 业务状态码：0 成功，非0 失败
	TaskID string `json:"task_id"` // 返回给前端用于轮询进度的唯一任务 ID
	Msg    string `json:"msg"`     // 人类可读的描述信息
}

// TaskQueryResponse 任务状态查询接口的响应体
// 前端通过 GET /api/task/:id 轮询此接口获取视频生成进度
type TaskQueryResponse struct {
	Code     int    `json:"code"`
	TaskID   string `json:"task_id"`
	Status   string `json:"status"`    // 当前状态：pending / phase_story / generating / success / failed 等
	VideoURL string `json:"video_url"` // 当 status=success 时，此字段包含可下载的视频链接
	Msg      string `json:"msg"`
}
