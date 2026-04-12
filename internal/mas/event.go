package mas

import "sync"

// ==================== SSE 事件发射器 (EventEmitter) ====================

// AgentEvent 多智能体协作过程中的阶段性事件
// 前端通过 SSE 实时接收这些事件来驱动 Agent 进度面板
type AgentEvent struct {
	// TaskID 当前任务的唯一标识
	TaskID string `json:"task_id"`

	// AgentName 当前正在执行的 Agent 名称 (如 "StoryAgent", "CriticAgent")
	AgentName string `json:"agent_name"`

	// Status 事件状态: "running" (开始执行) / "done" (执行完成) / "rejected" (质检打回)
	Status string `json:"status"`

	// Message 面向前端用户的人类可读描述
	Message string `json:"message"`
}

// EventEmitter 事件发射器
// 使用 channel 将 Orchestrator 内部的 Agent 调度事件推送给 SSE Handler
// 按 TaskID 分组管理，支持同时进行多个任务
type EventEmitter struct {
	mu       sync.RWMutex
	channels map[string]chan AgentEvent
}

// NewEventEmitter 创建事件发射器实例
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		channels: make(map[string]chan AgentEvent),
	}
}

// Subscribe 为指定 TaskID 创建一个事件监听通道
// SSE Handler 调用此方法获取通道，然后持续读取事件推送给浏览器
func (e *EventEmitter) Subscribe(taskID string) <-chan AgentEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan AgentEvent, 20) // 缓冲区防止生产者阻塞
	e.channels[taskID] = ch
	return ch
}

// Emit 向指定 TaskID 的监听通道发送一个事件
// Orchestrator 在调度每个 Agent 时调用此方法
func (e *EventEmitter) Emit(event AgentEvent) {
	e.mu.RLock()
	ch, ok := e.channels[event.TaskID]
	e.mu.RUnlock()

	if ok {
		// 非阻塞写入：如果通道满了就丢弃（防止消费者太慢拖垮生产者）
		select {
		case ch <- event:
		default:
		}
	}
}

// Close 任务完成后关闭并清理该 TaskID 的事件通道
// 关闭通道会导致 SSE Handler 中的 range 循环结束，从而关闭 HTTP 连接
func (e *EventEmitter) Close(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if ch, ok := e.channels[taskID]; ok {
		close(ch)
		delete(e.channels, taskID)
	}
}
