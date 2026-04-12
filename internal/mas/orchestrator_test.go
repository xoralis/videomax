package mas

import (
	"context"
	"fmt"
	"os"
	"testing"

	"video-max/internal/mas/protocol"
	"video-max/pkg/config"
	"video-max/pkg/logger"
)

// TestMain 在所有测试执行前初始化全局日志器（避免 nil pointer panic）
func TestMain(m *testing.M) {
	_ = logger.Init(config.LogConfig{Level: "error", Mode: "console"})
	os.Exit(m.Run())
}

// ==================== Mock Agent ====================

// mockAgent 是一个可编程的 Agent 桩
// 通过 processFunc 控制 Agent 的行为（成功/失败/写入特定字段）
type mockAgent struct {
	name        string
	processFunc func(ctx context.Context, masCtx *protocol.MASContext) error
	callCount   int // 记录被调用次数
}

func (m *mockAgent) Name() string { return m.name }

func (m *mockAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	m.callCount++
	if m.processFunc != nil {
		return m.processFunc(ctx, masCtx)
	}
	return nil
}

// ==================== 流水线基础流程测试 ====================

// TestOrchestrator_LinearPipeline 测试线性流水线的基本执行顺序
// 验证 Story → Character → Storyboard 三个 Agent 按注册顺序依次执行
func TestOrchestrator_LinearPipeline(t *testing.T) {
	// 记录执行顺序
	var executionOrder []string

	storyAgent := &mockAgent{
		name: "StoryAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			executionOrder = append(executionOrder, "story")
			m.Storyline = "测试大纲"
			return nil
		},
	}
	charAgent := &mockAgent{
		name: "CharacterAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			executionOrder = append(executionOrder, "char")
			// 验证上游 StoryAgent 已产出
			if m.Storyline == "" {
				return fmt.Errorf("CharacterAgent 未收到上游 Storyline")
			}
			m.Characters = "角色A：红发少女"
			return nil
		},
	}
	boardAgent := &mockAgent{
		name: "StoryboardAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			executionOrder = append(executionOrder, "board")
			m.SceneList = "Shot 1: 起幅"
			return nil
		},
	}

	// VisualAgent 直接产出最终提示词，不触发打回
	visualAgent := &mockAgent{
		name: "VisualAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			m.FinalPrompts = "Final prompt output"
			return nil
		},
	}
	// CriticAgent 直接通过
	criticAgent := &mockAgent{
		name: "CriticAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			m.ReviewPassed = true
			return nil
		},
	}

	orch := NewOrchestrator(3)
	orch.RegisterSequential(storyAgent)
	orch.RegisterSequential(charAgent)
	orch.RegisterSequential(boardAgent)
	orch.SetVisualAgent(visualAgent)
	orch.SetCriticAgent(criticAgent)

	result, err := orch.Run(context.Background(), "test-001", "测试创意", nil, "16:9")
	if err != nil {
		t.Fatalf("Orchestrator.Run() 意外失败: %v", err)
	}

	// 验证执行顺序
	expected := []string{"story", "char", "board"}
	if len(executionOrder) != len(expected) {
		t.Fatalf("执行顺序长度不匹配: got %v, want %v", executionOrder, expected)
	}
	for i, name := range expected {
		if executionOrder[i] != name {
			t.Errorf("执行顺序[%d]: got %q, want %q", i, executionOrder[i], name)
		}
	}

	// 验证黑板数据完整性
	if result.Storyline != "测试大纲" {
		t.Errorf("Storyline: got %q, want %q", result.Storyline, "测试大纲")
	}
	if result.FinalPrompts != "Final prompt output" {
		t.Errorf("FinalPrompts: got %q, want %q", result.FinalPrompts, "Final prompt output")
	}
	if !result.ReviewPassed {
		t.Error("ReviewPassed: got false, want true")
	}
}

// ==================== 质检打回重试测试 ====================

// TestOrchestrator_CriticRejectAndRetry 测试 Critic 打回后 VisualAgent 被重新调度
// 验证第一次 Critic 打回，第二次通过的场景
func TestOrchestrator_CriticRejectAndRetry(t *testing.T) {
	// 跳过线性流水线 Agent（空实现）
	storyAgent := &mockAgent{name: "StoryAgent"}

	visualAgent := &mockAgent{
		name: "VisualAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			if m.ReviewFeedback != "" {
				// 第二次执行：根据反馈修改后的提示词
				m.FinalPrompts = "Improved prompt after feedback"
			} else {
				// 第一次执行：初始提示词
				m.FinalPrompts = "Initial bad prompt"
			}
			return nil
		},
	}

	criticCallCount := 0
	criticAgent := &mockAgent{
		name: "CriticAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			criticCallCount++
			if criticCallCount == 1 {
				// 第一次审查：打回
				m.ReviewPassed = false
				m.ReviewFeedback = "REJECTED\n缺少运镜关键词"
				return fmt.Errorf("质检不通过")
			}
			// 第二次审查：通过
			m.ReviewPassed = true
			m.ReviewFeedback = ""
			return nil
		},
	}

	orch := NewOrchestrator(3)
	orch.RegisterSequential(storyAgent)
	orch.SetVisualAgent(visualAgent)
	orch.SetCriticAgent(criticAgent)

	result, err := orch.Run(context.Background(), "test-002", "创意", nil, "16:9")
	if err != nil {
		t.Fatalf("Run() 错误: %v", err)
	}

	// VisualAgent 应被调用 2 次
	if visualAgent.callCount != 2 {
		t.Errorf("VisualAgent 调用次数: got %d, want 2", visualAgent.callCount)
	}

	// CriticAgent 应被调用 2 次
	if criticCallCount != 2 {
		t.Errorf("CriticAgent 调用次数: got %d, want 2", criticCallCount)
	}

	// 最终应使用改进后的提示词
	if result.FinalPrompts != "Improved prompt after feedback" {
		t.Errorf("FinalPrompts: got %q, want %q", result.FinalPrompts, "Improved prompt after feedback")
	}

	if !result.ReviewPassed {
		t.Error("ReviewPassed: 最终应为 true")
	}
}

