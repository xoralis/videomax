package entity

import "time"

// TaskStatus 任务状态枚举常量
// 用于在数据库和业务逻辑中追踪一个视频生成任务的完整生命周期
const (
	TaskStatusPending    = "pending"      // 刚创建，等待 Kafka 消费
	TaskStatusStory      = "phase_story"  // 故事策划 Agent 正在工作
	TaskStatusCharacter  = "phase_char"   // 角色设定 Agent 正在工作
	TaskStatusStoryboard = "phase_board"  // 分镜规划 Agent 正在工作
	TaskStatusVisual     = "phase_visual" // 画面提示词 Agent 正在工作
	TaskStatusReview     = "phase_review" // 质检 Agent 正在审核
	TaskStatusGenerating = "generating"   // 已提交至视频大厂 API，等待生成
	TaskStatusSuccess    = "success"      // 视频生成成功
	TaskStatusFailed     = "failed"       // 流程中某个环节失败
)

// Task 核心任务实体，与 MySQL 中的 tasks 表一一映射
// 记录了一个视频生成请求从创建到完成的全部关键信息
type Task struct {
	ID             string    `gorm:"primaryKey;type:varchar(36)" json:"id"`          // UUID 主键
	UserID         string    `gorm:"index;type:varchar(64)" json:"user_id"`          // 用户标识（预留）
	Type           string    `gorm:"type:varchar(10)" json:"type"`                   // 任务类型: t2v(纯文生视频) / i2v(图生视频)
	OriginalIdea   string    `gorm:"type:text" json:"original_idea"`                 // 用户原始输入的文本描述
	InputImages    string    `gorm:"type:text" json:"input_images"`                  // 用户上传的参考图片路径列表 (JSON 数组格式)
	EnhancedPrompt string    `gorm:"type:text" json:"enhanced_prompt"`               // MAS 多智能体协作后生成的最终专业提示词
	Status         string    `gorm:"type:varchar(20);default:pending" json:"status"` // 当前任务状态（参见 TaskStatus 常量）
	VideoURL       string    `gorm:"type:varchar(1024)" json:"video_url"`            // 最终可供下载的视频链接
	ExternalTaskID string    `gorm:"type:varchar(128)" json:"external_task_id"`      // 第三方视频服务商返回的外部任务 ID
	ErrorMsg       string    `gorm:"type:text" json:"error_msg"`                     // 如果失败，记录具体错误原因
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
