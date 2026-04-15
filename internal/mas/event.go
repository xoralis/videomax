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
	done     map[string]bool         // 记录已完成（Close 已被调用）的 taskID
	buffer   map[string][]AgentEvent // 已发出事件的缓冲，供重连客户端回放
}

// NewEventEmitter 创建事件发射器实例
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		channels: make(map[string]chan AgentEvent),
		done:     make(map[string]bool),
		buffer:   make(map[string][]AgentEvent),
	}
}

// Subscribe 为指定 TaskID 创建一个事件监听通道
// SSE Handler 调用此方法获取通道，然后持续读取事件推送给浏览器
// 若任务已完成（Close 已被调用），返回一个已关闭的 channel，
// SSE Handler 会立即收到关闭信号并向客户端发送 close 事件
// 若客户端在任务进行中重连，缓冲区中的历史事件会立即写入新 channel
func (e *EventEmitter) Subscribe(taskID string) <-chan AgentEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 任务已完成：返回已关闭的 channel，SSE Handler 会立即结束
	if e.done[taskID] {
		ch := make(chan AgentEvent)
		close(ch)
		return ch
	}

	buffered := e.buffer[taskID]
	// channel 容量 = 缓冲区回放事件数 + 20 个新事件位置
	ch := make(chan AgentEvent, len(buffered)+20)

	// 将已错过的历史事件立即写入 channel，重连客户端会先收到这些事件
	for _, evt := range buffered {
		ch <- evt
	}

	e.channels[taskID] = ch
	return ch
}

// Emit 向指定 TaskID 的监听通道发送一个事件
// Orchestrator 在调度每个 Agent 时调用此方法
func (e *EventEmitter) Emit(event AgentEvent) {
	e.mu.Lock()
	// 缓冲事件，供重连客户端回放（最多保留 50 条，防止内存无限增长）
	buf := e.buffer[event.TaskID]
	if len(buf) < 50 {
		e.buffer[event.TaskID] = append(buf, event)
	}
	ch, ok := e.channels[event.TaskID]
	e.mu.Unlock()

	if ok {
		// 非阻塞写入：如果通道满了就丢弃（防止消费者太慢拖垮生产者）
		select {
		case ch <- event:
		default:
		}
	}
}

// Close 任务完成后关闭并清理该 TaskID 的事件通道
// 关闭通道会导致 SSE Handler 中的 select 循环结束，从而关闭 HTTP 连接
// 同时记录该 taskID 已完成，处理 SSE 客户端晚于任务完成才连接的竞态情况
func (e *EventEmitter) Close(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.done[taskID] = true    // 标记任务已完成，供晚到的 Subscribe 使用
	delete(e.buffer, taskID) // 任务完成后清理事件缓冲，释放内存
	if ch, ok := e.channels[taskID]; ok {
		close(ch)
		delete(e.channels, taskID)
	}
}
