package repository

import (
	"context"

	"gorm.io/gorm"

	"video-max/internal/domain/entity"
)

// mysqlTaskRepo 基于 MySQL + GORM 的 TaskRepository 具体实现
type mysqlTaskRepo struct {
	db *gorm.DB
}

// NewMySQLTaskRepo 创建一个基于 MySQL 的 TaskRepository 实现实例
func NewMySQLTaskRepo(db *gorm.DB) TaskRepository {
	return &mysqlTaskRepo{db: db}
}

// Create 向 tasks 表中插入一条新记录
func (r *mysqlTaskRepo) Create(ctx context.Context, task *entity.Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID 根据主键查询单条任务记录
func (r *mysqlTaskRepo) GetByID(ctx context.Context, id string) (*entity.Task, error) {
	var task entity.Task
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// UpdateStatus 仅更新任务的 status 字段
func (r *mysqlTaskRepo) UpdateStatus(ctx context.Context, id string, status string) error {
	return r.db.WithContext(ctx).Model(&entity.Task{}).Where("id = ?", id).Update("status", status).Error
}

// SaveResult 视频生成成功后，同时更新 video_url、external_task_id 和 status
func (r *mysqlTaskRepo) SaveResult(ctx context.Context, id string, videoURL string, externalTaskID string) error {
	return r.db.WithContext(ctx).Model(&entity.Task{}).Where("id = ?", id).Updates(map[string]interface{}{
		"video_url":        videoURL,
		"external_task_id": externalTaskID,
		"status":           entity.TaskStatusSuccess,
	}).Error
}

// SaveEnhancedPrompt 保存 MAS 多智能体协作产出的最终提示词
func (r *mysqlTaskRepo) SaveEnhancedPrompt(ctx context.Context, id string, prompt string) error {
	return r.db.WithContext(ctx).Model(&entity.Task{}).Where("id = ?", id).Update("enhanced_prompt", prompt).Error
}

// MarkFailed 将任务标记为失败，同时记录错误原因
func (r *mysqlTaskRepo) MarkFailed(ctx context.Context, id string, errMsg string) error {
	return r.db.WithContext(ctx).Model(&entity.Task{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":    entity.TaskStatusFailed,
		"error_msg": errMsg,
	}).Error
}

// ListByUserID 分页查询指定用户的任务列表，按创建时间倒序
func (r *mysqlTaskRepo) ListByUserID(ctx context.Context, userID string, offset, limit int) ([]*entity.Task, int64, error) {
	var tasks []*entity.Task
	var total int64

	db := r.db.WithContext(ctx).Model(&entity.Task{}).Where("user_id = ?", userID)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Order("created_at DESC").Offset(offset).Limit(limit).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// GetUserStats 查询指定用户的任务使用统计
func (r *mysqlTaskRepo) GetUserStats(ctx context.Context, userID string) (*TaskStats, error) {
	type statusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []statusCount
	if err := r.db.WithContext(ctx).Model(&entity.Task{}).
		Select("status, count(*) as count").
		Where("user_id = ?", userID).
		Group("status").
		Scan(&statusCounts).Error; err != nil {
		return nil, err
	}

	stats := &TaskStats{ModelDist: make(map[string]int64)}
	for _, sc := range statusCounts {
		stats.Total += sc.Count
		switch sc.Status {
		case entity.TaskStatusSuccess:
			stats.SuccessCount = sc.Count
		case entity.TaskStatusFailed:
			stats.FailedCount = sc.Count
		default:
			stats.InProgressCount += sc.Count
		}
	}

	// 模型分布统计
	type modelCount struct {
		Model string
		Count int64
	}
	var modelCounts []modelCount
	if err := r.db.WithContext(ctx).Model(&entity.Task{}).
		Select("model, count(*) as count").
		Where("user_id = ? AND model != ''", userID).
		Group("model").
		Scan(&modelCounts).Error; err != nil {
		return nil, err
	}
	for _, mc := range modelCounts {
		stats.ModelDist[mc.Model] = mc.Count
	}

	return stats, nil
}
