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

1. 角色一致性: 提示词中的人物描述是否与角色设定卡一致？有没有遗漏关键外貌锚点？
2. 风格连贯性: 各个 Shot 之间的色调、光影、画风是否统一？有没有突然跳变？
3. 运镜完整性: 每个 Shot 是否都包含了运镜关键词？是否有纯静态描述（不可接受）？
4. 参数合规性: 提示词是否使用了英文？是否包含画质修饰词（如 high quality, 4K）？
5. 过渡自然性: 相邻 Shot 之间的画面过渡是否有逻辑承接？

审核结果格式：

如果全部通过：
APPROVED

如果存在问题：
REJECTED
问题1: [具体哪个 Shot 的什么问题]
问题2: [具体哪个 Shot 的什么问题]
修改建议: [针对性的修改指导]

请保持极度严格的标准。宁可多打回一次，也不能让低质量的提示词浪费昂贵的视频生成 API 调用。`

// Process 执行质检 Agent 的核心逻辑 (Reflection 反思范式)
func (a *CriticAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("CriticAgent: 开始质检审核 (Reflection 模式)", "task_id", masCtx.TaskID)

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
		return nil
	}

	masCtx.ReviewFeedback = reviewResult
	masCtx.ReviewPassed = false
	logger.Log.Warnw("CriticAgent: ❌ 质检不通过，需要打回", "task_id", masCtx.TaskID)
	return fmt.Errorf("质检不通过: %s", reviewResult)
}
