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
