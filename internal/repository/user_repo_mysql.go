package repository

import (
	"context"

	"gorm.io/gorm"

	"video-max/internal/domain/entity"
)

type mysqlUserRepo struct {
	db *gorm.DB
}

// NewMySQLUserRepo 创建基于 MySQL 的 UserRepository 实现
func NewMySQLUserRepo(db *gorm.DB) UserRepository {
	return &mysqlUserRepo{db: db}
}

func (r *mysqlUserRepo) Create(ctx context.Context, user *entity.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *mysqlUserRepo) GetByEmail(ctx context.Context, email string) (*entity.User, error) {
	var user entity.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *mysqlUserRepo) GetByUserID(ctx context.Context, userID string) (*entity.User, error) {
	var user entity.User
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
