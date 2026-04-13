package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"video-max/internal/api/middleware"
	"video-max/internal/repository"
)

// HistoryHandler 历史记录与统计相关接口控制器
type HistoryHandler struct {
	taskRepo repository.TaskRepository
}

// NewHistoryHandler 创建 HistoryHandler 实例
func NewHistoryHandler(taskRepo repository.TaskRepository) *HistoryHandler {
	return &HistoryHandler{taskRepo: taskRepo}
}

// ListTasks 查询当前用户的任务历史列表（分页）
// GET /api/tasks?page=1&page_size=10
func (h *HistoryHandler) ListTasks(c *gin.Context) {
	userID, _ := c.Get(middleware.ContextUserIDKey)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	tasks, total, err := h.taskRepo.ListByUserID(c.Request.Context(), userID.(string), offset, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "msg": "查询失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      0,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"tasks":     tasks,
	})
}

// GetStats 查询当前用户的使用统计
// GET /api/stats
func (h *HistoryHandler) GetStats(c *gin.Context) {
	userID, _ := c.Get(middleware.ContextUserIDKey)

	stats, err := h.taskRepo.GetUserStats(c.Request.Context(), userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "msg": "查询统计失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  0,
		"stats": stats,
	})
}
