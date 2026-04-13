package mas

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

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
	emitter     *EventEmitter // SSE 事件发射器（可选，nil 时不推送事件）
}

func NewOrchestrator(maxRetries int) *Orchestrator {
	return &Orchestrator{
		agents:     make([]protocol.Agent, 0),
		maxRetries: maxRetries,
	}
}

// SetEventEmitter 注入 SSE 事件发射器
func (o *Orchestrator) SetEventEmitter(emitter *EventEmitter) {
	o.emitter = emitter
}

// emit 安全地发射事件（emitter 为 nil 时静默跳过）
func (o *Orchestrator) emit(event AgentEvent) {
	if o.emitter != nil {
		o.emitter.Emit(event)
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

	tracer := otel.Tracer("videomax")
	ctx, pipelineSpan := tracer.Start(ctx, "MAS-Pipeline",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "chain"),
			attribute.String("task.id", taskID),
			attribute.String("user.idea", userIdea),
		))
	defer pipelineSpan.End()

	logger.Log.Infow("Orchestrator: 多智能体协作流水线启动",
		"task_id", taskID, "agent_count", len(o.agents), "max_retries", o.maxRetries,
	)

	// 阶段一：线性流水线，依次执行前置 Agent
	for _, agent := range o.agents {
		if agent == o.visualAgent || agent == o.criticAgent {
			continue
		}
		logger.Log.Infow("Orchestrator: 调度 Agent", "task_id", taskID, "agent", agent.Name())
		o.emit(AgentEvent{TaskID: taskID, AgentName: agent.Name(), Status: "running", Message: agent.Name() + " 正在工作..."})

		agentCtx, agentSpan := tracer.Start(ctx, agent.Name(),
			trace.WithAttributes(
				attribute.String("gen_ai.operation.name", "chain"),
				attribute.String("agent.name", agent.Name()),
			))
		if err := agent.Process(agentCtx, masCtx); err != nil {
			agentSpan.RecordError(err)
			agentSpan.SetStatus(codes.Error, err.Error())
			agentSpan.End()
			o.emit(AgentEvent{TaskID: taskID, AgentName: agent.Name(), Status: "error", Message: err.Error()})
			return nil, fmt.Errorf("Agent '%s' 执行失败: %w", agent.Name(), err)
		}
		agentSpan.End()
		o.emit(AgentEvent{TaskID: taskID, AgentName: agent.Name(), Status: "done", Message: agent.Name() + " 执行完成"})
	}

	// 阶段二：Visual <-> Critic 质检循环 (Reflection 打回机制)
	if o.visualAgent == nil || o.criticAgent == nil {
		return masCtx, nil
	}

	for retry := 0; retry <= o.maxRetries; retry++ {
		logger.Log.Infow("Orchestrator: 调度 VisualAgent", "task_id", taskID, "attempt", retry+1)
		o.emit(AgentEvent{TaskID: taskID, AgentName: "VisualAgent", Status: "running", Message: fmt.Sprintf("VisualAgent 正在构建提示词 (第 %d 轮)...", retry+1)})

		visualCtx, visualSpan := tracer.Start(ctx, fmt.Sprintf("VisualAgent-attempt-%d", retry+1),
			trace.WithAttributes(
				attribute.String("gen_ai.operation.name", "chain"),
				attribute.String("agent.name", "VisualAgent"),
				attribute.Int("agent.attempt", retry+1),
			))
		if err := o.visualAgent.Process(visualCtx, masCtx); err != nil {
			visualSpan.RecordError(err)
			visualSpan.SetStatus(codes.Error, err.Error())
			visualSpan.End()
			o.emit(AgentEvent{TaskID: taskID, AgentName: "VisualAgent", Status: "error", Message: err.Error()})
			return nil, fmt.Errorf("VisualAgent 第 %d 次执行失败: %w", retry+1, err)
		}
		visualSpan.End()
		o.emit(AgentEvent{TaskID: taskID, AgentName: "VisualAgent", Status: "done", Message: "VisualAgent 提示词构建完成"})

		logger.Log.Infow("Orchestrator: 调度 CriticAgent 审查", "task_id", taskID, "attempt", retry+1)
		o.emit(AgentEvent{TaskID: taskID, AgentName: "CriticAgent", Status: "running", Message: "CriticAgent 正在质检审核..."})

		criticCtx, criticSpan := tracer.Start(ctx, fmt.Sprintf("CriticAgent-attempt-%d", retry+1),
			trace.WithAttributes(
				attribute.String("gen_ai.operation.name", "chain"),
				attribute.String("agent.name", "CriticAgent"),
				attribute.Int("agent.attempt", retry+1),
			))
		err := o.criticAgent.Process(criticCtx, masCtx)

		if err == nil && masCtx.ReviewPassed {
			logger.Log.Infow("Orchestrator: ✅ 质检通过，协作完成", "task_id", taskID, "total_attempts", retry+1)
			criticSpan.End()
			o.emit(AgentEvent{TaskID: taskID, AgentName: "CriticAgent", Status: "done", Message: "✅ 质检通过"})
			o.emit(AgentEvent{TaskID: taskID, AgentName: "Pipeline", Status: "done", Message: "多智能体协作完成，提交视频生成..."})
			return masCtx, nil
		}

		if retry < o.maxRetries {
			logger.Log.Warnw("Orchestrator: ❌ 质检未通过，打回 VisualAgent",
				"task_id", taskID, "attempt", retry+1, "feedback_preview", truncate(masCtx.ReviewFeedback, 100),
			)
			criticSpan.SetAttributes(attribute.String("critic.feedback", masCtx.ReviewFeedback))
			criticSpan.End()
			o.emit(AgentEvent{TaskID: taskID, AgentName: "CriticAgent", Status: "rejected", Message: fmt.Sprintf("❌ 质检未通过，打回重试 (%d/%d)", retry+1, o.maxRetries)})
		} else {
			criticSpan.End()
		}
	}

	logger.Log.Errorw("Orchestrator: 超过最大重试次数，强制使用当前版本", "task_id", taskID)
	o.emit(AgentEvent{TaskID: taskID, AgentName: "Pipeline", Status: "done", Message: "提示词已生成（超过最大重试次数），提交视频生成..."})
	return masCtx, nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
