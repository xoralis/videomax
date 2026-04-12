package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"video-max/internal/mas/protocol"
	"video-max/internal/tools"
	"video-max/pkg/llmclient"
	"video-max/pkg/logger"
)

// VisualAgent 画面提示词智能体
// 采用 ReAct (Reasoning and Acting) 范式运行
type VisualAgent struct {
	llm      llmclient.LLMClient
	aiTools  []tools.AITool
	maxLoops int
}

func NewVisualAgent(llm llmclient.LLMClient, aiTools []tools.AITool) *VisualAgent {
	return &VisualAgent{llm: llm, aiTools: aiTools, maxLoops: 5}
}

func (a *VisualAgent) Name() string {
	return "VisualAgent"
}

const visualSystemPrompt = `你是一个专业的视频生成提示词工程师（摄影指导）。你的任务是将分镜描述翻译成视频生成大模型能够理解的专业提示词。

你拥有以下可调用的工具：
- search_best_practices: 查询指定视频供应商的最佳实践规则（分辨率、时长、风格关键词等）

工作流程（ReAct 范式）：
1. 首先思考你是否了解目标视频平台的参数规范
2. 如果不确定，调用 search_best_practices 工具查询
3. 收到工具返回的信息后，结合分镜描述和角色锚点，构建最终提示词

【核心原则 - 绝对禁止重复】：
因为视频 API 只能接收一段总提示词，你必须将所有 Shot 的内容融合拼接，绝对禁止输出 "Shot 1:", "Shot 2:" 等标题块。全局画风、画质词只能在末尾出现一次！

最终输出格式要求（请输出一段紧凑纯英文提示词，由以下四部分拼接而成）：
[Characters]: 提取核心角色外貌锚点（只写一次）。
[Sequence of Actions]: 将各个分镜的动作按时间顺序串联（例如："Initially... Then... Finally..."）。
[Camera Work]: 总结全片的运镜变化过程（例如："Tracking shot transitioning to static close-up"）。
[Format & Style]: 全局光影风格与画质修饰词（如 "volumetric lighting, anime style, 4K, high quality" - 只写一次）。

注意事项：
- 总的字数禁止超过1000个字符
- 提示词必须完全使用纯正的英文
- 主体描述必须精确引用角色设定卡中的锚点词
- 如果上一次提交被质检员打回，请严格参考质检反馈进行修改`

// Process 执行 Visual Agent 的核心逻辑 (ReAct 循环)
func (a *VisualAgent) Process(ctx context.Context, masCtx *protocol.MASContext) error {
	logger.Log.Infow("VisualAgent: 开始构建专业提示词 (ReAct 模式)", "task_id", masCtx.TaskID)
	logger.Log.Debugw("VisualAgent: 【输入数据】",
		"task_id", masCtx.TaskID,
		"input.SceneList", masCtx.SceneList,
		"input.Characters", masCtx.Characters,
		"input.AspectRatio", masCtx.AspectRatio,
		"input.ReviewFeedback", masCtx.ReviewFeedback,
	)

	userMsg := fmt.Sprintf(
		"分镜表：\n%s\n\n角色设定：\n%s\n\n画面比例：%s",
		masCtx.SceneList, masCtx.Characters, masCtx.AspectRatio,
	)
	if masCtx.ReviewFeedback != "" {
		userMsg += fmt.Sprintf("\n\n⚠️ 质检员反馈（请务必根据以下意见修改）：\n%s", masCtx.ReviewFeedback)
	}

	// 使用自定义的 Message 类型（不依赖 go-openai）
	history := []llmclient.Message{
		{Role: "user", Content: userMsg},
	}

	for loop := 0; loop < a.maxLoops; loop++ {
		logger.Log.Debugw("VisualAgent: ReAct 循环迭代", "task_id", masCtx.TaskID, "loop", loop+1)

		resp, err := a.llm.Chat(ctx, llmclient.ChatRequest{
			SystemPrompt: visualSystemPrompt,
			UserMessage:  "",
			Tools:        a.aiTools,
			History:      history,
		})
		if err != nil {
			return fmt.Errorf("VisualAgent ReAct 循环第 %d 次调用失败: %w", loop+1, err)
		}

		// 大模型没有请求调用工具，说明已直接输出最终结果
		if len(resp.ToolCalls) == 0 {
			masCtx.FinalPrompts = resp.Content
			logger.Log.Infow("VisualAgent: 专业提示词构建完成", "task_id", masCtx.TaskID, "loops_used", loop+1)
			logger.Log.Debugw("VisualAgent: 【输出数据】",
				"task_id", masCtx.TaskID,
				"output.FinalPrompts", masCtx.FinalPrompts,
			)
			return nil
		}

		// 大模型请求调用工具 (Action)
		history = append(history, llmclient.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			logger.Log.Infow("VisualAgent: ReAct Action - 调用工具", "tool_name", tc.Function.Name)

			toolResult, execErr := a.executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				toolResult = fmt.Sprintf("工具调用失败: %s", execErr.Error())
			}

			// 工具返回结果反馈给大模型 (Observation)
			history = append(history, llmclient.Message{
				Role:       "tool",
				Content:    toolResult,
				ToolCallID: tc.ID,
			})
		}

		userMsg = "" // 后续循环只需 history
	}

	return fmt.Errorf("VisualAgent ReAct 循环超过最大迭代次数 (%d)", a.maxLoops)
}

func (a *VisualAgent) executeTool(ctx context.Context, name string, argsJSON string) (string, error) {
	for _, t := range a.aiTools {
		if t.Name() == name {
			return t.Execute(ctx, argsJSON)
		}
	}
	return "", fmt.Errorf("未找到名为 '%s' 的工具", name)
}

func validateJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
