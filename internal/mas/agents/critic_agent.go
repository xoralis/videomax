package agents

import (
	"context"
	"fmt"
	"strings"

	"video-max/internal/mas/protocol"
	"video-max/pkg/llmclient"
	"video-max/pkg/logger"
)

// CriticAgent 质检/风格一致性智能体
// 采用 Reflection (反思) 范式运行
// 不生产原创内容，专门审查前面 Agent 产出的最终提示词
type CriticAgent struct {
	llm llmclient.LLMClient
}

func NewCriticAgent(llm llmclient.LLMClient) *CriticAgent {
	return &CriticAgent{llm: llm}
}

func (a *CriticAgent) Name() string {
	return "CriticAgent"
}

const criticSystemPrompt = `你是一个严苛的视频质检审核师（制片人）。你的职责不是创作内容，而是审查其他团队成员产出的视频生成提示词。

你将收到：故事大纲、角色设定、分镜表、以及最终的视频生成提示词。

你需要逐条审查以下维度：
1. 角色一致性: 提示词中的人物描述是否与角色设定卡一致？
2. 动作连贯性: 提示词是否串联包含了原本多个分镜中的核心动作片段？
3. 运镜与风格: 是否包含了运镜词？末尾是否统一了画质和风格修饰词（如 high quality, 4K）？
4. 参数合规性: 提示词是否是纯英文？且没有暴露 "Shot 1", "Shot 2" 之类割裂的系统标题？
5. 整体长短: 提示词是否看起来冗长重复？要求极其紧凑高效。

审核结果格式：
如果全部通过：
APPROVED

如果存在问题：
REJECTED
问题1: [具体指出的问题]
修改建议: [针对性的修改指导]
`

// Process 执行质检 Agent 的核心逻辑 (Reflection 反思范式)
func (a *CriticAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("CriticAgent: 开始质检审核 (Reflection 模式)", "task_id", masCtx.TaskID)
	logger.Log.Debugw("CriticAgent: 【输入数据】",
		"task_id", masCtx.TaskID,
		"input.Storyline", masCtx.Storyline,
		"input.Characters", masCtx.Characters,
		"input.SceneList", masCtx.SceneList,
		"input.FinalPrompts", masCtx.FinalPrompts,
	)

	userMsg := fmt.Sprintf(
		"=== 故事大纲 ===\n%s\n\n=== 角色设定 ===\n%s\n\n=== 分镜表 ===\n%s\n\n=== 待审核的视频生成提示词 ===\n%s\n\n请进行严格审查。",
		masCtx.Storyline, masCtx.Characters, masCtx.SceneList, masCtx.FinalPrompts,
	)

	resp, err := a.llm.Chat(ctx, llmclient.ChatRequest{
		SystemPrompt: criticSystemPrompt,
		UserMessage:  userMsg,
	})
	if err != nil {
		return fmt.Errorf("CriticAgent 调用大模型失败: %w", err)
	}

	reviewResult := strings.TrimSpace(resp.Content)

	if strings.HasPrefix(strings.ToUpper(reviewResult), "APPROVED") {
		masCtx.ReviewFeedback = ""
		masCtx.ReviewPassed = true
		logger.Log.Infow("CriticAgent: ✅ 质检通过", "task_id", masCtx.TaskID)
		logger.Log.Debugw("CriticAgent: 【输出数据】",
			"task_id", masCtx.TaskID,
			"output.ReviewPassed", masCtx.ReviewPassed,
			"output.ReviewFeedback", masCtx.ReviewFeedback,
		)
		return nil
	}

	masCtx.ReviewFeedback = reviewResult
	masCtx.ReviewPassed = false
	logger.Log.Warnw("CriticAgent: ❌ 质检不通过，需要打回", "task_id", masCtx.TaskID)
	logger.Log.Debugw("CriticAgent: 【输出数据】",
		"task_id", masCtx.TaskID,
		"output.ReviewPassed", masCtx.ReviewPassed,
		"output.ReviewFeedback", masCtx.ReviewFeedback,
	)
	return fmt.Errorf("质检不通过: %s", reviewResult)
}
