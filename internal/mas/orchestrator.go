package mas

import (
	"context"
	"fmt"

	"video-max/internal/mas/protocol"
	"video-max/pkg/logger"
)

// Orchestrator 多智能体系统的中枢调度器
// 采用 Blackboard Pattern (黑板模式) 管理五大 Agent 的工作流
type Orchestrator struct {
	agents      []protocol.Agent
	visualAgent protocol.Agent
	criticAgent protocol.Agent
	maxRetries  int
}

func NewOrchestrator(maxRetries int) *Orchestrator {
	return &Orchestrator{
		agents:     make([]protocol.Agent, 0),
		maxRetries: maxRetries,
	}
}

func (o *Orchestrator) RegisterSequential(agent protocol.Agent) {
	o.agents = append(o.agents, agent)
}

func (o *Orchestrator) SetVisualAgent(agent protocol.Agent) {
	o.visualAgent = agent
}

func (o *Orchestrator) SetCriticAgent(agent protocol.Agent) {
	o.criticAgent = agent
}

// Run 启动多智能体协作流水线
// 执行流程：
// 1. 初始化共享黑板 (MASContext)
// 2. 依次调用 Story -> Character -> Storyboard Agent（线性流水线）
// 3. 进入 Visual <-> Critic 的质检循环（带打回重试机制）
// 4. 全部通过后返回完整的 MASContext
func (o *Orchestrator) Run(ctx context.Context, taskID string, userIdea string, images []string, aspectRatio string) (*protocol.MASContext, error) {
	masCtx := &protocol.MASContext{
		TaskID:      taskID,
		UserIdea:    userIdea,
		Images:      images,
		AspectRatio: aspectRatio,
	}

	logger.Log.Infow("Orchestrator: 多智能体协作流水线启动",
		"task_id", taskID, "agent_count", len(o.agents), "max_retries", o.maxRetries,
	)

	// 阶段一：线性流水线，依次执行前置 Agent
	for _, agent := range o.agents {
		if agent == o.visualAgent || agent == o.criticAgent {
			continue
		}
		logger.Log.Infow("Orchestrator: 调度 Agent", "task_id", taskID, "agent", agent.Name())
		if err := agent.Process(ctx, masCtx); err != nil {
			return nil, fmt.Errorf("Agent '%s' 执行失败: %w", agent.Name(), err)
		}
	}

	// 阶段二：Visual <-> Critic 质检循环 (Reflection 打回机制)
	if o.visualAgent == nil || o.criticAgent == nil {
		return masCtx, nil
	}

	for retry := 0; retry <= o.maxRetries; retry++ {
		logger.Log.Infow("Orchestrator: 调度 VisualAgent", "task_id", taskID, "attempt", retry+1)
		if err := o.visualAgent.Process(ctx, masCtx); err != nil {
			return nil, fmt.Errorf("VisualAgent 第 %d 次执行失败: %w", retry+1, err)
		}

		logger.Log.Infow("Orchestrator: 调度 CriticAgent 审查", "task_id", taskID, "attempt", retry+1)
		err := o.criticAgent.Process(ctx, masCtx)

		if err == nil && masCtx.ReviewPassed {
			logger.Log.Infow("Orchestrator: ✅ 质检通过，协作完成", "task_id", taskID, "total_attempts", retry+1)
			return masCtx, nil
		}

		if retry < o.maxRetries {
			logger.Log.Warnw("Orchestrator: ❌ 质检未通过，打回 VisualAgent",
				"task_id", taskID, "attempt", retry+1, "feedback_preview", truncate(masCtx.ReviewFeedback, 100),
			)
		}
	}

	logger.Log.Errorw("Orchestrator: 超过最大重试次数，强制使用当前版本", "task_id", taskID)
	return masCtx, nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
