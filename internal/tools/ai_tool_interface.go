package tools

import "context"

// AITool 大模型函数调用 (Function Calling) 的工具接口约束
// 在 ReAct 范式中，Agent 的大模型大脑会根据思考 (Thought) 的结果
// 自主选择并调用系统中注册的工具 (Action)，然后获取工具返回的结果 (Observation)
// 每一个具体工具（如查询最佳实践、裁剪图片等）都必须实现此接口
type AITool interface {
	// Name 返回工具的函数名称
	// 此名称会被注册到大模型的 Function Calling Schema 中
	// 必须符合 OpenAI 的命名规范：^[a-zA-Z0-9_-]{1,64}$
	Name() string

	// Description 返回工具的自然语言描述
	// 大模型会根据这段描述来判断「什么时候应该调用这个工具」
	// 描述越精准，大模型的工具选择准确率越高
	Description() string

	// ParametersSchema 返回工具入参的 JSON Schema 字符串
	// 大模型据此生成符合格式要求的调用参数
	// 例如：{"type":"object","properties":{"provider":{"type":"string","description":"视频供应商名称"}}}
	ParametersSchema() string

	// Execute 工具被大模型选中后的实际执行方法
	// argsJSON: 大模型生成的 JSON 格式参数字符串
	// 返回值: 工具执行结果的文本描述（会作为 Observation 反馈给大模型继续思考）
	Execute(ctx context.Context, argsJSON string) (string, error)
}
