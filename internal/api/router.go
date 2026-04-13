package api

import (
	"github.com/gin-gonic/gin"

	"video-max/internal/api/handler"
	"video-max/internal/api/middleware"
)

// SetupRouter 初始化并注册所有 HTTP 路由
func SetupRouter(
	videoHandler *handler.VideoHandler,
	sseHandler *handler.SSEHandler,
	authHandler *handler.AuthHandler,
	historyHandler *handler.HistoryHandler,
	jwtSecret string,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())

	apiGroup := r.Group("/api")
	{
		// 公开路由：注册 / 登录
		authGroup := apiGroup.Group("/auth")
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
		}

		// 需要 JWT 认证的路由
		protected := apiGroup.Group("", middleware.Auth(jwtSecret))
		{
			// 视频任务
			protected.POST("/video", videoHandler.CreateVideo)
			protected.GET("/task/:id", videoHandler.QueryTask)
			protected.GET("/events/:taskId", sseHandler.StreamEvents)

			// 历史记录 & 统计
			protected.GET("/tasks", historyHandler.ListTasks)
			protected.GET("/stats", historyHandler.GetStats)
		}
	}

	return r
}
