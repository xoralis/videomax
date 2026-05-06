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

## Output JSON Schema

你必须将最终输出严格格式化为以下 JSON 结构，不得在 JSON 之外输出任何内容：

` + "```" + `json
{
  "approved": <boolean: 是否通过质检>,
  "issues": [
    {
      "dimension": "<string: 问题所在维度，如 '角色一致性'>",
      "problem": "<string: 具体问题描述>",
      "suggestion": "<string: 针对性修改建议>"
    }
  ]
}
` + "```" + `

字段约束：
- approved: 必填布尔值
- issues: 通过时为空数组 []，拒绝时至少包含一个问题条目

## Examples

### 通过示例

` + "```" + `json
{
  "approved": true,
  "issues": []
}
` + "```" + `

### 拒绝示例

` + "```" + `json
{
  "approved": false,
  "issues": [
    {
      "dimension": "角色一致性",
      "problem": "提示词中使用了 'long hair' 但角色设定卡明确为 'short black bob hair'",
      "suggestion": "将 'long hair' 替换为 'short black bob hair' 以与角色设定保持一致"
    },
    {
      "dimension": "运镜与风格",
      "problem": "提示词中缺少运镜描述词，无法指导摄像机运动",
      "suggestion": "在 Camera Work 部分补充运镜词，如 'slow tracking shot' 或 'static wide shot transitioning to close-up'"
    }
  ]
}
` + "```"

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

	passed, feedback := parseCriticResult(reviewResult)
	masCtx.ReviewPassed = passed
	masCtx.ReviewFeedback = feedback

	if passed {
		logger.Log.Infow("CriticAgent: ✅ 质检通过", "task_id", masCtx.TaskID)
		logger.Log.Debugw("CriticAgent: 【输出数据】",
			"task_id", masCtx.TaskID,
			"output.ReviewPassed", masCtx.ReviewPassed,
			"output.ReviewFeedback", masCtx.ReviewFeedback,
		)
		return nil
	}

	logger.Log.Warnw("CriticAgent: ❌ 质检不通过，需要打回", "task_id", masCtx.TaskID)
	logger.Log.Debugw("CriticAgent: 【输出数据】",
		"task_id", masCtx.TaskID,
		"output.ReviewPassed", masCtx.ReviewPassed,
		"output.ReviewFeedback", masCtx.ReviewFeedback,
	)
	return fmt.Errorf("质检不通过: %s", masCtx.ReviewFeedback)
}

// parseCriticResult 解析 CriticAgent 的 JSON 输出，返回 (通过, 反馈文本)。
// 解析失败时降级为字符串前缀判断，保持向后兼容。
func parseCriticResult(rawContent string) (bool, string) {
	type issue struct {
		Dimension  string `json:"dimension"`
		Problem    string `json:"problem"`
		Suggestion string `json:"suggestion"`
	}
	type response struct {
		Approved bool    `json:"approved"`
		Issues   []issue `json:"issues"`
	}

	raw := extractJSONBlock(rawContent)
	var resp response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		// 降级：沿用原有字符串前缀判断
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(rawContent)), "APPROVED") {
			return true, ""
		}
		return false, rawContent
	}

	if resp.Approved {
		return true, ""
	}

	// 将 issues 数组格式化为可读反馈文本
	var sb strings.Builder
	for i, iss := range resp.Issues {
		sb.WriteString(fmt.Sprintf("问题%d [%s]: %s\n修改建议: %s\n", i+1, iss.Dimension, iss.Problem, iss.Suggestion))
		if i < len(resp.Issues)-1 {
			sb.WriteString("\n")
		}
	}
	return false, strings.TrimRight(sb.String(), "\n")
}
