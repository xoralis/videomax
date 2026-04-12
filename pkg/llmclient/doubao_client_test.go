package llmclient

import (
	"context"
	"encoding/json"
	"testing"

	"video-max/internal/tools"
)

// ==================== Mock AITool ====================

// mockTool 实现 tools.AITool 接口的测试桩
type mockTool struct {
	name        string
	description string
	schema      string
}

func (m *mockTool) Name() string             { return m.name }
func (m *mockTool) Description() string       { return m.description }
func (m *mockTool) ParametersSchema() string  { return m.schema }
func (m *mockTool) Execute(_ context.Context, _ string) (string, error) {
	return "mock result", nil
}

// ==================== 工具定义 JSON 结构测试 ====================

// TestDoubaoClient_BuildTools_FlatStructure 测试工具定义生成的 JSON 是扁平结构
// Responses API 要求 name/description/parameters 直接在顶层，不能嵌套在 function 包装器中
func TestDoubaoClient_BuildTools_FlatStructure(t *testing.T) {
	client := &DoubaoClient{model: "test-model"}

	aiTools := []tools.AITool{
		&mockTool{
			name:        "search_best_practices",
			description: "查询视频供应商的最佳实践规则",
			schema:      `{"type":"object","properties":{"provider":{"type":"string","description":"供应商名称"}}}`,
		},
	}

	defs := client.buildTools(aiTools)

	if len(defs) != 1 {
		t.Fatalf("工具数量: got %d, want 1", len(defs))
	}

	// 验证扁平结构（顶层包含 name/description/parameters）
	def := defs[0]
	if def.Type != "function" {
		t.Errorf("Type: got %q, want %q", def.Type, "function")
	}
	if def.Name != "search_best_practices" {
		t.Errorf("Name: got %q, want %q", def.Name, "search_best_practices")
	}
	if def.Description == "" {
		t.Error("Description 不应为空")
	}

	// 序列化验证：JSON 中不应出现嵌套的 "function" 字段
	jsonBytes, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("序列化工具定义失败: %v", err)
	}
	jsonStr := string(jsonBytes)

	// 关键检查：不存在嵌套的 "function": { "name": ... } 结构
	var parsed map[string]interface{}
	json.Unmarshal(jsonBytes, &parsed)
	if _, hasFunc := parsed["function"]; hasFunc {
		t.Errorf("工具定义不应包含嵌套的 'function' 字段 (Responses API 使用扁平结构): %s", jsonStr)
	}

	// name 应直接在顶层
	if _, hasName := parsed["name"]; !hasName {
		t.Errorf("工具定义应在顶层包含 'name' 字段: %s", jsonStr)
	}
}

// ==================== 历史消息拼装测试 ====================

// TestInputMessageSerialization_AssistantWithToolCalls 测试 assistant 带工具调用时的历史消息序列化
// 验证带有 ToolCalls 的 assistant 消息被正确转换为 function_call 类型（无 role 字段）
func TestInputMessageSerialization_AssistantWithToolCalls(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "请生成提示词"},
		{
			Role:    "assistant",
			Content: "", // assistant 发起工具调用时通常无文本内容
			ToolCalls: []ToolCall{
				{
					ID:   "call_abc123",
					Type: "function",
					Function: FunctionCall{
						Name:      "search_best_practices",
						Arguments: `{"provider":"kling"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    "推荐分辨率: 1920x1080",
			ToolCallID: "call_abc123",
		},
	}

	// 模拟 DoubaoClient.Chat() 中的历史消息转换逻辑
	inputMsgs := make([]interface{}, 0)
	for _, msg := range history {
		switch msg.Role {
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				if msg.Content != "" {
					inputMsgs = append(inputMsgs, doubaoInputMessage{
						Role:    "assistant",
						Content: msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					inputMsgs = append(inputMsgs, doubaoFunctionCallInput{
						Type:      "function_call",
						CallID:    tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
			} else {
				inputMsgs = append(inputMsgs, doubaoInputMessage{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case "tool":
			inputMsgs = append(inputMsgs, doubaoToolCallInput{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: msg.Content,
			})
		default:
			inputMsgs = append(inputMsgs, doubaoInputMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// 验证生成了 3 个消息元素（user + function_call + function_call_output）
	if len(inputMsgs) != 3 {
		t.Fatalf("消息数量: got %d, want 3", len(inputMsgs))
	}

	// 序列化整个 input 数组
	jsonBytes, err := json.Marshal(inputMsgs)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	jsonStr := string(jsonBytes)

	// 验证 function_call 类型消息不包含 role 字段
	var parsed []map[string]interface{}
	json.Unmarshal(jsonBytes, &parsed)

	// [0] = user message (has role)
	if parsed[0]["role"] != "user" {
		t.Errorf("第一条消息应为 user, got: %v", parsed[0])
	}

	// [1] = function_call (no role, has type)
	if _, hasRole := parsed[1]["role"]; hasRole {
		t.Errorf("function_call 消息不应包含 role 字段: %s", jsonStr)
	}
	if parsed[1]["type"] != "function_call" {
		t.Errorf("第二条消息 type 应为 function_call, got: %v", parsed[1]["type"])
	}
	if parsed[1]["call_id"] != "call_abc123" {
		t.Errorf("call_id 不匹配: got %v", parsed[1]["call_id"])
	}

	// [2] = function_call_output (no role, has type)
	if _, hasRole := parsed[2]["role"]; hasRole {
		t.Errorf("function_call_output 消息不应包含 role 字段: %s", jsonStr)
	}
	if parsed[2]["type"] != "function_call_output" {
		t.Errorf("第三条消息 type 应为 function_call_output, got: %v", parsed[2]["type"])
	}
}

// ==================== 请求体结构测试 ====================

// TestDoubaoRequest_Serialization 测试 Responses API 请求体的 JSON 序列化格式
func TestDoubaoRequest_Serialization(t *testing.T) {
	inputMsgs := []doubaoInputMessage{
		{Role: "user", Content: "你好"},
	}
	inputJSON, _ := json.Marshal(inputMsgs)

	reqBody := doubaoRequest{
		Model:        "doubao-seed-2-0-pro",
		Input:        inputJSON,
		Instructions: "你是一个助手",
		Stream:       false,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("序列化请求体失败: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(jsonBytes, &parsed)

	// 验证关键字段存在
	if parsed["model"] != "doubao-seed-2-0-pro" {
		t.Errorf("model: got %v", parsed["model"])
	}
	if parsed["instructions"] != "你是一个助手" {
		t.Errorf("instructions: got %v", parsed["instructions"])
	}
	if parsed["stream"] != false {
		t.Errorf("stream: got %v, want false", parsed["stream"])
	}

	// input 应为数组
	inputRaw := parsed["input"]
	inputArr, ok := inputRaw.([]interface{})
	if !ok {
		t.Fatalf("input 应为数组类型, got: %T", inputRaw)
	}
	if len(inputArr) != 1 {
		t.Errorf("input 数组长度: got %d, want 1", len(inputArr))
	}
}
