package agents

import (
	"context"
	"fmt"

	"video-max/internal/mas/protocol"
	"video-max/pkg/llmclient"
	"video-max/pkg/logger"
)

// StoryboardAgent 分镜规划智能体
// 充当「导演」角色，将故事大纲和角色设定拆解为一系列具体的镜头（Shot）
type StoryboardAgent struct {
	llm llmclient.LLMClient
}

func NewStoryboardAgent(llm llmclient.LLMClient) *StoryboardAgent {
	return &StoryboardAgent{llm: llm}
}

func (a *StoryboardAgent) Name() string {
	return "StoryboardAgent"
}

const storyboardSystemPrompt = `你是一个专业的分镜规划师（导演/剪辑师）。你的任务是将故事大纲拆解成适合短视频的分镜表。

你收到的输入包含：故事大纲、角色设定卡、以及用户可能提供的参考图数量。

请按照以下格式输出分镜表：

Shot 1:
- 时间段: 0s-3s
- 画面主体: [描述此镜头中出现的核心角色/物体]
- 动作/事件: [此镜头中发生了什么]
- 参考图: [如果有对应的用户参考图，标注使用第几张图作为起幅或落幅]

Shot 2:
- 时间段: 3s-6s
...

注意事项：
- 短视频通常 5-10 秒，建议拆分为 2-4 个 Shot
- 每个 Shot 应该有明确的画面焦点和动态变化
- 如果用户提供了多张图，优先将第一张图分配为首个 Shot 的起幅，最后一张图分配为末尾 Shot 的落幅
- 不需要在这里写具体的视频生成提示词，只需要描述「画面中发生了什么」
- 两个相邻 Shot 之间需要有自然的过渡逻辑`

// Process 执行分镜规划 Agent 的核心逻辑
func (a *StoryboardAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("StoryboardAgent: 开始分镜规划", "task_id", masCtx.TaskID)
	logger.Log.Debugw("StoryboardAgent: 【输入数据】",
		"task_id", masCtx.TaskID,
		"input.Storyline", masCtx.Storyline,
		"input.Characters", masCtx.Characters,
		"input.ImagesCount", len(masCtx.Images),
	)

	userMsg := fmt.Sprintf(
		"故事大纲：\n%s\n\n角色设定：\n%s\n\n参考图数量: %d 张\n\n请为这个故事设计分镜表。",
		masCtx.Storyline, masCtx.Characters, len(masCtx.Images),
	)

	resp, err := a.llm.Chat(ctx, llmclient.ChatRequest{
		SystemPrompt: storyboardSystemPrompt,
		UserMessage:  userMsg,
	})
	if err != nil {
		return fmt.Errorf("StoryboardAgent 调用大模型失败: %w", err)
	}

	masCtx.SceneList = resp.Content
	logger.Log.Infow("StoryboardAgent: 分镜规划完成", "task_id", masCtx.TaskID, "scene_list_length", len(resp.Content))
	logger.Log.Debugw("StoryboardAgent: 【输出数据】",
		"task_id", masCtx.TaskID,
		"output.SceneList", masCtx.SceneList,
	)
	return nil
}
