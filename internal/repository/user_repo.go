package repository

import (
	"context"

	"video-max/internal/domain/entity"
)

// UserRepository 用户数据存储层抽象接口
type UserRepository interface {
	// Create 创建新用户
	Create(ctx context.Context, user *entity.User) error

	// GetByEmail 根据邮箱查询用户（登录时使用）
	GetByEmail(ctx context.Context, email string) (*entity.User, error)

	// GetByUserID 根据对外 UUID 查询用户
	GetByUserID(ctx context.Context, userID string) (*entity.User, error)
}
