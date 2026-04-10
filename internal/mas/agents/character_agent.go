package agents

import (
	"context"
	"fmt"

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
2. 对图片中出现的每个主要角色/物体，输出一份精确的「外貌锚点卡」
3. 锚点卡必须包含：
   - 角色编号和简短名称（如：角色A - 短发女孩）
   - 外貌特征：发型、发色、肤色、体型
   - 服装描述：衣物类型、颜色、材质
   - 气质/情绪：表情、姿态传达的情绪
   - 特殊标识：眼镜、纹身、配饰等

如果用户没有提供图片，则根据故事大纲中提到的角色进行合理的设定。

注意：
- 描述必须用简洁、可复用的短语，便于后续直接嵌入视频生成提示词
- 使用英文描述核心视觉特征（因为视频生成模型对英文 Prompt 更敏感）
- 每个角色的描述控制在 3-5 行以内`

// Process 执行角色设定 Agent 的核心逻辑
func (a *CharacterAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("CharacterAgent: 开始分析角色特征", "task_id", masCtx.TaskID, "image_count", len(masCtx.Images))

	userMsg := fmt.Sprintf("以下是故事大纲（由上一个同事完成）：\n%s\n\n请根据以上故事背景和我提供的参考图片，为所有主要角色输出「外貌锚点卡」。", masCtx.Storyline)

	resp, err := a.llm.Chat(ctx, llmclient.ChatRequest{
		SystemPrompt: characterSystemPrompt,
		UserMessage:  userMsg,
		ImagePaths:   masCtx.Images,
	})
	if err != nil {
		return fmt.Errorf("CharacterAgent 调用大模型失败: %w", err)
	}

	masCtx.Characters = resp.Content
	logger.Log.Infow("CharacterAgent: 角色设定完成", "task_id", masCtx.TaskID, "characters_length", len(resp.Content))
	return nil
}
