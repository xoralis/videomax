package protocol

import "context"

// MASContext 多智能体协作的共享黑板 (Blackboard Pattern)
// 独立定义在 protocol 包中，避免 mas 与 protocol 之间的循环引用
// 这是五大 Agent 之间唯一的数据交换通道
type MASContext struct {
	// ===== 用户原始输入 (由 Orchestrator 初始化) =====

	// TaskID 当前任务的唯一标识
	TaskID string

	// UserIdea 用户提交的原始创意文本
	UserIdea string

	// Images 用户上传的参考图片本地路径列表
	Images []string

	// AspectRatio 用户选择的画面比例 (如 "16:9", "9:16")
	AspectRatio string

	// ===== Agent 逐步产出 (由各 Agent 依次填充) =====

	// Storyline 由 Story Agent 产出的剧情大纲（CoT 范式）
	Storyline string

	// Characters 由 Character Agent 产出的角色锚点描述（Vision 分析）
	Characters string

	// SceneList 由 Storyboard Agent 产出的分镜列表
	SceneList string

	// FinalPrompts 由 Visual Agent 产出、经 Critic Agent 审核通过的最终提示词
	FinalPrompts string

	// ===== 质检反馈区域 (由 Critic Agent 填充) =====

	// ReviewFeedback 质检 Agent 的审核反馈意见
	ReviewFeedback string

	// ReviewPassed 质检是否通过的标志位
	ReviewPassed bool
}

// Agent 多智能体系统中每一个独立代理人的通用接口约束
// 所有 Agent（故事、角色、分镜、画面、质检）都必须实现此接口
type Agent interface {
	// Name 返回当前 Agent 的名称标识（如 "StoryAgent"）
	Name() string

	// Process 执行当前 Agent 的核心逻辑
	// 传入共享黑板 MASContext，Agent 从中读取上游数据，并将自己的产出写回黑板
	Process(ctx context.Context, masCtx *MASContext) error
}
