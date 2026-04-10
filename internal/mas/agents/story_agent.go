package agents

import (
	"context"
	"fmt"

	"video-max/internal/mas/protocol"
	"video-max/pkg/llmclient"
	"video-max/pkg/logger"
)

// StoryAgent 故事策划智能体
// 使用 CoT (Chain of Thought) 思考链范式，强制大模型分步骤推理出完整的剧情大纲
// 它只负责「故事」的内容创作，不涉及画面、运镜等视觉元素
type StoryAgent struct {
	llm llmclient.LLMClient
}

func NewStoryAgent(llm llmclient.LLMClient) *StoryAgent {
	return &StoryAgent{llm: llm}
}

func (a *StoryAgent) Name() string {
	return "StoryAgent"
}

// storySystemPrompt CoT 范式的系统提示词
const storySystemPrompt = `你是一个专业的短视频故事策划师。你的任务是根据用户的创意描述（以及可能提供的参考图片信息），构思一个适合 5-10 秒短视频的精炼故事大纲。

你必须严格按照以下步骤进行思考（Chain of Thought）：

Step 1 - 核心提炼：从用户的描述中提取核心主题、情绪基调和关键元素。
Step 2 - 冲突设计：设计一个适合短视频的微型冲突或变化弧线（如情绪转折、场景变化、动态对比）。
Step 3 - 故事大纲：用 2-3 句话输出最终的故事大纲，包含开头、发展和结尾。

注意事项：
- 故事必须简洁有力，适合极短的视频时长
- 不要在大纲中描述运镜或画面细节，那是其他同事的工作
- 如果用户提供了多张参考图，思考如何将图片中的元素融入故事发展
- 直接输出你的思考过程和最终大纲，不要添加额外的格式标记`

// Process 执行故事策划 Agent 的核心逻辑
func (a *StoryAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("StoryAgent: 开始构思故事大纲", "task_id", masCtx.TaskID)

	userMsg := fmt.Sprintf("用户的创意描述：\n%s", masCtx.UserIdea)
	if len(masCtx.Images) > 0 {
		userMsg += fmt.Sprintf("\n\n用户同时提供了 %d 张参考图片，请在构思故事时考虑图片中的元素。", len(masCtx.Images))
	}

	resp, err := a.llm.Chat(ctx, llmclient.ChatRequest{
		SystemPrompt: storySystemPrompt,
		UserMessage:  userMsg,
		ImagePaths:   masCtx.Images,
	})
	if err != nil {
		return fmt.Errorf("StoryAgent 调用大模型失败: %w", err)
	}

	masCtx.Storyline = resp.Content
	logger.Log.Infow("StoryAgent: 故事大纲构思完成", "task_id", masCtx.TaskID, "storyline_length", len(resp.Content))
	return nil
}
