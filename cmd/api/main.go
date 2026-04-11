package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video-max/internal/api"
	"video-max/internal/api/handler"
	"video-max/internal/domain/entity"
	"video-max/internal/mas"
	"video-max/internal/mas/agents"
	"video-max/internal/queue"
	"video-max/internal/repository"
	"video-max/internal/tools"
	"video-max/internal/video"
	"video-max/pkg/config"
	kafkapkg "video-max/pkg/kafka"
	"video-max/pkg/llmclient"
	"video-max/pkg/logger"
)

func main() {
	// ==================== 1. 加载配置文件 ====================
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("配置加载成功, port: %d, video_provider: %s\n", cfg.Server.Port, cfg.Video.Provider)

	// ==================== 2. 初始化日志系统 ====================
	if err := logger.Init(cfg.Log); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	logger.Log.Info("🚀 videoMax 多智能体视频生成系统启动中...")

	// ==================== 3. 初始化 MySQL 数据库连接 ====================
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		logger.Log.Fatalw("MySQL 连接失败", "error", err)
	}
	// 自动迁移数据库表结构
	if err := db.AutoMigrate(&entity.Task{}); err != nil {
		logger.Log.Fatalw("数据库迁移失败", "error", err)
	}
	logger.Log.Info("MySQL 连接成功，表结构已同步")

	// ==================== 4. 初始化 Repository 层 ====================
	taskRepo := repository.NewMySQLTaskRepo(db)

	// ==================== 5. 初始化 LLM 客户端（工厂模式：支持 OpenAI / 豆包） ====================
	llm := llmclient.NewLLMClient(cfg.LLM.Provider, cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model)
	logger.Log.Infow("LLM 客户端初始化完成", "provider", llm.Provider(), "model", cfg.LLM.Model)

	// ==================== 6. 初始化工具箱 (供 ReAct Agent 使用) ====================
	aiTools := []tools.AITool{
		&tools.PresetSearchTool{},
	}
	logger.Log.Infow("AI 工具箱初始化完成", "tool_count", len(aiTools))

	// ==================== 7. 创建五大 Agent 并组装 Orchestrator ====================
	storyAgent := agents.NewStoryAgent(llm)
	characterAgent := agents.NewCharacterAgent(llm)
	storyboardAgent := agents.NewStoryboardAgent(llm)
	visualAgent := agents.NewVisualAgent(llm, aiTools)
	criticAgent := agents.NewCriticAgent(llm)

	orchestrator := mas.NewOrchestrator(3) // 质检最多打回 3 次
	orchestrator.RegisterSequential(storyAgent)
	orchestrator.RegisterSequential(characterAgent)
	orchestrator.RegisterSequential(storyboardAgent)
	orchestrator.RegisterSequential(visualAgent)
	orchestrator.RegisterSequential(criticAgent)
	orchestrator.SetVisualAgent(visualAgent)
	orchestrator.SetCriticAgent(criticAgent)

	logger.Log.Info("MAS 多智能体 Orchestrator 组装完成 (Story → Character → Storyboard → Visual ↔ Critic)")

	// ==================== 8. 初始化视频服务提供商 (工厂模式) ====================
	videoProvider, err := video.NewVideoProvider(cfg.Video.Provider, cfg.Video.APIKey, cfg.Video.BaseURL, cfg.Video.Model)
	if err != nil {
		logger.Log.Fatalw("视频服务提供商初始化失败", "error", err)
	}
	logger.Log.Infow("视频服务提供商初始化完成", "provider", videoProvider.Name())

	// ==================== 9. 初始化 Kafka 生产者与消费者 ====================
	kafkaProducer, err := kafkapkg.NewSyncProducer(cfg.Kafka.Brokers)
	if err != nil {
		logger.Log.Fatalw("Kafka 生产者创建失败", "error", err)
	}
	defer kafkaProducer.Close()

	producer := queue.NewProducer(kafkaProducer, cfg.Kafka.Topic)
	consumer := queue.NewConsumer(orchestrator, taskRepo, videoProvider)

	// 启动 Kafka 消费者协程（后台持续监听 Topic）
	kafkaConsumerGroup, err := kafkapkg.NewConsumerGroup(cfg.Kafka.Brokers, cfg.Kafka.GroupID)
	if err != nil {
		logger.Log.Fatalw("Kafka 消费者组创建失败", "error", err)
	}
	defer kafkaConsumerGroup.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			if err := kafkaConsumerGroup.Consume(ctx, []string{cfg.Kafka.Topic}, consumer); err != nil {
				logger.Log.Errorw("Kafka 消费者运行异常", "error", err)
			}
			// 检查上下文是否已取消
			if ctx.Err() != nil {
				return
			}
		}
	}()
	logger.Log.Infow("Kafka 消费者已启动", "topic", cfg.Kafka.Topic, "group", cfg.Kafka.GroupID)

	// ==================== 10. 确保上传目录存在 ====================
	if err := os.MkdirAll(cfg.Storage.UploadDir, 0755); err != nil {
		logger.Log.Fatalw("创建上传目录失败", "error", err)
	}

	// ==================== 11. 初始化 HTTP 路由并启动 Web 服务 ====================
	videoHandler := handler.NewVideoHandler(taskRepo, producer, cfg.Storage.UploadDir)
	router := api.SetupRouter(videoHandler)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Log.Infow("🎬 videoMax HTTP 服务已就绪", "addr", addr)

	// 启动 HTTP 服务（非阻塞，在独立 goroutine 中运行）
	go func() {
		if err := router.Run(addr); err != nil {
			logger.Log.Fatalw("HTTP 服务启动失败", "error", err)
		}
	}()

	// ==================== 12. 优雅关闭：监听系统信号 ====================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("📴 收到关闭信号，videoMax 正在优雅退出...")
	cancel() // 取消 Kafka 消费者的上下文
	logger.Log.Info("videoMax 已安全关闭。再见！")
}
