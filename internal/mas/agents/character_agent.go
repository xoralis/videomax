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

// CharacterAgent 角色设定智能体
// 利用 GPT-4o 的 Vision 能力深度分析用户上传的参考图片
// 提取出角色的外貌、服装、气质等恒定文本锚点
type CharacterAgent struct {
	llm llmclient.LLMClient
}

func NewCharacterAgent(llm llmclient.LLMClient) *CharacterAgent {
	return &CharacterAgent{llm: llm}
}

func (a *CharacterAgent) Name() string {
	return "CharacterAgent"
}

const characterSystemPrompt = `你是一个专业的角色设定师，擅长从图片中精确提取人物和物体的视觉特征。

你的任务：
1. 仔细观察用户提供的每一张参考图片
2. 对图片中出现的每个主要角色/物体，提取精确的「外貌锚点卡」
3. 锚点卡必须包含：
   - 角色编号和简短名称（如：角色A - 短发女孩）
   - 外貌特征：发型、发色、肤色、体型
   - 服装描述：衣物类型、颜色、材质
   - 气质/情绪：表情、姿态传达的情绪
   - 特殊标识：眼镜、纹身、配饰等

如果用户没有提供图片，则根据故事大纲中提到的角色进行合理的设定。

注意：
- 使用英文描述核心视觉特征（因为视频生成模型对英文 Prompt 更敏感）
- 每个角色的描述控制在 3-5 行以内

## Output JSON Schema

你必须将最终输出严格格式化为以下 JSON 结构，不得在 JSON 之外输出任何内容：

` + "```" + `json
{
  "characters": [
    {
      "id": "<string: 角色编号，如 'A'>",
      "name": "<string: 简短名称，如 '短发女孩'>",
      "appearance": "<string: 发型发色肤色体型等外貌短语，逗号分隔>",
      "costume": "<string: 服装类型颜色材质短语，逗号分隔>",
      "mood": "<string: 气质情绪短语，逗号分隔>",
      "identifiers": "<string: 特殊标识短语，无则填 'none'>"
    }
  ]
}
` + "```" + `

字段约束：
- characters: 必填数组，至少包含一个角色

## Example

` + "```" + `json
{
  "characters": [
    {
      "id": "A",
      "name": "短发女孩",
      "appearance": "short black bob hair, fair skin, slender build, dark expressive eyes",
      "costume": "white oversized hoodie, light blue jeans, white sneakers",
      "mood": "calm, slightly melancholic, introspective",
      "identifiers": "small star-shaped silver earrings"
    },
    {
      "id": "B",
      "name": "橘猫",
      "appearance": "orange tabby cat, chubby build, round amber eyes",
      "costume": "none",
      "mood": "curious, affectionate, alert",
      "identifiers": "white paws, distinctive M-shaped forehead marking"
    }
  ]
}
` + "```"

// Process 执行角色设定 Agent 的核心逻辑
func (a *CharacterAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("CharacterAgent: 开始分析角色特征", "task_id", masCtx.TaskID, "image_count", len(masCtx.Images))
	logger.Log.Debugw("CharacterAgent: 【输入数据】",
		"task_id", masCtx.TaskID,
		"input.Storyline", masCtx.Storyline,
		"input.ImagesCount", len(masCtx.Images),
		"input.Images", masCtx.Images,
	)

	userMsg := fmt.Sprintf("以下是故事大纲（由上一个同事完成）：\n%s\n\n请根据以上故事背景和我提供的参考图片，为所有主要角色输出「外貌锚点卡」。", masCtx.Storyline)

	resp, err := a.llm.Chat(ctx, llmclient.ChatRequest{
		SystemPrompt: characterSystemPrompt,
		UserMessage:  userMsg,
		ImagePaths:   masCtx.Images,
	})
	if err != nil {
		return fmt.Errorf("CharacterAgent 调用大模型失败: %w", err)
	}

	masCtx.Characters = parseCharacters(resp.Content)
	logger.Log.Infow("CharacterAgent: 角色设定完成", "task_id", masCtx.TaskID, "characters_length", len(masCtx.Characters))
	logger.Log.Debugw("CharacterAgent: 【输出数据】",
		"task_id", masCtx.TaskID,
		"output.Characters", masCtx.Characters,
	)
	return nil
}

// parseCharacters 从 LLM 响应中解析角色 JSON 并格式化为下游可读的纯文本锚点卡。
// 解析失败时降级返回原始文本，避免中断 pipeline。
func parseCharacters(rawContent string) string {
	type character struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Appearance  string `json:"appearance"`
		Costume     string `json:"costume"`
		Mood        string `json:"mood"`
		Identifiers string `json:"identifiers"`
	}
	type response struct {
		Characters []character `json:"characters"`
	}

	raw := extractJSONBlock(rawContent)
	var resp response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil || len(resp.Characters) == 0 {
		return rawContent
	}

	var sb strings.Builder
	for _, c := range resp.Characters {
		sb.WriteString(fmt.Sprintf("角色%s - %s\n", c.ID, c.Name))
		sb.WriteString(fmt.Sprintf("  Appearance: %s\n", c.Appearance))
		sb.WriteString(fmt.Sprintf("  Costume: %s\n", c.Costume))
		sb.WriteString(fmt.Sprintf("  Mood: %s\n", c.Mood))
		if c.Identifiers != "" && c.Identifiers != "none" {
			sb.WriteString(fmt.Sprintf("  Identifiers: %s\n", c.Identifiers))
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
