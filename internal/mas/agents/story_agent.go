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

## Output JSON Schema

你必须将最终输出严格格式化为以下 JSON 结构，不得在 JSON 之外输出任何内容：

` + "```" + `json
{
  "thinking": "<string: 你的 CoT 分析过程，包含 Step 1/2/3 的推理>",
  "storyline": "<string: 最终故事大纲，2-3 句话，包含开头、发展和结尾>"
}
` + "```" + `

字段约束：
- thinking: 必填，记录完整的三步推理过程
- storyline: 必填，纯中文叙述，不含任何画面/运镜描述，50 字以内

## Example

用户输入：一只橘猫每天守在窗边等主人回家

` + "```" + `json
{
  "thinking": "Step 1 - 核心提炼：主题是忠诚与等待，情绪基调温暖而略带孤独，关键元素是橘猫、窗边、归家。Step 2 - 冲突设计：用猫咪从孤独等待到主人推门而入的情绪反差作为弧线，制造瞬间的情感爆发。Step 3 - 故事大纲：形成如下大纲。",
  "storyline": "黄昏时分，橘猫独自趴在窗台上，目光落寞地望着楼道口。门锁咔哒一声，它猛地抬头，瞳孔放大。主人刚推开门，橘猫已经飞奔扑来，用力蹭着主人的腿。"
}
` + "```"

// Process 执行故事策划 Agent 的核心逻辑
func (a *StoryAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("StoryAgent: 开始构思故事大纲", "task_id", masCtx.TaskID)
	logger.Log.Debugw("StoryAgent: 【输入数据】",
		"task_id", masCtx.TaskID,
		"input.UserIdea", masCtx.UserIdea,
		"input.ImagesCount", len(masCtx.Images),
		"input.Images", masCtx.Images,
	)

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

	storyline := parseStringField(resp.Content, "storyline", resp.Content)
	masCtx.Storyline = storyline
	logger.Log.Infow("StoryAgent: 故事大纲构思完成", "task_id", masCtx.TaskID, "storyline_length", len(masCtx.Storyline))
	logger.Log.Debugw("StoryAgent: 【输出数据】",
		"task_id", masCtx.TaskID,
		"output.Storyline", masCtx.Storyline,
	)
	return nil
}