// ==================== 最大重试次数测试 ====================

// TestOrchestrator_MaxRetriesExhausted 测试超过最大重试次数后强制使用当前版本
func TestOrchestrator_MaxRetriesExhausted(t *testing.T) {
	storyAgent := &mockAgent{name: "StoryAgent"}

	visualAgent := &mockAgent{
		name: "VisualAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			m.FinalPrompts = "Still not good enough"
			return nil
		},
	}

	// Critic 永远打回
	criticAgent := &mockAgent{
		name: "CriticAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			m.ReviewPassed = false
			m.ReviewFeedback = "REJECTED\n依然不满足要求"
			return fmt.Errorf("质检不通过")
		},
	}

	maxRetries := 2
	orch := NewOrchestrator(maxRetries)
	orch.RegisterSequential(storyAgent)
	orch.SetVisualAgent(visualAgent)
	orch.SetCriticAgent(criticAgent)

	result, err := orch.Run(context.Background(), "test-003", "创意", nil, "16:9")
	// 超过最大重试后应该不报错，而是强制使用当前版本
	if err != nil {
		t.Fatalf("超过最大重试后不应报错: %v", err)
	}

	// VisualAgent 应被调用 maxRetries+1 次
	expectedCalls := maxRetries + 1
	if visualAgent.callCount != expectedCalls {
		t.Errorf("VisualAgent 调用次数: got %d, want %d", visualAgent.callCount, expectedCalls)
	}

	// 应带有未通过的提示词内容（强制使用）
	if result.FinalPrompts != "Still not good enough" {
		t.Errorf("FinalPrompts: got %q, want %q", result.FinalPrompts, "Still not good enough")
	}
}

// ==================== 线性流水线中途失败测试 ====================

// TestOrchestrator_AgentFailure 测试某个前置 Agent 失败时，整个流水线中断
func TestOrchestrator_AgentFailure(t *testing.T) {
	storyAgent := &mockAgent{
		name: "StoryAgent",
		processFunc: func(_ context.Context, _ *protocol.MASContext) error {
			return fmt.Errorf("LLM API 调用超时")
		},
	}
	charAgent := &mockAgent{name: "CharacterAgent"}

	orch := NewOrchestrator(1)
	orch.RegisterSequential(storyAgent)
	orch.RegisterSequential(charAgent)

	_, err := orch.Run(context.Background(), "test-004", "创意", nil, "16:9")
	if err == nil {
		t.Fatal("期望获得错误，但返回 nil")
	}

	// CharacterAgent 不应被调用
	if charAgent.callCount != 0 {
		t.Errorf("CharacterAgent 不应被调用, 但被调用了 %d 次", charAgent.callCount)
	}
}

// ==================== Blackboard 数据传递测试 ====================

// TestOrchestrator_BlackboardDataFlow 测试黑板模式下的数据传递完整性
// 验证用户输入的原始数据在整个流水线中都可访问
func TestOrchestrator_BlackboardDataFlow(t *testing.T) {
	images := []string{"uploads/ref_1.png", "uploads/ref_2.png"}
	var capturedCtx *protocol.MASContext

	storyAgent := &mockAgent{
		name: "StoryAgent",
		processFunc: func(_ context.Context, m *protocol.MASContext) error {
			capturedCtx = m
			m.Storyline = "大纲"
			return nil
		},
	}

	orch := NewOrchestrator(0)
	orch.RegisterSequential(storyAgent)

	_, err := orch.Run(context.Background(), "test-005", "用户创意", images, "9:16")
	if err != nil {
		t.Fatalf("Run() 错误: %v", err)
	}

	// 验证黑板初始化的数据
	if capturedCtx.TaskID != "test-005" {
		t.Errorf("TaskID: got %q, want %q", capturedCtx.TaskID, "test-005")
	}
	if capturedCtx.UserIdea != "用户创意" {
		t.Errorf("UserIdea: got %q, want %q", capturedCtx.UserIdea, "用户创意")
	}
	if len(capturedCtx.Images) != 2 {
		t.Errorf("Images: got %d, want 2", len(capturedCtx.Images))
	}
	if capturedCtx.AspectRatio != "9:16" {
		t.Errorf("AspectRatio: got %q, want %q", capturedCtx.AspectRatio, "9:16")
	}
}
