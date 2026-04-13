package repository

import (
	"context"

	"video-max/internal/domain/entity"
)

// TaskStats 用户任务使用统计
type TaskStats struct {
	Total           int64            `json:"total"`
	SuccessCount    int64            `json:"success_count"`
	FailedCount     int64            `json:"failed_count"`
	InProgressCount int64            `json:"in_progress_count"`
	ModelDist       map[string]int64 `json:"model_distribution"`
}

// TaskRepository 任务数据存储层的抽象接口
// 上层业务逻辑（如 Orchestrator、Handler）只依赖此接口，不关心底层是 MySQL、SQLite 还是其他存储
// 这使得我们可以在不修改业务代码的情况下更换数据库实现
type TaskRepository interface {
	// Create 创建一条新的视频生成任务记录
	Create(ctx context.Context, task *entity.Task) error

	// GetByID 根据任务 ID 查询任务详情（供用户轮询进度使用）
	GetByID(ctx context.Context, id string) (*entity.Task, error)

	// UpdateStatus 更新任务当前的流转状态（如 pending -> phase_story -> generating）
	UpdateStatus(ctx context.Context, id string, status string) error

	// SaveResult 当视频生成成功后，回写最终的视频 URL 和外部任务 ID
	SaveResult(ctx context.Context, id string, videoURL string, externalTaskID string) error

	// SaveEnhancedPrompt 当 MAS 多智能体处理完毕后，保存生成的专业提示词
	SaveEnhancedPrompt(ctx context.Context, id string, prompt string) error

	// MarkFailed 标记任务为失败状态，并记录错误信息
	MarkFailed(ctx context.Context, id string, errMsg string) error

	// ListByUserID 分页查询指定用户的任务列表，按创建时间倒序
	ListByUserID(ctx context.Context, userID string, offset, limit int) ([]*entity.Task, int64, error)

	// GetUserStats 查询指定用户的任务使用统计
	GetUserStats(ctx context.Context, userID string) (*TaskStats, error)
}
