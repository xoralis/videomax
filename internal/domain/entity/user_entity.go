package entity

import "time"

// User 用户实体，与 MySQL 中的 users 表一一映射
type User struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID       string    `gorm:"uniqueIndex;type:varchar(64);not null" json:"user_id"`       // 对外暴露的 UUID 标识
	Username     string    `gorm:"type:varchar(100);not null" json:"username"`
	Email        string    `gorm:"uniqueIndex;type:varchar(255);not null" json:"email"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
