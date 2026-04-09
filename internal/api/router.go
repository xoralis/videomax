package api

import (
	"github.com/gin-gonic/gin"

	"video-max/internal/api/handler"
	"video-max/internal/api/middleware"
)

// SetupRouter 初始化并注册所有 HTTP 路由
// 将 Handler 与具体的 URL 路径绑定
func SetupRouter(videoHandler *handler.VideoHandler) *gin.Engine {
	// 生产模式下关闭 Gin 的调试日志
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	// 全局中间件
	r.Use(gin.Recovery())   // 捕获 panic 防止服务崩溃
	r.Use(middleware.CORS()) // 跨域支持

	// API 路由组
	apiGroup := r.Group("/api")
	{
		// POST /api/video - 提交视频生成任务（图文上传）
		apiGroup.POST("/video", videoHandler.CreateVideo)

		// GET /api/task/:id - 轮询视频生成任务状态
		apiGroup.GET("/task/:id", videoHandler.QueryTask)
	}

	return r
}
