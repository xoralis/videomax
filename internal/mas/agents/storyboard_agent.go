package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

分镜规划原则：
- 短视频通常 5-10 秒，建议拆分为 2-4 个 Shot
- 每个 Shot 应该有明确的画面焦点和动态变化
- 如果用户提供了多张图，优先将第一张图分配为首个 Shot 的起幅，最后一张图分配为末尾 Shot 的落幅
- 只描述「画面中发生了什么」，不要写具体的视频生成提示词
- 两个相邻 Shot 之间需要有自然的过渡逻辑

## Output JSON Schema

你必须将最终输出严格格式化为以下 JSON 结构，不得在 JSON 之外输出任何内容：

` + "```" + `json
{
  "shots": [
    {
      "id": <number: 镜头序号，从 1 开始>,
      "timeRange": "<string: 时间段，如 '0s-3s'>",
      "subject": "<string: 此镜头中出现的核心角色或物体>",
      "action": "<string: 此镜头中发生了什么动作或事件>",
      "referenceImage": <number | null: 对应第几张参考图，没有则为 null>
    }
  ]
}
` + "```" + `

字段约束：
- shots: 必填数组，包含 2-4 个元素
- id: 从 1 开始的连续整数
- timeRange: 格式固定为 "Xs-Ys"
- subject/action: 中文描述，简洁清晰
- referenceImage: 整数（1 起始）或 null

## Example

` + "```" + `json
{
  "shots": [
    {
      "id": 1,
      "timeRange": "0s-3s",
      "subject": "橘猫（角色B）独自坐在窗台上",
      "action": "橘猫望向窗外楼道口，眼神落寞，尾巴缓慢摆动",
      "referenceImage": 1
    },
    {
      "id": 2,
      "timeRange": "3s-6s",
      "subject": "短发女孩（角色A）推开房门",
      "action": "女孩推门进入，橘猫猛地跳下窗台向她扑来，女孩弯腰接住并抱紧",
      "referenceImage": 2
    }
  ]
}
` + "```"

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

	masCtx.SceneList = parseShots(resp.Content)
	logger.Log.Infow("StoryboardAgent: 分镜规划完成", "task_id", masCtx.TaskID, "scene_list_length", len(masCtx.SceneList))
	logger.Log.Debugw("StoryboardAgent: 【输出数据】",
		"task_id", masCtx.TaskID,
		"output.SceneList", masCtx.SceneList,
	)
	return nil
}

// parseShots 从 LLM 响应中解析分镜 JSON 并格式化为 Shot N: 格式的纯文本，供 VisualAgent 消费。
// 解析失败时降级返回原始文本。
func parseShots(rawContent string) string {
	type shot struct {
		ID             int    `json:"id"`
		TimeRange      string `json:"timeRange"`
		Subject        string `json:"subject"`
		Action         string `json:"action"`
		ReferenceImage *int   `json:"referenceImage"`
	}
	type response struct {
		Shots []shot `json:"shots"`
	}

	raw := extractJSONBlock(rawContent)
	var resp response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil || len(resp.Shots) == 0 {
		return rawContent
	}

	var sb strings.Builder
	for _, s := range resp.Shots {
		sb.WriteString(fmt.Sprintf("Shot %d:\n", s.ID))
		sb.WriteString(fmt.Sprintf("- 时间段: %s\n", s.TimeRange))
		sb.WriteString(fmt.Sprintf("- 画面主体: %s\n", s.Subject))
		sb.WriteString(fmt.Sprintf("- 动作/事件: %s\n", s.Action))
		if s.ReferenceImage != nil {
			sb.WriteString(fmt.Sprintf("- 参考图: 第 %d 张\n", *s.ReferenceImage))
		} else {
			sb.WriteString("- 参考图: 无\n")
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
