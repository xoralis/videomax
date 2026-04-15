package handler

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gin-gonic/gin"

	"video-max/internal/mas"
	"video-max/pkg/logger"
)

// SSEHandler 处理 Server-Sent Events 长连接
// 前端通过 EventSource 监听此接口，实时获取 Agent 协作进度
type SSEHandler struct {
	emitter *mas.EventEmitter
}

// NewSSEHandler 创建 SSE Handler 实例
func NewSSEHandler(emitter *mas.EventEmitter) *SSEHandler {
	return &SSEHandler{emitter: emitter}
}

// StreamEvents 处理 SSE 长连接请求
// GET /api/events/:taskId
// 浏览器通过 EventSource 连接此端点，接收实时的 Agent 调度事件
func (h *SSEHandler) StreamEvents(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		c.JSON(400, gin.H{"error": "task_id 不能为空"})
		return
	}

	// 订阅该 TaskID 的事件通道
	eventCh := h.emitter.Subscribe(taskID)

	// 设置 SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // Nginx 反向代理时禁止缓冲
	c.Writer.WriteHeader(200)
	c.Writer.Flush()

	logger.Log.Infow("SSE: 客户端已连接", "task_id", taskID)

	// 持续监听事件通道，直到通道关闭或客户端断连
	// // keep-alive ticker：每 15 秒发送一次注释行，防止代理/浏览器因空闲关闭连接
	// ticker := time.NewTicker(15 * time.Second)
	// defer ticker.Stop()

	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			logger.Log.Infow("SSE: 客户端已断开", "task_id", taskID)
			return
		// case <-ticker.C:
		// 	// SSE 规范允许以 ": comment\n\n" 作为心跳，不触发客户端 onmessage
		// 	io.WriteString(c.Writer, ": ping\n\n")
		// 	c.Writer.Flush()
		case event, ok := <-eventCh:
			if !ok {
				// 通道已关闭 → 任务完成，发送结束标记并退出
				io.WriteString(c.Writer, "event: close\ndata: {}\n\n")
				c.Writer.Flush()
				logger.Log.Infow("SSE: 事件通道已关闭，连接结束", "task_id", taskID)
				return
			}

			// 将事件序列化为 JSON 并发送
			data, _ := json.Marshal(event)
			fmt.Fprintf(c.Writer, "event: agent\ndata: %s\n\n", string(data))
			c.Writer.Flush()
		}
	}
}
